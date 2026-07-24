package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultComposerBin        = "/usr/local/bin/composer"
	DefaultWPCLIBin           = "/usr/local/bin/wp"
	DefaultClamScanBin        = "/usr/bin/clamscan"
	DefaultFreshclamBin       = "/usr/bin/freshclam"
	DefaultPECLBin            = "/usr/bin/pecl"
	DefaultRemiPECLRoot       = "/opt/remi"
	DefaultACMEHome           = "/root/.acme.sh"
	DefaultACMEBin            = "/root/.acme.sh/acme.sh"
	DefaultBackupRoot         = "/var/backups/servika"
	DefaultLaravelLogDir      = "/var/log/servika-laravel"
	DefaultPluginRoot         = "/opt/servika/plugins"
	DefaultLogDir             = "/opt/servika/logs"
	DefaultUpdateLog          = "/opt/servika/logs/update.log"
	DefaultKernelCareLog      = "/opt/servika/logs/kernelcare-update.log"
	DefaultKernelCareWrapper  = "/opt/servika/kernelcare-update.sh"
	DefaultCVELog             = "/opt/servika/logs/cve-update.log"
	DefaultInstallationID     = "/etc/servika/installation-id"
	DefaultVersionCache       = "/opt/servika/version-cache.json"
	DefaultPMAToken           = "/etc/servika/pma-internal.token"
	DefaultPMASignonDir       = "/opt/servika/pma-signon"
	DefaultPHPMyAdminRoot     = "/opt/phpmyadmin"
	DefaultPHPMyAdminConfig   = "/opt/phpmyadmin/config.inc.php"
	DefaultCertRoot           = "/etc/pki/servika"
	DefaultNginxCacheDir      = "/var/cache/nginx/servikacache"
	DefaultNginxCacheConf     = "/etc/nginx/conf.d/servikacache.conf"
	DefaultNginxCacheTempConf = "/etc/nginx/conf.d/00-servikacache-temporary.conf"
	DefaultNginxCacheLogConf  = "/etc/nginx/conf.d/00-servika-cache-log.conf"
	DefaultGitHubAPI          = "https://api.github.com"
	DefaultIonCubeURL         = "https://downloads.ioncube.com/loader_downloads/ioncube_loaders_lin_x86-64.tar.gz"
	DefaultUpdateBootstrapURL = "https://raw.githubusercontent.com/KilimcininKorOglu/servika/main/assets/ops/servika-update"
	DefaultVersionEndpoint    = "https://raw.githubusercontent.com/KilimcininKorOglu/servika/main/version.json"
)

// EnvString returns a trimmed environment value or its fallback.
func EnvString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// EnvAbsPath returns an absolute path from the environment or its fallback.
func EnvAbsPath(key, fallback string) (string, error) {
	value := EnvString(key, fallback)
	if strings.ContainsRune(value, '\x00') || !filepath.IsAbs(value) {
		return "", fmt.Errorf("%s must be an absolute path", key)
	}
	return filepath.Clean(value), nil
}

// EnvURL returns an HTTP or HTTPS URL from the environment or its fallback.
func EnvURL(key, fallback string) (string, error) {
	value := EnvString(key, fallback)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("%s must be an http or https URL", key)
	}
	return value, nil
}

// ShellQuote quotes a value for POSIX shell command strings.
func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func mustAbsPath(key, fallback string) string {
	value, err := EnvAbsPath(key, fallback)
	if err != nil {
		panic(err)
	}
	return value
}

func mustURL(key, fallback string) string {
	value, err := EnvURL(key, fallback)
	if err != nil {
		panic(err)
	}
	return value
}

func ComposerBin() string   { return mustAbsPath("SERVIKA_COMPOSER_BIN", DefaultComposerBin) }
func WPCLIBin() string      { return mustAbsPath("SERVIKA_WPCLI_BIN", DefaultWPCLIBin) }
func ClamScanBin() string   { return mustAbsPath("SERVIKA_CLAMSCAN_BIN", DefaultClamScanBin) }
func FreshclamBin() string  { return mustAbsPath("SERVIKA_FRESHCLAM_BIN", DefaultFreshclamBin) }
func PECLBin() string       { return mustAbsPath("SERVIKA_PECL_BIN", DefaultPECLBin) }
func RemiPECLRoot() string  { return mustAbsPath("SERVIKA_REMI_PECL_ROOT", DefaultRemiPECLRoot) }
func ACMEHome() string      { return mustAbsPath("SERVIKA_ACME_HOME", DefaultACMEHome) }
func ACMEBin() string       { return mustAbsPath("SERVIKA_ACME_BIN", DefaultACMEBin) }
func BackupRoot() string    { return mustAbsPath("SERVIKA_BACKUP_ROOT", DefaultBackupRoot) }
func LaravelLogDir() string { return mustAbsPath("SERVIKA_LARAVEL_LOG_DIR", DefaultLaravelLogDir) }
func PluginRoot() string    { return mustAbsPath("SERVIKA_PLUGIN_ROOT", DefaultPluginRoot) }
func LogDir() string        { return mustAbsPath("SERVIKA_LOG_DIR", DefaultLogDir) }
func UpdateLog() string     { return mustAbsPath("SERVIKA_UPDATE_LOG", DefaultUpdateLog) }
func KernelCareLog() string { return mustAbsPath("SERVIKA_KERNELCARE_LOG", DefaultKernelCareLog) }
func KernelCareWrapper() string {
	return mustAbsPath("SERVIKA_KERNELCARE_WRAPPER", DefaultKernelCareWrapper)
}
func CVELog() string { return mustAbsPath("SERVIKA_CVE_LOG", DefaultCVELog) }
func InstallationIDPath() string {
	return mustAbsPath("SERVIKA_INSTALLATION_ID", DefaultInstallationID)
}
func VersionCachePath() string { return mustAbsPath("SERVIKA_VERSION_CACHE", DefaultVersionCache) }
func PMATokenPath() string     { return mustAbsPath("SERVIKA_PMA_TOKEN", DefaultPMAToken) }
func PMASignonDir() string     { return mustAbsPath("SERVIKA_PMA_SIGNON_DIR", DefaultPMASignonDir) }
func PHPMyAdminConfig() string {
	return mustAbsPath("SERVIKA_PHPMYADMIN_CONFIG", DefaultPHPMyAdminConfig)
}
func CertRoot() string       { return mustAbsPath("SERVIKA_CERT_ROOT", DefaultCertRoot) }
func NginxCacheDir() string  { return mustAbsPath("SERVIKA_NGINX_CACHE_DIR", DefaultNginxCacheDir) }
func NginxCacheConf() string { return mustAbsPath("SERVIKA_NGINX_CACHE_CONF", DefaultNginxCacheConf) }
func NginxCacheTempConf() string {
	return mustAbsPath("SERVIKA_NGINX_CACHE_TEMP_CONF", DefaultNginxCacheTempConf)
}
func NginxCacheLogConf() string {
	return mustAbsPath("SERVIKA_NGINX_CACHE_LOG_CONF", DefaultNginxCacheLogConf)
}
func GitHubAPI() string  { return mustURL("SERVIKA_GITHUB_API", DefaultGitHubAPI) }
func IonCubeURL() string { return mustURL("SERVIKA_IONCUBE_URL", DefaultIonCubeURL) }
func UpdateBootstrapURL() string {
	return mustURL("SERVIKA_UPDATE_BOOTSTRAP_URL", DefaultUpdateBootstrapURL)
}
func VersionEndpoint() string { return mustURL("SERVIKA_VERSION_ENDPOINT", DefaultVersionEndpoint) }

// OpsTool returns the absolute path for an operations helper under SERVIKA_OPSBIN.
func OpsTool(name string) string {
	return filepath.Join(mustAbsPath("SERVIKA_OPSBIN", "/usr/local/bin"), name)
}

// ValidateRuntimePaths validates env-backed paths and URLs once during startup.
func ValidateRuntimePaths() error {
	checks := []struct {
		key      string
		fallback string
		isURL    bool
	}{
		{"SERVIKA_COMPOSER_BIN", DefaultComposerBin, false},
		{"SERVIKA_WPCLI_BIN", DefaultWPCLIBin, false},
		{"SERVIKA_CLAMSCAN_BIN", DefaultClamScanBin, false},
		{"SERVIKA_FRESHCLAM_BIN", DefaultFreshclamBin, false},
		{"SERVIKA_PECL_BIN", DefaultPECLBin, false},
		{"SERVIKA_REMI_PECL_ROOT", DefaultRemiPECLRoot, false},
		{"SERVIKA_ACME_HOME", DefaultACMEHome, false},
		{"SERVIKA_ACME_BIN", DefaultACMEBin, false},
		{"SERVIKA_BACKUP_ROOT", DefaultBackupRoot, false},
		{"SERVIKA_LARAVEL_LOG_DIR", DefaultLaravelLogDir, false},
		{"SERVIKA_PLUGIN_ROOT", DefaultPluginRoot, false},
		{"SERVIKA_LOG_DIR", DefaultLogDir, false},
		{"SERVIKA_UPDATE_LOG", DefaultUpdateLog, false},
		{"SERVIKA_KERNELCARE_LOG", DefaultKernelCareLog, false},
		{"SERVIKA_KERNELCARE_WRAPPER", DefaultKernelCareWrapper, false},
		{"SERVIKA_CVE_LOG", DefaultCVELog, false},
		{"SERVIKA_INSTALLATION_ID", DefaultInstallationID, false},
		{"SERVIKA_VERSION_CACHE", DefaultVersionCache, false},
		{"SERVIKA_PMA_TOKEN", DefaultPMAToken, false},
		{"SERVIKA_PMA_SIGNON_DIR", DefaultPMASignonDir, false},
		{"SERVIKA_PHPMYADMIN_ROOT", DefaultPHPMyAdminRoot, false},
		{"SERVIKA_PHPMYADMIN_CONFIG", DefaultPHPMyAdminConfig, false},
		{"SERVIKA_CERT_ROOT", DefaultCertRoot, false},
		{"SERVIKA_NGINX_CACHE_DIR", DefaultNginxCacheDir, false},
		{"SERVIKA_NGINX_CACHE_CONF", DefaultNginxCacheConf, false},
		{"SERVIKA_NGINX_CACHE_TEMP_CONF", DefaultNginxCacheTempConf, false},
		{"SERVIKA_NGINX_CACHE_LOG_CONF", DefaultNginxCacheLogConf, false},
		{"SERVIKA_OPSBIN", "/usr/local/bin", false},
		{"SERVIKA_GITHUB_API", DefaultGitHubAPI, true},
		{"SERVIKA_IONCUBE_URL", DefaultIonCubeURL, true},
		{"SERVIKA_UPDATE_BOOTSTRAP_URL", DefaultUpdateBootstrapURL, true},
		{"SERVIKA_VERSION_ENDPOINT", DefaultVersionEndpoint, true},
	}
	for _, check := range checks {
		var err error
		if check.isURL {
			_, err = EnvURL(check.key, check.fallback)
		} else {
			_, err = EnvAbsPath(check.key, check.fallback)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
