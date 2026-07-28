package transfers

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// archiveFile writes the test archive to disk; readSmallTarMembers takes a path.
func archiveFile(t *testing.T, entries ...testEntry) string {
	t.Helper()
	r := archive(t, entries...)
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadSmallTarMembersFindsAllInOnePass(t *testing.T) {
	p := archiveFile(t,
		testEntry{name: "backup-demo/sslcerts/example.com.crt", body: "CERT"},
		testEntry{name: "backup-demo/sslkeys/example.com.key", body: "KEY"},
		testEntry{name: "backup-demo/va/example.com", body: "sales: out@example.net\n"},
		testEntry{name: "backup-demo/homedir/public_html/index.php", body: "<?php"},
	)
	got, err := readSmallTarMembers(p, []string{
		"backup-demo/sslcerts/example.com.crt",
		"backup-demo/sslkeys/example.com.key",
		"backup-demo/va/example.com",
		"backup-demo/missing/member",
		"",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 members, got %d: %v", len(got), got)
	}
	if string(got["backup-demo/sslcerts/example.com.crt"]) != "CERT" ||
		string(got["backup-demo/sslkeys/example.com.key"]) != "KEY" {
		t.Fatalf("member bodies are wrong: %v", got)
	}
	if _, ok := got["backup-demo/missing/member"]; ok {
		t.Fatal("an absent member must not appear in the result")
	}
}

func TestArchiveExtrasCollectsSSLAndAliasInOnePass(t *testing.T) {
	p := archiveFile(t,
		testEntry{name: "backup-demo/homedir/ssl/certs/example.com.crt", body: "CERT"},
		testEntry{name: "backup-demo/homedir/ssl/private/example.com.key", body: "KEY"},
		testEntry{name: "backup-demo/sslcerts/example.com.cabundle", body: "BUNDLE"},
		testEntry{name: "backup-demo/va/example.com", body: "sales: out@example.net\ninfobox: info\n"},
	)
	inv := Inventory{ArchiveRoot: "backup-demo", PrimaryDomain: "example.com"}
	extras, err := readArchiveExtras(p, inv)
	if err != nil {
		t.Fatal(err)
	}
	cert, key, err := extras.sslPair()
	if err != nil {
		t.Fatalf("SSL pair not found: %v", err)
	}
	// Chain: leaf certificate + CA bundle must be concatenated.
	if !bytes.Contains(cert, []byte("CERT")) || !bytes.Contains(cert, []byte("BUNDLE")) {
		t.Fatalf("certificate chain is incomplete: %q", cert)
	}
	if string(key) != "KEY" {
		t.Fatalf("private key is wrong: %q", key)
	}

	aliases := readAliases(extras, "example.com", "new.com")
	if len(aliases) != 2 {
		t.Fatalf("expected 2 aliases, got %d: %+v", len(aliases), aliases)
	}
	if aliases[0].Local != "sales" || aliases[0].Destination != "out@example.net" {
		t.Fatalf("alias is wrong: %+v", aliases[0])
	}
	// A destination without '@' must be bound to the new domain.
	if aliases[1].Destination != "info@new.com" {
		t.Fatalf("local destination must bind to the new domain: %+v", aliases[1])
	}
}

func TestSSLPairMissingKeyReturnsNotFound(t *testing.T) {
	p := archiveFile(t,
		testEntry{name: "backup-demo/sslcerts/example.com.crt", body: "CERT"},
	)
	extras, err := readArchiveExtras(p, Inventory{ArchiveRoot: "backup-demo", PrimaryDomain: "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := extras.sslPair(); err == nil {
		t.Fatal("sslPair must not return a pair when no matching private key exists")
	}
}
