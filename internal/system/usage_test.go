package system

import (
	"errors"
	"strings"
	"testing"
)

type failingReader struct {
	data []byte
	err  error
}

func (r *failingReader) Read(p []byte) (int, error) {
	if len(r.data) > 0 {
		n := copy(p, r.data)
		r.data = r.data[n:]
		return n, nil
	}
	return 0, r.err
}

func TestScanLinesReturnsScannerError(t *testing.T) {
	readErr := errors.New("system data read failed")
	reader := &failingReader{data: []byte("first\nsecond\n"), err: readErr}
	var lines []string

	err := scanLines(reader, func(line string) bool {
		lines = append(lines, line)
		return true
	})

	if !errors.Is(err, readErr) {
		t.Fatalf("scanLines() error = %v, want %v", err, readErr)
	}
	if got := strings.Join(lines, ","); got != "first,second" {
		t.Fatalf("scanLines() visited %q, want %q", got, "first,second")
	}
}

func TestScanLinesStopsWhenVisitorRequests(t *testing.T) {
	var lines []string

	err := scanLines(strings.NewReader("first\nsecond\n"), func(line string) bool {
		lines = append(lines, line)
		return false
	})

	if err != nil {
		t.Fatalf("scanLines() returned an unexpected error: %v", err)
	}
	if got := strings.Join(lines, ","); got != "first" {
		t.Fatalf("scanLines() visited %q, want %q", got, "first")
	}
}
