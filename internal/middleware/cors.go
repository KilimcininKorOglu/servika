// Package middleware provides HTTP middleware for the Servika panel.
package middleware

import (
	"net/http"
	"net/url"
)

// CORS reflects the request's own origin only. The panel is served from a single
// origin via nginx, so a same-origin browser never needs CORS at all; reflecting
// only the matching origin means a stolen bearer token cannot be replayed from an
// attacker's site through a browser (the previous wildcard origin allowed exactly
// that). A cross-origin request receives no Access-Control-Allow-Origin header and
// is blocked by the browser. Vary: Origin keeps caches from mixing responses.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		if origin := r.Header.Get("Origin"); origin != "" && sameOrigin(origin, r.Host) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Requested-With")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sameOrigin reports whether the Origin header's host matches the request Host.
// Only the host:port is compared; the scheme is not, because nginx terminates TLS
// and the Go server sees plain HTTP while the browser origin is https.
func sameOrigin(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return u.Host == host
}
