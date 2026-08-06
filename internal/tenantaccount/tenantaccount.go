// Package tenantaccount builds the ownership chain a tenant needs to exist in
// the panel's account model.
//
// Authorization is resolved from the database on every request, never from a
// list embedded in a token:
//
//	domains.customer_id -> customers.id -> customers.owner_user_id  (reseller)
//	                                    -> customers.user_id        (customer)
//
// Without the middle links an account can sign in and see nothing, because
// middleware.ScopeSQL narrows every list through that chain.
//
// This is deliberately separate from internal/datamigrate. That package holds
// one-shot backfills for state that predates a feature and is only meant to run
// at startup; calling it from a live request path would blur what it is for.
// The backfill delegates here instead, so one definition of "the chain" serves
// both the old rows and every new one.
package tenantaccount

import (
	"context"
	"database/sql"
)

// Ensure returns the customers row for a tenant, creating the panel account and
// the customer record when they do not exist yet. It is idempotent: every step
// reuses an existing row, so a repeated call is a no-op that returns the same id.
//
// owner_user_id is deliberately left NULL, which means the customer belongs
// directly to an administrator. Every caller reaches this with an administrator's
// authority: a reseller cannot create a domain without naming one of its own
// customers, so the auto-creation path is never taken on its behalf.
//
// The generated users row carries an EMPTY password_hash. An empty hash matches
// no password at all (auth.PasswordMatches), so the account cannot sign in until
// somebody assigns one. That is chosen over generating a password nobody is
// told: it leaves no account that has a password no one knows, and the Customer
// Accounts screen flags every passwordless row so none is missed.
func Ensure(ctx context.Context, db *sql.DB, systemUser, displayName string) (int64, error) {
	if systemUser == "" {
		// Nothing identifies the tenant, so there is nothing to attach an account
		// to. Returning early keeps a malformed domain row from producing an
		// account named after nobody.
		return 0, nil
	}
	if displayName == "" {
		displayName = systemUser
	}

	transaction, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = transaction.Rollback() }() // no-op once the commit succeeds

	userID, err := ensureUser(ctx, transaction, systemUser, displayName)
	if err != nil {
		return 0, err
	}
	customerID, err := ensureCustomer(ctx, transaction, userID, displayName)
	if err != nil {
		return 0, err
	}
	if err := transaction.Commit(); err != nil {
		return 0, err
	}
	return customerID, nil
}

func ensureUser(ctx context.Context, transaction *sql.Tx, systemUser, displayName string) (int64, error) {
	var userID int64
	err := transaction.QueryRowContext(ctx,
		`SELECT id FROM users WHERE username=?`, systemUser).Scan(&userID)
	if err == nil {
		return userID, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	result, err := transaction.ExecContext(ctx, `
		INSERT INTO users(username, email, password_hash, role, full_name, status)
		VALUES(?, '', '', 'user', ?, 'active')`, systemUser, displayName)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func ensureCustomer(ctx context.Context, transaction *sql.Tx, userID int64, displayName string) (int64, error) {
	var customerID int64
	err := transaction.QueryRowContext(ctx,
		`SELECT id FROM customers WHERE user_id=?`, userID).Scan(&customerID)
	if err == nil {
		return customerID, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	result, err := transaction.ExecContext(ctx, `
		INSERT INTO customers(name, email, status, notes, user_id)
		VALUES(?, '', 'active', 'created with the tenant', ?)`, displayName, userID)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
