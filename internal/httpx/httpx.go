package httpx

import (
	"encoding/json"
	"net/http"

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
// Use this where the request is available; WriteError remains available for callers
// that only have a ResponseWriter.
func WriteErrorR(w http.ResponseWriter, r *http.Request, status int, message string) {
	reqID := middleware.GetReqID(r.Context())
	WriteJSON(w, status, ErrorBody{Error: message, RequestID: reqID})
}

// ClientIP returns the originating client address, honoring reverse-proxy headers.
func ClientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		for i := 0; i < len(v); i++ {
			if v[i] == ',' {
				return v[:i]
			}
		}
		return v
	}
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return v
	}
	if i := lastColon(r.RemoteAddr); i > 0 {
		return r.RemoteAddr[:i]
	}
	return r.RemoteAddr
}

func lastColon(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return i
		}
	}
	return -1
}
