// IonCube Loader installation and removal downloads the commercial loader from ioncube.com.
package phpext

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"servika/internal/config"
	"servika/internal/httpx"
)

func ionCubeURL() string { return config.IonCubeURL() }

type ioncubeReq struct {
	Version string `json:"version"`
}

// IonCubeInstall installs the IonCube zend_extension for a PHP version.
func (h *Handlers) IonCubeInstall(w http.ResponseWriter, r *http.Request) {
	var req ioncubeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	s, ok := versionByID(req.Version)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "unsupported version")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	// 1. Read PHP extension_dir.
	extOut, err := exec.CommandContext(ctx, s.PHPBin, "-r", "echo ini_get('extension_dir');").Output()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to read PHP extension directory")
		return
	}
	extDir := strings.TrimSpace(string(extOut))
	if extDir == "" {
		httpx.WriteError(w, http.StatusInternalServerError, "pHP extension directory is empty")
		return
	}

	// 2. Create a temporary directory and download the archive.
	tmpDir, err := os.MkdirTemp("", "ioncube-*")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create temporary directory")
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	tarPath := filepath.Join(tmpDir, "ioncube.tar.gz")
	if err := download(ctx, ionCubeURL(), tarPath); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to download IonCube Loader")
		return
	}

	// 3. Extract the archive.
	if _, err := exec.CommandContext(ctx, "tar", "xzf", tarPath, "-C", tmpDir).CombinedOutput(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to extract IonCube Loader archive")
		return
	}

	// 4. Select the shared object matching the PHP version.
	soSrc := filepath.Join(tmpDir, "ioncube", "ioncube_loader_lin_"+req.Version+".so")
	if _, err := os.Stat(soSrc); err != nil {
		// A missing non-thread-safe loader means the version is unavailable.
		httpx.WriteError(w, http.StatusBadRequest,
			"IonCube Loader is unavailable for PHP "+req.Version)
		return
	}

	// 5. Copy the loader into extension_dir.
	soDst := filepath.Join(extDir, "ioncube_loader_lin_"+req.Version+".so")
	if err := copyFile(soSrc, soDst); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to copy IonCube Loader")
		return
	}
	_ = os.Chmod(soDst, 0644)

	// 6. Write an .ini file that loads the extension before OPcache through the 00 prefix.
	iniPath := filepath.Join(s.IniDir, "00-ioncube.ini")
	iniContent := "; IonCube Loader must load before OPcache\nzend_extension = " + soDst + "\n"
	if err := os.WriteFile(iniPath, []byte(iniContent), 0644); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to write IonCube Loader configuration")
		return
	}

	// 7. Reload PHP-FPM.
	if _, err := exec.CommandContext(ctx, "systemctl", "reload-or-restart", s.Service).CombinedOutput(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError,
			"failed to reload PHP-FPM")
		return
	}

	// 8. Verify that php -m reports IonCube.
	verifyCtx, vc := context.WithTimeout(r.Context(), 5*time.Second)
	defer vc()
	mOut, _ := exec.CommandContext(verifyCtx, s.PHPBin, "-m").Output()
	loaded := strings.Contains(strings.ToLower(string(mOut)), "ioncube")

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"ok":            true,
		"version":       req.Version,
		"shared_object": soDst,
		"ini":           iniPath,
		"loaded":        loaded,
	})
}

// IonCubeRemove deletes the .ini and shared object, then reloads PHP-FPM.
func (h *Handlers) IonCubeRemove(w http.ResponseWriter, r *http.Request) {
	var req ioncubeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	s, ok := versionByID(req.Version)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "unsupported version")
		return
	}
	iniPath := filepath.Join(s.IniDir, "00-ioncube.ini")
	_ = os.Remove(iniPath)
	extOut, _ := exec.Command(s.PHPBin, "-r", "echo ini_get('extension_dir');").Output()
	extDir := strings.TrimSpace(string(extOut))
	if extDir != "" {
		_ = os.Remove(filepath.Join(extDir, "ioncube_loader_lin_"+req.Version+".so"))
	}
	_, _ = exec.Command("systemctl", "reload-or-restart", s.Service).CombinedOutput()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "version": req.Version})
}

func download(ctx context.Context, url, destination string) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(f, resp.Body)
	return err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, in)
	return err
}
