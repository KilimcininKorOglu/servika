package mail

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"servika/internal/dns"
)

// EnableDomain enables mail for a domain and prepares its Maildir root.
func EnableDomain(ctx context.Context, db *sql.DB, domainID int64) error {
	var domainName, systemUser, ipv4 string
	if err := db.QueryRowContext(ctx,
		`SELECT domain_name, system_user, COALESCE(ipv4,'') FROM domains WHERE id=?`, domainID).
		Scan(&domainName, &systemUser, &ipv4); err != nil {
		return fmt.Errorf("read domain: %w", err)
	}
	uid, gid, err := uidGid(systemUser)
	if err != nil {
		return fmt.Errorf("lookup system user: %w", err)
	}
	maildirRoot := filepath.Join("/home", systemUser, "mail")
	if err := os.MkdirAll(maildirRoot, 0o750); err != nil {
		return fmt.Errorf("create Maildir root: %w", err)
	}
	_ = os.Chown(maildirRoot, uid, gid)

	if _, err := db.ExecContext(ctx,
		`INSERT INTO mail_domains(domain_id, domain_name, system_user, uid_n, gid_n, maildir_root)
		 VALUES(?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE system_user=VALUES(system_user), uid_n=VALUES(uid_n),
		   gid_n=VALUES(gid_n), maildir_root=VALUES(maildir_root), status='active'`,
		domainID, domainName, systemUser, uid, gid, maildirRoot); err != nil {
		return fmt.Errorf("write mail domain: %w", err)
	}

	if _, err := dns.SeedDefaults(ctx, db, domainID, domainName, ipv4); err != nil {
		log.Printf("mail: dns seed %s: %v", domainName, err)
	}
	if err := dns.WriteZone(ctx, db, domainID); err != nil {
		log.Printf("mail: write DNS zone %s: %v", domainName, err)
	}
	return nil
}

// DisableDomain soft-disables mail for a domain without deleting mailbox rows or Maildir data.
func DisableDomain(ctx context.Context, db *sql.DB, domainID int64) error {
	_, err := db.ExecContext(ctx, `UPDATE mail_domains SET status='suspended' WHERE domain_id=?`, domainID)
	return err
}

// CleanupDomain is a domain-deletion hook for future non-cascading mail side effects.
func CleanupDomain(_ *sql.DB, _ int64, _ string) {}

func uidGid(name string) (int, int, error) {
	account, err := user.Lookup(name)
	if err != nil {
		return 0, 0, err
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		return 0, 0, err
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}
