package mail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"servika/internal/dns"
	"servika/internal/files"
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

// removeMailFiles deletes the domain's Maildir root. It is a variable so a test
// can exercise both the success and the failure path: the real implementation is
// Linux-only (safeio_stub.go returns an error on macOS), which would otherwise
// make every local run take the failure branch.
var removeMailFiles = func(systemUser string) error {
	return files.RemoveAllBeneath(filepath.Join("/home", systemUser), "mail")
}

// nonCascadingMailTables are the per-domain mail tables that hang off domains(id)
// rather than mail_domains(id), so deleting the mail domain does NOT take them
// with it. Verified against the migrations rather than assumed: mailboxes cascade
// from mail_domains, and mail_autoresponders and mail_filters cascade from
// mailboxes, but these five do not cascade from anything being deleted here.
//
// Missing one would leave the rows behind and hand them straight back the next
// time the domain enabled mail: old forwarders, old spam thresholds, and
// single-use webmail sign-on tokens for mailboxes that no longer exist.
var nonCascadingMailTables = []string{
	"mail_aliases",
	"mail_send_log",
	"mail_spam_settings",
	"mail_delivery_log",
	"webmail_tokens",
}

// PurgeDomain is the destructive opposite of EnableDomain: it removes mail
// hosting for a domain outright. Mailboxes, forwarders, filters, autoresponders
// and the stored messages on disk are all deleted, and none of it comes back.
// DisableDomain is the reversible one; the two must not be confused.
//
// The DNS records EnableDomain seeded (MX, SPF, DKIM, DMARC) are deliberately
// left in place. The customer may have pointed MX at an outside provider, and
// those records are managed from the DNS page; deleting them silently would be
// worse than leaving them.
//
// ORDER MATTERS, database first and disk second. Losing the disk step leaves
// orphaned files behind a service that is already gone, which is the harmless
// direction. The other way round, Dovecot would be pointed at Maildirs that no
// longer exist.
//
// diskFailed reports that the database is clean but some files survived. The
// caller has to surface that: the service really is gone, yet the space is not.
func PurgeDomain(ctx context.Context, db *sql.DB, domainID int64, systemUser string) (diskFailed bool, err error) {
	transaction, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()

	for _, table := range nonCascadingMailTables {
		// #nosec G202 -- table comes from the package-level allowlist above, never from a request.
		if _, err := transaction.ExecContext(ctx, `DELETE FROM `+table+` WHERE domain_id=?`, domainID); err != nil {
			return false, fmt.Errorf("delete %s: %w", table, err)
		}
	}
	// Last, because this is what cascades: mailboxes go with it, and their
	// autoresponders and filters go with them.
	if _, err := transaction.ExecContext(ctx,
		`DELETE FROM mail_domains WHERE domain_id=?`, domainID); err != nil {
		return false, fmt.Errorf("delete mail_domains: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}

	// /home/<system_user>/mail holds every mailbox's Maildir and the .dovecot.sieve
	// compiled into it, so one removal covers both, plus any Maildir left behind by
	// a mailbox deleted earlier.
	//
	// Through files.RemoveAllBeneath, never os.RemoveAll: the panel runs as root
	// inside a tenant's home, and a tenant who replaced "mail" with a symlink could
	// otherwise redirect the deletion anywhere on the host.
	if err := removeMailFiles(systemUser); err != nil && !errors.Is(err, os.ErrNotExist) {
		// #nosec G706 -- logged values are integer IDs, validated identifiers (^c_[A-Za-z0-9_]+$), template-derived names, or error/command output; no raw tenant string with CR/LF reaches the log.
		log.Printf("purge mail domain=%d: mail files not removed: %v", domainID, err)
		return true, nil
	}
	return false, nil
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
