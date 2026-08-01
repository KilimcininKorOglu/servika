package panelsettings

// Server-default panel language. Before a user signs in (the login screen) this
// decides which language the frontend opens in, so Language is a PUBLIC (no-auth)
// endpoint. A signed-in user's own pref_lang (users table) always overrides it;
// this is only the shared first impression. English is the primary language.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"servika/internal/config"
	"servika/internal/httpx"
)

// Language — GET /api/v1/public/language (no auth required).
func (h *Handlers) Language(w http.ResponseWriter, r *http.Request) {
	var lang string
	err := h.DB.QueryRowContext(r.Context(), `SELECT default_lang FROM panel_settings WHERE id=1`).Scan(&lang)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		// Never fail the login screen over this: fall back to the primary language.
		lang = "en"
	}
	lang = config.NormalizeLang(lang)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"lang": lang})
}

type saveLanguageRequest struct {
	Lang string `json:"lang"`
}

// SaveLanguage — PUT /api/v1/system/panel-language (AdminOnly). Lets an admin
// change the server-default language after installation.
func (h *Handlers) SaveLanguage(w http.ResponseWriter, r *http.Request) {
	var req saveLanguageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Reject an unknown code rather than silently coercing it to "en", so an admin
	// gets clear feedback. NormalizeLang canonicalizes the region form (pt-br → pt-BR).
	if !config.IsSupportedLang(req.Lang) {
		httpx.WriteError(w, http.StatusBadRequest, "unsupported language code")
		return
	}
	lang := config.NormalizeLang(req.Lang)
	if _, err := h.DB.ExecContext(r.Context(), `UPDATE panel_settings SET default_lang=? WHERE id=1`, lang); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "panel settings could not be saved")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "lang": lang})
}
