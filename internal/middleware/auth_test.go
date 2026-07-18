package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"servika/internal/auth"
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
