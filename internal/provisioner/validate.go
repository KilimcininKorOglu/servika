package provisioner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ValidateNginxDirectives validates custom nginx directives entered at the plan or domain level
// without disrupting the live configuration:
//   - It embeds the directives in a temporary server{} block under /etc/nginx/conf.d.
//   - It runs `nginx -t`, which parses and validates the full configuration without opening sockets.
//   - It always removes the temporary file.
//
// For invalid input, it returns nginx's error output so the caller can display the error
// and reject the change. Empty input is valid.
//
// Validation runs in the server context because the directives are also injected into
// the server block of the actual domain vhost, matching per-domain extra_directives.
func ValidateNginxDirectives(directives string) error {
	d := strings.TrimSpace(directives)
	if d == "" {
		return nil
	}

	tmp, err := os.CreateTemp("/etc/nginx/conf.d", "_planvalidate_*.conf.tmp")
	if err != nil {
		return fmt.Errorf("create temporary validation file: %w", err)
	}
	tmpPath := tmp.Name()
	// nginx reads only *.conf files, so the ".tmp" suffix excludes the file from validation.
	// Rename it to an actual ".conf" file before running the check.
	finalPath := strings.TrimSuffix(tmpPath, ".tmp")

	block := fmt.Sprintf(`# Temporary plan-directive validation, removed automatically
server {
    listen 127.0.0.1:65071;
    server_name _servika_plan_validate;
    root /var/www/_default80;
    # ---- validated directives ----
%s
}
`, indentLines(d, "    "))

	if _, err := tmp.WriteString(block); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temporary validation file: %w", err)
	}
	_ = tmp.Close()

	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("prepare temporary validation file: %w", err)
	}
	defer func() { _ = os.Remove(finalPath) }()

	out, err := exec.Command("nginx", "-t").CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		// Simplify the temporary file path in the user-facing message.
		msg = strings.ReplaceAll(msg, finalPath, "(directives)")
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

var forbiddenNginxDirectives = map[string]bool{
	"alias": true, "root": true,
	"proxy_pass": true, "fastcgi_pass": true, "uwsgi_pass": true, "scgi_pass": true,
	"grpc_pass": true, "memcached_pass": true,
	"include": true, "load_module": true,
	"ssl_certificate": true, "ssl_certificate_key": true, "ssl_trusted_certificate": true,
	"error_log": true, "access_log": true, "fastcgi_param": true,
	"auth_basic_user_file": true, "secure_link_secret": true,
}

// DangerousNginxDirective returns the first forbidden custom directive name.
func DangerousNginxDirective(directives string) string {
	var uncommented strings.Builder
	for _, line := range strings.Split(directives, "\n") {
		if index := strings.IndexByte(line, '#'); index >= 0 {
			line = line[:index]
		}
		uncommented.WriteString(line)
		uncommented.WriteByte('\n')
	}

	replacer := strings.NewReplacer("{", "\n", "}", "\n", ";", "\n")
	for _, statement := range strings.Split(replacer.Replace(uncommented.String()), "\n") {
		fields := strings.Fields(statement)
		if len(fields) == 0 {
			continue
		}
		name := strings.ToLower(fields[0])
		if forbiddenNginxDirectives[name] ||
			strings.Contains(name, "_by_lua") ||
			strings.HasPrefix(name, "lua_") ||
			strings.HasPrefix(name, "js_") ||
			strings.HasPrefix(name, "perl") {
			return name
		}
	}
	return ""
}

// indentLines adds a prefix to each non-empty line to keep the nginx block readable.
func indentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
