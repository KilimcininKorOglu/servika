package domains

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

// A recording driver, as used elsewhere in this package: the repository carries
// no sqlmock dependency, and what matters is which row the handler looked for.
type refRecorder struct {
	mu sync.Mutex
	// present decides which ids the database claims to hold.
	customerExists bool
	planExists     bool
	// failOn makes the matching lookup return a driver error instead of rows.
	failOn     string
	statements []string
}

func (r *refRecorder) saw(fragment string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, statement := range r.statements {
		if strings.Contains(statement, fragment) {
			return true
		}
	}
	return false
}

var (
	refStateMu sync.Mutex
	refState   = map[string]*refRecorder{}
)

type refDriver struct{}

func (refDriver) Open(name string) (driver.Conn, error) {
	refStateMu.Lock()
	defer refStateMu.Unlock()
	rec, ok := refState[name]
	if !ok {
		return nil, fmt.Errorf("no recorder registered for %q", name)
	}
	return &refConn{rec: rec}, nil
}

func init() { sql.Register("domains_ref_recorder", refDriver{}) }

type refConn struct{ rec *refRecorder }

func (c *refConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare is not used by this test")
}
func (c *refConn) Close() error { return nil }
func (c *refConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("transactions are not used here")
}

var errRefLookup = errors.New("the database is unavailable")

func (c *refConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.rec.mu.Lock()
	c.rec.statements = append(c.rec.statements, query)
	failOn := c.rec.failOn
	customer, plan := c.rec.customerExists, c.rec.planExists
	c.rec.mu.Unlock()

	if failOn != "" && strings.Contains(query, failOn) {
		return nil, errRefLookup
	}
	switch {
	case strings.Contains(query, "FROM customers WHERE id=?"):
		return &idRow{found: customer}, nil
	case strings.Contains(query, "FROM service_plans WHERE id=?"):
		return &idRow{found: plan}, nil
	}
	return &idRow{}, nil
}

type idRow struct {
	found bool
	done  bool
}

func (r *idRow) Columns() []string { return []string{"id"} }
func (r *idRow) Close() error      { return nil }
func (r *idRow) Next(dest []driver.Value) error {
	if !r.found || r.done {
		return io.EOF
	}
	dest[0] = int64(7)
	r.done = true
	return nil
}

func refHarness(t *testing.T, rec *refRecorder) *Handlers {
	t.Helper()
	name := t.Name()
	refStateMu.Lock()
	refState[name] = rec
	refStateMu.Unlock()
	t.Cleanup(func() {
		refStateMu.Lock()
		delete(refState, name)
		refStateMu.Unlock()
	})

	db, err := sql.Open("domains_ref_recorder", name)
	if err != nil {
		t.Fatalf("open the recording database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Handlers{DB: db}
}

func id(v int64) *int64 { return &v }

// domains.customer_id has no foreign key, so an id matching nothing is written
// verbatim and the domain hangs off a customer ScopeSQL can never find: an
// administrator still sees it while the reseller and the customer it was meant
// for never do.
func TestACustomerThatDoesNotExistIsRefused(t *testing.T) {
	handlers := refHarness(t, &refRecorder{customerExists: false, planExists: true})
	reason, err := handlers.referencedAccountsExist(context.Background(), id(42), id(1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reason != reasonCustomerNotFound {
		t.Errorf("reason = %q, want %q", reason, reasonCustomerNotFound)
	}
}

func TestAPlanThatDoesNotExistIsRefused(t *testing.T) {
	handlers := refHarness(t, &refRecorder{customerExists: true, planExists: false})
	reason, err := handlers.referencedAccountsExist(context.Background(), id(42), id(99))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reason != reasonPlanNotFound {
		t.Errorf("reason = %q, want %q", reason, reasonPlanNotFound)
	}
}

func TestRealIdsAreAccepted(t *testing.T) {
	handlers := refHarness(t, &refRecorder{customerExists: true, planExists: true})
	reason, err := handlers.referencedAccountsExist(context.Background(), id(42), id(1))
	if err != nil || reason != "" {
		t.Errorf("reason = %q, err = %v, want both empty", reason, err)
	}
}

// Omitting either id is how a domain is created unattached, which is the
// default. Looking rows up anyway would refuse the ordinary case.
func TestOmittedIdsAreNotLookedUp(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		customerID, planID   *int64
		wantCustomerLookedUp bool
		wantPlanLookedUp     bool
	}{
		{name: "neither given"},
		{name: "zero is treated as absent", customerID: id(0), planID: id(0)},
		{name: "only a plan", planID: id(1), wantPlanLookedUp: true},
		{name: "only a customer", customerID: id(42), wantCustomerLookedUp: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := &refRecorder{customerExists: true, planExists: true}
			handlers := refHarness(t, recorder)
			reason, err := handlers.referencedAccountsExist(context.Background(), tc.customerID, tc.planID)
			if err != nil || reason != "" {
				t.Fatalf("reason = %q, err = %v, want both empty", reason, err)
			}
			if got := recorder.saw("FROM customers WHERE id=?"); got != tc.wantCustomerLookedUp {
				t.Errorf("customer looked up = %v, want %v", got, tc.wantCustomerLookedUp)
			}
			if got := recorder.saw("FROM service_plans WHERE id=?"); got != tc.wantPlanLookedUp {
				t.Errorf("plan looked up = %v, want %v", got, tc.wantPlanLookedUp)
			}
		})
	}
}

// A lookup that FAILED is not evidence the row is missing. Reporting it as
// "not found" would turn a database hiccup into a refusal the caller cannot
// tell apart from a genuinely wrong id, and reporting it as success would
// provision against an id nobody checked.
func TestAFailedLookupIsAnErrorAndNotAVerdict(t *testing.T) {
	for _, failOn := range []string{"FROM customers WHERE id=?", "FROM service_plans WHERE id=?"} {
		t.Run(failOn, func(t *testing.T) {
			handlers := refHarness(t, &refRecorder{customerExists: true, planExists: true, failOn: failOn})
			reason, err := handlers.referencedAccountsExist(context.Background(), id(42), id(1))
			if err == nil {
				t.Fatalf("a failed lookup returned reason %q and no error", reason)
			}
			if reason != "" {
				t.Errorf("reason = %q, want empty so the caller cannot mistake it for a verdict", reason)
			}
		})
	}
}
