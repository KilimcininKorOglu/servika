package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"servika/internal/auth"

	"github.com/go-chi/chi/v5"
)

func TestRequireAuthRejectsOversizedTokenBeforeParsing(t *testing.T) {
	nextCalled := false
	handler := RequireAuth([]byte("01234567890123456789012345678901"))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 8193))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("RequireAuth() status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if nextCalled {
		t.Fatal("RequireAuth() called the protected handler for an oversized token")
	}
}

func TestCustomerScopeRejectsSuspendedCustomer(t *testing.T) {
	originalLookup := suspendedDomainLookup
	t.Cleanup(func() { suspendedDomainLookup = originalLookup })
	suspendedDomainLookup = func(context.Context, int64) (bool, error) {
		return true, nil
	}

	nextCalled := false
	handler := CustomerScope(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	}))
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", "42")
	requestContext := context.WithValue(context.Background(), chi.RouteCtxKey, routeContext)
	requestContext = context.WithValue(requestContext, customerClaimsKey, &auth.CustomerClaims{DomainID: 42})
	request := httptest.NewRequest(http.MethodGet, "/domains/42", nil).WithContext(requestContext)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("CustomerScope() status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if nextCalled {
		t.Fatal("CustomerScope() allowed a suspended customer")
	}
	if !strings.Contains(response.Body.String(), "account is suspended") {
		t.Fatalf("CustomerScope() response = %s", response.Body.String())
	}
}

func TestCustomerScopeFailsClosedWhenSuspensionCannotBeVerified(t *testing.T) {
	originalLookup := suspendedDomainLookup
	t.Cleanup(func() { suspendedDomainLookup = originalLookup })
	suspendedDomainLookup = func(context.Context, int64) (bool, error) {
		return false, context.Canceled
	}

	nextCalled := false
	handler := CustomerScope(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	}))
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", "42")
	requestContext := context.WithValue(context.Background(), chi.RouteCtxKey, routeContext)
	requestContext = context.WithValue(requestContext, customerClaimsKey, &auth.CustomerClaims{DomainID: 42})
	request := httptest.NewRequest(http.MethodGet, "/domains/42", nil).WithContext(requestContext)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("CustomerScope() status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if nextCalled {
		t.Fatal("CustomerScope() allowed access without verifying suspension state")
	}
}

func TestDomainOwnedByEnforcesCustomerDomain(t *testing.T) {
	tests := []struct {
		name     string
		context  context.Context
		domainID int64
		allowed  bool
	}{
		{name: "administrator may access any domain", context: context.WithValue(context.Background(), claimsKey, &auth.Claims{Role: RoleAdmin}), domainID: 42, allowed: true},
		{name: "customer may access token domain", context: context.WithValue(context.Background(), customerClaimsKey, &auth.CustomerClaims{DomainID: 42}), domainID: 42, allowed: true},
		{name: "customer may not access another domain", context: context.WithValue(context.Background(), customerClaimsKey, &auth.CustomerClaims{DomainID: 7}), domainID: 42, allowed: false},
		{name: "missing identity is denied", context: context.Background(), domainID: 42, allowed: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/", nil).WithContext(test.context)
			if got := DomainOwnedBy(request, test.domainID); got != test.allowed {
				t.Fatalf("DomainOwnedBy() = %t, want %t", got, test.allowed)
			}
		})
	}
}

// reqRole builds a request carrying an admin-type (auth.Claims) token of the given role.
func reqRole(role string, uid int64) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	return r.WithContext(context.WithValue(r.Context(), claimsKey, &auth.Claims{UserID: uid, Username: "t", Role: role}))
}

// reqCustomer builds a request carrying a customer (auth.CustomerClaims) token.
func reqCustomer(domainID int64) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	return r.WithContext(context.WithValue(r.Context(), customerClaimsKey, &auth.CustomerClaims{DomainID: domainID}))
}

func TestAdminOnly(t *testing.T) {
	run := func(r *http.Request) int {
		rec := httptest.NewRecorder()
		AdminOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rec, r)
		return rec.Code
	}

	if code := run(reqRole(RoleAdmin, 1)); code != http.StatusOK {
		t.Errorf("admin should pass, code=%d", code)
	}
	// The real regression: a reseller token carries auth.Claims; without the
	// role check it was treated as admin.
	if code := run(reqRole(RoleReseller, 2)); code != http.StatusForbidden {
		t.Errorf("reseller should get 403, code=%d", code)
	}
	if code := run(reqRole(RoleUser, 3)); code != http.StatusForbidden {
		t.Errorf("user role should get 403, code=%d", code)
	}
	if code := run(reqCustomer(5)); code != http.StatusForbidden {
		t.Errorf("customer token should get 403, code=%d", code)
	}
	if code := run(httptest.NewRequest(http.MethodGet, "/", nil)); code != http.StatusForbidden {
		t.Errorf("anonymous should get 403, code=%d", code)
	}
}

func TestResellerOrAbove(t *testing.T) {
	run := func(r *http.Request) int {
		rec := httptest.NewRecorder()
		ResellerOrAbove(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rec, r)
		return rec.Code
	}

	if code := run(reqRole(RoleAdmin, 1)); code != http.StatusOK {
		t.Errorf("admin should pass, code=%d", code)
	}
	if code := run(reqRole(RoleReseller, 2)); code != http.StatusOK {
		t.Errorf("reseller should pass, code=%d", code)
	}
	if code := run(reqRole(RoleUser, 3)); code != http.StatusForbidden {
		t.Errorf("user role should get 403, code=%d", code)
	}
	if code := run(reqCustomer(5)); code != http.StatusForbidden {
		t.Errorf("customer token should get 403, code=%d", code)
	}
}

func TestScopeSQL(t *testing.T) {
	// Admin: no narrowing.
	if cond, arg := ScopeSQL(reqRole(RoleAdmin, 1), "d"); cond != "" || arg != nil {
		t.Errorf("admin must not be narrowed, cond=%q arg=%v", cond, arg)
	}

	// Reseller: EXISTS over the ownership chain + its own user id.
	cond, arg := ScopeSQL(reqRole(RoleReseller, 7), "d")
	if cond == "" || len(arg) != 1 || arg[0] != int64(7) {
		t.Errorf("reseller scope wrong: cond=%q arg=%v", cond, arg)
	}

	// Customer: only its own domain.
	cond, arg = ScopeSQL(reqCustomer(42), "d")
	if cond == "" || len(arg) != 1 || arg[0] != int64(42) {
		t.Errorf("customer scope wrong: cond=%q arg=%v", cond, arg)
	}

	// Anonymous: fail-closed — no row must match.
	cond, _ = ScopeSQL(httptest.NewRequest(http.MethodGet, "/", nil), "d")
	if cond != " WHERE 1 = 0" {
		t.Errorf("anonymous request must be fail-closed, cond=%q", cond)
	}
}

func TestDomainOwnedByFailsClosed(t *testing.T) {
	// scopeDB is nil (no DB in this test) → a reseller's ownership cannot be
	// verified, so access must be DENIED. If it failed open, a DB error would
	// grant the reseller access to every domain.
	if DomainOwnedBy(reqRole(RoleReseller, 2), 99) {
		t.Error("reseller access must be denied without a DB (fail-closed)")
	}
	// Admin passes without touching the DB.
	if !DomainOwnedBy(reqRole(RoleAdmin, 1), 99) {
		t.Error("admin should access every domain")
	}
	// Customer only its own domain.
	if !DomainOwnedBy(reqCustomer(99), 99) {
		t.Error("customer should access its own domain")
	}
	if DomainOwnedBy(reqCustomer(98), 99) {
		t.Error("customer must not access another domain")
	}
}

func TestCustomerScopeResellerWithoutScopeCannotPass(t *testing.T) {
	// scopeDB is nil → ResellerOwnsDomain is false → the reseller must get 403.
	// Regression guard: the old code passed resellers straight through because
	// ClaimsFrom != nil.
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", "42")
	ctx := context.WithValue(context.Background(), chi.RouteCtxKey, routeContext)
	ctx = context.WithValue(ctx, claimsKey, &auth.Claims{UserID: 2, Role: RoleReseller})
	request := httptest.NewRequest(http.MethodGet, "/domains/42", nil).WithContext(ctx)

	rec := httptest.NewRecorder()
	CustomerScope(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, request)

	if rec.Code != http.StatusForbidden {
		t.Errorf("reseller whose scope cannot be verified should get 403, code=%d", rec.Code)
	}
}
