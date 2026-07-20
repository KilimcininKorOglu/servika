// Package git provides per-domain Git deployment with deploy keys, repositories, and webhook auto-pull.
package git

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type Repo struct {
	ID            int64  `json:"id"`
	DomainID      int64  `json:"domain_id"`
	RepoURL       string `json:"repo_url"`
	Branch        string `json:"branch"`
	TargetDir     string `json:"target_dir"`
	DeployKeyPub  string `json:"deploy_key_pub"`
	WebhookSecret string `json:"webhook_secret"`
	LastSync      string `json:"last_sync,omitempty"`
	LastCommit    string `json:"last_commit,omitempty"`
	LastStatus    string `json:"last_status"`
	CreatedAt     string `json:"created_at"`
}

type Handlers struct {
	DB *sql.DB
}

const selectAll = `SELECT id, domain_id, repo_url, branch, target_dir,
  deploy_key_pub, webhook_secret,
  COALESCE(DATE_FORMAT(last_sync,'%Y-%m-%d %H:%i'),''),
  last_commit, last_status,
  DATE_FORMAT(created_at,'%Y-%m-%d %H:%i')
  FROM git_repos`

func scan(rs interface{ Scan(...any) error }) (Repo, error) {
	var r Repo
	err := rs.Scan(&r.ID, &r.DomainID, &r.RepoURL, &r.Branch, &r.TargetDir,
		&r.DeployKeyPub, &r.WebhookSecret, &r.LastSync, &r.LastCommit, &r.LastStatus, &r.CreatedAt)
	return r, err
}

func (h *Handlers) lookupDomain(r *http.Request) (id int64, systemUser string, demo bool, err error) {
	id, _ = strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var dmo int
	err = h.DB.QueryRowContext(r.Context(),
		`SELECT system_user, is_demo FROM domains WHERE id=?`, id).Scan(&systemUser, &dmo)
	demo = dmo == 1
	return
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func deployKeyDir(systemUser string) string {
	return "/home/" + systemUser + "/.ssh"
}

var (
	targetDirPattern = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)
	branchPattern    = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)
)

func validTargetDir(targetDir string) bool {
	return targetDir != "" && targetDir == strings.TrimSpace(targetDir) && len(targetDir) <= 128 &&
		!filepath.IsAbs(targetDir) && filepath.Clean(targetDir) != "." &&
		!strings.Contains(targetDir, "..") && targetDirPattern.MatchString(targetDir)
}

func validBranch(branch string) bool {
	return branch != "" && branch == strings.TrimSpace(branch) && len(branch) <= 128 &&
		!strings.HasPrefix(branch, "-") && !strings.Contains(branch, "..") &&
		branchPattern.MatchString(branch)
}

func validRepoURL(repoURL string) bool {
	if repoURL != strings.TrimSpace(repoURL) || len(repoURL) == 0 || len(repoURL) > 2048 {
		return false
	}
	if !strings.HasPrefix(repoURL, "https://") &&
		!strings.HasPrefix(repoURL, "ssh://") &&
		!strings.HasPrefix(repoURL, "git@") {
		return false
	}
	return !strings.ContainsAny(repoURL, " \t\r\n;&|`$(){}[]<>\\\"'")
}

func clearDirectoryContents(directory string) error {
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("target directory cannot be a symlink")
	}
	if !info.IsDir() {
		return os.Remove(directory)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(directory, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// generateDeployKey creates a passphrase-free Ed25519 key under /home/<systemUser>/.ssh/.
func generateDeployKey(systemUser string) (pubKey string, err error) {
	dir := deployKeyDir(systemUser)
	_ = os.MkdirAll(dir, 0700)
	priv := filepath.Join(dir, "servika_deploy")
	pub := priv + ".pub"

	if _, err := os.Stat(pub); err == nil {
		// Reuse the current key.
		b, _ := os.ReadFile(pub)
		return strings.TrimSpace(string(b)), nil
	}
	_, _ = exec.Command("rm", "-f", priv, pub).CombinedOutput()
	out, err := exec.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-C", "deploy@servika/"+systemUser, "-f", priv).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ssh-keygen: %s: %w", strings.TrimSpace(string(out)), err)
	}
	// Apply ownership and permissions.
	_, _ = exec.Command("chown", "-R", systemUser+":"+systemUser, dir).CombinedOutput()
	_ = os.Chmod(priv, 0600)
	_ = os.Chmod(pub, 0644)

	// Configure this key for github.com in the per-user ~/.ssh/config.
	cfg := filepath.Join(dir, "config")
	cfgBody := `Host github.com
    HostName github.com
    User git
    IdentityFile ~/.ssh/servika_deploy
    StrictHostKeyChecking no
    UserKnownHostsFile=/dev/null
`
	_ = os.WriteFile(cfg, []byte(cfgBody), 0600)
	_, _ = exec.Command("chown", systemUser+":"+systemUser, cfg).CombinedOutput()
	_, _ = exec.Command("restorecon", "-R", dir).CombinedOutput()

	b, _ := os.ReadFile(pub)
	return strings.TrimSpace(string(b)), nil
}

// runAsUserArgs executes a command without a shell as the system user.
func runAsUserArgs(systemUser, cwd, name string, commandArgs ...string) (string, error) {
	if !strings.HasPrefix(systemUser, "c_") {
		return "", errors.New("invalid system user")
	}
	environment := []string{
		"HOME=/home/" + systemUser,
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	}
	sudoArgs := append([]string{"-u", systemUser, "-H", "--", name}, commandArgs...)
	cmd := exec.Command("sudo", sudoArgs...)
	cmd.Dir = cwd
	cmd.Env = environment
	out, err := cmd.CombinedOutput()
	if errors.Is(err, exec.ErrNotFound) {
		runuserArgs := append([]string{"-u", systemUser, "--", name}, commandArgs...)
		cmd = exec.Command("runuser", runuserArgs...)
		cmd.Dir = cwd
		cmd.Env = environment
		out, err = cmd.CombinedOutput()
	}
	return string(out), err
}

// gitClone performs the initial clone and replaces an existing target directory.
func gitClone(systemUser, repoURL, branch, targetDir string) (sha string, log string, err error) {
	if !validRepoURL(repoURL) {
		return "", "", errors.New("invalid repository URL")
	}
	if !validBranch(branch) {
		return "", "", errors.New("invalid branch")
	}
	if !validTargetDir(targetDir) {
		return "", "", errors.New("invalid target directory")
	}
	home := "/home/" + systemUser
	dst := filepath.Join(home, targetDir)
	// Clear the target. Existing public_html content is lost, as warned in the UI.
	if err := clearDirectoryContents(dst); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return "", "", err
	}
	_, _ = exec.Command("chown", systemUser+":"+systemUser, dst).CombinedOutput()

	out, err := runAsUserArgs(systemUser, home, "git", "clone", "--depth", "1", "--branch", branch, "--", repoURL, dst)
	log = out
	if err != nil {
		return "", out, err
	}
	shaOut, _ := runAsUserArgs(systemUser, dst, "git", "-C", dst, "rev-parse", "HEAD")
	sha = strings.TrimSpace(shaOut)
	_, _ = exec.Command("restorecon", "-R", dst).CombinedOutput()
	return sha, log, nil
}

// gitPull updates an existing repository.
func gitPull(systemUser, targetDir, branch string) (sha string, log string, err error) {
	if !validTargetDir(targetDir) {
		return "", "", errors.New("invalid target directory")
	}
	if !validBranch(branch) {
		return "", "", errors.New("invalid branch")
	}
	home := "/home/" + systemUser
	dst := filepath.Join(home, targetDir)
	if _, err := os.Stat(filepath.Join(dst, ".git")); err != nil {
		return "", "", errors.New("target directory is not a Git repository; clone it first")
	}
	out, err := runAsUserArgs(systemUser, dst, "git", "-C", dst, "fetch", "origin", branch)
	if err == nil {
		resetOutput, resetErr := runAsUserArgs(systemUser, dst, "git", "-C", dst, "reset", "--hard", "origin/"+branch)
		out += resetOutput
		err = resetErr
	}
	log = out
	if err != nil {
		return "", out, err
	}
	shaOut, _ := runAsUserArgs(systemUser, dst, "git", "-C", dst, "rev-parse", "HEAD")
	sha = strings.TrimSpace(shaOut)
	_, _ = exec.Command("restorecon", "-R", dst).CombinedOutput()
	return sha, log, nil
}

// ----- HTTP handlers -----

type connectRequest struct {
	RepoURL   string `json:"repo_url"`
	Branch    string `json:"branch"`
	TargetDir string `json:"target_dir"`
}

func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	row := h.DB.QueryRowContext(r.Context(), selectAll+" WHERE domain_id=? LIMIT 1", id)
	repo, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteJSON(w, http.StatusOK, nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, repo)
}

// Connect creates a deploy key and stores the repository URL without cloning.
func (h *Handlers) Connect(w http.ResponseWriter, r *http.Request) {
	id, systemUser, demo, err := h.lookupDomain(r)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "git cannot be connected to a demo subscription")
		return
	}
	var req connectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.RepoURL = strings.TrimSpace(req.RepoURL)
	req.Branch = strings.TrimSpace(req.Branch)
	req.TargetDir = strings.TrimSpace(req.TargetDir)
	if req.Branch == "" {
		req.Branch = "main"
	}
	if req.TargetDir == "" {
		req.TargetDir = "public_html"
	}
	if !validRepoURL(req.RepoURL) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid repo_url")
		return
	}
	if !validBranch(req.Branch) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid branch")
		return
	}
	if !validTargetDir(req.TargetDir) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid target_dir")
		return
	}
	pub, err := generateDeployKey(systemUser)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	secret := randomHex(20)
	res, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO git_repos(domain_id, repo_url, branch, target_dir, deploy_key_pub, webhook_secret, last_status)
		 VALUES(?,?,?,?,?,?, 'pending')
		 ON DUPLICATE KEY UPDATE repo_url=VALUES(repo_url), branch=VALUES(branch),
		   target_dir=VALUES(target_dir), deploy_key_pub=VALUES(deploy_key_pub)`,
		id, req.RepoURL, req.Branch, req.TargetDir, pub, secret)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	gid, _ := res.LastInsertId()
	row := h.DB.QueryRowContext(r.Context(), selectAll+" WHERE id=?", gid)
	repo, _ := scan(row)
	httpx.WriteJSON(w, http.StatusCreated, repo)
}

// Clone performs the initial clone.
func (h *Handlers) Clone(w http.ResponseWriter, r *http.Request) {
	id, systemUser, demo, err := h.lookupDomain(r)
	if err != nil || demo {
		httpx.WriteError(w, http.StatusForbidden, "permission denied")
		return
	}
	var repoURL, branch, targetDir string
	var gid int64
	err = h.DB.QueryRowContext(r.Context(),
		`SELECT id, repo_url, branch, target_dir FROM git_repos WHERE domain_id=? LIMIT 1`, id).
		Scan(&gid, &repoURL, &branch, &targetDir)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusBadRequest, "connect a repository first")
		return
	}
	sha, log, err := gitClone(systemUser, repoURL, branch, targetDir)
	status := "successful"
	if err != nil {
		status = "error"
	}
	_, _ = h.DB.ExecContext(r.Context(),
		`UPDATE git_repos SET last_sync=NOW(), last_commit=?, last_status=? WHERE id=?`,
		sha, status, gid)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "commit": sha, "log": log,
	})
}

// Pull updates an existing repository.
func (h *Handlers) Pull(w http.ResponseWriter, r *http.Request) {
	id, systemUser, demo, err := h.lookupDomain(r)
	if err != nil || demo {
		httpx.WriteError(w, http.StatusForbidden, "permission denied")
		return
	}
	var branch, targetDir string
	var gid int64
	err = h.DB.QueryRowContext(r.Context(),
		`SELECT id, branch, target_dir FROM git_repos WHERE domain_id=? LIMIT 1`, id).
		Scan(&gid, &branch, &targetDir)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusBadRequest, "repository not found")
		return
	}
	sha, log, err := gitPull(systemUser, targetDir, branch)
	status := "successful"
	if err != nil {
		status = "error"
	}
	_, _ = h.DB.ExecContext(r.Context(),
		`UPDATE git_repos SET last_sync=NOW(), last_commit=?, last_status=? WHERE id=?`,
		sha, status, gid)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "commit": sha, "log": log,
	})
}

// Delete removes the repository record but leaves the deploy key on disk.
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_, _ = h.DB.ExecContext(r.Context(), `DELETE FROM git_repos WHERE domain_id=?`, id)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Webhook validates a GitHub push event secret and pulls the repository.
// URL: POST /api/v1/git-webhook/:secret
// Authentication is not required because the secret is in the URL. Only that secret is matched.
func (h *Handlers) Webhook(w http.ResponseWriter, r *http.Request) {
	secret := chi.URLParam(r, "secret")
	if len(secret) < 16 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid secret")
		return
	}
	var gid, domainID int64
	var systemUser, branch, targetDir string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT g.id, g.domain_id, d.system_user, g.branch, g.target_dir
		 FROM git_repos g JOIN domains d ON d.id=g.domain_id
		 WHERE g.webhook_secret=? LIMIT 1`, secret).Scan(&gid, &domainID, &systemUser, &branch, &targetDir)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "secret did not match")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	sha, _, perr := gitPull(systemUser, targetDir, branch)
	status := "successful"
	if perr != nil {
		status = "error-webhook"
	}
	_, _ = h.DB.ExecContext(r.Context(),
		`UPDATE git_repos SET last_sync=NOW(), last_commit=?, last_status=? WHERE id=?`,
		sha, status, gid)
	if perr != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "operation failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "commit": sha,
	})
}
