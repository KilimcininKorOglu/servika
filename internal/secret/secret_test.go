package secret

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	if err := Init([]byte("test-key-at-least-32-bytes-long!!")); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for _, pt := range []string{"", "hunter2", "p@ss w0rd with spaces", strings.Repeat("x", 300)} {
		enc, err := Encrypt(pt)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", pt, err)
		}
		if !strings.HasPrefix(enc, prefix) {
			t.Errorf("Encrypt(%q) missing prefix: %q", pt, enc)
		}
		got, err := Decrypt(enc)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if got != pt {
			t.Errorf("round trip: got %q want %q", got, pt)
		}
	}
}

func TestEncryptDistinctNonce(t *testing.T) {
	if err := Init([]byte("test-key-at-least-32-bytes-long!!")); err != nil {
		t.Fatalf("Init: %v", err)
	}
	a, _ := Encrypt("same")
	b, _ := Encrypt("same")
	if a == b {
		t.Error("expected distinct ciphertexts for repeated plaintext")
	}
}

func TestDecryptLegacyPlaintext(t *testing.T) {
	if err := Init([]byte("test-key-at-least-32-bytes-long!!")); err != nil {
		t.Fatalf("Init: %v", err)
	}
	got, err := Decrypt("legacy-cleartext-password")
	if err != nil {
		t.Fatalf("Decrypt legacy: %v", err)
	}
	if got != "legacy-cleartext-password" {
		t.Errorf("legacy passthrough: got %q", got)
	}
}

func TestDecryptCorruptCiphertext(t *testing.T) {
	if err := Init([]byte("test-key-at-least-32-bytes-long!!")); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := Decrypt(prefix + "not-valid-base64!!!"); err == nil {
		t.Error("expected error for invalid base64")
	}
	if _, err := Decrypt(prefix + "AAAA"); err == nil {
		t.Error("expected error for too-short ciphertext")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	if err := Init([]byte("first-key-at-least-32-bytes-long!")); err != nil {
		t.Fatalf("Init: %v", err)
	}
	enc, _ := Encrypt("secret-value")
	if err := Init([]byte("second-key-at-least-32-bytes-lon!")); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := Decrypt(enc); err == nil {
		t.Error("expected authentication failure with wrong key")
	}
}
