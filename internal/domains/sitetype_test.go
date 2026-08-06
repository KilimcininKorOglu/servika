package domains

import (
	"database/sql"
	"reflect"
	"runtime"
	"testing"

	"servika/internal/credentials"
)

func funcName(f any) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

// An unrecognised value must land on PHP, not on static. Falling the other way
// would withhold a database from every caller that predates the field, and the
// omission would only surface once the site tried to connect.
func TestAnUnknownSiteTypeBecomesPHP(t *testing.T) {
	for _, tc := range []struct {
		requested string
		want      string
	}{
		{requested: "", want: siteTypePHP},
		{requested: "laravel", want: siteTypePHP},
		{requested: "STATIC", want: siteTypeStatic},
		{requested: "  WordPress  ", want: siteTypeWordPress},
		{requested: "php", want: siteTypePHP},
		{requested: "static", want: siteTypeStatic},
	} {
		if got := normalizeSiteType(tc.requested); got != tc.want {
			t.Errorf("normalizeSiteType(%q) = %q, want %q", tc.requested, got, tc.want)
		}
	}
}

// The names are what the connection-details screen reads off the domains row. A
// static site gets an empty pair so the screen shows nothing rather than naming
// a database that was never opened.
func TestOnlyAStaticSiteGetsNoDatabaseNames(t *testing.T) {
	for _, tc := range []struct {
		siteType string
		wantName string
		wantUser string
	}{
		{siteType: siteTypePHP, wantName: "c_example_main", wantUser: "c_example_db"},
		{siteType: siteTypeWordPress, wantName: "c_example_main", wantUser: "c_example_db"},
		{siteType: siteTypeStatic, wantName: "", wantUser: ""},
	} {
		name, user := databaseNamesFor(tc.siteType, "c_example")
		if name != tc.wantName || user != tc.wantUser {
			t.Errorf("databaseNamesFor(%q) = (%q, %q), want (%q, %q)",
				tc.siteType, name, user, tc.wantName, tc.wantUser)
		}
	}
}

// The whole chain, from the requested type to whether MariaDB is touched at all.
// Testing the two halves separately would not catch a Create that computed empty
// names and then created the database anyway.
func TestOnlyAStaticSiteSkipsTheDatabase(t *testing.T) {
	for _, tc := range []struct {
		requested   string
		wantCreated bool
	}{
		{requested: "php", wantCreated: true},
		{requested: "wordpress", wantCreated: true},
		{requested: "", wantCreated: true},
		{requested: "static", wantCreated: false},
	} {
		t.Run(tc.requested, func(t *testing.T) {
			var created bool
			var gotName, gotUser, gotPass string
			original := mysqlCreateDB
			mysqlCreateDB = func(_ *sql.DB, _ int64, dbName, dbUser, dbPass string) error {
				created = true
				gotName, gotUser, gotPass = dbName, dbUser, dbPass
				return nil
			}
			t.Cleanup(func() { mysqlCreateDB = original })

			handlers := &Handlers{}
			siteType := normalizeSiteType(tc.requested)
			name, user := databaseNamesFor(siteType, "c_example")
			password := handlers.provisionDatabase(7, name, user)

			if created != tc.wantCreated {
				t.Fatalf("database created = %v, want %v", created, tc.wantCreated)
			}
			if !tc.wantCreated {
				// An empty password is what keeps a credential for a database
				// nobody opened out of the create response.
				if password != "" {
					t.Errorf("a password was handed out for a database that was never opened: %q", password)
				}
				return
			}
			if gotName != "c_example_main" || gotUser != "c_example_db" {
				t.Errorf("created (%q, %q), want (c_example_main, c_example_db)", gotName, gotUser)
			}
			if password == "" || gotPass != password {
				t.Errorf("the returned password %q does not match the one used to create the account %q", password, gotPass)
			}
		})
	}
}

// The seam must default to the real implementation, or production would silently
// stop creating databases while every test still passed.
func TestTheDatabaseSeamPointsAtTheRealImplementation(t *testing.T) {
	if mysqlCreateDB == nil {
		t.Fatal("mysqlCreateDB is nil")
	}
	// Comparing funcs is not allowed, so compare what they resolve to.
	want := funcName(credentials.MySQLCreateDB)
	if got := funcName(mysqlCreateDB); got != want {
		t.Errorf("mysqlCreateDB resolves to %s, want %s", got, want)
	}
}
