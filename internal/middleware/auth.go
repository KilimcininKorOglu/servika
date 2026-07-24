package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"servika/internal/auth"
	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

var scopeDB *sql.DB

var suspendedDomainLookup = func(ctx context.Context, domainID int64) (bool, error) {
	if scopeDB == nil {
		return false, nil
	}
	var suspended int
	err := scopeDB.QueryRowContext(ctx,
		`SELECT COALESCE(suspended,0) FROM domains WHERE id=?`, domainID).
		Scan(&suspended)
	return suspended == 1, err
}

// Init configures the database used to enforce suspended customer scopes.
func Init(db *sql.DB) {
	scopeDB = db
}

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
				httpx.WriteError(w, http.StatusUnauthorized, "authorization required")
				return
			}
			tokenRaw := raw[len(p):]
			if len(tokenRaw) > 8192 {
				httpx.WriteError(w, http.StatusUnauthorized, "invalid session")
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
			httpx.WriteError(w, http.StatusUnauthorized, "invalid session")
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
				httpx.WriteError(w, http.StatusForbidden, "insufficient permissions")
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
			httpx.WriteError(w, http.StatusForbidden, "administrator access required")
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
				httpx.WriteError(w, http.StatusUnauthorized, "authorization required")
				return
			}
			urlID, _ := strconv.ParseInt(chi.URLParam(r, param), 10, 64)
			if urlID != mc.DomainID {
				httpx.WriteError(w, http.StatusForbidden, "access to this domain is forbidden")
				return
			}
			suspended, err := suspendedDomainLookup(r.Context(), mc.DomainID)
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "could not verify account status")
				return
			}
			if suspended {
				httpx.WriteError(w, http.StatusForbidden, "account is suspended")
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

// EnforceCustomerNotSuspended applies the same suspended-domain gate as CustomerScope
// for handlers that cannot use the CustomerScope middleware because their route is not
// keyed by the "id" parameter (for example the pma-token route keyed by dbId).
// Administrators bypass the check, mirroring CustomerScope. It writes the HTTP error and
// returns false when access must be denied; callers must stop on false.
func EnforceCustomerNotSuspended(w http.ResponseWriter, r *http.Request, domainID int64) bool {
	if ClaimsFrom(r) != nil {
		return true // Administrator.
	}
	suspended, err := suspendedDomainLookup(r.Context(), domainID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not verify account status")
		return false
	}
	if suspended {
		httpx.WriteError(w, http.StatusForbidden, "account is suspended")
		return false
	}
	return true
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
