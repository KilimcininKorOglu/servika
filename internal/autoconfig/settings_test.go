package autoconfig

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

// The panel's connection card and the two discovery endpoints must hand out the
// same thing. If they can drift, a customer copies a port off the card, the
// client is told a different one by autoconfig, and only one of the two accounts
// works.
func TestSettingsForMatchesWhatThunderbirdIsTold(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SERVIKA_CERT_ROOT", root)
	writeMailCertificate(t, root, "example.com", time.Now().Add(30*24*time.Hour), "mail.example.com")

	h := &Handlers{DB: failingDB(t)}
	settings, err := SettingsFor(context.Background(), h.DB, "example.com")
	if err != nil {
		t.Fatalf("SettingsFor: %v", err)
	}

	// The XML the client actually receives, built the same way the handler builds
	// it, so this comparison breaks if either side is edited alone.
	host, _, err := announceableHost(context.Background(), h.DB, "example.com")
	if err != nil {
		t.Fatalf("announceableHost: %v", err)
	}
	incoming := serverConfig{
		Type: "imap", Hostname: host, Port: imapPort,
		SocketType: socketTypeSTARTTLS, Username: usernameIsFullAddress,
		Authentication: "password-cleartext",
	}
	if settings.Hostname != incoming.Hostname {
		t.Errorf("hostname = %q, but the client is told %q", settings.Hostname, incoming.Hostname)
	}
	if settings.IMAPPort != incoming.Port {
		t.Errorf("IMAP port = %d, but the client is told %d", settings.IMAPPort, incoming.Port)
	}
	if settings.Security != incoming.SocketType {
		t.Errorf("security = %q, but the client is told %q", settings.Security, incoming.SocketType)
	}
	if settings.SubmissionPort != submissionPort {
		t.Errorf("submission port = %d, want %d", settings.SubmissionPort, submissionPort)
	}
}

// Implicit TLS is never announced, because servika-mail-setup opens 25, 587 and
// 143 and nothing else. A card printing 993 produces an account that configures
// itself and then hangs on its first connection.
func TestSettingsNeverAnnounceAPortTheFirewallKeepsShut(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SERVIKA_CERT_ROOT", root)
	writeMailCertificate(t, root, "example.com", time.Now().Add(30*24*time.Hour), "mail.example.com")

	settings, err := SettingsFor(context.Background(), (&Handlers{DB: failingDB(t)}).DB, "example.com")
	if err != nil {
		t.Fatalf("SettingsFor: %v", err)
	}
	for _, shut := range []int{993, 465} {
		if settings.IMAPPort == shut || settings.SubmissionPort == shut {
			t.Errorf("port %d is announced but the firewall does not open it", shut)
		}
	}
	if settings.Security != "STARTTLS" {
		t.Errorf("security = %q, but both ports negotiate STARTTLS", settings.Security)
	}
}

// A hostname is never invented. Until a certificate covers one the caller has to
// be told there is none, so the screen can say the certificate is pending rather
// than print a name the client will warn about.
func TestSettingsRefuseToGuessAHostname(t *testing.T) {
	t.Setenv("SERVIKA_CERT_ROOT", t.TempDir())

	settings, err := SettingsFor(context.Background(), (&Handlers{DB: failingDB(t)}).DB, "example.com")
	if err == nil {
		t.Fatalf("SettingsFor returned %q with no certificate and no database", settings.Hostname)
	}
	if settings.Hostname != "" {
		t.Errorf("SettingsFor returned %q alongside an error", settings.Hostname)
	}
	// The request is unused here beyond proving the handler path shares this
	// refusal rather than having its own.
	if _, _, ok := (&Handlers{DB: failingDB(t)}).resolve(httptest.NewRecorder(),
		httptest.NewRequest("GET", "http://example.com/", nil)); ok {
		t.Error("the discovery endpoint answered while SettingsFor refused")
	}
}
