package mail

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// The realignment must never touch a mailbox somebody set by hand. The guard is
// in the WHERE clause, so this checks the statement itself: a query without it
// would silently undo every per-mailbox decision on the next plan edit.
func TestSendLimitRealignmentSkipsHandSetMailboxes(t *testing.T) {
	statement := sendLimitRealignSQL()
	if !strings.Contains(statement, "send_limits_manual = 0") {
		t.Errorf("the realignment statement does not exclude hand-set mailboxes:\n%s", statement)
	}
	if !strings.Contains(statement, "domain_id = ?") {
		t.Errorf("the realignment statement is not scoped to one domain:\n%s", statement)
	}
}

// A plan that leaves a send limit at 0 must leave the column alone. Writing the
// zero would make the mailbox unlimited, because that is what 0 means to the
// policy server, so an operator adding a quota-only plan would quietly disable
// spam protection for every mailbox under it.
func TestZeroSendLimitLeavesTheColumnUntouched(t *testing.T) {
	statement := sendLimitRealignSQL()
	for _, column := range []string{"send_limit_hour", "send_limit_day"} {
		if !strings.Contains(statement, "IF(? > 0, ?, "+column+")") {
			t.Errorf("a zero plan value overwrites %s instead of keeping it:\n%s", column, statement)
		}
	}
}

// The whole point of a plan limit is that it reaches the row Dovecot and the
// policy server read. An update that changed nothing would leave both enforcing
// the previous plan.
func TestApplyPlanLimitsReportsTheReadFailureRatherThanClaimingSuccess(t *testing.T) {
	db, err := sql.Open("mail_quota_failing_db", "")
	if err != nil {
		t.Fatalf("open the failing database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	changed, err := ApplyPlanLimitsToDomain(context.Background(), db, 1)
	if err == nil {
		t.Error("ApplyPlanLimitsToDomain reported success against an unreachable database")
	}
	if changed != 0 {
		t.Errorf("ApplyPlanLimitsToDomain reported %d changed rows against an unreachable database", changed)
	}
}

// Same for the plan-wide pass: a domain listing that fails must not read as
// "no domains needed changing".
func TestApplyPlanLimitsToPlanReportsTheReadFailure(t *testing.T) {
	db, err := sql.Open("mail_quota_failing_db", "")
	if err != nil {
		t.Fatalf("open the failing database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := ApplyPlanLimitsToPlan(context.Background(), db, 1); err == nil {
		t.Error("ApplyPlanLimitsToPlan reported success against an unreachable database")
	}
}
