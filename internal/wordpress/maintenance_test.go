package wordpress

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaintenancePathsStayInsideWordPressContent(t *testing.T) {
	root := t.TempDir()
	pluginDir, pluginFile, flag := maintenancePaths(root)
	contentDir := filepath.Join(root, "wp-content")
	if pluginDir != filepath.Join(contentDir, "mu-plugins") {
		t.Fatalf("plugin directory = %q", pluginDir)
	}
	if pluginFile != filepath.Join(pluginDir, "servika-maintenance.php") {
		t.Fatalf("plugin file = %q", pluginFile)
	}
	if flag != filepath.Join(contentDir, ".servika-maintenance") {
		t.Fatalf("flag file = %q", flag)
	}
}

func TestMaintenancePluginLimitsBypassChecksToRequestPaths(t *testing.T) {
	if !strings.Contains(maintenancePluginPHP, "parse_url($servika_uri, PHP_URL_PATH)") {
		t.Fatal("maintenance plugin does not isolate the request path before checking bypass routes")
	}
	if strings.Contains(maintenancePluginPHP, "strpos($servika_uri") {
		t.Fatal("maintenance plugin allows query strings to trigger a maintenance bypass")
	}
}

func TestMaintenanceEnabledTracksPersistentFlag(t *testing.T) {
	root := t.TempDir()
	_, _, flag := maintenancePaths(root)
	if maintenanceEnabled(root) {
		t.Fatal("maintenanceEnabled() reported a missing flag as enabled")
	}
	if err := os.MkdirAll(filepath.Dir(flag), 0755); err != nil {
		t.Fatalf("create content directory: %v", err)
	}
	if err := os.WriteFile(flag, []byte("maintenance"), 0644); err != nil {
		t.Fatalf("create maintenance flag: %v", err)
	}
	if !maintenanceEnabled(root) {
		t.Fatal("maintenanceEnabled() did not detect the persistent flag")
	}
	if err := disableMaintenance(root); err != nil {
		t.Fatalf("disableMaintenance(): %v", err)
	}
	if maintenanceEnabled(root) {
		t.Fatal("maintenanceEnabled() remained true after disabling maintenance")
	}
}
