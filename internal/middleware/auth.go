package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"

	"servika/internal/auth"
	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
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

// tokenVersionMatches reports whether the token's embedded version still equals
// the identity's current version in the given table. A mismatch means the
// session was revoked (the version was bumped). It fails closed: a real query
// error returns (false, err) so the caller denies access. When scopeDB is unset
// (tests) it accepts, so token-only tests keep working. The table argument is a
// fixed internal literal, never user input.
func tokenVersionMatches(ctx context.Context, table string, id, claimVersion int64) (bool, error) {
	if scopeDB == nil {
		return true, nil
	}
	var current int64
	err := scopeDB.QueryRowContext(ctx,
		"SELECT token_version FROM "+table+" WHERE id=?", id).Scan(&current)
	if err != nil {
		return false, err
	}
	return current == claimVersion, nil
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
			// The session JWT is carried only by the HttpOnly servika_session
			// cookie; it is never read from the Authorization header so a stolen
			// bearer value (e.g. from JS/localStorage) cannot be replayed.
			ck, err := r.Cookie(httpx.SessionCookie)
			if err != nil || ck.Value == "" {
				httpx.WriteError(w, http.StatusUnauthorized, "authorization required")
				return
			}
			tokenRaw := ck.Value
			if len(tokenRaw) > 8192 {
				httpx.WriteError(w, http.StatusUnauthorized, "invalid session")
				return
			}

			// Try administrator claims first.
			if c, err := auth.Parse(secret, tokenRaw); err == nil {
				ok, verr := tokenVersionMatches(r.Context(), "users", c.UserID, c.TokenVersion)
				if verr != nil {
					httpx.WriteError(w, http.StatusInternalServerError, "could not verify session")
					return
				}
				if !ok {
					httpx.WriteError(w, http.StatusUnauthorized, "session has been revoked")
					return
				}
				ctx := context.WithValue(r.Context(), claimsKey, c)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			// Then try customer claims.
			if mc, err := auth.ParseCustomer(secret, tokenRaw); err == nil {
				ok, verr := tokenVersionMatches(r.Context(), "ftp_accounts", mc.FTPAccountID, mc.TokenVersion)
				if verr != nil {
					httpx.WriteError(w, http.StatusInternalServerError, "could not verify session")
					return
				}
				if !ok {
					httpx.WriteError(w, http.StatusUnauthorized, "session has been revoked")
					return
				}
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

// Role constants — one-to-one with users.role ENUM('admin','reseller','user').
const (
	RoleAdmin    = "admin"
	RoleReseller = "reseller"
	RoleUser     = "user"
)

// AdminOnly accepts only role=admin and returns 403 otherwise.
//
// SECURITY: this used to check only whether an admin-type token existed
// (ClaimsFrom(r) == nil) and never read the role. That was harmless while a
// single token type was issued (root → role=admin), but the moment reseller
// accounts are given an auth.Claims token, all 87 admin endpoints — firewall,
// service restart, package installation included — would open to that reseller.
// The role check is a PRECONDITION for multi-user support, not a later refinement.
func AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := ClaimsFrom(r)
		if c == nil || c.Role != RoleAdmin {
			httpx.WriteError(w, http.StatusForbidden, "administrator access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ResellerOrAbove accepts role=admin or role=reseller.
//
// Used on two kinds of endpoint:
//   - Account operations (domain, customer, DNS, SSL...) where the reseller acts
//     within ITS OWN scope; the scope narrowing is applied separately via
//     DomainOwnedBy/ScopeSQL — this middleware answers only "is the role enough".
//   - Read-only server information (service status, load, version) visible so a
//     reseller can offer support, while every mutating endpoint stays AdminOnly.
func ResellerOrAbove(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := ClaimsFrom(r)
		if c == nil || (c.Role != RoleAdmin && c.Role != RoleReseller) {
			httpx.WriteError(w, http.StatusForbidden, "insufficient permissions")
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
			// SECURITY: this used to be a bare "ClaimsFrom(r) != nil", so EVERY
			// token carrying auth.Claims was treated as admin and skipped the
			// scope check. Reseller tokens are auth.Claims too, so that meant
			// unscoped access to all 141 customer-scoped endpoints — the
			// wider-surface twin of the same bug in AdminOnly.
			if c := ClaimsFrom(r); c != nil {
				switch c.Role {
				case RoleAdmin:
					next.ServeHTTP(w, r) // Administrator: all domains.
					return
				case RoleReseller:
					urlID, _ := strconv.ParseInt(chi.URLParam(r, param), 10, 64)
					if !ResellerOwnsDomain(r, c.UserID, urlID) {
						httpx.WriteError(w, http.StatusForbidden, "access to this domain is forbidden")
						return
					}
					next.ServeHTTP(w, r)
					return
				case RoleUser:
					// A customer signed in with a panel account (Phase 5C).
					// Unlike the legacy FTP-identity session it may own several
					// domains, so the scope is resolved from the chain, not the
					// token.
					urlID, _ := strconv.ParseInt(chi.URLParam(r, param), 10, 64)
					if !CustomerUserOwnsDomain(r, c.UserID, urlID) {
						httpx.WriteError(w, http.StatusForbidden, "access to this domain is forbidden")
						return
					}
					suspended, err := suspendedDomainLookup(r.Context(), urlID)
					if err != nil {
						httpx.WriteError(w, http.StatusInternalServerError, "could not verify account status")
						return
					}
					if suspended {
						httpx.WriteError(w, http.StatusForbidden, "account is suspended")
						return
					}
					next.ServeHTTP(w, r)
					return
				default:
					httpx.WriteError(w, http.StatusForbidden, "access to this domain is forbidden")
					return
				}
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

// RequestIDHeader echoes the chi RequestID from the request context into the
// X-Request-Id response header, so every response (including error responses that
// use httpx.WriteError) carries a correlation ID without touching each call site.
// Mount it after chimw.RequestID, which populates the context value.
func RequestIDHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := chimw.GetReqID(r.Context()); id != "" {
			w.Header().Set("X-Request-Id", id)
		}
		next.ServeHTTP(w, r)
	})
}

// DomainOwnedBy reports whether the authenticated identity may access a domain.
//   - Admin token    => always true (accesses every domain).
//   - Reseller token => true when the domain belongs to a customer the reseller manages.
//   - Customer token => true only when it matches its own DomainID.
//   - No identity     => false.
//
// This is the in-handler counterpart of CustomerScope: on endpoints whose URL
// carries no {id} domain param (e.g. a derived resource like {dbId}), ownership
// is verified with this function after the resource's domain_id is resolved from
// the database.
func DomainOwnedBy(r *http.Request, domainID int64) bool {
	if c := ClaimsFrom(r); c != nil {
		if c.Role == RoleAdmin {
			return true // Administrator: accesses every domain.
		}
		if c.Role == RoleReseller {
			return ResellerOwnsDomain(r, c.UserID, domainID)
		}
		if c.Role == RoleUser {
			return CustomerUserOwnsDomain(r, c.UserID, domainID)
		}
		return false
	}
	if claims := CustomerClaimsFrom(r); claims != nil {
		return claims.DomainID == domainID
	}
	return false
}

// ResellerOwnsDomain reports whether a domain belongs to a customer managed by
// the given reseller.
//
// The ownership chain is resolved in one place: domains.customer_id ->
// customers.owner_user_id. The authorization decision is always read from the
// database, never from a list embedded in the token — when a reseller loses or
// transfers a customer, its old token must become invalid immediately.
//
// FAIL-CLOSED: returns false (access denied) when the database cannot be read.
func ResellerOwnsDomain(r *http.Request, resellerUserID, domainID int64) bool {
	if scopeDB == nil || resellerUserID <= 0 {
		return false
	}
	var n int
	err := scopeDB.QueryRowContext(r.Context(), `
		SELECT COUNT(*)
		FROM domains d
		JOIN customers c ON c.id = d.customer_id
		WHERE d.id = ? AND c.owner_user_id = ?`, domainID, resellerUserID).Scan(&n)
	return err == nil && n > 0
}

// CustomerUserOwnsDomain reports whether a domain belongs to the given CUSTOMER
// account.
//
// Chain: users.id -> customers.user_id -> domains.customer_id. In Phase 5C
// customers moved onto users accounts; the legacy FTP-identity sessions still
// carry a single DomainID in CustomerClaims (see CustomerScopeParam), but a
// customer signed in with a users account may own SEVERAL domains — so the
// scope is resolved from the chain, not the token.
//
// FAIL-CLOSED: returns false when the database cannot be read.
func CustomerUserOwnsDomain(r *http.Request, userID, domainID int64) bool {
	if scopeDB == nil || userID <= 0 {
		return false
	}
	var n int
	err := scopeDB.QueryRowContext(r.Context(), `
		SELECT COUNT(*)
		FROM domains d
		JOIN customers c ON c.id = d.customer_id
		WHERE d.id = ? AND c.user_id = ?`, domainID, userID).Scan(&n)
	return err == nil && n > 0
}

// ResellerOwnsCustomer reports whether a customer record belongs to the given
// reseller (for endpoints operating on customers directly, without going through
// the domain chain).
func ResellerOwnsCustomer(r *http.Request, resellerUserID, customerID int64) bool {
	if scopeDB == nil || resellerUserID <= 0 {
		return false
	}
	var n int
	err := scopeDB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM customers WHERE id = ? AND owner_user_id = ?`,
		customerID, resellerUserID).Scan(&n)
	return err == nil && n > 0
}

// ScopeSQL produces the WHERE fragment and argument for list endpoints.
//
// On list endpoints verifying ownership row by row does not work — the query
// itself must be narrowed, otherwise a reseller receives a list showing ALL
// records. Usage:
//
//	cond, arg := middleware.ScopeSQL(r, "d")
//	query := "SELECT ... FROM domains d " + cond
//
// Returns an empty string for admins (no narrowing). For resellers it returns an
// EXISTS condition joining customers. For customer/anonymous it returns a
// condition that matches no row (fail-closed).
func ScopeSQL(r *http.Request, domainAlias string) (string, []any) {
	c := ClaimsFrom(r)
	if c != nil && c.Role == RoleAdmin {
		return "", nil
	}
	if c != nil && c.Role == RoleReseller {
		return " WHERE EXISTS (SELECT 1 FROM customers sc WHERE sc.id = " +
			domainAlias + ".customer_id AND sc.owner_user_id = ?)", []any{c.UserID}
	}
	if mc := CustomerClaimsFrom(r); mc != nil {
		return " WHERE " + domainAlias + ".id = ?", []any{mc.DomainID}
	}
	return " WHERE 1 = 0", nil
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
