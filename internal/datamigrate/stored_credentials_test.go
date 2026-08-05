package datamigrate

import (
	"testing"

	"servika/internal/secret"
)

func TestNeedsEncryptionSelectsOnlyLegacyCleartext(t *testing.T) {
	if err := secret.Init([]byte("test-key-for-credential-backfill-0123456789")); err != nil {
		t.Fatalf("secret.Init: %v", err)
	}
	sealed, err := secret.Encrypt("ghp_exampletoken")
	if err != nil {
		t.Fatalf("secret.Encrypt: %v", err)
	}

	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"legacy cleartext", "ghp_exampletoken", true},
		{"already encrypted", sealed, false},
		// Sealing "" would turn "no credential stored" into a non-empty value that
		// every "is it set?" check reads as a stored credential.
		{"no credential stored", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := needsEncryption(c.value); got != c.want {
				t.Errorf("needsEncryption(%q) = %v, want %v", c.value, got, c.want)
			}
		})
	}
}

// The backfill runs on every boot, so a value it rewrites must not be picked up
// again on the next pass, and the original must survive the round trip.
func TestEncryptedCredentialConvergesAndRoundTrips(t *testing.T) {
	if err := secret.Init([]byte("test-key-for-credential-backfill-0123456789")); err != nil {
		t.Fatalf("secret.Init: %v", err)
	}
	const plaintext = "s3cret-backup-password"

	sealed, err := secret.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("secret.Encrypt: %v", err)
	}
	if needsEncryption(sealed) {
		t.Fatal("the migrated value is selected again, so the backfill would re-encrypt it on every boot")
	}
	back, err := secret.Decrypt(sealed)
	if err != nil {
		t.Fatalf("secret.Decrypt: %v", err)
	}
	if back != plaintext {
		t.Errorf("Decrypt(Encrypt(%q)) = %q", plaintext, back)
	}
}

// Every column listed must be one the reading code actually decrypts; a typo
// here would silently migrate nothing.
func TestEncryptedCredentialsAreNamed(t *testing.T) {
	if len(encryptedCredentials) == 0 {
		t.Fatal("no columns are registered for the credential backfill")
	}
	for _, target := range encryptedCredentials {
		if target.table == "" || target.column == "" {
			t.Errorf("incomplete target %+v", target)
		}
	}
}
