package resourcelimit

import (
	"strings"
	"testing"
)

func TestMySQLLimitSQLRejectsInjectedAccountName(t *testing.T) {
	injected := "tenant'@'localhost' WITH MAX_USER_CONNECTIONS 0; DROP USER 'root"

	query, err := mysqlLimitSQL(injected, 12)

	if err == nil {
		t.Fatalf("mysqlLimitSQL() accepted an injected account name and returned %q", query)
	}
}

func TestMySQLLimitSQLBuildsStatementForValidAccount(t *testing.T) {
	query, err := mysqlLimitSQL("c_tenant_db", 12)
	if err != nil {
		t.Fatalf("mysqlLimitSQL() returned an unexpected error: %v", err)
	}
	if !strings.Contains(query, "TO 'c_tenant_db'@'localhost'") {
		t.Fatalf("mysqlLimitSQL() query = %q, want the validated account", query)
	}
	if !strings.Contains(query, "MAX_USER_CONNECTIONS 12") {
		t.Fatalf("mysqlLimitSQL() query = %q, want connection limit 12", query)
	}
}

func TestResourceCommandUsesExplicitEnvironment(t *testing.T) {
	t.Setenv("SERVIKA_JWT_SECRET", "must-not-leak")
	command := resourceCommand("systemctl", "daemon-reload")
	environment := strings.Join(command.Env, "\n")
	if strings.Contains(environment, "SERVIKA_JWT_SECRET") {
		t.Fatal("resource command inherited a panel secret")
	}
	if !strings.Contains(environment, "PATH=/usr/sbin:/usr/bin:/sbin:/bin") {
		t.Fatal("resource command does not define its executable search path")
	}
}

func TestTenantCutoverRegressedOnlyOnNewServerError(t *testing.T) {
	tests := []struct {
		name     string
		baseline int
		post     int
		want     bool
	}{
		{name: "healthy site becomes server error", baseline: 200, post: 500, want: true},
		{name: "client error becomes server error", baseline: 404, post: 503, want: true},
		{name: "existing server error remains server error", baseline: 500, post: 503, want: false},
		{name: "healthy site remains healthy", baseline: 200, post: 200, want: false},
		{name: "unreachable probe remains inconclusive", baseline: 0, post: 0, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := tenantCutoverRegressed(test.baseline, test.post); got != test.want {
				t.Fatalf("tenantCutoverRegressed(%d, %d) = %v, want %v", test.baseline, test.post, got, test.want)
			}
		})
	}
}

func TestCalculatePMMaxChildrenUsesPlanValueOrMemoryBudget(t *testing.T) {
	tests := []struct {
		name   string
		limits Limits
		want   int
	}{
		{name: "explicit plan value", limits: Limits{PMMaxChildren: 12, RAMMB: 256}, want: 12},
		{name: "memory derived", limits: Limits{RAMMB: 1024}, want: 16},
		{name: "minimum worker count", limits: Limits{RAMMB: 128}, want: 4},
		{name: "missing plan fallback", limits: Limits{}, want: 8},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := calculatePMMaxChildren(test.limits); got != test.want {
				t.Fatalf("calculatePMMaxChildren() = %d, want %d", got, test.want)
			}
		})
	}
}
