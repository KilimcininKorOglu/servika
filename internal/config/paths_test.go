package config

import "testing"

func TestEnvStringReturnsFallbackWhenUnsetOrEmpty(t *testing.T) {
	t.Setenv("SERVIKA_TEST_VALUE", "")
	if got := EnvString("SERVIKA_TEST_VALUE", "fallback"); got != "fallback" {
		t.Fatalf("EnvString() = %q, want fallback", got)
	}
}

func TestEnvStringReturnsTrimmedValue(t *testing.T) {
	t.Setenv("SERVIKA_TEST_VALUE", "  value  ")
	if got := EnvString("SERVIKA_TEST_VALUE", "fallback"); got != "value" {
		t.Fatalf("EnvString() = %q, want value", got)
	}
}

func TestEnvAbsPathRequiresAbsolutePath(t *testing.T) {
	t.Setenv("SERVIKA_TEST_PATH", "relative/bin")
	if _, err := EnvAbsPath("SERVIKA_TEST_PATH", "/fallback"); err == nil {
		t.Fatal("EnvAbsPath() error = nil, want error")
	}
}

func TestEnvAbsPathReturnsCleanAbsolutePath(t *testing.T) {
	t.Setenv("SERVIKA_TEST_PATH", "/opt/servika/../servika/bin")
	got, err := EnvAbsPath("SERVIKA_TEST_PATH", "/fallback")
	if err != nil {
		t.Fatalf("EnvAbsPath() error = %v", err)
	}
	if got != "/opt/servika/bin" {
		t.Fatalf("EnvAbsPath() = %q, want cleaned path", got)
	}
}

func TestEnvURLRequiresHTTPURL(t *testing.T) {
	t.Setenv("SERVIKA_TEST_URL", "file:///tmp/file")
	if _, err := EnvURL("SERVIKA_TEST_URL", "https://example.com"); err == nil {
		t.Fatal("EnvURL() error = nil, want error")
	}
}

func TestEnvURLReturnsHTTPSURL(t *testing.T) {
	t.Setenv("SERVIKA_TEST_URL", "https://example.com/path")
	got, err := EnvURL("SERVIKA_TEST_URL", "https://fallback.example")
	if err != nil {
		t.Fatalf("EnvURL() error = %v", err)
	}
	if got != "https://example.com/path" {
		t.Fatalf("EnvURL() = %q, want configured URL", got)
	}
}

func TestShellQuote(t *testing.T) {
	got := ShellQuote("/tmp/it's here")
	want := `'/tmp/it'\''s here'`
	if got != want {
		t.Fatalf("ShellQuote() = %q, want %q", got, want)
	}
}

func TestOpsToolUsesConfiguredOpsBin(t *testing.T) {
	t.Setenv("SERVIKA_OPSBIN", "/custom/bin")
	if got := OpsTool("servika-jail"); got != "/custom/bin/servika-jail" {
		t.Fatalf("OpsTool() = %q, want configured tool path", got)
	}
}
