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
		{name: "administrator may access any domain", context: context.WithValue(context.Background(), claimsKey, &auth.Claims{}), domainID: 42, allowed: true},
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
