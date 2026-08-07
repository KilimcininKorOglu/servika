package mail

import (
	"encoding/json"
	"net/http"
	"strings"

	"servika/internal/httpx"
	"servika/internal/middleware"
)

// migrationRequestLimit bounds the decoded body. Nothing here is large, and the
// password field means the body must not be spooled anywhere either.
const migrationRequestLimit = 8 << 10

// Discover proposes servers the old mailbox might live on.
// POST /domains/{id}/mail/migration/discover
func (h *Handlers) Discover(w http.ResponseWriter, r *http.Request) {
	id, _, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if !middleware.EnforceCustomerNotSuspended(w, r, id) {
		return
	}

	var request struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, migrationRequestLimit)).Decode(&request); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	address := strings.TrimSpace(request.Email)
	if addressDomain(address) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid email address")
		return
	}

	candidates := DiscoverCandidates(r.Context(), address)
	if candidates == nil {
		candidates = []Candidate{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"candidates": candidates,
		// Present even when discovery found nothing, because a Microsoft address
		// cannot be migrated with a password whatever the server list says.
		"provider_notice": providerHint(addressDomain(address), address),
	})
}

// Verify signs in to the remote server so a wrong password is refused now
// rather than after hours of copying.
// POST /domains/{id}/mail/migration/verify
func (h *Handlers) Verify(w http.ResponseWriter, r *http.Request) {
	id, _, _, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if !middleware.EnforceCustomerNotSuspended(w, r, id) {
		return
	}

	var request struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Security string `json:"security"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, migrationRequestLimit)).Decode(&request); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}

	host := strings.ToLower(strings.TrimSpace(request.Host))
	if !isDiscoverableDomain(host) || !validPort(request.Port) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid server address")
		return
	}
	if request.Username == "" || request.Password == "" {
		httpx.WriteError(w, http.StatusBadRequest, "credentials are required")
		return
	}

	accepted, reason := VerifyLogin(r.Context(), host, request.Port, request.Security, request.Username, request.Password)

	// The audit line records the attempt and the remote account, never the
	// password and never the reason, which can name the customer's provider.
	h.audit(r, "mail.migration.verify", request.Username+" @ "+host, accepted)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":     accepted,
		"reason": reason,
	})
}
