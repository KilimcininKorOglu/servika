package overview

import (
	"strconv"
	"strings"
	"testing"

	"servika/internal/domains"
)

// The ordering expression has to name the value that is actually stored. A typo
// would compile, run, and silently sort every self-signed certificate as if it
// were fine. internal/domains owns the constant and writes the column, so this
// checks against it rather than against a second copy of the spelling.
func TestTheOrderingNamesTheStoredSelfSignedValue(t *testing.T) {
	if !strings.Contains(sslUrgencyExpr, "'"+domains.SSLSourceSelfSigned+"'") {
		t.Fatalf("the ordering expression does not name %q:\n%s", domains.SSLSourceSelfSigned, sslUrgencyExpr)
	}
	// Only self-signed is demoted. Naming a trusted source here would push real
	// certificates up the list and bury the ones that need attention.
	for _, trusted := range []string{domains.SSLSourceLetsEncrypt, domains.SSLSourceImported} {
		if strings.Contains(sslUrgencyExpr, "'"+trusted+"'") {
			t.Errorf("the ordering expression singles out %q, which is a real certificate", trusted)
		}
	}
}

// The interval is what makes a self-signed certificate sort with the urgent ones
// instead of a year out. Deriving it from the constant is the point: a second
// literal would drift from the threshold it is named after.
func TestTheOrderingUsesTheUrgencyThreshold(t *testing.T) {
	want := "INTERVAL " + strconv.Itoa(selfSignedSortDays) + " DAY"
	if !strings.Contains(sslUrgencyExpr, want) {
		t.Errorf("the ordering expression does not carry %q:\n%s", want, sslUrgencyExpr)
	}
	// Sorting a self-signed certificate as expiring FURTHER out than a real one
	// would reproduce the defect this replaced.
	if selfSignedSortDays > 30 {
		t.Errorf("selfSignedSortDays = %d, which is past the screen's own warning band", selfSignedSortDays)
	}
}

// cond is a WHERE clause. It has to land after the FROM and before the ORDER BY,
// and the urgency expression has to be in the ORDER BY rather than only in the
// projection, or the rows come back in the old order.
func TestTheScopeConditionLandsBetweenFromAndOrderBy(t *testing.T) {
	const cond = " WHERE EXISTS (SELECT 1 FROM customers c WHERE c.id = d.customer_id AND c.owner_user_id = ?)"
	query := sslListQuery(cond)

	from := strings.Index(query, "FROM domains d")
	where := strings.Index(query, cond)
	order := strings.Index(query, "ORDER BY")
	if from < 0 || where < 0 || order < 0 {
		t.Fatalf("the statement is missing a clause:\n%s", query)
	}
	if from >= where || where >= order {
		t.Errorf("clause order is FROM=%d WHERE=%d ORDER BY=%d, want FROM < WHERE < ORDER BY", from, where, order)
	}
	if !strings.Contains(query[order:], sslUrgencyExpr) {
		t.Error("the urgency expression is not in the ORDER BY, so the ordering is unchanged")
	}
}

// An administrator passes an empty condition, and the statement still has to be
// valid: no dangling WHERE, and the ORDER BY straight after the FROM.
func TestAnUnscopedStatementIsStillWellFormed(t *testing.T) {
	query := sslListQuery("")
	if strings.Contains(query, "WHERE") {
		t.Errorf("an empty scope produced a WHERE clause:\n%s", query)
	}
	if !strings.Contains(query, "FROM domains d\nORDER BY") {
		t.Errorf("the ORDER BY does not follow the FROM:\n%s", query)
	}
}

// The source has to be selected for the screen to distinguish the states at all;
// the ordering alone would move the row without explaining why.
func TestTheSourceIsSelected(t *testing.T) {
	if !strings.Contains(sslListQuery(""), "COALESCE(d.ssl_source, '')") {
		t.Error("ssl_source is not in the projection, so the badge has nothing to read")
	}
}
