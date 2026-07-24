package panelsettings

import (
	"os/exec"
	"testing"
)

func TestIsACMERenewSkip(t *testing.T) {
	err := exec.Command("sh", "-c", "exit 2").Run()
	if err == nil {
		t.Fatal("expected command to fail")
	}
	if !isACMERenewSkip(err) {
		t.Fatal("expected acme renew skip exit code to be accepted")
	}
}

func TestIsACMERenewSkipRejectsOtherErrors(t *testing.T) {
	err := exec.Command("sh", "-c", "exit 1").Run()
	if err == nil {
		t.Fatal("expected command to fail")
	}
	if isACMERenewSkip(err) {
		t.Fatal("expected non-renew-skip exit code to be rejected")
	}
}

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
