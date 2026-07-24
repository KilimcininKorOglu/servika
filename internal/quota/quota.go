// Package quota enforces plan limits before domains, databases, or FTP accounts are added.
package quota

import (
	"context"
	"database/sql"
	"fmt"
)

// LimitError reports that a plan quota has been reached.
type LimitError struct {
	Message string
}

func (e *LimitError) Error() string { return e.Message }

// CheckDomainAllowed checks the customer's plan.max_domain limit when customerID is set.
func CheckDomainAllowed(ctx context.Context, db *sql.DB, customerID *int64) error {
	if customerID == nil {
		return nil // Administrators have no quota limit.
	}
	var planID *int64
	if err := db.QueryRowContext(ctx, `SELECT plan_id FROM customers WHERE id=?`, *customerID).Scan(&planID); err != nil {
		return nil
	}
	if planID == nil {
		return nil
	}
	var maximum int
	if err := db.QueryRowContext(ctx, `SELECT max_domain FROM service_plans WHERE id=?`, *planID).Scan(&maximum); err != nil {
		return nil
	}
	if maximum <= 0 {
		return nil // Unlimited.
	}
	var current int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM domains WHERE customer_id=?`, *customerID).Scan(&current)
	if current >= maximum {
		return &LimitError{Message: fmt.Sprintf("plan limit exceeded: maximum %d domains", maximum)}
	}
	return nil
}

// CheckDatabaseAllowed checks the domain customer's plan.max_db limit.
func CheckDatabaseAllowed(ctx context.Context, db *sql.DB, domainID int64) error {
	var customerID *int64
	if err := db.QueryRowContext(ctx, `SELECT customer_id FROM domains WHERE id=?`, domainID).Scan(&customerID); err != nil {
		return nil
	}
	if customerID == nil {
		return nil
	}
	var planID *int64
	_ = db.QueryRowContext(ctx, `SELECT plan_id FROM customers WHERE id=?`, *customerID).Scan(&planID)
	if planID == nil {
		return nil
	}
	var maximum int
	_ = db.QueryRowContext(ctx, `SELECT max_db FROM service_plans WHERE id=?`, *planID).Scan(&maximum)
	if maximum <= 0 {
		return nil
	}
	var current int
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM db_accounts a JOIN domains d ON d.id=a.domain_id WHERE d.customer_id=?`,
		*customerID).Scan(&current)
	if current >= maximum {
		return &LimitError{Message: fmt.Sprintf("plan limit exceeded: maximum %d databases", maximum)}
	}
	return nil
}

// CheckMailboxAllowed checks the domain customer's plan.max_email limit.
func CheckMailboxAllowed(ctx context.Context, db *sql.DB, domainID int64) error {
	var customerID *int64
	if err := db.QueryRowContext(ctx, `SELECT customer_id FROM domains WHERE id=?`, domainID).Scan(&customerID); err != nil {
		return nil
	}
	if customerID == nil {
		return nil
	}
	var planID *int64
	_ = db.QueryRowContext(ctx, `SELECT plan_id FROM customers WHERE id=?`, *customerID).Scan(&planID)
	if planID == nil {
		return nil
	}
	var maximum int
	_ = db.QueryRowContext(ctx, `SELECT max_email FROM service_plans WHERE id=?`, *planID).Scan(&maximum)
	if maximum <= 0 {
		return nil
	}
	var current int
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mailboxes m JOIN domains d ON d.id=m.domain_id WHERE d.customer_id=?`,
		*customerID).Scan(&current)
	if current >= maximum {
		return &LimitError{Message: fmt.Sprintf("plan limit exceeded: maximum %d mailboxes", maximum)}
	}
	return nil
}
