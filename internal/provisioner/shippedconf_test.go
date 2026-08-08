package provisioner

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Everything under assets/nginx that the installer copies verbatim into
// /etc/nginx/conf.d. php-fpm.conf is a PHP-FPM pool file and is excluded.
var shippedNginxConfs = []string{"_panel.conf", "_default80.conf", "_default443.conf"}

// listenPort matches the port in a `listen` directive, with or without the IPv6
// `[::]:` form. Anchored to the directive so no other number in the file can be
// caught by it.
var listenPort = regexp.MustCompile(`(?m)^(\s*listen\s+(?:\[::\]:)?)(\d+)\b`)

// unprivilegedPorts are the ports the catch-all vhosts claim, mapped somewhere a
// test process is allowed to bind. They stay clear of 8443, which the panel vhost
// claims and which is already unprivileged.
var unprivilegedPorts = map[string]string{"80": "18080", "443": "18443"}

// unprivilegedListen rewrites the privileged ports out of the shipped listen
// directives.
//
// The nginx builds on the CI runners open the listening sockets while testing a
// configuration, and the runner is not root, so a shipped `listen 80` fails with
// "bind() to 0.0.0.0:80 failed (13: Permission denied)" AFTER nginx has already
// printed "syntax is ok". That is the sandbox refusing a privileged port, not a
// defect in the file, and letting it fail the gate means the gate reports the
// machine rather than the configuration. macOS hides this: it lets an
// unprivileged process bind a low port, so the same test passes on a developer's
// machine and fails in a clean Linux container.
//
// Only the port NUMBER changes. Every directive, including default_server and
// the ssl flag, is still checked exactly as it ships, and the rewritten ports
// stay distinct from each other so two vhosts claiming default_server on one
// port would still be rejected.
func unprivilegedListen(conf string) string {
	return listenPort.ReplaceAllStringFunc(conf, func(match string) string {
		parts := listenPort.FindStringSubmatch(match)
		mapped, ok := unprivilegedPorts[parts[2]]
		if !ok {
			return match
		}
		return parts[1] + mapped
	})
}

// Nothing validated these files before. The installer copies them onto a fresh
// host and the panel now ships the two catch-alls to existing hosts as well, so
// a syntax error in any of them means nginx refuses to start on the next reboot
// and takes the panel and every hosted site with it. CI cannot reach a real
// host, but it can reach nginx -t.
//
// All three are loaded together, which also proves they can coexist: each one
// claims default_server, and two claiming it on the same port would be rejected.
func TestShippedNginxConfsAreValidSyntax(t *testing.T) {
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
		// Only what the sandbox cannot provide is rewritten (paths it cannot reach,
		// ports it may not bind); the directives themselves are checked exactly as
		// they ship.
		rewritten := strings.NewReplacer(
			"/etc/ssl/servika/panel.crt", certPath,
			"/etc/ssl/servika/panel.key", keyPath,
			"/var/log/nginx/", prefix+"/",
		).Replace(string(content))
		body.WriteString(unprivilegedListen(rewritten))
		body.WriteString("\n")
	}
	body.WriteString("}\n")

	checkNginxSyntax(t, prefix, body.String(), "the shipped nginx configuration is not valid")
}

// The rewrite above decides what the gate is allowed to touch, so it is worth
// pinning: too little and CI still fails on a port it may not bind, too much and
// the gate stops checking the file that actually ships.
func TestUnprivilegedListen(t *testing.T) {
	t.Run("moves the privileged ports off both address forms", func(t *testing.T) {
		got := unprivilegedListen("server {\n    listen 80 default_server;\n    listen [::]:443 ssl default_server;\n}\n")
		for _, want := range []string{"listen 18080 default_server;", "listen [::]:18443 ssl default_server;"} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in:\n%s", want, got)
			}
		}
		if strings.Contains(got, "listen 80 ") || strings.Contains(got, ":443 ") {
			t.Errorf("a privileged port survived:\n%s", got)
		}
	})

	t.Run("leaves a port the sandbox can already bind alone", func(t *testing.T) {
		const conf = "    listen 8443 ssl default_server;\n    listen [::]:8443 ssl default_server;\n"
		if got := unprivilegedListen(conf); got != conf {
			t.Errorf("8443 was rewritten:\n%s", got)
		}
	})

	// The two catch-alls both claim default_server. If the rewrite collapsed them
	// onto one port, nginx would reject the pair and the gate would report a
	// defect the shipped files do not have.
	t.Run("keeps the rewritten ports distinct", func(t *testing.T) {
		if unprivilegedPorts["80"] == unprivilegedPorts["443"] {
			t.Fatal("80 and 443 map to the same port")
		}
		for from, to := range unprivilegedPorts {
			if to == "8443" {
				t.Errorf("%s maps onto the panel's own port", from)
			}
		}
	})

	// Anchoring to the directive is the whole reason this is a regexp rather than
	// a string replace: 80 and 443 appear in other values too.
	t.Run("touches nothing outside a listen directive", func(t *testing.T) {
		const conf = "    client_max_body_size 80m;\n    error_page 443 /x;\n    # listen 80 in a comment\n"
		if got := unprivilegedListen(conf); got != conf {
			t.Errorf("rewrote something that was not a listen directive:\n%s", got)
		}
	})
}
