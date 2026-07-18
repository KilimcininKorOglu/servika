package credentials

import (
	"errors"
	"testing"
)

func TestValidPasswordRejectsLineInjection(t *testing.T) {
	tests := []struct {
		name     string
		password string
		valid    bool
	}{
		{name: "ordinary password", password: "SafePassword234", valid: true},
		{name: "carriage return", password: "safe\rroot:changed", valid: false},
		{name: "newline", password: "safe\nroot:changed", valid: false},
		{name: "NUL", password: "safe\x00changed", valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ValidPassword(test.password); got != test.valid {
				t.Fatalf("ValidPassword(%q) = %t, want %t", test.password, got, test.valid)
			}
		})
	}
}

func TestValidateMySQLCredentialsRejectsInjectedDatabaseName(t *testing.T) {
	err := validateMySQLCredentials(
		"tenant`; DROP DATABASE `panel",
		"tenant_user",
		"SafePasswrd234",
	)

	if !errors.Is(err, ErrInvalidMySQLCredentials) {
		t.Fatalf("validateMySQLCredentials() error = %v, want ErrInvalidMySQLCredentials", err)
	}
}

func TestValidateMySQLCredentialsRejectsInjectedDatabaseUser(t *testing.T) {
	err := validateMySQLCredentials(
		"tenant_db",
		"tenant'@'localhost'; DROP USER 'root",
		"SafePasswrd234",
	)

	if !errors.Is(err, ErrInvalidMySQLCredentials) {
		t.Fatalf("validateMySQLCredentials() error = %v, want ErrInvalidMySQLCredentials", err)
	}
}

func TestValidateMySQLCredentialsAcceptsManagedValues(t *testing.T) {
	if err := validateMySQLCredentials("wp_deadbeef", "wpu_deadbeef", "SafePasswrd234"); err != nil {
		t.Fatalf("validateMySQLCredentials() returned an unexpected error: %v", err)
	}
}

func TestMySQLDropDBRejectsInjectedIdentifierBeforeExecution(t *testing.T) {
	err := MySQLDropDB(nil, "wp_deadbeef`; DROP DATABASE `panel", "wpu_deadbeef")

	if !errors.Is(err, ErrInvalidMySQLCredentials) {
		t.Fatalf("MySQLDropDB() error = %v, want ErrInvalidMySQLCredentials", err)
	}
}

func TestMySQLChangePasswordRejectsInjectedPasswordBeforeExecution(t *testing.T) {
	err := MySQLChangePassword(nil, "c_tenant", "safe'; DROP USER 'root")

	if !errors.Is(err, ErrInvalidMySQLCredentials) {
		t.Fatalf("MySQLChangePassword() error = %v, want ErrInvalidMySQLCredentials", err)
	}
}

func TestValidCustomerDBIdentifierEnforcesNamespace(t *testing.T) {
	tests := []struct {
		name       string
		systemUser string
		identifier string
		valid      bool
	}{
		{name: "matching namespace", systemUser: "c_tenant", identifier: "c_tenant_app", valid: true},
		{name: "missing namespace", systemUser: "c_tenant", identifier: "app", valid: false},
		{name: "different namespace", systemUser: "c_tenant", identifier: "c_other_app", valid: false},
		{name: "identifier injection", systemUser: "c_tenant", identifier: "c_tenant_app;drop", valid: false},
		{name: "invalid system user", systemUser: "c-tenant", identifier: "c-tenant_app", valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ValidCustomerDBIdentifier(test.systemUser, test.identifier); got != test.valid {
				t.Fatalf("ValidCustomerDBIdentifier(%q, %q) = %t, want %t", test.systemUser, test.identifier, got, test.valid)
			}
		})
	}
}

func TestEscapeSQLStringEscapesQuotesAndBackslashes(t *testing.T) {
	const input = `pa\\ss'word`
	const want = `pa\\\\ss\'word`
	if got := escapeSQLString(input); got != want {
		t.Fatalf("escapeSQLString(%q) = %q, want %q", input, got, want)
	}
}
