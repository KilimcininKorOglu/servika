package apps

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

// The suspension path is a decision about WHICH applications systemd is told
// about, so the test records the decisions rather than reaching for systemd.

type appRow struct {
	id      int64
	enabled int64
}

type listConn struct {
	rows []appRow
	fail error
}

func (c *listConn) Connect(context.Context) (driver.Conn, error) { return c, nil }
func (c *listConn) Driver() driver.Driver                        { return listDriver{} }
func (c *listConn) Prepare(string) (driver.Stmt, error)          { return nil, errors.New("unused") }
func (c *listConn) Close() error                                 { return nil }
func (c *listConn) Begin() (driver.Tx, error)                    { return nil, errors.New("unused") }

func (c *listConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	if c.fail != nil {
		return nil, c.fail
	}
	return &listRows{rows: c.rows}, nil
}

type listDriver struct{}

func (listDriver) Open(string) (driver.Conn, error) { return nil, errors.New("unused") }

type listRows struct {
	rows []appRow
	at   int
}

func (r *listRows) Columns() []string {
	return []string{"id", "domain_id", "subdomain_id", "name", "runtime",
		"runtime_version", "app_root", "start_command", "mount_path", "port", "enabled"}
}
func (r *listRows) Close() error { return nil }
func (r *listRows) Next(dest []driver.Value) error {
	if r.at >= len(r.rows) {
		return io.EOF
	}
	row := r.rows[r.at]
	r.at++
	values := []driver.Value{
		row.id, int64(1), int64(0), "app", "node", "system",
		"apps/api", "node server.js", "/api/", int64(30000 + row.id), row.enabled,
	}
	copy(dest, values)
	return nil
}

func listDB(t *testing.T, conn *listConn) *sql.DB {
	t.Helper()
	db := sql.OpenDB(conn)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// recordSystemd replaces Enable and Disable with recorders and returns the log.
func recordSystemd(t *testing.T) *[]string {
	t.Helper()
	var calls []string
	enable, disable := enableUnit, disableUnit
	enableUnit = func(id int64) error { calls = append(calls, "enable"); return nil }
	disableUnit = func(id int64) error { calls = append(calls, "disable"); return nil }
	t.Cleanup(func() { enableUnit, disableUnit = enable, disable })
	return &calls
}

// Suspending must reach systemd, not just kill the processes. The unit carries
// Restart=always, so a killed application is back within seconds and the
// suspended account keeps serving.
func TestSuspendingStopsEveryApplicationThroughSystemd(t *testing.T) {
	calls := recordSystemd(t)
	db := listDB(t, &listConn{rows: []appRow{{id: 1, enabled: 1}, {id: 2, enabled: 0}}})

	if err := SuspendForUser(context.Background(), db, "c_example", true); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if want := []string{"disable", "disable"}; !slices.Equal(*calls, want) {
		t.Errorf("calls = %v, want %v: an application the customer had already stopped must still be disabled, because the unit could restart it", *calls, want)
	}
}

// Resuming starts only what the database says was running: an application the
// customer stopped before the suspension must stay stopped.
func TestResumingRestoresOnlyWhatTheCustomerHadRunning(t *testing.T) {
	calls := recordSystemd(t)
	db := listDB(t, &listConn{rows: []appRow{{id: 1, enabled: 1}, {id: 2, enabled: 0}}})

	if err := SuspendForUser(context.Background(), db, "c_example", false); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if want := []string{"enable"}; !slices.Equal(*calls, want) {
		t.Errorf("calls = %v, want %v", *calls, want)
	}
}

// A login that is not a tenant must not reach systemd at all.
func TestSuspendRefusesANonTenantLogin(t *testing.T) {
	calls := recordSystemd(t)
	db := listDB(t, &listConn{rows: []appRow{{id: 1, enabled: 1}}})

	for _, name := range []string{"root", "nginx", "", "../c_other"} {
		if err := SuspendForUser(context.Background(), db, name, true); err == nil {
			t.Errorf("%q was accepted as a tenant login", name)
		}
	}
	if len(*calls) != 0 {
		t.Errorf("systemd was reached for a non-tenant login: %v", *calls)
	}
}

// An unreadable list is reported rather than passed off as a completed
// suspension, because the caller records the domain as suspended either way.
func TestAnUnreadableListFailsTheSuspension(t *testing.T) {
	recordSystemd(t)
	db := listDB(t, &listConn{fail: errors.New("connection refused")})

	err := SuspendForUser(context.Background(), db, "c_example", true)
	if err == nil {
		t.Fatal("a failed read was reported as a completed suspension")
	}
	if !strings.Contains(err.Error(), "c_example") {
		t.Errorf("the error does not name the tenant: %v", err)
	}
}
