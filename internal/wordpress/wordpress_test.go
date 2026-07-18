package wordpress

import "testing"

func TestManagedDBAccountRejectsInjectedDatabaseName(t *testing.T) {
	injected := "wp_deadbeef`; DROP DATABASE `panel"

	if dbUser, ok := managedDBAccount(injected); ok {
		t.Fatalf("managedDBAccount() accepted an injected database name and returned %q", dbUser)
	}
}

func TestManagedDBAccountDerivesPairedAccount(t *testing.T) {
	dbUser, ok := managedDBAccount("wp_deadbeef")
	if !ok {
		t.Fatal("managedDBAccount() rejected a package-managed database name")
	}
	if dbUser != "wpu_deadbeef" {
		t.Fatalf("managedDBAccount() user = %q, want %q", dbUser, "wpu_deadbeef")
	}
}
