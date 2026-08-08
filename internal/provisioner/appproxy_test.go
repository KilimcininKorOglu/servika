package provisioner

import (
	"bytes"
	"strings"
	"testing"
)

func renderVhost(t *testing.T, opts VhostOpts) string {
	t.Helper()
	opts.SecHeaders = buildSecurityHeaders(opts)
	opts.DenyBlocks = denyBlocksNginx
	var buf bytes.Buffer
	if err := vhostTmpl.Execute(&buf, opts); err != nil {
		t.Fatalf("render the vhost: %v", err)
	}
	return buf.String()
}

func baseOpts() VhostOpts {
	return VhostOpts{
		DomainName: "example.com",
		WebRoot:    "/home/c_example/public_html",
		PHPSocket:  "/run/php-fpm-c_example/c_example.sock",
		PHPVersion: "8.3",
	}
}

// The prefix is `^~` rather than a plain prefix because nginx tries regular
// expressions BEFORE a plain prefix match. Without it a request for
// /api/index.php would be handed to PHP-FPM out of the document root instead of
// reaching the application: the tenant would be serving their own source.
func TestAnApplicationLocationBeatsThePHPRegularExpression(t *testing.T) {
	proxies := []appProxy{{Name: "api", Mount: "/api/", Port: 30007}}
	opts := baseOpts()
	opts.AppBlocks, opts.AppOwnsRoot = renderAppProxies(proxies), appOwnsRoot(proxies)
	config := renderVhost(t, opts)

	if !strings.Contains(config, "location ^~ /api/ {") {
		t.Fatalf("the application location is missing or not a ^~ prefix:\n%s", config)
	}
	if strings.Contains(config, "location /api/ {") {
		t.Error("a plain prefix survived, which the .php regular expression would beat")
	}
	// The PHP location must still exist for the rest of the site.
	if !strings.Contains(config, `location ~ \.php$`) {
		t.Error("the PHP location disappeared")
	}
	if !strings.Contains(config, "proxy_pass http://127.0.0.1:30007;") {
		t.Error("the proxy does not point at the allocated loopback port")
	}
}

// A WebSocket upgrade needs both the 1.1 version and the mapped Connection
// header; either one alone silently downgrades the connection.
func TestTheProxyCarriesAWebSocketUpgrade(t *testing.T) {
	config := renderAppProxies([]appProxy{{Name: "ws", Mount: "/ws/", Port: 30010}})
	for _, want := range []string{
		"proxy_http_version 1.1;",
		"proxy_set_header Upgrade $http_upgrade;",
		"proxy_set_header Connection $connection_upgrade;",
	} {
		if !strings.Contains(config, want) {
			t.Errorf("the proxy is missing %q:\n%s", want, config)
		}
	}
	if !strings.Contains(upgradeMapBody(), "map $http_upgrade $connection_upgrade") {
		t.Error("the map that defines $connection_upgrade is missing, so nginx would reject the whole configuration")
	}
}

// An application at "/" takes the domain. Emitting the vhost's own `location /`
// as well would be a duplicate prefix, which nginx refuses outright, so the
// whole site would go down rather than the application taking over.
func TestAnApplicationAtTheRootReplacesTheDefaultLocation(t *testing.T) {
	proxies := []appProxy{{Name: "app", Mount: "/", Port: 30011}}
	for _, backend := range []string{"php-fpm", "static", "apache"} {
		opts := baseOpts()
		opts.Backend = backend
		opts.AppBlocks, opts.AppOwnsRoot = renderAppProxies(proxies), appOwnsRoot(proxies)
		config := renderVhost(t, opts)

		if count := strings.Count(config, "location / {"); count != 0 {
			t.Errorf("backend %s still emits its own root location %d time(s):\n%s", backend, count, config)
		}
		if !strings.Contains(config, "location ^~ / {") {
			t.Errorf("backend %s lost the application's root location:\n%s", backend, config)
		}
	}
}

// The opposite direction: without a root application every backend keeps the
// root location it always had.
func TestWithoutARootApplicationEveryBackendKeepsItsOwnRoot(t *testing.T) {
	for _, backend := range []string{"php-fpm", "static", "apache"} {
		opts := baseOpts()
		opts.Backend = backend
		config := renderVhost(t, opts)
		if !strings.Contains(config, "location / {") {
			t.Errorf("backend %s lost its root location:\n%s", backend, config)
		}
	}
}

// A domain with no applications must render exactly as it did before this
// feature existed, so the blocks are absent rather than empty-but-present.
func TestADomainWithoutApplicationsGainsNothing(t *testing.T) {
	config := renderVhost(t, baseOpts())
	// `location ^~ /_srv/` is the panel's own error-page assets and predates
	// applications, so the markers here are the ones only a proxy emits.
	for _, unwanted := range []string{
		"# ---- Applications", "proxy_pass http://127.0.0.1:30", "$connection_upgrade",
	} {
		if strings.Contains(config, unwanted) {
			t.Errorf("an unused application directive leaked in: %q", unwanted)
		}
	}
}

// A longer mount is emitted first so /api/v2/ is matched before /api/. nginx
// picks the longest matching prefix on its own, but the order is what an
// operator reads, and it must not contradict what nginx does.
func TestLongerMountsAreListedFirst(t *testing.T) {
	config := renderAppProxies([]appProxy{
		{Name: "v2", Mount: "/api/v2/", Port: 30020},
		{Name: "api", Mount: "/api/", Port: 30021},
	})
	deep, shallow := strings.Index(config, "/api/v2/"), strings.Index(config, "location ^~ /api/ {")
	if deep < 0 || shallow < 0 {
		t.Fatalf("a location is missing:\n%s", config)
	}
	if deep > shallow {
		t.Errorf("the shallower mount is listed first:\n%s", config)
	}
}
