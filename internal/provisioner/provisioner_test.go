package provisioner

import (
	"bytes"
	"os"
	"path/filepath"
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
	if definition := "keys_zone=" + cacheZoneName + ":"; !strings.Contains(cacheZoneBody(), definition) {
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

func TestLegacyTenantHomePermissionsBlockOtherUsersAndPermitWebGroup(t *testing.T) {
	home := t.TempDir()
	publicHTML := filepath.Join(home, "public_html")
	if err := os.Mkdir(publicHTML, 0755); err != nil {
		t.Fatalf("create public_html: %v", err)
	}

	applyLegacyHomePerms(home, os.Getuid(), os.Getgid())

	homeInfo, err := os.Stat(home)
	if err != nil {
		t.Fatalf("stat tenant home: %v", err)
	}
	if got := homeInfo.Mode().Perm(); got != 0710 {
		t.Fatalf("tenant home mode = %#o, want 0710", got)
	}
	publicInfo, err := os.Stat(publicHTML)
	if err != nil {
		t.Fatalf("stat public_html: %v", err)
	}
	if got := publicInfo.Mode().Perm(); got != 0750 {
		t.Fatalf("public_html mode = %#o, want 0750", got)
	}
}

func TestPMASignonAssetMatchesStartupRepairContent(t *testing.T) {
	asset, err := os.ReadFile("../../assets/phpmyadmin/pma-signon.php")
	if err != nil {
		t.Fatalf("read phpMyAdmin signon asset: %v", err)
	}
	if string(asset) != pmaSignonPHP() {
		t.Fatal("phpMyAdmin signon asset differs from startup repair content")
	}
}

func TestPMAConfigHostUsesLocalSocketAccount(t *testing.T) {
	config := "$cfg['Servers'][$i]['host'] = '127.0.0.1';"
	got := pmaConfigHost.ReplaceAllString(config, `${1}'localhost';`)
	want := "$cfg['Servers'][$i]['host'] = 'localhost';"
	if got != want {
		t.Fatalf("phpMyAdmin host repair = %q, want %q", got, want)
	}
}

func TestCertificateSystemDirUsesServikaPKIPath(t *testing.T) {
	if got := certSystemDir("example.com"); got != "/etc/pki/servika/example.com" {
		t.Fatalf("certSystemDir() = %q, want %q", got, "/etc/pki/servika/example.com")
	}
}

func TestReadTenantCertificateAcceptsOwnedRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "example.com.crt")
	content := []byte("test certificate")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatalf("write certificate fixture: %v", err)
	}

	got, err := readTenantCertificate(path, os.Getuid())
	if err != nil {
		t.Fatalf("readTenantCertificate() returned an unexpected error: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("readTenantCertificate() = %q, want %q", got, content)
	}
}

func TestReadTenantCertificateRejectsUnexpectedOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "example.com.crt")
	if err := os.WriteFile(path, []byte("test certificate"), 0600); err != nil {
		t.Fatalf("write certificate fixture: %v", err)
	}

	if _, err := readTenantCertificate(path, os.Getuid()+1); err == nil {
		t.Fatal("readTenantCertificate() accepted a file owned by another account")
	}
}

func TestReadTenantCertificateRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.key")
	if err := os.WriteFile(target, []byte("private key"), 0600); err != nil {
		t.Fatalf("write private key fixture: %v", err)
	}
	link := filepath.Join(directory, "example.com.key")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create certificate symlink: %v", err)
	}

	if _, err := readTenantCertificate(link, os.Getuid()); err == nil {
		t.Fatal("readTenantCertificate() accepted a symlink")
	}
}

func TestTenantCommandUsesExplicitArgumentsAndEnvironment(t *testing.T) {
	t.Setenv("SERVIKA_JWT_SECRET", "must-not-leak")
	command := tenantCommand("pkill", "-KILL", "-u", "c_example_com")
	if got := command.Args; len(got) != 4 || got[1] != "-KILL" || got[2] != "-u" || got[3] != "c_example_com" {
		t.Fatalf("tenant command argv = %#v", got)
	}
	environment := strings.Join(command.Env, "\n")
	if strings.Contains(environment, "SERVIKA_JWT_SECRET") {
		t.Fatal("tenant command inherited a panel secret")
	}
	if !strings.Contains(environment, "PATH=/usr/sbin:/usr/bin:/sbin:/bin") {
		t.Fatal("tenant command does not define its executable search path")
	}
}

func TestACMECommandUsesRootHomeWithoutPanelSecrets(t *testing.T) {
	t.Setenv("SERVIKA_DB_DSN", "must-not-leak")
	command := acmeCommand("--issue", "-d", "example.com")
	environment := strings.Join(command.Env, "\n")
	if strings.Contains(environment, "SERVIKA_DB_DSN") {
		t.Fatal("ACME command inherited a panel secret")
	}
	if !strings.Contains(environment, "HOME=/root") {
		t.Fatal("ACME command does not define the root account home")
	}
}

func TestTenantFPMUnitUsesServikaSliceAndHomeIsolation(t *testing.T) {
	unit := renderTenantUnit("c_example_com", "/usr/sbin/php-fpm")
	for _, directive := range []string{
		"Description=Servika per-tenant PHP-FPM for c_example_com",
		"Slice=servika-c_example_com.slice",
		"ProtectHome=tmpfs",
		"BindPaths=/home/c_example_com",
		"ReadWritePaths=/home/c_example_com /var/log/php-fpm",
	} {
		if !strings.Contains(unit, directive) {
			t.Errorf("tenant PHP-FPM unit does not contain %q", directive)
		}
	}
}

func TestResolveTenantPMMaxChildrenUsesPlanOrMemory(t *testing.T) {
	tests := []struct {
		name         string
		planChildren int
		memoryMB     int
		wantChildren int
	}{
		{name: "explicit plan value", planChildren: 12, memoryMB: 256, wantChildren: 12},
		{name: "memory derived", memoryMB: 1024, wantChildren: 16},
		{name: "minimum worker count", memoryMB: 128, wantChildren: 4},
		{name: "missing plan fallback", wantChildren: 8},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveTenantPMMaxChildren(test.planChildren, test.memoryMB); got != test.wantChildren {
				t.Fatalf("resolveTenantPMMaxChildren() = %d, want %d", got, test.wantChildren)
			}
		})
	}
}

func TestTenantPoolSanitizesScalarSettings(t *testing.T) {
	if got := tenantSanitizeScalar("512M\nphp_admin_value[open_basedir] = /", "256M"); got != "256M" {
		t.Fatalf("newline setting = %q, want fallback", got)
	}
	if got := tenantSanitizeScalar("512M\x00unsafe", "256M"); got != "256M" {
		t.Fatalf("NUL setting = %q, want fallback", got)
	}
	if got := tenantSanitizeScalar(" 512M ", "256M"); got != "512M" {
		t.Fatalf("valid setting = %q, want 512M", got)
	}
}

func TestTenantPoolUsesSafeDefaultWorkerLimit(t *testing.T) {
	pool := renderTenantPool(nil, "c_example_com", 0)
	if !strings.Contains(pool, "pm.max_children = 8") {
		t.Fatal("tenant PHP-FPM pool does not use the safe worker fallback")
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
