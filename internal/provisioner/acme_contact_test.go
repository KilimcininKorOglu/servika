package provisioner

import (
	"errors"
	"os/exec"
	"testing"
)

// acme.sh exits 2 (RENEW_SKIP) when the store already holds a valid certificate. Callers
// must keep going and deploy it; reporting a failure leaves a good certificate unused.
func TestIsACMERenewSkipAcceptsExitCodeTwo(t *testing.T) {
	err := exec.Command("sh", "-c", "exit 2").Run()
	if err == nil {
		t.Fatal("expected the command to fail")
	}
	if !IsACMERenewSkip(err) {
		t.Fatal("expected exit code 2 to be recognised as RENEW_SKIP")
	}
}

func TestIsACMERenewSkipRejectsOtherFailures(t *testing.T) {
	for _, code := range []string{"1", "3", "127"} {
		err := exec.Command("sh", "-c", "exit "+code).Run()
		if err == nil {
			t.Fatalf("expected exit %s to fail", code)
		}
		if IsACMERenewSkip(err) {
			t.Errorf("exit code %s must not be treated as RENEW_SKIP", code)
		}
	}
	if IsACMERenewSkip(nil) {
		t.Error("a nil error must not be treated as RENEW_SKIP")
	}
	if IsACMERenewSkip(errors.New("boom")) {
		t.Error("a non-exit error must not be treated as RENEW_SKIP")
	}
}
