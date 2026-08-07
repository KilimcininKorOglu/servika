package provisioner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Without Roundcube no block may be produced, or every vhost would carry a path
// that answers 404.
func TestWebmailBlockIsEmptyWithoutRoundcube(t *testing.T) {
	if got := webmailBlockFor(filepath.Join(t.TempDir(), "absent")); got != "" {
		t.Errorf("a block was produced without Roundcube: %q", got)
	}
}

// A file where the web root should be is not an installation either.
func TestWebmailBlockIsEmptyWhenTheRootIsNotADirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "public_html")
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write the stand-in: %v", err)
	}
	if got := webmailBlockFor(path); got != "" {
		t.Errorf("a block was produced for a plain file: %q", got)
	}
}

func TestWebmailBlockCarriesTheRoundcubeDirectives(t *testing.T) {
	block := webmailBlockFor(t.TempDir())
	if block == "" {
		t.Fatal("no block was produced for an installed Roundcube")
	}
	for _, want := range []string{
		"location ^~ /webmail/",
		"unix:/run/php-fpm/roundcube.sock",
		"location = /webmail { return 301 /webmail/; }",
		"(config|temp|logs|SQL|bin|tests)",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("the block does not contain %q", want)
		}
	}
	// static.php has to be matched BEFORE the extension-based static block:
	// Roundcube serves skin and plugin assets through it with PATH_INFO, and the
	// other way round nginx treats those as missing .css files and answers 404.
	if strings.Index(block, `static\.php`) > strings.Index(block, "jpg|jpeg|gif") {
		t.Error("the static.php location comes after the static extension location")
	}
	// No location inside the block may define add_header: that would drop the
	// domain's own inherited security headers, HSTS included, and replace a
	// per-domain setting with a hardcoded policy.
	if strings.Contains(block, "add_header") {
		t.Error("the block defines add_header and so discards the inherited headers")
	}
}

// The block is written into every TLS vhost on the host, so a syntax error in it
// takes every hosted site down at the next reload.
func TestWebmailBlockIsValidNginxSyntax(t *testing.T) {
	prefix := t.TempDir()
	if err := os.WriteFile(filepath.Join(prefix, "fastcgi_params"),
		[]byte("fastcgi_param SCRIPT_NAME $fastcgi_script_name;\n"), 0o600); err != nil {
		t.Fatalf("write the fastcgi_params stand-in: %v", err)
	}
	body := "events {}\nhttp {\n    server {\n        listen 8080;\n        server_name example.com;\n" +
		webmailNginx + "    }\n}\n"
	checkNginxSyntax(t, prefix, body, "the webmail block is not valid nginx syntax")
}

// The repair recognises its own work through these two markers; a rename that
// left them behind would turn the repair into a silent no-op.
func TestWebmailRepairMarkersMatchTheRenderedConfiguration(t *testing.T) {
	if !strings.Contains(webmailNginx, webmailMarker) {
		t.Errorf("%q does not appear in the webmail block", webmailMarker)
	}
	if !strings.Contains(denyBlocksNginx, normalVhostMarker) {
		t.Errorf("%q does not appear in the deny blocks", normalVhostMarker)
	}
}

// The block reaches the TLS vhost, and only the TLS vhost.
func TestWebmailBlockIsRenderedOnlyOnTheTLSVhost(t *testing.T) {
	base := VhostOpts{
		DomainName:   "example.com",
		WebRoot:      "/home/example/public_html",
		PHPSocket:    "/run/php-fpm/example.sock",
		Backend:      "php-fpm",
		WebmailBlock: webmailNginx,
	}

	secure := base
	secure.CertPath = "/etc/pki/servika/example.com/fullchain.pem"
	secure.KeyPath = "/etc/pki/servika/example.com/privkey.pem"
	var withTLS bytes.Buffer
	if err := vhostTmpl.Execute(&withTLS, secure); err != nil {
		t.Fatalf("TLS vhost render: %v", err)
	}
	if !strings.Contains(withTLS.String(), webmailMarker) {
		t.Errorf("the TLS vhost does not carry the webmail block:\n%s", withTLS.String())
	}

	var withoutTLS bytes.Buffer
	if err := vhostTmpl.Execute(&withoutTLS, base); err != nil {
		t.Fatalf("plain vhost render: %v", err)
	}
	if strings.Contains(withoutTLS.String(), webmailMarker) {
		t.Error("the plain HTTP vhost carries the webmail block, which would expose mailbox passwords")
	}
}
