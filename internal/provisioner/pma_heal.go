package provisioner

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"servika/internal/config"
)

const (
	pmaPoolPath   = "/etc/php-fpm.d/phpmyadmin.conf"
	pmaSocketPath = "/var/lib/mysql/mysql.sock"
)

func pmaSignonDir() string { return config.PMASignonDir() }

func pmaSignonPath() string { return filepath.Join(pmaSignonDir(), "pma-signon.php") }

func pmaTokenPath() string { return config.PMATokenPath() }

func pmaConfigPath() string { return config.PHPMyAdminConfig() }

var (
	pmaTokenPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
	pmaConfigHost   = regexp.MustCompile(`(?m)(\$cfg\['Servers'\]\[\$i\]\['host'\]\s*=\s*)'[^']*';`)
)

const pmaSignonPHPTemplate = `<?php
/**
 * Exchanges a short-lived Servika token for phpMyAdmin signon credentials.
 */
declare(strict_types=1);

session_name('pma_signon');
ini_set('session.cookie_path', '/');
session_start();

$token = isset($_GET['t']) ? (string) $_GET['t'] : '';
if (!preg_match('/^[a-f0-9]{16,128}$/', $token)) {
    http_response_code(400);
    exit('Invalid signon token. Open phpMyAdmin from Servika.');
}

$internalToken = trim((string) @file_get_contents('{{PMA_TOKEN_PATH}}'));
if ($internalToken === '') {
    http_response_code(500);
    exit('phpMyAdmin signon is not configured.');
}

$payload = json_encode(['token' => $token], JSON_THROW_ON_ERROR);
$curl = curl_init('http://127.0.0.1:8080/api/v1/internal/pma-redeem');
if ($curl === false) {
    http_response_code(500);
    exit('phpMyAdmin signon could not be initialized.');
}

curl_setopt_array($curl, [
    CURLOPT_RETURNTRANSFER => true,
    CURLOPT_POST => true,
    CURLOPT_POSTFIELDS => $payload,
    CURLOPT_HTTPHEADER => [
        'Content-Type: application/json',
        'X-Internal-Auth: ' . $internalToken,
    ],
    CURLOPT_CONNECTTIMEOUT => 3,
    CURLOPT_TIMEOUT => 5,
]);
$response = curl_exec($curl);
$status = (int) curl_getinfo($curl, CURLINFO_HTTP_CODE);
curl_close($curl);

if ($status !== 200 || !is_string($response)) {
    http_response_code(401);
    exit('The signon token could not be redeemed. Open phpMyAdmin from Servika again.');
}

$data = json_decode($response, true);
if (!is_array($data)
    || !is_string($data['username'] ?? null)
    || !is_string($data['password'] ?? null)
    || !is_string($data['db'] ?? null)
) {
    http_response_code(500);
    exit('The signon service returned an invalid response.');
}

session_regenerate_id(true);
$_SESSION['PMA_single_signon_user'] = $data['username'];
$_SESSION['PMA_single_signon_password'] = $data['password'];
$_SESSION['PMA_single_signon_host'] = 'localhost';
$_SESSION['PMA_single_signon_only_db'] = [$data['db']];
session_write_close();

header('Location: /pma/', true, 302);
exit;
`

func pmaSignonPHP() string {
	return strings.ReplaceAll(pmaSignonPHPTemplate, "{{PMA_TOKEN_PATH}}", addcslashes(pmaTokenPath()))
}

func addcslashes(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `'`, `\'`)
}

func ensurePMAStartup() {
	ensurePMASignon()
	ensurePMAToken()
	ensurePMAPoolSocket()
	ensurePMAConfigHost()
}

func ensurePMASignon() {
	signonDir := pmaSignonDir()
	signonPath := pmaSignonPath()
	signonPHP := pmaSignonPHP()
	if err := os.MkdirAll(signonDir, 0755); err != nil {
		log.Printf("phpMyAdmin repair: could not create signon directory: %v", err)
		return
	}
	current, err := os.ReadFile(signonPath)
	if err == nil && string(current) == signonPHP {
		return
	}
	if err := os.WriteFile(signonPath, []byte(signonPHP), 0644); err != nil {
		log.Printf("phpMyAdmin repair: could not write signon endpoint: %v", err)
		return
	}
	_, _ = tenantCommand("restorecon", signonPath).CombinedOutput()
}

func ensurePMAToken() {
	tokenPath := pmaTokenPath()
	current, err := os.ReadFile(tokenPath)
	token := strings.TrimSpace(string(current))
	if err != nil || !pmaTokenPattern.MatchString(token) {
		if err := os.MkdirAll(filepath.Dir(tokenPath), 0755); err != nil {
			log.Printf("phpMyAdmin repair: could not create token directory: %v", err)
			return
		}
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			log.Printf("phpMyAdmin repair: could not generate internal token: %v", err)
			return
		}
		if err := os.WriteFile(tokenPath, []byte(hex.EncodeToString(raw)+"\n"), 0600); err != nil {
			log.Printf("phpMyAdmin repair: could not write internal token: %v", err)
			return
		}
	}

	group, err := user.LookupGroup("apache")
	if err != nil {
		_ = os.Chown(tokenPath, 0, 0)
		_ = os.Chmod(tokenPath, 0600)
		log.Printf("phpMyAdmin repair: apache group unavailable, internal token remains root-only")
		return
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		log.Printf("phpMyAdmin repair: invalid apache group ID")
		return
	}
	if err := os.Chown(tokenPath, 0, gid); err != nil {
		log.Printf("phpMyAdmin repair: could not set internal token ownership: %v", err)
		return
	}
	if err := os.Chmod(tokenPath, 0640); err != nil {
		log.Printf("phpMyAdmin repair: could not set internal token permissions: %v", err)
	}
}

func ensurePMAPoolSocket() {
	current, err := os.ReadFile(pmaPoolPath)
	if err != nil {
		return
	}
	updated := string(current)
	changed := false
	for _, setting := range []string{"mysqli.default_socket", "pdo_mysql.default_socket"} {
		pattern := regexp.MustCompile(`(?m)^\s*php_value\[` + regexp.QuoteMeta(setting) + `\]\s*=.*$`)
		line := "php_value[" + setting + "] = " + pmaSocketPath
		if pattern.MatchString(updated) {
			replaced := pattern.ReplaceAllString(updated, line)
			changed = changed || replaced != updated
			updated = replaced
		} else {
			updated = strings.TrimRight(updated, "\n") + "\n" + line + "\n"
			changed = true
		}
	}
	if !changed {
		return
	}
	if err := os.WriteFile(pmaPoolPath, []byte(updated), 0644); err != nil {
		log.Printf("phpMyAdmin repair: could not update PHP-FPM socket settings: %v", err)
		return
	}
	if output, err := tenantCommand("systemctl", "reload-or-restart", "php-fpm").CombinedOutput(); err != nil {
		log.Printf("phpMyAdmin repair: PHP-FPM reload failed: %s", strings.TrimSpace(string(output)))
	}
}

func ensurePMAConfigHost() {
	configPath := pmaConfigPath()
	current, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	updated := pmaConfigHost.ReplaceAllString(string(current), `${1}'localhost';`)
	if updated == string(current) {
		return
	}
	if err := os.WriteFile(configPath, []byte(updated), 0644); err != nil {
		log.Printf("phpMyAdmin repair: could not update database host: %v", err)
	}
}
