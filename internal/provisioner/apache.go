// Per-domain vhost management for the Apache backend.
// nginx acts as the TLS terminator and edge proxy. Apache listens on 127.0.0.1:10080
// and bridges PHP to the active PHP-FPM socket through mod_proxy_fcgi.
package provisioner

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

const ApacheUpstream = "127.0.0.1:10080"

var apacheVhostTmpl = template.Must(template.New("a").Parse(`# {{.DomainName}} — Servika Apache backend (nginx reverse proxy)
# Response security headers are applied at the nginx edge to avoid duplicates.
# Apache enforces execution and access policy: CGI is disabled, scripts and
# backup files are denied, and symlinks require matching ownership.
<VirtualHost 127.0.0.1:10080>
    ServerName {{.DomainName}}
    ServerAlias www.{{.DomainName}}
    DocumentRoot {{.WebRoot}}

    <Directory {{.WebRoot}}>
        Options -ExecCGI -Indexes -Includes -FollowSymLinks +SymLinksIfOwnerMatch
        AllowOverride AuthConfig FileInfo Indexes Limit Options=Indexes,MultiViews
        Require all granted

        RemoveHandler .cgi .pl .py .sh .rb .lua .fcgi .fpl
        <FilesMatch "\.(cgi|pl|py|sh|rb|lua|fcgi)$">
            Require all denied
        </FilesMatch>
        <FilesMatch "\.(sql|bak|old|orig|save|swp|dump|tar|tgz|gz|zip|rar|7z|log|inc)$">
            Require all denied
        </FilesMatch>
        <FilesMatch "\.php\.bak$">
            Require all denied
        </FilesMatch>
    </Directory>

    <FilesMatch "\.php$">
        SetHandler "proxy:unix:{{.PHPSocket}}|fcgi://localhost"
    </FilesMatch>

    DirectoryIndex index.php index.html index.htm

    # Read the original client IP from nginx
    RemoteIPHeader X-Forwarded-For
    RemoteIPInternalProxy 127.0.0.1

    ErrorLog /var/log/httpd/{{.DomainName}}.error.log
    CustomLog /var/log/httpd/{{.DomainName}}.access.log combined
</VirtualHost>
`))

func apacheVhostPath(systemUser string) string {
	return "/etc/httpd/conf.d/dom_" + systemUser + ".conf"
}

func writeApacheVhost(opts VhostOpts, systemUser string) error {
	var buf bytes.Buffer
	if err := apacheVhostTmpl.Execute(&buf, opts); err != nil {
		return fmt.Errorf("Apache template: %w", err)
	}
	if err := os.WriteFile(apacheVhostPath(systemUser), buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write Apache vhost: %w", err)
	}
	return apacheTestReload()
}

func deleteApacheVhostIfExists(systemUser string) error {
	p := apacheVhostPath(systemUser)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return nil
	}
	if err := os.Remove(p); err != nil {
		return fmt.Errorf("delete Apache vhost: %w", err)
	}
	return apacheTestReload()
}

func apacheTestReload() error {
	if out, err := exec.Command("httpd", "-t").CombinedOutput(); err != nil {
		return fmt.Errorf("httpd -t failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if out, err := exec.Command("systemctl", "reload-or-restart", "httpd").CombinedOutput(); err != nil {
		return fmt.Errorf("httpd reload: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
