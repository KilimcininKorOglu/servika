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
