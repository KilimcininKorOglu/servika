package users

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"servika/internal/auth"
	"servika/internal/middleware"
)

// A recording driver, matching the pattern used in internal/mail and
// internal/tenantaccount: the repository carries no sqlmock dependency, and what
// matters here is which statement ran with which owner value.
type createRecorder struct {
	mu         sync.Mutex
	statements []string
	values     [][]driver.Value
}

func (r *createRecorder) record(query string, args []driver.NamedValue) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statements = append(r.statements, query)
	plain := make([]driver.Value, 0, len(args))
	for _, arg := range args {
		plain = append(plain, arg.Value)
	}
	r.values = append(r.values, plain)
}

// customerInsert returns the arguments of the customers INSERT, or nil when it
// never ran.
func (r *createRecorder) customerInsert() []driver.Value {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, statement := range r.statements {
		if strings.Contains(statement, "INSERT INTO customers") {
			return r.values[i]
		}
	}
	return nil
}

var (
	createStateMu sync.Mutex
	createState   = map[string]*createRecorder{}
)

type createDriver struct{}

func (createDriver) Open(name string) (driver.Conn, error) {
	createStateMu.Lock()
	defer createStateMu.Unlock()
	rec, ok := createState[name]
	if !ok {
		return nil, fmt.Errorf("no recorder registered for %q", name)
	}
	return &createConn{rec: rec}, nil
}

func init() { sql.Register("users_create_recorder", createDriver{}) }

type createConn struct{ rec *createRecorder }

func (c *createConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare is not used by this test")
}
func (c *createConn) Close() error { return nil }
func (c *createConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("transactions are not used here")
}

func (c *createConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.rec.record(query, args)
	return createResult{}, nil
}

func (c *createConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.rec.record(query, args)
	return emptyRows{}, nil
}

type createResult struct{}

func (createResult) LastInsertId() (int64, error) { return 55, nil }
func (createResult) RowsAffected() (int64, error) { return 1, nil }

type emptyRows struct{}

func (emptyRows) Columns() []string         { return []string{"id"} }
func (emptyRows) Close() error              { return nil }
func (emptyRows) Next([]driver.Value) error { return io.EOF }

func createHarness(t *testing.T) (*Handlers, *createRecorder) {
	t.Helper()
	rec := &createRecorder{}
	name := t.Name()

	createStateMu.Lock()
	createState[name] = rec
	createStateMu.Unlock()
	t.Cleanup(func() {
		createStateMu.Lock()
		delete(createState, name)
		createStateMu.Unlock()
	})

	db, err := sql.Open("users_create_recorder", name)
	if err != nil {
		t.Fatalf("open the recording database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Handlers{DB: db}, rec
}

func createRequest(t *testing.T, actor *auth.Claims, body string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	return request.WithContext(auth.WithClaims(request.Context(), actor))
}

// The customers row is the middle link of the ownership chain. Without it the
// account signs in and sees nothing, because ScopeSQL's RoleUser branch matches
// on customers.user_id, and no domain can be attached to it either. It used to be
// created only when a RESELLER opened the account, which left every account an
// administrator opened stranded exactly that way.
func TestACustomerAccountGetsItsChainWhoeverCreatesIt(t *testing.T) {
	for _, tc := range []struct {
		name      string
		actor     *auth.Claims
		wantOwner any
	}{
		{
			name:      "an administrator leaves the customer directly under admin",
			actor:     &auth.Claims{UserID: 1, Username: "admin", Role: middleware.RoleAdmin},
			wantOwner: nil,
		},
		{
			name:      "a reseller owns the customer it opened",
			actor:     &auth.Claims{UserID: 9, Username: "reseller", Role: middleware.RoleReseller},
			wantOwner: int64(9),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handlers, recorder := createHarness(t)
			response := httptest.NewRecorder()
			handlers.Create(response, createRequest(t, tc.actor,
				`{"username":"cust1","password":"Str0ng-Passw0rd!","role":"user","full_name":"Customer One"}`))

			if response.Code != http.StatusCreated {
				t.Fatalf("status = %d, want %d (%s)", response.Code, http.StatusCreated, response.Body.String())
			}
			args := recorder.customerInsert()
			if args == nil {
				t.Fatal("no customers row was created, so the account would see nothing")
			}
			// name, email, user_id, owner_user_id
			if len(args) != 4 {
				t.Fatalf("customers INSERT bound %d values, want 4", len(args))
			}
			if args[3] != tc.wantOwner {
				t.Errorf("owner_user_id = %v, want %v", args[3], tc.wantOwner)
			}
		})
	}
}

// Only a CUSTOMER account belongs in the customers table. Creating the row for a
// reseller or an administrator would put a staff login into the customer list
// and count it against a quota it has nothing to do with.
func TestStaffAccountsGetNoCustomerRecord(t *testing.T) {
	for _, role := range []string{middleware.RoleAdmin, middleware.RoleReseller} {
		t.Run(role, func(t *testing.T) {
			handlers, recorder := createHarness(t)
			response := httptest.NewRecorder()
			handlers.Create(response, createRequest(t,
				&auth.Claims{UserID: 1, Username: "admin", Role: middleware.RoleAdmin},
				`{"username":"staff1","password":"Str0ng-Passw0rd!","role":"`+role+`"}`))

			if response.Code != http.StatusCreated {
				t.Fatalf("status = %d, want %d (%s)", response.Code, http.StatusCreated, response.Body.String())
			}
			if recorder.customerInsert() != nil {
				t.Errorf("a %s account was given a customers row", role)
			}
		})
	}
}
