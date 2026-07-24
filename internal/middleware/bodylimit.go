package middleware

import (
	"net/http"
	"strings"
)

const maxJSONBodyBytes = 10 << 20 // 10 MB

// BodyLimit caps the request body at maxJSONBodyBytes for JSON endpoints.
// The multipart file upload endpoint is exempted because it streams large files
// and enforces its own 10 GiB limit via http.MaxBytesReader; applying the JSON
// cap here would truncate valid uploads over 10 MB before the upload limit runs.
func BodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/files/upload") {
			next.ServeHTTP(w, r)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
		next.ServeHTTP(w, r)
	})
}
