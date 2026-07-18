package resourcelimit

import (
	"strings"
	"testing"
)

func TestMySQLLimitSQLRejectsInjectedOrProtectedAccount(t *testing.T) {
	limits := Limits{MySQLMaxConnections: 12}
	for _, account := range []string{
		"tenant'@'localhost' WITH MAX_USER_CONNECTIONS 0; DROP USER 'root",
		"root",
		"panel",
	} {
		query, err := mysqlLimitSQL(account, "localhost", limits)
		if err == nil {
			t.Fatalf("mysqlLimitSQL() accepted account %q and returned %q", account, query)
		}
	}
}

func TestMySQLLimitSQLBuildsAllLimitsAndClampsNegativeValues(t *testing.T) {
	limits := Limits{
		MySQLMaxConnections: 12,
		DBMaxQueriesPerHour: 300,
		DBMaxUpdatesPerHour: -1,
	}
	query, err := mysqlLimitSQL("c_tenant_db", "localhost", limits)
	if err != nil {
		t.Fatalf("mysqlLimitSQL() returned an unexpected error: %v", err)
	}
	for _, expected := range []string{
		"ALTER USER 'c_tenant_db'@'localhost'",
		"MAX_USER_CONNECTIONS 12",
		"MAX_QUERIES_PER_HOUR 300",
		"MAX_UPDATES_PER_HOUR 0",
	} {
		if !strings.Contains(query, expected) {
			t.Fatalf("mysqlLimitSQL() omitted %q from %q", expected, query)
		}
	}
}

func TestParseMySQLAccountHostsPreservesEveryRegisteredHost(t *testing.T) {
	hosts := parseMySQLAccountHosts("wpu_deadbeef\tlocalhost\nwpu_deadbeef\t%\nc_tenant_db\t127.0.0.1\nmalformed\n")

	if got := strings.Join(hosts["wpu_deadbeef"], ","); got != "localhost,%" {
		t.Fatalf("parseMySQLAccountHosts() hosts = %q, want %q", got, "localhost,%")
	}
	if got := strings.Join(hosts["c_tenant_db"], ","); got != "127.0.0.1" {
		t.Fatalf("parseMySQLAccountHosts() hosts = %q, want %q", got, "127.0.0.1")
	}
}

func TestMySQLLimitStatementsCoverEveryActualHost(t *testing.T) {
	statements := mysqlLimitStatements(
		[]string{"wpu_deadbeef"},
		map[string][]string{"wpu_deadbeef": {"localhost", "%"}},
		Limits{MySQLMaxConnections: 12, DBMaxQueriesPerHour: 300},
	)

	if len(statements) != 2 {
		t.Fatalf("mysqlLimitStatements() returned %d statements, want 2", len(statements))
	}
	for _, host := range []string{"localhost", "%"} {
		expected := "ALTER USER 'wpu_deadbeef'@'" + host + "'"
		if !strings.Contains(strings.Join(statements, "\n"), expected) {
			t.Fatalf("mysqlLimitStatements() omitted %q from %q", expected, statements)
		}
	}
}

func TestQueryExceedsLimitOnlyAfterConfiguredThreshold(t *testing.T) {
	tests := []struct {
		seconds int
		limit   int
		want    bool
	}{
		{seconds: 6, limit: 5, want: true},
		{seconds: 5, limit: 5, want: false},
		{seconds: 60, limit: 0, want: false},
		{seconds: 60, limit: -1, want: false},
	}
	for _, test := range tests {
		if got := queryExceedsLimit(test.seconds, test.limit); got != test.want {
			t.Fatalf("queryExceedsLimit(%d, %d) = %v, want %v", test.seconds, test.limit, got, test.want)
		}
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

func TestIOSliceLinesWritesOnlyConfiguredAbsoluteLimits(t *testing.T) {
	limits := Limits{IOReadMBps: 25, IOWriteIOPS: 800}
	lines := ioSliceLines(limits)

	if !strings.Contains(lines, "IOReadBandwidthMax=/home 25M") {
		t.Fatalf("ioSliceLines() omitted the read bandwidth limit: %q", lines)
	}
	if !strings.Contains(lines, "IOWriteIOPSMax=/home 800") {
		t.Fatalf("ioSliceLines() omitted the write IOPS limit: %q", lines)
	}
	if strings.Contains(lines, "IOWriteBandwidthMax") || strings.Contains(lines, "IOReadIOPSMax") {
		t.Fatalf("ioSliceLines() emitted an unlimited property: %q", lines)
	}
}

func TestIOSetPropertyArgsClearsUnlimitedValues(t *testing.T) {
	arguments := strings.Join(ioSetPropertyArgs(Limits{IOReadMBps: 25, IOWriteIOPS: 800}), "\n")

	for _, expected := range []string{
		"IOReadBandwidthMax=/home 25M",
		"IOWriteBandwidthMax=",
		"IOReadIOPSMax=",
		"IOWriteIOPSMax=/home 800",
	} {
		if !strings.Contains(arguments, expected) {
			t.Fatalf("ioSetPropertyArgs() omitted %q from %q", expected, arguments)
		}
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
