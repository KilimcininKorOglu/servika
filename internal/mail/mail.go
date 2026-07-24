package mail

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"servika/internal/auth"
	"servika/internal/credentials"
	"servika/internal/httpx"
	"servika/internal/middleware"
	"servika/internal/quota"

	"github.com/go-chi/chi/v5"
)

type Handlers struct {
	DB *sql.DB
}

type Mailbox struct {
	ID        int64  `json:"id"`
	LocalPart string `json:"local_part"`
	Email     string `json:"email"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type Status struct {
	Enabled      bool   `json:"enabled"`
	DKIMSelector string `json:"dkim_selector,omitempty"`
}

var localPartPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9._-]{0,62}[a-z0-9])?$`)

func (h *Handlers) domain(r *http.Request) (id int64, systemUser string, demo, ok bool) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var isDemo int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT system_user, COALESCE(is_demo,0) FROM domains WHERE id=?`, id).
		Scan(&systemUser, &isDemo); err != nil {
		return id, "", false, false
	}
	return id, systemUser, isDemo == 1, true
}

func (h *Handlers) audit(r *http.Request, action, target string, ok bool) {
	claims := middleware.ClaimsFrom(r)
	if claims == nil {
		return
	}
	auth.WriteAudit(h.DB, claims.UserID, claims.Username, httpx.ClientIP(r), action, target, ok)
}

// MailStatus reports whether native mail hosting is enabled for a domain.
func (h *Handlers) MailStatus(w http.ResponseWriter, r *http.Request) {
	id, _, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	var status, selector string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT status, dkim_selector FROM mail_domains WHERE domain_id=?`, id).Scan(&status, &selector)
	httpx.WriteJSON(w, http.StatusOK, Status{Enabled: err == nil && status == "active", DKIMSelector: selector})
}

// Enable enables native mail hosting for a domain.
func (h *Handlers) Enable(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "mail is unavailable for demo subscriptions")
		return
	}
	if err := EnableDomain(r.Context(), h.DB, id); err != nil {
		log.Printf("enable mail domain=%d: %v", id, err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not enable mail")
		return
	}
	h.audit(r, "mail.enable", strconv.FormatInt(id, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Disable disables native mail hosting for a domain without deleting mailboxes.
func (h *Handlers) Disable(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "mail is unavailable for demo subscriptions")
		return
	}
	if err := DisableDomain(r.Context(), h.DB, id); err != nil {
		log.Printf("disable mail domain=%d: %v", id, err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not disable mail")
		return
	}
	h.audit(r, "mail.disable", strconv.FormatInt(id, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// List returns mailboxes for a domain.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id, _, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, local_part, email, status, DATE_FORMAT(created_at,'%Y-%m-%d %H:%i') FROM mailboxes WHERE domain_id=? ORDER BY local_part`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list mailboxes")
		return
	}
	defer func() { _ = rows.Close() }()
	out := make([]Mailbox, 0)
	for rows.Next() {
		var mailbox Mailbox
		if err := rows.Scan(&mailbox.ID, &mailbox.LocalPart, &mailbox.Email, &mailbox.Status, &mailbox.CreatedAt); err == nil {
			out = append(out, mailbox)
		}
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list mailboxes")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// Create creates a mailbox for a domain.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "mail is unavailable for demo subscriptions")
		return
	}
	var req struct {
		LocalPart string `json:"local_part"`
		Password  string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	localPart := strings.ToLower(strings.TrimSpace(req.LocalPart))
	if !localPartPattern.MatchString(localPart) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid mailbox name")
		return
	}
	if req.Password == "" {
		req.Password = credentials.RandomPassword(20)
	}
	if !credentials.ValidPassword(req.Password) {
		httpx.WriteError(w, http.StatusBadRequest, "password contains invalid characters")
		return
	}

	var mailDomainID int64
	var domainName, maildirRoot string
	var uidN, gidN int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT id, domain_name, maildir_root, uid_n, gid_n FROM mail_domains WHERE domain_id=? AND status='active'`, id).
		Scan(&mailDomainID, &domainName, &maildirRoot, &uidN, &gidN)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusBadRequest, "enable mail for this domain first")
		return
	}
	if err != nil {
		log.Printf("read mail domain=%d: %v", id, err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not read mail domain")
		return
	}
	if err := quota.CheckMailboxAllowed(r.Context(), h.DB, id); err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}

	email := localPart + "@" + domainName
	hash, err := HashPassword(req.Password)
	if err != nil {
		log.Printf("hash mailbox password domain=%d: %v", id, err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not prepare mailbox password")
		return
	}
	maildir := filepath.Join(maildirRoot, localPart) + "/"
	if err := os.MkdirAll(maildir, 0o700); err != nil {
		log.Printf("create Maildir %q: %v", maildir, err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not create mailbox storage")
		return
	}
	_ = os.Chown(maildir, uidN, gidN)
	cmd := exec.Command("/sbin/restorecon", "-R", maildir)
	cmd.Env = subprocessEnv
	_ = cmd.Run()

	res, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO mailboxes(domain_id, mail_domain_id, local_part, email, password_hash, maildir)
		 VALUES(?,?,?,?,?,?)`,
		id, mailDomainID, localPart, email, hash, maildir)
	if err != nil {
		httpx.WriteError(w, http.StatusConflict, "mailbox already exists or could not be created")
		return
	}
	mailboxID, _ := res.LastInsertId()
	h.audit(r, "mail.create", email, true)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"id": mailboxID, "email": email, "password": req.Password})
}

// Delete removes a mailbox row while preserving its Maildir data on disk.
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "mail is unavailable for demo subscriptions")
		return
	}
	mailboxID, _ := strconv.ParseInt(chi.URLParam(r, "mid"), 10, 64)
	var email string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT email FROM mailboxes WHERE id=? AND domain_id=?`, mailboxID, id).Scan(&email); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "mailbox not found")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM mailboxes WHERE id=? AND domain_id=?`, mailboxID, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not delete mailbox")
		return
	}
	h.audit(r, "mail.delete", email, true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ResetPassword updates a mailbox password or generates a new one.
func (h *Handlers) ResetPassword(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "mail is unavailable for demo subscriptions")
		return
	}
	mailboxID, _ := strconv.ParseInt(chi.URLParam(r, "mid"), 10, 64)
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Password == "" {
		req.Password = credentials.RandomPassword(20)
	}
	if !credentials.ValidPassword(req.Password) {
		httpx.WriteError(w, http.StatusBadRequest, "password contains invalid characters")
		return
	}
	hash, err := HashPassword(req.Password)
	if err != nil {
		log.Printf("hash mailbox password domain=%d mailbox=%d: %v", id, mailboxID, err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not prepare mailbox password")
		return
	}
	res, err := h.DB.ExecContext(r.Context(),
		`UPDATE mailboxes SET password_hash=? WHERE id=? AND domain_id=?`, hash, mailboxID, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not update mailbox")
		return
	}
	if rowsAffected, _ := res.RowsAffected(); rowsAffected == 0 {
		httpx.WriteError(w, http.StatusNotFound, "mailbox not found")
		return
	}
	h.audit(r, "mail.password", strconv.FormatInt(mailboxID, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "password": req.Password})
}

// SetStatus changes a mailbox status.
func (h *Handlers) SetStatus(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "mail is unavailable for demo subscriptions")
		return
	}
	mailboxID, _ := strconv.ParseInt(chi.URLParam(r, "mid"), 10, 64)
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.Status != "active" && req.Status != "suspended") {
		httpx.WriteError(w, http.StatusBadRequest, "invalid status")
		return
	}
	res, err := h.DB.ExecContext(r.Context(),
		`UPDATE mailboxes SET status=? WHERE id=? AND domain_id=?`, req.Status, mailboxID, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not update mailbox")
		return
	}
	if rowsAffected, _ := res.RowsAffected(); rowsAffected == 0 {
		httpx.WriteError(w, http.StatusNotFound, "mailbox not found")
		return
	}
	h.audit(r, "mail.status", strconv.FormatInt(mailboxID, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
