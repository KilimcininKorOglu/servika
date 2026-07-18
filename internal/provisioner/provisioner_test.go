package provisioner

import (
	"bytes"
	"strings"
	"testing"
)

func TestDangerousNginxDirectiveRejectsPrivilegedOperations(t *testing.T) {
	tests := []struct {
		name       string
		directives string
		want       string
	}{
		{name: "proxy SSRF", directives: "location /internal { proxy_pass http://127.0.0.1; }", want: "proxy_pass"},
		{name: "local file disclosure", directives: "location /secret { alias /etc/; }", want: "alias"},
		{name: "module loading", directives: "load_module modules/ngx_http_perl_module.so;", want: "load_module"},
		{name: "Lua execution", directives: "content_by_lua_block { ngx.say('unsafe') }", want: "content_by_lua_block"},
		{name: "commented directive", directives: "# proxy_pass http://127.0.0.1;\nclient_max_body_size 10m;", want: ""},
		{name: "safe directive", directives: "client_max_body_size 10m;\nadd_header X-Test safe;", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := DangerousNginxDirective(test.directives); got != test.want {
				t.Fatalf("DangerousNginxDirective() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestManagedCacheZoneMatchesVhostUsage(t *testing.T) {
	var rendered bytes.Buffer
	if err := vhostTmpl.Execute(&rendered, VhostOpts{
		DomainName:          "example.com",
		WebRoot:             "/home/c_example_com/public_html",
		PHPSocket:           "/run/php-fpm/c_example_com.sock",
		PHPVersion:          "8.3",
		FastCgiCache:        true,
		FastCgiCacheMinutes: 60,
	}); err != nil {
		t.Fatalf("render vhost: %v", err)
	}

	if usage := "fastcgi_cache " + cacheZoneName + ";"; !strings.Contains(rendered.String(), usage) {
		t.Fatalf("vhost does not use managed cache zone %q", cacheZoneName)
	}
	if definition := "keys_zone=" + cacheZoneName + ":"; !strings.Contains(cacheZoneBody, definition) {
		t.Fatalf("managed configuration does not define cache zone %q", cacheZoneName)
	}
}

func TestTenantVhostAppliesHardeningAtEveryHeaderBoundary(t *testing.T) {
	opts := VhostOpts{
		DomainName:       "example.com",
		WebRoot:          "/home/c_example_com/public_html",
		PHPSocket:        "/run/php-fpm/c_example_com.sock",
		PHPVersion:       "8.3",
		HdrXContentType:  true,
		HdrXXSS:          true,
		HdrReferrer:      true,
		HdrPermissions:   true,
		BrowserCache:     true,
		BrowserCacheDays: 30,
	}
	opts.SecHeaders = buildSecurityHeaders(opts)
	opts.DenyBlocks = denyBlocksNginx

	var rendered bytes.Buffer
	if err := vhostTmpl.Execute(&rendered, opts); err != nil {
		t.Fatalf("render vhost: %v", err)
	}
	config := rendered.String()

	for _, directive := range []string{
		"disable_symlinks if_not_owner;",
		`location ~* \.(cgi|pl|py|sh|rb|lua|fcgi)$ { deny all; }`,
		`location ~* \.(sql|sql\.gz|bak|old|orig|save|swp|swo|dump|inc|log|php\.bak|php~|php\.save)$ { deny all; }`,
	} {
		if !strings.Contains(config, directive) {
			t.Errorf("vhost does not contain hardening directive %q", directive)
		}
	}
	if count := strings.Count(config, `add_header X-Frame-Options "SAMEORIGIN" always;`); count != 3 {
		t.Errorf("X-Frame-Options appears %d times, want server, PHP, and browser-cache locations", count)
	}
	if strings.Contains(config, "Strict-Transport-Security") {
		t.Error("HTTP-only vhost must not emit HSTS")
	}
	browserCacheLocation := `location ~* \.(jpg|jpeg|png|gif|ico|css|js|woff2?|svg|webp|avif|mp4|webm|pdf|zip|gz)$ {`
	if !strings.Contains(config, browserCacheLocation) {
		t.Error("browser-cache location must allow static files and legitimate ZIP or GZIP downloads")
	}
	for _, archiveExtension := range []string{"|tar|", "|tgz|", "|zip|", "|rar|", "|7z|"} {
		if strings.Contains(denyBlocksNginx, archiveExtension) {
			t.Errorf("sensitive-file deny block must not reject legitimate archive extension %q", archiveExtension)
		}
	}
}

func TestTLSVhostRepeatsHSTSAtEveryHeaderBoundary(t *testing.T) {
	opts := VhostOpts{
		DomainName:       "example.com",
		WebRoot:          "/home/c_example_com/public_html",
		PHPSocket:        "/run/php-fpm/c_example_com.sock",
		PHPVersion:       "8.3",
		CertPath:         "/etc/letsencrypt/live/example.com/fullchain.pem",
		KeyPath:          "/etc/letsencrypt/live/example.com/privkey.pem",
		HdrHSTS:          true,
		HSTSMaxAge:       31536000,
		HSTSSubdomains:   true,
		BrowserCache:     true,
		BrowserCacheDays: 30,
	}
	opts.SecHeaders = buildSecurityHeaders(opts)
	opts.DenyBlocks = denyBlocksNginx

	var rendered bytes.Buffer
	if err := vhostTmpl.Execute(&rendered, opts); err != nil {
		t.Fatalf("render TLS vhost: %v", err)
	}
	if count := strings.Count(rendered.String(), "add_header Strict-Transport-Security"); count != 3 {
		t.Errorf("HSTS appears %d times, want server, PHP, and browser-cache locations", count)
	}
}

func TestPHPPoolConfinesTenantAndDisablesProcessExecution(t *testing.T) {
	var rendered bytes.Buffer
	if err := phpPoolTmpl.Execute(&rendered, map[string]string{
		"User":   "c_example_com",
		"Socket": "/run/php-fpm/c_example_com.sock",
	}); err != nil {
		t.Fatalf("render PHP pool: %v", err)
	}
	config := rendered.String()

	if !strings.Contains(config, "php_admin_value[open_basedir] = /home/c_example_com/:/tmp/") {
		t.Error("PHP pool does not confine filesystem access to the tenant home and temporary directory")
	}
	for _, function := range []string{"exec", "proc_open", "pcntl_exec", "symlink", "posix_setuid"} {
		if !strings.Contains(config, function) {
			t.Errorf("PHP pool does not disable %q", function)
		}
	}
}

func TestApacheVhostDeniesScriptsBackupsAndForeignSymlinks(t *testing.T) {
	var rendered bytes.Buffer
	if err := apacheVhostTmpl.Execute(&rendered, VhostOpts{
		DomainName: "example.com",
		WebRoot:    "/home/c_example_com/public_html",
		PHPSocket:  "/run/php-fpm/c_example_com.sock",
	}); err != nil {
		t.Fatalf("render Apache vhost: %v", err)
	}
	config := rendered.String()

	for _, directive := range []string{
		"Options -ExecCGI -Indexes -Includes -FollowSymLinks +SymLinksIfOwnerMatch",
		"RemoveHandler .cgi .pl .py .sh .rb .lua .fcgi .fpl",
		`<FilesMatch "\.(cgi|pl|py|sh|rb|lua|fcgi)$">`,
		`<FilesMatch "\.(sql|sql\.gz|bak|old|orig|save|swp|swo|dump|inc|log)$">`,
		`<FilesMatch "\.(php\.bak|php~|php\.save)$">`,
		"AllowOverride AuthConfig FileInfo Indexes Limit Options=Indexes,MultiViews",
	} {
		if !strings.Contains(config, directive) {
			t.Errorf("Apache vhost does not contain hardening directive %q", directive)
		}
	}
	for _, archiveExtension := range []string{"|tar|", "|tgz|", "|zip|", "|rar|", "|7z|"} {
		if strings.Contains(config, archiveExtension) {
			t.Errorf("Apache vhost must not reject legitimate archive extension %q", archiveExtension)
		}
	}
}
