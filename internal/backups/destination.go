// Backup off-site destinations: FTP/SFTP and S3-compatible remote storage.
package backups

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"servika/internal/netguard"
	"servika/internal/secret"
)

// Destination describes a remote backup upload destination.
type Destination struct {
	ID         int64  `json:"id"`
	DomainID   int64  `json:"domain_id"`
	Type       string `json:"type"` // "ftp" | "sftp"
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password,omitempty"` // write-only: returns empty on GET
	RemoteDir  string `json:"remote_dir"`
	Bucket     string `json:"bucket,omitempty"`
	Region     string `json:"region,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
	PathStyle  bool   `json:"path_style"`
	Enabled    bool   `json:"active"`
	LastUpload string `json:"last_upload,omitempty"`
	LastStatus string `json:"last_status,omitempty"`
	LastError  string `json:"last_error,omitempty"`
}

func validType(t string) bool {
	return t == "ftp" || t == "sftp" || t == "s3" || t == "b2"
}

func objectStorageType(t string) bool { return t == "s3" || t == "b2" }

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// readDestination: returns a domain destination record (nil, nil if none).
func readDestination(ctx context.Context, db *sql.DB, domainID int64) (*Destination, error) {
	d := &Destination{DomainID: domainID}
	var enabled int
	var lastUpload sql.NullString
	err := db.QueryRowContext(ctx,
		`SELECT id, type, host, port, username, password, remote_dir,
		        bucket, region, endpoint, path_style, enabled,
		        DATE_FORMAT(last_upload,'%Y-%m-%d %H:%i'), last_status, last_error
		 FROM backup_destinations WHERE domain_id=?`, domainID).
		Scan(&d.ID, &d.Type, &d.Host, &d.Port, &d.Username, &d.Password, &d.RemoteDir,
			&d.Bucket, &d.Region, &d.Endpoint, &d.PathStyle, &enabled, &lastUpload,
			&d.LastStatus, &d.LastError)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.Enabled = enabled == 1
	if lastUpload.Valid {
		d.LastUpload = lastUpload.String
	}
	// Stored password is encrypted at rest; decrypt so runtime consumers
	// (upload, connection test) receive the usable plaintext. Legacy plaintext
	// rows pass through unchanged.
	pw, err := secret.Decrypt(d.Password)
	if err != nil {
		return nil, err
	}
	d.Password = pw
	return d, nil
}

// lftpURL builds an lftp URL from the type, host, and port.
func lftpURL(d *Destination) string {
	if d.Type == "sftp" {
		return fmt.Sprintf("sftp://%s:%d", d.Host, d.Port)
	}
	return fmt.Sprintf("ftp://%s:%d", d.Host, d.Port)
}

// uploadToRemote: uploads the local tar.gz to the remote destination.
// With lftp: connect, cd, put. Auto-confirm host key for SFTP.
func uploadToRemote(ctx context.Context, d *Destination, localPath, fileName string) error {
	if !d.Enabled {
		return nil // Skip disabled destinations.
	}
	if objectStorageType(d.Type) {
		return uploadS3Object(ctx, d, localPath, fileName)
	}
	if err := netguard.CheckHost(d.Host); err != nil {
		return fmt.Errorf("destination host not permitted: %w", err)
	}
	url := lftpURL(d)
	// with cmd:fail-exit, lftp exits non-zero if any command fails
	script := fmt.Sprintf(
		`set cmd:fail-exit yes; `+
			`set sftp:auto-confirm yes; `+
			`set ssl:verify-certificate no; `+
			`set ftp:ssl-allow no; `+
			`set net:max-retries 1; `+
			`set net:timeout 15; `+
			`set net:reconnect-interval-base 2; `+
			`open -u "%s","%s" %s; `+
			`mkdir -p -f "%s"; `+
			`cd "%s"; `+
			`put -O . "%s"; `+
			`bye`,
		lftpEscape(d.Username), lftpEscape(d.Password), url,
		lftpEscape(d.RemoteDir), lftpEscape(d.RemoteDir), localPath)

	cmd := exec.CommandContext(ctx, "lftp", "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("lftp: %s: %w", strings.TrimSpace(string(out)), err)
	}
	// Treat known error text in otherwise successful output as a failure for defense in depth.
	bad := []string{"Login failed", "Access failed", "Connection refused", "Permission denied",
		"Could not resolve", "Host key verification failed", "No route to host"}
	for _, p := range bad {
		if strings.Contains(string(out), p) {
			return fmt.Errorf("lftp: %s", strings.TrimSpace(string(out)))
		}
	}
	_ = fileName
	return nil
}

// downloadFromRemote fetches a single backup file from the destination into localPath.
func downloadFromRemote(ctx context.Context, d *Destination, fileName, localPath string) error {
	if objectStorageType(d.Type) {
		return downloadS3Object(ctx, d, fileName, localPath)
	}
	if err := netguard.CheckHost(d.Host); err != nil {
		return fmt.Errorf("destination host not permitted: %w", err)
	}
	script := fmt.Sprintf(
		`set cmd:fail-exit yes; set sftp:auto-confirm yes; `+
			`set ssl:verify-certificate no; set ftp:ssl-allow no; `+
			`set net:max-retries 1; set net:timeout 15; `+
			`open -u "%s","%s" %s; cd "%s"; get "%s" -o "%s"; bye`,
		lftpEscape(d.Username), lftpEscape(d.Password), lftpURL(d),
		lftpEscape(d.RemoteDir), lftpEscape(fileName), lftpEscape(localPath))
	out, err := exec.CommandContext(ctx, "lftp", "-c", script).CombinedOutput()
	if err != nil {
		_ = os.Remove(localPath)
		return fmt.Errorf("lftp: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// deleteFromRemote removes a single backup file from the destination.
func deleteFromRemote(ctx context.Context, d *Destination, fileName string) error {
	if objectStorageType(d.Type) {
		return deleteS3Object(ctx, d, fileName)
	}
	if err := netguard.CheckHost(d.Host); err != nil {
		return fmt.Errorf("destination host not permitted: %w", err)
	}
	script := fmt.Sprintf(
		`set cmd:fail-exit yes; set sftp:auto-confirm yes; `+
			`set ssl:verify-certificate no; set ftp:ssl-allow no; `+
			`set net:max-retries 1; set net:timeout 15; `+
			`open -u "%s","%s" %s; cd "%s"; rm "%s"; bye`,
		lftpEscape(d.Username), lftpEscape(d.Password), lftpURL(d),
		lftpEscape(d.RemoteDir), lftpEscape(fileName))
	out, err := exec.CommandContext(ctx, "lftp", "-c", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("lftp: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// lftpEscape: escapes values placed inside double quotes on the lftp command line.
func lftpEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// validHost returns true when h is a plausible hostname or IP address.
// It rejects values containing shell/lftp meta-characters that could enable
// command injection when the host is embedded in command-line arguments.
var hostPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-\.]{0,251})[a-zA-Z0-9]$|^[a-fA-F0-9:\.\[\]]+$`)

func validHost(h string) bool {
	if len(h) > 253 {
		return false
	}
	return hostPattern.MatchString(h)
}

// testConnection verifies the destination credentials.
// sshpass+ssh for SFTP, curl for FTP; both return an auth-specific exit code.
func testConnection(ctx context.Context, d *Destination) error {
	if objectStorageType(d.Type) {
		return testS3Connection(ctx, d)
	}
	if err := netguard.CheckHost(d.Host); err != nil {
		return fmt.Errorf("destination host not permitted: %w", err)
	}
	if d.Type == "sftp" {
		// Force password authentication through sshpass and disable public-key fallback.
		// This ensures the supplied user password is actually valid.
		host := fmt.Sprintf("%s@%s", d.Username, d.Host)
		args := []string{
			"-p", d.Password,
			"ssh",
			"-p", fmt.Sprintf("%d", d.Port),
			"-o", "ConnectTimeout=10",
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"-o", "PreferredAuthentications=password",
			"-o", "PubkeyAuthentication=no",
			"-o", "BatchMode=no",
			host, "true",
		}
		cmd := exec.CommandContext(ctx, "sshpass", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			short := strings.TrimSpace(string(out))
			if short == "" {
				short = err.Error()
			}
			return fmt.Errorf("%s", short)
		}
		return nil
	}
	// Use curl to list the FTP root with the supplied credentials.
	url := fmt.Sprintf("ftp://%s:%d/", d.Host, d.Port)
	args := []string{
		"-sS",
		"--connect-timeout", "10",
		"--max-time", "15",
		"--user", d.Username + ":" + d.Password,
		"--ftp-skip-pasv-ip",
		url,
	}
	cmd := exec.CommandContext(ctx, "curl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		short := strings.TrimSpace(string(out))
		if short == "" {
			short = err.Error()
		}
		return fmt.Errorf("%s", short)
	}
	return nil
}

// pushToDestinationAsync: triggers a background upload after the backup is created successfully.
// Does not block the API response even on error; last_status/last_error are written to the DB.
func pushToDestinationAsync(db *sql.DB, domainID, backupID int64, localPath, fileName string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		d, err := readDestination(ctx, db, domainID)
		if err != nil || d == nil || !d.Enabled {
			return
		}
		_, _ = db.Exec(`UPDATE backups SET remote_status='uploading', remote_error='' WHERE id=?`, backupID)
		if err := uploadToRemote(ctx, d, localPath, fileName); err != nil {
			short := err.Error()
			if len(short) > 500 {
				short = short[:500]
			}
			_, _ = db.Exec(`UPDATE backup_destinations
				SET last_status='failed', last_error=?, last_upload=NOW() WHERE domain_id=?`,
				short, domainID)
			_, _ = db.Exec(`UPDATE backups SET remote_status='failed', remote_error=? WHERE id=?`,
				short, backupID)
			log.Printf("backup destination upload domain=%d: %v", domainID, err)
			return
		}
		_, _ = db.Exec(`UPDATE backup_destinations
			SET last_status='successful', last_error='', last_upload=NOW() WHERE domain_id=?`,
			domainID)
		remoteKey := strings.Trim(strings.TrimSpace(d.RemoteDir), "/")
		if remoteKey != "" {
			remoteKey += "/"
		}
		remoteKey += fileName
		_, _ = db.Exec(`UPDATE backups
			SET remote_status='successful', remote_key=?, remote_error='' WHERE id=?`,
			remoteKey, backupID)
		log.Printf("backup destination upload domain=%d successful: %s", domainID, fileName)
	}()
}
