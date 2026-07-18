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
