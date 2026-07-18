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
