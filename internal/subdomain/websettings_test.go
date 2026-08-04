package subdomain

import (
	"strings"
	"testing"

	"servika/internal/nginxset"
)

func TestStaticBackendRemovesTheFastCGIPass(t *testing.T) {
	web := renderWebSettings(nginxset.Defaults(), "app.example.com", false)
	web.Static = true
	config := vhost("app.example.com", "/home/c_example_com/subdomains/app.example.com",
		"/run/php-fpm-c_example_com/sub-3.sock", "", web)
	if strings.Contains(config, "fastcgi_pass") {
		t.Error("a static subdomain vhost still passes requests to PHP-FPM")
	}
	if !strings.Contains(config, "location / { try_files $uri $uri/ =404; }") {
		t.Error("a static subdomain vhost does not serve files directly")
	}
}

func TestSubdomainVhostRendersTheStoredCacheSettings(t *testing.T) {
	settings := nginxset.Defaults()
	settings.FastCgiCache = true
	settings.FastCgiCacheMinutes = 15
	settings.BrowserCacheDays = 7
	web := renderWebSettings(settings, "app.example.com", false)
	config := vhost("app.example.com", "/home/c_example_com/subdomains/app.example.com",
		"/run/php-fpm-c_example_com/sub-3.sock", "", web)
	for _, want := range []string{
		"fastcgi_cache servikacache;",
		"fastcgi_cache_valid 200 301 302 15m;",
		"set $skip_cache 0;",
		"expires 7d;",
	} {
		if !strings.Contains(config, want) {
			t.Errorf("subdomain vhost is missing %q", want)
		}
	}
}

func TestSubdomainHSTSOnlyOnTheHTTPSVhost(t *testing.T) {
	settings := nginxset.Defaults()
	settings.HdrHSTS = true
	if plain := renderWebSettings(settings, "app.example.com", false); strings.Contains(plain.Headers, "Strict-Transport-Security") {
		t.Error("the plain HTTP subdomain vhost emits HSTS")
	}
	secure := renderWebSettings(settings, "app.example.com", true)
	if !strings.Contains(secure.Headers, "Strict-Transport-Security") {
		t.Error("the HTTPS subdomain vhost does not emit HSTS")
	}
}

func TestDisabledSecurityHeadersAreNotRendered(t *testing.T) {
	settings := nginxset.Defaults()
	settings.HdrXContentType = false
	settings.HdrReferrer = false
	web := renderWebSettings(settings, "app.example.com", false)
	if strings.Contains(web.Headers, "X-Content-Type-Options") {
		t.Error("a disabled header is still rendered")
	}
	if strings.Contains(web.Headers, "Referrer-Policy") {
		t.Error("a disabled header is still rendered")
	}
	if !strings.Contains(web.Headers, "X-XSS-Protection") {
		t.Error("an enabled header was dropped")
	}
}

func TestSubdomainInheritsParentExtraDirectivesUntilConfigured(t *testing.T) {
	// The plan's client_max_body_size reaches a domain through the extra_directives
	// of its own nginx_settings row. A subdomain with no row of its own must render
	// that same directive, or it would reject uploads the parent accepts.
	parent := nginxset.Defaults()
	parent.ExtraDirectives = "client_max_body_size 64m;"
	web := renderWebSettings(parent, "app.example.com", false)
	config := vhost("app.example.com", "/home/c_example_com/subdomains/app.example.com",
		"/run/php-fpm-c_example_com/sub-3.sock", "", web)
	if !strings.Contains(config, "client_max_body_size 64m;") {
		t.Error("the subdomain vhost dropped the inherited upload limit")
	}
}
