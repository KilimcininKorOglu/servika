package provisioner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Everything under assets/nginx that the installer copies verbatim into
// /etc/nginx/conf.d. php-fpm.conf is a PHP-FPM pool file and is excluded.
var shippedNginxConfs = []string{"_panel.conf", "_default80.conf", "_default443.conf"}

// Nothing validated these files before. The installer copies them onto a fresh
// host and the panel now ships the two catch-alls to existing hosts as well, so
// a syntax error in any of them means nginx refuses to start on the next reboot
// and takes the panel and every hosted site with it. CI cannot reach a real
// host, but it can reach nginx -t.
//
// All three are loaded together, which also proves they can coexist: each one
// claims default_server, and two claiming it on the same port would be rejected.
func TestShippedNginxConfsAreValidSyntax(t *testing.T) {
	nginx, err := exec.LookPath("nginx")
	if err != nil {
		t.Skip("nginx is unavailable")
	}
	prefix := t.TempDir()

	// nginx -t opens the certificate for real, so the vhosts need one that
	// exists. Everything else the files name (roots, aliases, FastCGI sockets)
	// is not resolved at configuration-test time.
	certPath := filepath.Join(prefix, "panel.crt")
	keyPath := filepath.Join(prefix, "panel.key")
	writeCertificateFixture(t, certPath, keyPath, "panel.example.com", true)

	// A relative include resolves against the prefix, so a stand-in is enough.
	if err := os.WriteFile(filepath.Join(prefix, "fastcgi_params"),
		[]byte("fastcgi_param SCRIPT_NAME $fastcgi_script_name;\n"), 0o600); err != nil {
		t.Fatalf("write the fastcgi_params stand-in: %v", err)
	}

	var body strings.Builder
	body.WriteString("events {}\nhttp {\n")
	for _, name := range shippedNginxConfs {
		content, err := os.ReadFile(filepath.Join("..", "..", "assets", "nginx", name))
		if err != nil {
			t.Fatalf("read assets/nginx/%s: %v", name, err)
		}
		// Only the paths the sandbox cannot provide are rewritten; the directives
		// themselves are checked exactly as they ship.
		rewritten := strings.NewReplacer(
			"/etc/ssl/servika/panel.crt", certPath,
			"/etc/ssl/servika/panel.key", keyPath,
			"/var/log/nginx/", prefix+"/",
		).Replace(string(content))
		body.WriteString(rewritten)
		body.WriteString("\n")
	}
	body.WriteString("}\n")

	config := filepath.Join(prefix, "nginx.conf")
	if err := os.WriteFile(config, []byte(body.String()), 0o600); err != nil {
		t.Fatalf("write the configuration: %v", err)
	}
	out, err := exec.Command(nginx, "-t", "-p", prefix, "-c", config, "-e", "stderr").CombinedOutput()
	if err != nil {
		t.Fatalf("the shipped nginx configuration is not valid: %v\n%s\n%s", err, out, body.String())
	}
}
