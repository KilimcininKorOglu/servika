package stats

import "testing"

func TestParseTrafficLineUsesRequestMonthAndResponseBytes(t *testing.T) {
	line := `192.0.2.1 - - [17/Jul/2026:12:00:00 +0000] "GET / HTTP/1.1" 200 1234 "-" "agent"`
	month, bytes, ok := parseTrafficLine(line)
	if !ok {
		t.Fatal("parseTrafficLine() rejected a valid combined access log line")
	}
	if month != "2026-07" || bytes != 1234 {
		t.Fatalf("parseTrafficLine() = (%q, %d), want (%q, %d)", month, bytes, "2026-07", 1234)
	}
}

func TestParseTrafficLineTreatsMissingByteCountAsZero(t *testing.T) {
	line := `192.0.2.1 - - [17/Jul/2026:12:00:00 +0000] "GET / HTTP/1.1" 304 - "-" "agent"`
	month, bytes, ok := parseTrafficLine(line)
	if !ok || month != "2026-07" || bytes != 0 {
		t.Fatalf("parseTrafficLine() = (%q, %d, %t), want (%q, 0, true)", month, bytes, ok, "2026-07")
	}
}
