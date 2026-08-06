package mail

import (
	"context"
	"database/sql"
)

// A plan that carries mail limits but never writes them anywhere is a number on
// a screen. These two functions are what makes the plan actually govern the
// mailboxes underneath it: one for a single domain, one for every domain on a
// plan, and both leave a mailbox alone once somebody has set its limits by hand.

// PlanMailLimits is what a plan says a mailbox should get.
type PlanMailLimits struct {
	// QuotaBytes is the per-mailbox storage limit; 0 means no limit.
	QuotaBytes int64
	// SendLimitHour and SendLimitDay are 0 when the plan does not override the
	// built-in per-mailbox default, NOT when it wants no limit. The mailbox
	// column already treats 0 as unlimited, so a plan left at zero must not be
	// allowed to strip the spam protection every mailbox ships with.
	SendLimitHour int
	SendLimitDay  int
}

// planLimitsFor reads the limits the domain's plan imposes.
func planLimitsFor(ctx context.Context, db *sql.DB, domainID int64) (PlanMailLimits, error) {
	var quotaMB int64
	var limits PlanMailLimits
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(p.mailbox_quota_mb, 0),
		        COALESCE(p.mail_send_limit_hour, 0),
		        COALESCE(p.mail_send_limit_day, 0)
		   FROM domains d LEFT JOIN service_plans p ON p.id = d.plan_id
		  WHERE d.id = ?`, domainID).
		Scan(&quotaMB, &limits.SendLimitHour, &limits.SendLimitDay)
	if err != nil {
		return PlanMailLimits{}, err
	}
	limits.QuotaBytes = quotaBytesFromMB(quotaMB)
	return limits, nil
}

// ApplyPlanLimitsToDomain realigns a domain's mailboxes with its plan and
// returns how many rows changed.
//
// The quota is written unconditionally because the plan is its only source; the
// send limits are written only where send_limits_manual is 0, so a mailbox an
// operator tuned by hand survives every later plan change. A plan that does not
// set send limits leaves those columns alone rather than resetting them.
func ApplyPlanLimitsToDomain(ctx context.Context, db *sql.DB, domainID int64) (int64, error) {
	limits, err := planLimitsFor(ctx, db, domainID)
	if err != nil {
		return 0, err
	}

	result, err := db.ExecContext(ctx,
		`UPDATE mailboxes SET quota_bytes = ? WHERE domain_id = ?`, limits.QuotaBytes, domainID)
	if err != nil {
		return 0, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	if limits.SendLimitHour > 0 || limits.SendLimitDay > 0 {
		hour, day := limits.SendLimitHour, limits.SendLimitDay
		result, err = db.ExecContext(ctx, sendLimitRealignSQL(), hour, hour, day, day, domainID)
		if err != nil {
			return changed, err
		}
		sendChanged, err := result.RowsAffected()
		if err != nil {
			return changed, err
		}
		changed += sendChanged
	}
	return changed, nil
}

// ApplyPlanLimitsToPlan realigns every domain on a plan and returns how many
// mailbox rows changed.
//
// One failing domain does not stop the rest: the first error is returned once
// every other domain has been handled, so a single broken row cannot leave the
// remaining customers on stale limits.
func ApplyPlanLimitsToPlan(ctx context.Context, db *sql.DB, planID int64) (int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT id FROM domains WHERE plan_id = ?`, planID)
	if err != nil {
		return 0, err
	}
	var domainIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		domainIDs = append(domainIDs, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	var changed int64
	var firstErr error
	for _, id := range domainIDs {
		n, err := ApplyPlanLimitsToDomain(ctx, db, id)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		changed += n
	}
	return changed, firstErr
}

// sendLimitRealignSQL is the statement that pushes a plan's send limits onto a
// domain's mailboxes.
//
// Two things in it are load-bearing, and a test asserts both. send_limits_manual
// = 0 is what protects a mailbox somebody tuned by hand. The IF wrappers leave a
// column alone when the plan does not set it, because writing the zero would
// read as "unlimited" to the policy server and remove the spam protection the
// mailbox shipped with.
func sendLimitRealignSQL() string {
	return `UPDATE mailboxes
	    SET send_limit_hour = IF(? > 0, ?, send_limit_hour),
	        send_limit_day  = IF(? > 0, ?, send_limit_day)
	  WHERE domain_id = ? AND send_limits_manual = 0`
}
