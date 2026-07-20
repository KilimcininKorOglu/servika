package auth

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"

	"servika/internal/httpx"
)

// claims: RequireAuth middleware already validated; we re-parse the header
// (to avoid auth→middleware import cycle) to obtain the UserID.
func (h *Handlers) claims(r *http.Request) *Claims {
	raw := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	c, err := Parse(h.Secret, raw)
	if err != nil {
		return nil
	}
	return c
}

// PUT /me — profile information (full name + email + preferences)
func (h *Handlers) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	c := h.claims(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "no active session")
		return
	}
	var b struct {
		FullName  string `json:"full_name"`
		Email     string `json:"email"`
		PrefTheme string `json:"pref_theme"`
		PrefLang  string `json:"pref_lang"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	b.FullName = strings.TrimSpace(b.FullName)
	b.Email = strings.TrimSpace(b.Email)
	if b.Email != "" && !strings.Contains(b.Email, "@") {
		httpx.WriteError(w, http.StatusBadRequest, "invalid email address")
		return
	}
	theme := "system"
	if b.PrefTheme == "light" || b.PrefTheme == "dark" || b.PrefTheme == "system" {
		theme = b.PrefTheme
	}
	language := "en"
	if b.PrefLang == "tr" {
		language = "tr"
	}
	if _, err := h.DB.Exec(
		`UPDATE users SET full_name=?, email=?, pref_theme=?, pref_lang=?, updated_at=NOW() WHERE id=?`,
		b.FullName, b.Email, theme, language, c.UserID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "profile update failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /me/password — change server root password (current password verified → chpasswd)
func (h *Handlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	c := h.claims(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "no active session")
		return
	}
	var b struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(b.New) < 8 {
		httpx.WriteError(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}
	if !verifyRootPassword(b.Current) {
		writeAudit(h.DB, c.UserID, "root", httpx.ClientIP(r), "auth.password", "root", false)
		httpx.WriteError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	cmd := exec.Command("chpasswd")
	cmd.Stdin = strings.NewReader("root:" + b.New)
	if _, err := cmd.CombinedOutput(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "password change failed")
		return
	}
	writeAudit(h.DB, c.UserID, "root", httpx.ClientIP(r), "auth.password", "root", true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GET /me/2fa/setup — generate a new secret (not yet activated), return otpauth URI
func (h *Handlers) TwoFASetup(w http.ResponseWriter, r *http.Request) {
	if h.claims(r) == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "no active session")
		return
	}
	secret := TOTPGenerateSecret()
	uri := TOTPURI(secret, "root", "Servika")
	resp := map[string]any{
		"secret":      secret,
		"otpauth":     uri, // backwards-compatible (manual entry fallback)
		"otpauth_uri": uri,
	}
	// QR PNG data-URI for scanning with an authenticator app. When generation
	// fails the manual-entry fallback (secret + otpauth) is still present.
	if dataURI, err := TOTPQRDataURI(uri); err == nil {
		resp["qr_data_uri"] = dataURI
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// POST /me/2fa/enable — {secret, code}: enables 2FA if the code validates against the secret
func (h *Handlers) TwoFAEnable(w http.ResponseWriter, r *http.Request) {
	c := h.claims(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "no active session")
		return
	}
	var b struct {
		Secret string `json:"secret"`
		Code   string `json:"code"`
	}
	_ = json.NewDecoder(r.Body).Decode(&b)
	b.Secret = strings.TrimSpace(b.Secret)
	if !TOTPVerify(b.Secret, b.Code) {
		httpx.WriteError(w, http.StatusBadRequest, "code verification failed; enter the six-digit code from your authenticator app")
		return
	}
	if _, err := h.DB.Exec(`UPDATE users SET totp_secret=?, totp_enabled=1 WHERE id=?`, b.Secret, c.UserID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "2FA settings could not be saved")
		return
	}
	writeAudit(h.DB, c.UserID, "root", httpx.ClientIP(r), "auth.2fa.enable", "root", true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /me/2fa/disable — {code}: disables 2FA with a valid code
func (h *Handlers) TwoFADisable(w http.ResponseWriter, r *http.Request) {
	c := h.claims(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "no active session")
		return
	}
	var b struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(r.Body).Decode(&b)
	var secret string
	_ = h.DB.QueryRow(`SELECT totp_secret FROM users WHERE id=?`, c.UserID).Scan(&secret)
	if !TOTPVerify(secret, b.Code) {
		httpx.WriteError(w, http.StatusBadRequest, "code verification failed")
		return
	}
	if _, err := h.DB.Exec(`UPDATE users SET totp_secret='', totp_enabled=0 WHERE id=?`, c.UserID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "2FA could not be disabled")
		return
	}
	writeAudit(h.DB, c.UserID, "root", httpx.ClientIP(r), "auth.2fa.disable", "root", true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
