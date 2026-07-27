// Package quota enforces plan limits before domains, databases, or FTP accounts are added.
package quota

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

// customerMu serializes quota-check-then-create sequences per customer within this
// single-process panel. Without it, concurrent create requests can each pass the count
// check before any insert lands and exceed a plan limit (a check/insert race). Callers
// hold the lock across both the CheckDatabaseAllowed call and the account creation.
var customerMu sync.Map // customerID (int64) -> *sync.Mutex

// LockCustomerForDomain resolves the domain's customer and locks a per-customer mutex,
// returning an unlock function. Admin-owned domains (no customer) return a no-op unlock.
// Use it to make a quota check and the subsequent resource creation atomic:
//
//	unlock := quota.LockCustomerForDomain(ctx, db, domainID)
//	defer unlock()
//	// ... CheckDatabaseAllowed + create ...
func LockCustomerForDomain(ctx context.Context, db *sql.DB, domainID int64) func() {
	var customerID *int64
	if err := db.QueryRowContext(ctx, `SELECT customer_id FROM domains WHERE id=?`, domainID).Scan(&customerID); err != nil || customerID == nil {
		return func() {} // No customer (admin) or lookup failed: the quota check itself will fail closed.
	}
	actual, _ := customerMu.LoadOrStore(*customerID, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// LimitError reports that a plan quota has been reached.
type LimitError struct {
	Message string
}

func (e *LimitError) Error() string { return e.Message }

// ---------- Reseller limits (WHM "reseller limits" equivalent) ----------
//
// A reseller with no row in reseller_limits is UNLIMITED; a 0 value also means
// unlimited (the same quota contract service_plans already uses). Defining a
// limit therefore stays optional and existing behavior is unchanged.

// CheckResellerCustomerAllowed reports whether a reseller may create one more customer.
func CheckResellerCustomerAllowed(ctx context.Context, db *sql.DB, resellerUserID int64) error {
	maximum, err := resellerLimit(ctx, db, resellerUserID, "max_customer")
	if err != nil || maximum <= 0 {
		return nil
	}
	var current int
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM customers WHERE owner_user_id=?`, resellerUserID).Scan(&current)
	if current >= maximum {
		return &LimitError{Message: fmt.Sprintf("reseller limit reached: at most %d customers", maximum)}
	}
	return nil
}

// CheckResellerDomainAllowed reports whether the reseller's total domain quota is full.
//
// This is separate from the customer plan's max_domain: that caps a single
// customer, this caps the sum across all of the reseller's customers. Both apply.
func CheckResellerDomainAllowed(ctx context.Context, db *sql.DB, resellerUserID int64) error {
	maximum, err := resellerLimit(ctx, db, resellerUserID, "max_domain")
	if err != nil || maximum <= 0 {
		return nil
	}
	var current int
	_ = db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM domains d JOIN customers c ON c.id = d.customer_id
		WHERE c.owner_user_id = ?`, resellerUserID).Scan(&current)
	if current >= maximum {
		return &LimitError{Message: fmt.Sprintf("reseller limit reached: at most %d domains", maximum)}
	}
	return nil
}

// resellerLimit reads a single numeric limit from reseller_limits. Returns 0
// (unlimited) when the reseller has no row.
func resellerLimit(ctx context.Context, db *sql.DB, resellerUserID int64, column string) (int, error) {
	// The column name arrives only as a constant string from within this package
	// (no SQL-injection surface); it is still constrained to the expected values.
	switch column {
	case "max_customer", "max_domain":
	default:
		return 0, fmt.Errorf("unknown limit column: %s", column)
	}
	var v int
	err := db.QueryRowContext(ctx,
		`SELECT `+column+` FROM reseller_limits WHERE user_id=?`, resellerUserID).Scan(&v)
	if err != nil {
		return 0, nil // No row = unlimited.
	}
	return v, nil
}

// CheckDomainAllowed checks the customer's plan.max_domain limit when customerID is set.
func CheckDomainAllowed(ctx context.Context, db *sql.DB, customerID *int64) error {
	if customerID == nil {
		return nil // Administrators have no quota limit.
	}
	var planID *int64
	if err := db.QueryRowContext(ctx, `SELECT plan_id FROM customers WHERE id=?`, *customerID).Scan(&planID); err != nil {
		return err
	}
	if planID == nil {
		return nil
	}
	var maximum int
	if err := db.QueryRowContext(ctx, `SELECT max_domain FROM service_plans WHERE id=?`, *planID).Scan(&maximum); err != nil {
		return err
	}
	if maximum <= 0 {
		return nil // Unlimited.
	}
	var current int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM domains WHERE customer_id=?`, *customerID).Scan(&current); err != nil {
		return err // FAIL-CLOSED: never bypass the limit gate on a count error.
	}
	if current >= maximum {
		return &LimitError{Message: fmt.Sprintf("plan limit exceeded: maximum %d domains", maximum)}
	}
	return nil
}

// CheckDatabaseAllowed checks the domain customer's plan.max_db limit.
func CheckDatabaseAllowed(ctx context.Context, db *sql.DB, domainID int64) error {
	var customerID *int64
	if err := db.QueryRowContext(ctx, `SELECT customer_id FROM domains WHERE id=?`, domainID).Scan(&customerID); err != nil {
		return err
	}
	if customerID == nil {
		return nil
	}
	var planID *int64
	if err := db.QueryRowContext(ctx, `SELECT plan_id FROM customers WHERE id=?`, *customerID).Scan(&planID); err != nil {
		return err
	}
	if planID == nil {
		return nil
	}
	var maximum int
	if err := db.QueryRowContext(ctx, `SELECT max_db FROM service_plans WHERE id=?`, *planID).Scan(&maximum); err != nil {
		return err
	}
	if maximum <= 0 {
		return nil
	}
	var current int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM db_accounts a JOIN domains d ON d.id=a.domain_id WHERE d.customer_id=?`,
		*customerID).Scan(&current); err != nil {
		return err // FAIL-CLOSED: never bypass the limit gate on a count error.
	}
	if current >= maximum {
		return &LimitError{Message: fmt.Sprintf("plan limit exceeded: maximum %d databases", maximum)}
	}
	return nil
}

// CheckMailboxAllowed checks the domain customer's plan.max_email limit.
func CheckMailboxAllowed(ctx context.Context, db *sql.DB, domainID int64) error {
	var customerID *int64
	if err := db.QueryRowContext(ctx, `SELECT customer_id FROM domains WHERE id=?`, domainID).Scan(&customerID); err != nil {
		return err
	}
	if customerID == nil {
		return nil
	}
	var planID *int64
	if err := db.QueryRowContext(ctx, `SELECT plan_id FROM customers WHERE id=?`, *customerID).Scan(&planID); err != nil {
		return err
	}
	if planID == nil {
		return nil
	}
	var maximum int
	if err := db.QueryRowContext(ctx, `SELECT max_email FROM service_plans WHERE id=?`, *planID).Scan(&maximum); err != nil {
		return err
	}
	if maximum <= 0 {
		return nil
	}
	var current int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mailboxes m JOIN domains d ON d.id=m.domain_id WHERE d.customer_id=?`,
		*customerID).Scan(&current); err != nil {
		return err // FAIL-CLOSED: never bypass the limit gate on a count error.
	}
	if current >= maximum {
		return &LimitError{Message: fmt.Sprintf("plan limit exceeded: maximum %d mailboxes", maximum)}
	}
	return nil
}
