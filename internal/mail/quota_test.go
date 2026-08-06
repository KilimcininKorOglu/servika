package mail

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
)

// The plan stores megabytes because that is what an operator types; Dovecot's
// quota_rule and the mailbox row both speak bytes. Getting the factor wrong by
// 1000 vs 1024 silently hands out a different limit than the plan promises.
func TestQuotaBytesFromMBUsesBinaryMegabytes(t *testing.T) {
	for _, tc := range []struct {
		quotaMB int64
		want    int64
	}{
		{quotaMB: 1, want: 1048576},
		{quotaMB: 2048, want: 2147483648},
	} {
		if got := quotaBytesFromMB(tc.quotaMB); got != tc.want {
			t.Errorf("quotaBytesFromMB(%d) = %d, want %d", tc.quotaMB, got, tc.want)
		}
	}
}

// 0 has to stay 0 all the way down: the userdb query turns a zero into a NULL
// quota_rule, so an unlimited plan really is unlimited. Turning it into a limit
// of zero bytes would reject every message the domain receives.
func TestQuotaBytesFromMBTreatsNonPositiveAsUnlimited(t *testing.T) {
	for _, quotaMB := range []int64{0, -1} {
		if got := quotaBytesFromMB(quotaMB); got != 0 {
			t.Errorf("quotaBytesFromMB(%d) = %d, want 0", quotaMB, got)
		}
	}
}

var errQuotaDBUnavailable = errors.New("connection refused")

type quotaFailingConnector struct{}

func (quotaFailingConnector) Open(string) (driver.Conn, error) { return nil, errQuotaDBUnavailable }

func init() { sql.Register("mail_quota_failing_db", quotaFailingConnector{}) }

// A lookup that cannot reach the database must not invent a limit. Returning a
// non-zero guess would cap a mailbox the operator never capped; returning 0
// leaves the plan unapplied, which the caller logs and an operator can fix.
func TestPlanMailboxQuotaBytesFallsBackToUnlimitedOnReadFailure(t *testing.T) {
	db, err := sql.Open("mail_quota_failing_db", "")
	if err != nil {
		t.Fatalf("open the failing database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if got := planMailboxQuotaBytes(context.Background(), db, 1); got != 0 {
		t.Errorf("planMailboxQuotaBytes with an unreachable database = %d, want 0", got)
	}
}
