package mail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// stubMailServices makes the systemd check answer from a set instead of the
// host, so the test does not depend on what happens to be running on it.
func stubMailServices(t *testing.T, active ...string) {
	t.Helper()
	original := mailServiceActive
	mailServiceActive = func(_ context.Context, unit string) bool {
		return slices.Contains(active, unit)
	}
	t.Cleanup(func() { mailServiceActive = original })
}

// The gate has to be OPEN on a working server. A check that reported something
// missing on a healthy stack would make enabling mail impossible, which is worse
// than the silent failure it exists to prevent.
func TestNothingIsMissingWhenTheStackIsUp(t *testing.T) {
	stubMailServices(t, "postfix", "dovecot")

	if missing := MissingMailServices(context.Background()); len(missing) > 0 {
		t.Errorf("MissingMailServices on a healthy stack = %v, want none", missing)
	}
}

// Dovecot is not just IMAP: Postfix hands every message to it over LMTP
// (virtual_transport in main.cf.append), so with Dovecot down nothing is
// delivered even though Postfix is running and looks fine.
func TestOnlyTheServiceThatIsDownIsNamed(t *testing.T) {
	stubMailServices(t, "postfix")

	missing := MissingMailServices(context.Background())
	if len(missing) != 1 || missing[0] != "dovecot" {
		t.Errorf("MissingMailServices = %v, want [dovecot]", missing)
	}
}

// Postfix sets milter_default_action=accept, so it accepts messages when the
// milters are unreachable. Mail flows without Rspamd and OpenDKIM; it only loses
// spam filtering and DKIM signing. Requiring them would refuse a server that
// works perfectly well.
func TestSpamAndSigningServicesAreNotRequired(t *testing.T) {
	stubMailServices(t, "postfix", "dovecot")

	if missing := MissingMailServices(context.Background()); len(missing) > 0 {
		t.Fatalf("unexpected missing services: %v", missing)
	}
	for _, unit := range []string{"rspamd", "opendkim"} {
		if slices.Contains(requiredMailServices, unit) {
			t.Errorf("%s is required, so a server without spam filtering could not enable mail at all", unit)
		}
	}
}

// The behaviour actually being protected. Enabling publishes MX, SPF, DKIM and
// DMARC, so running it against a dead stack tells the world this server takes
// the domain's mail while nothing is listening. Refusing has to happen BEFORE
// any of that, which means EnableDomain must not be reached at all.
func TestEnableIsRefusedAndWritesNothingWhenTheStackIsDown(t *testing.T) {
	stubMailServices(t) // nothing running
	db, recorder := purgeHarness(t, "", nil)

	handlers := &Handlers{DB: db}
	router := chi.NewRouter()
	router.Post("/domains/{id}/mail/enable", handlers.Enable)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/domains/1/mail/enable", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	// EnableDomain's first act is to read the domain's name and IPv4 for the DNS
	// seed. That statement appearing at all means the refusal came too late.
	for _, step := range recorder.recorded() {
		if strings.Contains(step, "domain_name") && strings.Contains(step, "ipv4") {
			t.Fatalf("EnableDomain ran despite the refusal: %q", step)
		}
		if strings.Contains(step, "INSERT INTO mail_domains") {
			t.Fatalf("the mail domain row was written despite the refusal: %q", step)
		}
	}
}
