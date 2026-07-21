package httpx

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
)

// ErrorBody is the standard HTTP API error response.
type ErrorBody struct {
	Error     string `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

// WriteJSON writes a JSON response with the provided HTTP status.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// WriteError writes a standard JSON error response.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, ErrorBody{Error: message})
}

// WriteErrorR writes a JSON error response annotated with the chi RequestID from context.
func WriteErrorR(w http.ResponseWriter, r *http.Request, status int, message string) {
	reqID := middleware.GetReqID(r.Context())
	WriteJSON(w, status, ErrorBody{Error: message, RequestID: reqID})
}

// ClientIP returns the originating client address.
//
// SECURITY: Proxy headers are trusted ONLY when the request arrives from the
// local reverse-proxy (nginx, 127.0.0.1). Otherwise the client could forge
// X-Forwarded-For to bypass login rate-limiting and poison the audit log with
// spoofed IPs.
//
// Header priority (matching nginx behavior):
//   - X-Real-IP: written AUTHORITATIVELY by nginx with $remote_addr; any value
//     sent by the client is OVERWRITTEN — this is the trusted source.
//   - X-Forwarded-For: nginx uses $proxy_add_x_forwarded_for which APPENDS to
//     the client-supplied value; the LAST element is therefore the trusted one.
func ClientIP(r *http.Request) string {
	remote := hostOnly(r.RemoteAddr)
	if !isLocalProxy(remote) {
		return remote // direct connection — do not trust headers
	}
	if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); v != "" {
		return v
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		parts := strings.Split(v, ",")
		if last := strings.TrimSpace(parts[len(parts)-1]); last != "" {
			return last
		}
	}
	return remote
}

// isLocalProxy reports whether ip is a loopback address (our nginx).
func isLocalProxy(ip string) bool {
	p := net.ParseIP(ip)
	return p != nil && p.IsLoopback()
}

// hostOnly strips the port from "ip:port" (including IPv6 brackets).
func hostOnly(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}
