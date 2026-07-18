package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"servika/internal/auth"
	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type ctxKey int

const (
	claimsKey         ctxKey = 1
	customerClaimsKey ctxKey = 2
)

// RequireAuth accepts both admin and customer tokens.
// It stores CustomerClaims for customers and Claims for administrators in the request context.
func RequireAuth(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := strings.TrimSpace(r.Header.Get("Authorization"))
			const p = "Bearer "
			if !strings.HasPrefix(raw, p) {
				httpx.WriteError(w, http.StatusUnauthorized, "Authorization required")
				return
			}
			tokenRaw := raw[len(p):]
			if len(tokenRaw) > 8192 {
				httpx.WriteError(w, http.StatusUnauthorized, "Invalid session")
				return
			}

			// Try administrator claims first.
			if c, err := auth.Parse(secret, tokenRaw); err == nil {
				ctx := context.WithValue(r.Context(), claimsKey, c)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			// Then try customer claims.
			if mc, err := auth.ParseCustomer(secret, tokenRaw); err == nil {
				ctx := context.WithValue(r.Context(), customerClaimsKey, mc)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			httpx.WriteError(w, http.StatusUnauthorized, "Invalid session")
		})
	}
}

// RequireRole restricts access to administrators with an allowed role.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := ClaimsFrom(r)
			if c == nil || !allowed[c.Role] {
				httpx.WriteError(w, http.StatusForbidden, "Insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AdminOnly accepts only administrator tokens and returns 403 for customers.
func AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ClaimsFrom(r) == nil {
			httpx.WriteError(w, http.StatusForbidden, "Administrator access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CustomerScope requires the URL domain ID to match the customer token domain ID.
// Administrators are unrestricted. Use CustomerScopeParam for a parameter other than "id".
func CustomerScope(next http.Handler) http.Handler {
	return CustomerScopeParam("id")(next)
}

func CustomerScopeParam(param string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ClaimsFrom(r) != nil {
				next.ServeHTTP(w, r) // Administrator.
				return
			}
			mc := CustomerClaimsFrom(r)
			if mc == nil {
				httpx.WriteError(w, http.StatusUnauthorized, "Authorization required")
				return
			}
			urlID, _ := strconv.ParseInt(chi.URLParam(r, param), 10, 64)
			if urlID != mc.DomainID {
				httpx.WriteError(w, http.StatusForbidden, "Access to this domain is forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// DomainOwnedBy reports whether the authenticated identity may access a domain.
// Administrators may access every domain; customers may access only their token domain.
func DomainOwnedBy(r *http.Request, domainID int64) bool {
	if ClaimsFrom(r) != nil {
		return true
	}
	if claims := CustomerClaimsFrom(r); claims != nil {
		return claims.DomainID == domainID
	}
	return false
}

func ClaimsFrom(r *http.Request) *auth.Claims {
	v := r.Context().Value(claimsKey)
	if v == nil {
		return nil
	}
	c, _ := v.(*auth.Claims)
	return c
}

func CustomerClaimsFrom(r *http.Request) *auth.CustomerClaims {
	v := r.Context().Value(customerClaimsKey)
	if v == nil {
		return nil
	}
	c, _ := v.(*auth.CustomerClaims)
	return c
}
