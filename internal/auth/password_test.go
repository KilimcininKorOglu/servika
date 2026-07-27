package auth

import "testing"

func TestIsRootUser(t *testing.T) {
	for _, name := range []string{"root", "ROOT", " root ", "Root"} {
		if !IsRootUser(name) {
			t.Errorf("%q should be treated as root", name)
		}
	}
	// Names that look like a "root" variant but are different must not be routed
	// to the system password.
	for _, name := range []string{"root2", "rooot", "reseller", "", "toor"} {
		if IsRootUser(name) {
			t.Errorf("%q should not be treated as root", name)
		}
	}
}

func TestHashPasswordAndVerify(t *testing.T) {
	const p = "verySecurePass1"
	h, err := HashPassword(p)
	if err != nil {
		t.Fatalf("could not hash: %v", err)
	}
	if h == p {
		t.Fatal("password stored as plaintext")
	}
	if !PasswordMatches(h, p) {
		t.Error("the correct password should match")
	}
	if PasswordMatches(h, "wrongPassword1") {
		t.Error("a wrong password should not match")
	}

	// Hashing the same password twice must produce different output (salt).
	h2, _ := HashPassword(p)
	if h == h2 {
		t.Error("bcrypt does not appear to be salting")
	}
}

func TestHashPasswordRejectsShort(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Error("a password shorter than 8 characters should be rejected")
	}
}

func TestEmptyHashBlocksLogin(t *testing.T) {
	// An account whose password was never set (password_hash=''): neither an
	// empty password nor any password may log in.
	if PasswordMatches("", "") {
		t.Error("empty hash + empty password was accepted")
	}
	if PasswordMatches("", "anything") {
		t.Error("empty hash accepted some password")
	}
}
