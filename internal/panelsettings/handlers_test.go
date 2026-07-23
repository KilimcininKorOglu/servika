package panelsettings

import "testing"

func TestACMEEnvUsesExplicitAllowlist(t *testing.T) {
	env := acmeEnv()
	if len(env) != 2 {
		t.Fatalf("expected two explicit environment entries, got %d", len(env))
	}
	if env[0] == "" || env[1] == "" {
		t.Fatal("expected non-empty environment entries")
	}
	for _, entry := range env {
		if len(entry) >= len("SERVIKA_") && entry[:len("SERVIKA_")] == "SERVIKA_" {
			t.Fatalf("ACME environment must not inherit Servika secrets: %q", entry)
		}
	}
}

func TestNullableTrimsEmptyValues(t *testing.T) {
	if nullable("  ") != nil {
		t.Fatal("expected blank value to become nil")
	}
	if got := nullable("example.com"); got != "example.com" {
		t.Fatalf("expected non-empty value to pass through, got %#v", got)
	}
}
