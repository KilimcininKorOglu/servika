package autoconfig

import (
	"database/sql"
	"database/sql/driver"
	"io"
	"net/http/httptest"
	"testing"
)

// A database that answers the one question hostsMail asks: does this exact name
// have active mail service? The set of hosted names is fixed per connection
// string, which is enough to drive the host-mapping cases below.
type hostedMailDriver struct{}

type hostedMailConn struct{ hosted map[string]bool }

type hostedMailStmt struct{ hosted map[string]bool }

type hostedMailRows struct {
	count int
	done  bool
}

func (hostedMailDriver) Open(name string) (driver.Conn, error) {
	hosted := map[string]bool{}
	for _, host := range splitCommas(name) {
		hosted[host] = true
	}
	return hostedMailConn{hosted: hosted}, nil
}

func splitCommas(value string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(value); i++ {
		if i == len(value) || value[i] == ',' {
			if i > start {
				out = append(out, value[start:i])
			}
			start = i + 1
		}
	}
	return out
}

func (c hostedMailConn) Prepare(string) (driver.Stmt, error) { return hostedMailStmt(c), nil }
func (hostedMailConn) Close() error                          { return nil }
func (hostedMailConn) Begin() (driver.Tx, error)             { return nil, io.EOF }

func (hostedMailStmt) Close() error  { return nil }
func (hostedMailStmt) NumInput() int { return 1 }
func (hostedMailStmt) Exec([]driver.Value) (driver.Result, error) {
	return nil, io.EOF
}

func (s hostedMailStmt) Query(args []driver.Value) (driver.Rows, error) {
	name, _ := args[0].(string)
	count := 0
	if s.hosted[name] {
		count = 1
	}
	return &hostedMailRows{count: count}, nil
}

func (*hostedMailRows) Columns() []string { return []string{"COUNT(*)"} }
func (*hostedMailRows) Close() error      { return nil }
func (r *hostedMailRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = int64(r.count)
	return nil
}

func init() { sql.Register("autoconfig_hosted_mail", hostedMailDriver{}) }

func hostedDB(t *testing.T, hosts string) *sql.DB {
	t.Helper()
	db, err := sql.Open("autoconfig_hosted_mail", hosts)
	if err != nil {
		t.Fatalf("open the hosted-mail database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// The two discovery hostnames stand for the domain underneath them, and the
// literal name still wins when it is itself hosted.
func TestDiscoveryHostMapsToTheDomainUnderneathIt(t *testing.T) {
	cases := []struct {
		name       string
		hosted     string
		requested  string
		wantDomain string
		wantFound  bool
	}{
		{"the domain itself", "example.com", "example.com", "example.com", true},
		{"thunderbird host", "example.com", "autoconfig.example.com", "example.com", true},
		{"outlook host", "example.com", "autodiscover.example.com", "example.com", true},
		{
			// A customer may host a domain that is literally called
			// autoconfig.example.com. Its own settings must win over those of
			// example.com, which is why the literal name is tried first.
			name:       "a hosted domain that is itself named autoconfig",
			hosted:     "autoconfig.example.com,example.com",
			requested:  "autoconfig.example.com",
			wantDomain: "autoconfig.example.com",
			wantFound:  true,
		},
		{"a domain without mail here", "example.com", "autoconfig.example.net", "", false},
		{"the bare prefix", "example.com", "autoconfig.", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handlers{DB: hostedDB(t, tc.hosted)}
			domain, found, err := h.hostedDomainFor(httptest.NewRequest("GET", "/", nil), tc.requested)
			if err != nil {
				t.Fatalf("hostedDomainFor: %v", err)
			}
			if found != tc.wantFound {
				t.Fatalf("found = %v, want %v", found, tc.wantFound)
			}
			if domain != tc.wantDomain {
				t.Errorf("domain = %q, want %q", domain, tc.wantDomain)
			}
		})
	}
}

// A database error must reach the caller, never be read as "not hosted": the
// endpoint answers 503 for the first and 404 for the second, and the difference
// is what tells a client whether to try again.
func TestHostMappingSurfacesADatabaseFailure(t *testing.T) {
	h := &Handlers{DB: failingDB(t)}
	if _, _, err := h.hostedDomainFor(httptest.NewRequest("GET", "/", nil), "autoconfig.example.com"); err == nil {
		t.Error("a database failure was reported as an unhosted domain")
	}
}
