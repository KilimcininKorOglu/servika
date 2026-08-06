package provisioner

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The probe writes into the real ACME webroot, which a test machine does not
// have. Point it at a temporary directory instead.
func sandboxWebroot(t *testing.T) {
	t.Helper()
	previous := acmeWebrootDir
	acmeWebrootDir = t.TempDir()
	t.Cleanup(func() { acmeWebrootDir = previous })
}

func stubProbe(t *testing.T, answer func(url string) (int, string, error)) {
	t.Helper()
	previous := challengeProbe
	challengeProbe = answer
	t.Cleanup(func() { challengeProbe = previous })
}

// The pass: the token written under the webroot comes back byte for byte.
func TestChallengeReachableAcceptsTheTokenItWrote(t *testing.T) {
	sandboxWebroot(t)
	stubProbe(t, func(url string) (int, string, error) {
		token := url[strings.LastIndex(url, "/")+1:]
		body, err := os.ReadFile(filepath.Join(acmeWebrootDir, ".well-known", "acme-challenge", token))
		if err != nil {
			return 404, "", nil
		}
		return http.StatusOK, string(body), nil
	})

	if reason, err := challengeReachable("example.com"); reason != "" || err != nil {
		t.Errorf("challengeReachable = (%q, %v), want a pass", reason, err)
	}
}

// Every way a name can resolve and still fail validation.
func TestChallengeReachableRejectsWhatWouldFailValidation(t *testing.T) {
	tests := map[string]struct {
		answer func(string) (int, string, error)
		want   challengeReason
	}{
		"port 80 unreachable": {
			answer: func(string) (int, string, error) { return 0, "", errors.New("connection refused") },
			want:   reasonUnreachable,
		},
		"another vhost answers 404": {
			answer: func(string) (int, string, error) { return http.StatusNotFound, "", nil },
			want:   reasonWrongStatus,
		},
		"a redirect is not a pass": {
			answer: func(string) (int, string, error) { return http.StatusMovedPermanently, "", nil },
			want:   reasonWrongStatus,
		},
		"something else serves the path": {
			answer: func(string) (int, string, error) { return http.StatusOK, "hello from a CDN", nil },
			want:   reasonWrongContent,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sandboxWebroot(t)
			stubProbe(t, test.answer)
			reason, _ := challengeReachable("example.com")
			if reason != test.want {
				t.Errorf("reason = %q, want %q", reason, test.want)
			}
		})
	}
}

// A server that echoes the request path back must not read as a pass, which is
// why the body carries more than the token.
func TestChallengeReachableRejectsAnEchoOfTheToken(t *testing.T) {
	sandboxWebroot(t)
	stubProbe(t, func(url string) (int, string, error) {
		return http.StatusOK, url[strings.LastIndex(url, "/")+1:], nil
	})
	if reason, _ := challengeReachable("example.com"); reason != reasonWrongContent {
		t.Errorf("reason = %q, want %q for a server echoing the token", reason, reasonWrongContent)
	}
}

// The token must not be left behind: the webroot is public, and a probe per
// issuance would otherwise litter it forever.
func TestChallengeReachableRemovesItsToken(t *testing.T) {
	sandboxWebroot(t)
	stubProbe(t, func(string) (int, string, error) { return http.StatusOK, "", nil })
	_, _ = challengeReachable("example.com")

	entries, err := os.ReadDir(filepath.Join(acmeWebrootDir, ".well-known", "acme-challenge"))
	if err != nil {
		t.Fatalf("read the challenge directory: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("%d file(s) left in the challenge directory, want none", len(entries))
	}
}

// www failing must cost www alone. The apex keeps its certificate, which is the
// whole reason for filtering rather than ordering and hoping.
func TestValidatedSANHostsKeepsTheNamesThatAnswer(t *testing.T) {
	sandboxWebroot(t)
	stubProbe(t, func(url string) (int, string, error) {
		if strings.Contains(url, "://www.") {
			return http.StatusNotFound, "", nil
		}
		token := url[strings.LastIndex(url, "/")+1:]
		body, err := os.ReadFile(filepath.Join(acmeWebrootDir, ".well-known", "acme-challenge", token))
		if err != nil {
			return 404, "", nil
		}
		return http.StatusOK, string(body), nil
	})

	kept, dropped := validatedSANHosts([]string{"example.com", "www.example.com"})
	if len(kept) != 1 || kept[0] != "example.com" {
		t.Errorf("kept = %v, want the apex only", kept)
	}
	if dropped["www.example.com"] != reasonWrongStatus {
		t.Errorf("dropped = %v, want www recorded with its reason", dropped)
	}
}
