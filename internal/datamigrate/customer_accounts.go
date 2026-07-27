// Package datamigrate holds one-shot data migrations that run once per version
// upgrade at server startup.
//
// Unlike SQL migrations, these produce DATA (not schema) and need Go-side
// computation (bcrypt hashing, grouping, and so on). Every migration here is
// idempotent: it runs on each boot and exits silently when there is no work.
package datamigrate

import (
	"context"
	"database/sql"
	"log"
)

// BackfillCustomerAccounts moves existing domains onto the multi-user account
// model.
//
// The panel began as single-admin: customers signed into /cp with their FTP
// identity only, so neither a customers record nor a panel login account
// existed. This migration produces the missing links for every existing tenant:
//
//	domains.system_user  (tenant)
//	   -> customers       (billing/contact record)
//	        -> users      (role='user', the panel login account)
//
// Grouped by TENANT, not by domain: one tenant may own several domains
// (addon/parked) and in the cPanel model they belong to a SINGLE account.
// Producing one account per domain would create multiple panel accounts for the
// same system user.
//
// The generated users rows are left with an EMPTY password_hash. An empty hash
// never matches any password (see auth.PasswordMatches), so these accounts
// cannot log in until an admin or reseller assigns a password from the Customer
// Accounts screen. This was chosen over generating a random password no one is
// told: it leaves no "has a password but nobody knows it" accounts, and the
// Customer Accounts list flags every passwordless row so none is overlooked.
func BackfillCustomerAccounts(ctx context.Context, db *sql.DB) {
	// Tenants that still have domains not linked to a customer record.
	rows, err := db.QueryContext(ctx, `
		SELECT system_user, MIN(domain_name), COUNT(*)
		FROM domains
		WHERE customer_id IS NULL AND system_user <> ''
		GROUP BY system_user`)
	if err != nil {
		log.Printf("customer account backfill: could not read tenant list: %v", err)
		return
	}
	type tenant struct {
		systemUser string
		domainName string
		domains    int
	}
	var list []tenant
	for rows.Next() {
		var t tenant
		if err := rows.Scan(&t.systemUser, &t.domainName, &t.domains); err == nil {
			list = append(list, t)
		}
	}
	rows.Close()
	if len(list) == 0 {
		return
	}

	var created, skipped int
	for _, t := range list {
		if err := migrateTenant(ctx, db, t.systemUser, t.domainName); err != nil {
			log.Printf("customer account backfill: %s skipped: %v", t.systemUser, err)
			skipped++
			continue
		}
		created++
	}
	log.Printf("customer account backfill: %d tenants migrated, %d skipped (FTP login stays valid until a password is set)",
		created, skipped)
}

// migrateTenant creates the customers+users links for one tenant inside a
// single transaction, so a half-finished migration is retried whole on the next
// boot. Every step reuses an existing row when found, which is what makes the
// whole migration idempotent.
func migrateTenant(ctx context.Context, db *sql.DB, systemUser, domainName string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op once the commit succeeds

	// users: reuse the row when the username already exists (a prior run may
	// have been interrupted). The empty password_hash means the account cannot
	// log in until a password is assigned.
	var userID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username=?`, systemUser).Scan(&userID)
	if err == sql.ErrNoRows {
		res, iErr := tx.ExecContext(ctx, `
			INSERT INTO users(username, email, password_hash, role, full_name, status)
			VALUES(?, '', '', 'user', ?, 'active')`, systemUser, domainName)
		if iErr != nil {
			return iErr
		}
		userID, _ = res.LastInsertId()
	} else if err != nil {
		return err
	}

	// customers: reuse the record already bound to this panel account.
	var customerID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM customers WHERE user_id=?`, userID).Scan(&customerID)
	if err == sql.ErrNoRows {
		res, iErr := tx.ExecContext(ctx, `
			INSERT INTO customers(name, email, status, notes, user_id)
			VALUES(?, '', 'active', 'auto-created by panel account migration', ?)`, domainName, userID)
		if iErr != nil {
			return iErr
		}
		customerID, _ = res.LastInsertId()
	} else if err != nil {
		return err
	}

	// Link this tenant's unowned domains to the customer.
	if _, err := tx.ExecContext(ctx, `
		UPDATE domains SET customer_id=?
		WHERE system_user=? AND customer_id IS NULL`, customerID, systemUser); err != nil {
		return err
	}

	return tx.Commit()
}
