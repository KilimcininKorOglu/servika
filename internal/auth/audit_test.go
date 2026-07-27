package auth

import (
	"strings"
	"testing"
)

func TestAuditLimit(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{"empty defaults to 200", "", 200},
		{"valid mid-range value", "150", 150},
		{"exact cap", "1000", 1000},
		{"above cap clamps to 1000", "5000", 1000},
		{"zero falls back to default", "0", 200},
		{"negative falls back to default", "-10", 200},
		{"non-numeric falls back to default", "abc", 200},
		{"one is honored", "1", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := auditLimit(c.raw); got != c.want {
				t.Errorf("auditLimit(%q) = %d, want %d", c.raw, got, c.want)
			}
		})
	}
}

func TestBuildAuditQuery(t *testing.T) {
	t.Run("no filters: bare select, only limit bound", func(t *testing.T) {
		q, arg := buildAuditQuery("", false, 200)
		if strings.Contains(q, "WHERE") {
			t.Errorf("no filters should produce no WHERE clause: %q", q)
		}
		if !strings.HasSuffix(q, "ORDER BY id DESC LIMIT ?") {
			t.Errorf("query must end with ordered limit: %q", q)
		}
		if len(arg) != 1 || arg[0] != 200 {
			t.Errorf("args = %v, want [200]", arg)
		}
	})

	t.Run("action filter is bound as placeholder, never interpolated", func(t *testing.T) {
		// A SQL-injection-shaped action must appear ONLY as a bound arg; the
		// query text must carry a single `?` placeholder for it, not the value.
		inj := "auth.login'; DROP TABLE audit_log;--"
		q, arg := buildAuditQuery(inj, false, 200)
		if strings.Contains(q, "DROP TABLE") || strings.Contains(q, inj) {
			t.Fatalf("action value leaked into SQL text: %q", q)
		}
		if !strings.Contains(q, "action = ?") {
			t.Errorf("expected bound action predicate: %q", q)
		}
		if len(arg) != 2 || arg[0] != inj || arg[1] != 200 {
			t.Errorf("args = %v, want [%q 200]", arg, inj)
		}
	})

	t.Run("only_failed adds constant predicate with no arg", func(t *testing.T) {
		q, arg := buildAuditQuery("", true, 500)
		if !strings.Contains(q, "ok = 0") {
			t.Errorf("expected ok = 0 predicate: %q", q)
		}
		if len(arg) != 1 || arg[0] != 500 {
			t.Errorf("args = %v, want [500]", arg)
		}
	})

	t.Run("both filters joined with AND, args ordered action then limit", func(t *testing.T) {
		q, arg := buildAuditQuery("auth.2fa", true, 42)
		if !strings.Contains(q, "WHERE action = ? AND ok = 0") {
			t.Errorf("expected both predicates AND-joined: %q", q)
		}
		if len(arg) != 2 || arg[0] != "auth.2fa" || arg[1] != 42 {
			t.Errorf("args = %v, want [auth.2fa 42]", arg)
		}
	})

	t.Run("whitespace-only action is treated as absent", func(t *testing.T) {
		q, arg := buildAuditQuery("   ", false, 200)
		if strings.Contains(q, "WHERE") {
			t.Errorf("blank action should not add WHERE: %q", q)
		}
		if len(arg) != 1 {
			t.Errorf("args = %v, want just [limit]", arg)
		}
	})
}
