package wordpress

import (
	"os"
	"os/exec"
	"path/filepath"
)

const maintenancePluginPHP = `<?php
/*
 * Plugin Name: Servika Maintenance Mode
 * Description: Persistent maintenance mode managed by Servika.
 */
if (php_sapi_name() === 'cli') { return; }
$servika_flag = __DIR__ . '/../.servika-maintenance';
if (!file_exists($servika_flag)) { return; }
$servika_uri = isset($_SERVER['REQUEST_URI']) ? $_SERVER['REQUEST_URI'] : '';
$servika_path = parse_url($servika_uri, PHP_URL_PATH);
if (!is_string($servika_path)) { $servika_path = ''; }
$servika_is_admin = $servika_path === '/wp-admin' || strpos($servika_path, '/wp-admin/') === 0;
if ($servika_is_admin || $servika_path === '/wp-login.php' || $servika_path === '/wp-cron.php') { return; }
if (!headers_sent()) {
    header($_SERVER['SERVER_PROTOCOL'] . ' 503 Service Unavailable', true, 503);
    header('Retry-After: 3600');
    header('Content-Type: text/html; charset=utf-8');
}
$servika_message = @file_get_contents($servika_flag);
if (!$servika_message) { $servika_message = 'This website is temporarily undergoing maintenance. Please try again later.'; }
echo '<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Maintenance Mode</title>';
echo '<style>body{font-family:system-ui,Segoe UI,sans-serif;background:#f8fafc;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0}.card{max-width:520px;background:#fff;border:1px solid #e2e8f0;border-radius:16px;padding:48px;text-align:center;box-shadow:0 10px 25px rgba(0,0,0,.05)}h1{font-size:22px;color:#0f172a;margin:0 0 10px}p{color:#64748b;line-height:1.6;margin:0}</style></head>';
echo '<body><div class="card"><h1>Maintenance Mode</h1><p>' . htmlspecialchars($servika_message, ENT_QUOTES, 'UTF-8') . '</p></div></body></html>';
exit;
`

func maintenancePaths(dir string) (pluginDir, pluginFile, flag string) {
	contentDir := filepath.Join(dir, "wp-content")
	pluginDir = filepath.Join(contentDir, "mu-plugins")
	pluginFile = filepath.Join(pluginDir, "servika-maintenance.php")
	flag = filepath.Join(contentDir, ".servika-maintenance")
	return
}

func maintenanceEnabled(dir string) bool {
	_, _, flag := maintenancePaths(dir)
	_, err := os.Stat(flag)
	return err == nil
}

func enableMaintenance(systemUser, dir string) error {
	pluginDir, pluginFile, flag := maintenancePaths(dir)
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(pluginFile, []byte(maintenancePluginPHP), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(flag, []byte("This website is temporarily undergoing maintenance. Please try again later."), 0644); err != nil {
		return err
	}
	_ = exec.Command("chown", "-R", systemUser+":"+systemUser, pluginDir, flag).Run()
	_ = exec.Command("restorecon", "-R", pluginDir, flag).Run()
	return nil
}

func disableMaintenance(dir string) error {
	_, _, flag := maintenancePaths(dir)
	if err := os.Remove(flag); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
