package middleware

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"
)

var errUnreachable = errors.New("connection refused")

type refusingConnector struct{}

func (refusingConnector) Open(string) (driver.Conn, error) { return nil, errUnreachable }

func init() { sql.Register("middleware_failing_db", refusingConnector{}) }

// A blip shorter than the retries take must not reach the caller at all.
func TestReadStateRetriesATransientFailure(t *testing.T) {
	resetStateCache()
	calls := 0
	got, err := readState(context.Background(), "k", func(context.Context) (int64, error) {
		calls++
		if calls < 2 {
			return 0, errUnreachable
		}
		return 42, nil
	})
	if err != nil {
		t.Fatalf("readState: %v", err)
	}
	if got != 42 || calls != 2 {
		t.Errorf("value = %d after %d calls, want 42 after 2", got, calls)
	}
}

// With the database down and nothing cached, the read still fails, so the
// caller still denies the request. This is the property the cache must not cost
// us: "I cannot check" never turns into "it is fine".
func TestReadStateFailsWhenNothingWasEverRead(t *testing.T) {
	resetStateCache()
	if _, err := readState(context.Background(), "k", func(context.Context) (int64, error) {
		return 0, errUnreachable
	}); !errors.Is(err, errUnreachable) {
		t.Errorf("error = %v, want the read failure to reach the caller", err)
	}
}

// The fallback replays what the database last returned. That is what keeps a
// revoked session revoked during an outage: the stored value is the CURRENT
// token_version, so a token carrying the old one still fails to match.
func TestReadStateFallsBackToTheLastValueTheDatabaseReturned(t *testing.T) {
	resetStateCache()
	ctx := context.Background()
	if _, err := readState(ctx, "k", func(context.Context) (int64, error) { return 9, nil }); err != nil {
		t.Fatalf("seed the cache: %v", err)
	}
	got, err := readState(ctx, "k", func(context.Context) (int64, error) { return 0, errUnreachable })
	if err != nil {
		t.Fatalf("readState during the outage: %v", err)
	}
	if got != 9 {
		t.Errorf("value = %d, want the 9 the database last returned", got)
	}
}

// The fallback is bounded. Past the TTL an outage denies again rather than
// serving an answer nobody has confirmed for half a minute.
func TestReadStateStopsFallingBackOnceTheEntryIsStale(t *testing.T) {
	resetStateCache()
	base := time.Now()
	stateNow = func() time.Time { return base }
	t.Cleanup(func() { stateNow = time.Now })

	ctx := context.Background()
	if _, err := readState(ctx, "k", func(context.Context) (int64, error) { return 9, nil }); err != nil {
		t.Fatalf("seed the cache: %v", err)
	}
	stateNow = func() time.Time { return base.Add(stateCacheTTL + time.Second) }
	if _, err := readState(ctx, "k", func(context.Context) (int64, error) {
		return 0, errUnreachable
	}); !errors.Is(err, errUnreachable) {
		t.Errorf("error = %v, want the stale entry to be ignored", err)
	}
}

// An absent row is an answer. Retrying it only delays the response, and caching
// it would cache something the caller already knows.
func TestReadStateDoesNotRetryAMissingRow(t *testing.T) {
	resetStateCache()
	calls := 0
	_, err := readState(context.Background(), "k", func(context.Context) (int64, error) {
		calls++
		return 0, sql.ErrNoRows
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("error = %v, want sql.ErrNoRows", err)
	}
	if calls != 1 {
		t.Errorf("query ran %d times, want 1", calls)
	}
	if _, ok := stateLoad("k", time.Now()); ok {
		t.Error("a missing row was cached")
	}
}

// The whole point of caching the version rather than a verdict: while the
// database is unreachable, a session whose version was already bumped is still
// refused, and only the matching one is let through.
func TestTokenVersionIsStillEnforcedFromTheFallback(t *testing.T) {
	resetStateCache()
	db, err := sql.Open("middleware_failing_db", "")
	if err != nil {
		t.Fatalf("open the failing database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	previous := scopeDB
	Init(db)
	t.Cleanup(func() { scopeDB = previous })

	// What the database returned the last time it answered for this user.
	stateStore("token_version:users:7", 3, time.Now())

	ok, err := tokenVersionMatches(context.Background(), "users", 7, 3)
	if err != nil || !ok {
		t.Errorf("current session = (%v, %v), want it accepted from the fallback", ok, err)
	}
	ok, err = tokenVersionMatches(context.Background(), "users", 7, 2)
	if err != nil {
		t.Errorf("revoked session errored (%v); it should simply not match", err)
	}
	if ok {
		t.Error("a session carrying a superseded token_version was accepted during an outage")
	}
}

// Without a cached value the failure reaches the caller, which denies. One
// identity's cached state must not answer for another.
func TestTokenVersionFailsClosedForAnIdentityNeverRead(t *testing.T) {
	resetStateCache()
	db, err := sql.Open("middleware_failing_db", "")
	if err != nil {
		t.Fatalf("open the failing database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	previous := scopeDB
	Init(db)
	t.Cleanup(func() { scopeDB = previous })

	stateStore("token_version:users:7", 3, time.Now())
	if ok, err := tokenVersionMatches(context.Background(), "users", 8, 3); err == nil || ok {
		t.Errorf("user 8 = (%v, %v), want the read failure; user 7's state answered for it", ok, err)
	}
}
