package overview

import "testing"

func TestParseDBSizes(t *testing.T) {
	t.Run("well-formed multi-line output", func(t *testing.T) {
		raw := []byte("panel\t512\ntenant_shop\t10240\ntenant_blog\t0\n")
		got := parseDBSizes(raw)
		want := map[string]int64{"panel": 512, "tenant_shop": 10240, "tenant_blog": 0}
		if len(got) != len(want) {
			t.Fatalf("size mismatch: got %d rows, want %d (%v)", len(got), len(want), got)
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%q = %d, want %d", k, got[k], v)
			}
		}
	})

	t.Run("skips malformed lines without dropping valid ones", func(t *testing.T) {
		// A blank line, a line with no tab, a 3-field line, and a non-numeric
		// size must all be skipped while the two valid rows survive.
		raw := []byte("panel\t512\n\nnotabhere\ntoo\tmany\tfields\ntenant\tNaN\ngood\t42")
		got := parseDBSizes(raw)
		if len(got) != 2 {
			t.Fatalf("expected 2 valid rows, got %d: %v", len(got), got)
		}
		if got["panel"] != 512 {
			t.Errorf("panel = %d, want 512", got["panel"])
		}
		if got["good"] != 42 {
			t.Errorf("good = %d, want 42", got["good"])
		}
		if _, ok := got["tenant"]; ok {
			t.Error("non-numeric size row should have been skipped")
		}
	})

	t.Run("empty output yields empty non-nil map", func(t *testing.T) {
		got := parseDBSizes([]byte("   \n  "))
		if got == nil {
			t.Fatal("map must be non-nil")
		}
		if len(got) != 0 {
			t.Errorf("expected empty map, got %v", got)
		}
	})

	t.Run("trims surrounding whitespace on both fields", func(t *testing.T) {
		got := parseDBSizes([]byte("  spaced_db \t  99  "))
		if got["spaced_db"] != 99 {
			t.Errorf("spaced_db = %d, want 99 (map: %v)", got["spaced_db"], got)
		}
	})
}
