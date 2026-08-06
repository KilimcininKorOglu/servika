package provisioner

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The block goes into every TLS vhost on the host, so a syntax error in it takes
// every hosted site down at the next reload.
func TestAutoconfigBlockIsValidNginxSyntax(t *testing.T) {
	nginx, err := exec.LookPath("nginx")
	if err != nil {
		t.Skip("nginx is unavailable")
	}
	prefix := t.TempDir()
	config := filepath.Join(prefix, "nginx.conf")
	body := "events {}\nhttp {\n    server {\n        listen 8080;\n        server_name example.com;\n" +
		autoconfigNginx + "    }\n}\n"
	if err := os.WriteFile(config, []byte(body), 0o600); err != nil {
		t.Fatalf("write the configuration: %v", err)
	}
	out, err := exec.Command(nginx, "-t", "-p", prefix, "-c", config, "-e", "stderr").CombinedOutput()
	if err != nil {
		t.Fatalf("the autoconfig block is not valid nginx syntax: %v\n%s\n%s", err, out, body)
	}
}

// Defining add_header in a location replaces the whole inherited set, so a
// domain's own security headers (HSTS above all) would silently stop applying to
// these two paths.
func TestAutoconfigBlockInheritsTheDomainHeaders(t *testing.T) {
	if strings.Contains(autoconfigNginx, "add_header") {
		t.Error("the block defines add_header and so discards the inherited headers")
	}
}

// The repair recognises its own work through this marker; a rename that left it
// behind would turn the repair into a silent no-op.
func TestAutoconfigRepairMarkerMatchesTheRenderedConfiguration(t *testing.T) {
	if !strings.Contains(autoconfigNginx, autoconfigMarker) {
		t.Errorf("%q does not appear in the autoconfig block", autoconfigMarker)
	}
}

// The block reaches the TLS vhost, and only the TLS vhost: over plain HTTP these
// endpoints would tell a client where to send a password on the word of whoever
// is on the network path.
func TestAutoconfigBlockIsRenderedOnlyOnTheTLSVhost(t *testing.T) {
	base := VhostOpts{
		DomainName:      "example.com",
		WebRoot:         "/home/example/public_html",
		PHPSocket:       "/run/php-fpm/example.sock",
		Backend:         "php-fpm",
		AutoconfigBlock: autoconfigNginx,
	}

	secure := base
	secure.CertPath = "/etc/pki/servika/example.com/fullchain.pem"
	secure.KeyPath = "/etc/pki/servika/example.com/privkey.pem"
	var withTLS bytes.Buffer
	if err := vhostTmpl.Execute(&withTLS, secure); err != nil {
		t.Fatalf("TLS vhost render: %v", err)
	}
	if !strings.Contains(withTLS.String(), autoconfigMarker) {
		t.Errorf("the TLS vhost does not carry the autoconfig block:\n%s", withTLS.String())
	}

	var withoutTLS bytes.Buffer
	if err := vhostTmpl.Execute(&withoutTLS, base); err != nil {
		t.Fatalf("plain vhost render: %v", err)
	}
	if strings.Contains(withoutTLS.String(), autoconfigMarker) {
		t.Error("the plain HTTP vhost carries the autoconfig block")
	}
}

// The repair must re-render a vhost that is missing any expected block, and
// leave one that already has them all alone. Getting that backwards either
// reloads nginx on every boot for nothing or never applies the block at all.
func TestMissingAnyBlockDrivesTheRepair(t *testing.T) {
	required := []string{autoconfigMarker, webmailMarker}
	complete := autoconfigMarker + "\n" + webmailMarker
	if missingAnyBlock(complete, required) {
		t.Error("a vhost carrying both blocks was reported as needing repair")
	}
	if !missingAnyBlock(webmailMarker, required) {
		t.Error("a vhost without the autoconfig block was not reported as needing repair")
	}
	if missingAnyBlock("", nil) {
		t.Error("with nothing required, no vhost needs repair")
	}
}
