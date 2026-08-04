package auth

import (
	"os/exec"
	"testing"
)

// The reference hashes below were produced by PHP's crypt(), which carries its own
// SHA-crypt and MD5-crypt implementation. Generating them with openssl would only
// prove that openssl agrees with itself; an independent implementation is what
// makes this a real check that a legacy /etc/shadow entry still verifies.
var legacyCryptVectors = []struct {
	name     string
	hash     string
	password string
}{
	{
		name:     "sha512 default rounds",
		hash:     `$6$saltstring$svn8UoSVapNtMuq1ukKS4tPQd8iKwSMHWjl/O817G3uBnIFNjnQJuesI68u4OTLiBFdcbYEdFCoEOfaS35inz1`,
		password: "Hello world!",
	},
	{
		name:     "sha256 default rounds",
		hash:     `$5$saltstring$5B8vYYiY.CVt1RlTTf8KbXBH3hsxY/GNooZaBBGWEc5`,
		password: "Hello world!",
	},
	{
		name:     "md5crypt",
		hash:     `$1$saltstri$YMyguxXMBpd2TEZ.vS/3q1`,
		password: "Hello world!",
	},
	{
		name:     "sha512 explicit rounds",
		hash:     `$6$rounds=10000$saltstringsaltst$OW1/O6BYHV6BcXZu8QVeXbDWra3Oeqh0sbHbbMCVNSnCM/UrjmM0Dp8vOuZeHBy/YTBmSK6H9qs/y3RnOaw5v.`,
		password: "Hello world!",
	},
	{
		name:     "sha256 explicit rounds",
		hash:     `$5$rounds=10000$saltstringsaltst$3xv.VbSHBb41AL9AvLeujZkZRBAwqFMz2.opqey6IcA`,
		password: "Hello world!",
	},
	{
		name:     "sha512 rounds equal to the default",
		hash:     `$6$rounds=5000$abcdefghijklmnop$UY4jc6.rVibJ9tqDqiG0GMdZRHkv1j4sPRRH2eUSo3Kszltzbk30CmYcWPNRTD/KsYFHF7WTtNkAxF3dZ3zPE.`,
		password: "correct horse battery staple",
	},
	{
		name:     "salt using the full crypt alphabet",
		hash:     `$6$Xy1./Zq9$wDwbxlzG5wPMBR1GiLpLZryLkEPaCoYv6MmEXFfapzARyCddmjSi0kc/4ZWk3gR1mGyabf9boIm87PzFP5WLn0`,
		password: "p@ssw0rd with spaces",
	},
}

func TestLegacyCryptSaltExtractsTheRoundsPrefix(t *testing.T) {
	tests := []struct {
		name, hash, wantID, wantSalt string
		wantOK                       bool
	}{
		{name: "no rounds", hash: "$6$saltstring$digest", wantID: "6", wantSalt: "saltstring", wantOK: true},
		{name: "with rounds", hash: "$6$rounds=10000$saltstr$digest", wantID: "6", wantSalt: "rounds=10000$saltstr", wantOK: true},
		{name: "md5", hash: "$1$saltstri$digest", wantID: "1", wantSalt: "saltstri", wantOK: true},
		// openssl invents a random salt for anything it cannot parse, which turns
		// every login into a silent mismatch, so each of these must be refused here.
		{name: "empty salt", hash: "$6$$digest"},
		{name: "non-numeric rounds", hash: "$6$rounds=abc$saltstr$digest"},
		{name: "empty rounds", hash: "$6$rounds=$saltstr$digest"},
		{name: "salt outside the crypt alphabet", hash: "$6$salt with spaces$digest"},
		{name: "salt that looks like an option", hash: "$6$-6$digest"},
		{name: "too few segments", hash: "$6$saltstring"},
		{name: "unexpected extra segment", hash: "$6$a$b$c$d"},
		{name: "not a crypt hash", hash: "locked"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id, salt, ok := legacyCryptSalt(tc.hash)
			if ok != tc.wantOK {
				t.Fatalf("legacyCryptSalt(%q) ok = %v, want %v", tc.hash, ok, tc.wantOK)
			}
			if ok && (id != tc.wantID || salt != tc.wantSalt) {
				t.Fatalf("legacyCryptSalt(%q) = (%q, %q), want (%q, %q)", tc.hash, id, salt, tc.wantID, tc.wantSalt)
			}
		})
	}
}

func TestLegacyCryptVerifyAgainstReferenceHashes(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl is not on PATH; it ships with every Servika host")
	}
	for _, tc := range legacyCryptVectors {
		t.Run(tc.name, func(t *testing.T) {
			if !legacyCryptVerify(tc.password, tc.hash) {
				t.Error("the correct password was rejected")
			}
			if legacyCryptVerify(tc.password+"x", tc.hash) {
				t.Error("a wrong password was accepted")
			}
		})
	}
}

// A format openssl passwd cannot recompute must fail closed rather than fall
// through to an empty or partial comparison.
func TestLegacyCryptVerifyRejectsUnsupportedFormats(t *testing.T) {
	for _, hash := range []string{
		`$2b$12$KIXQKcCd1kzrOxCF6oIuIuWiPmBFVVpEUJnI0zLDT2WEfF5f9Ykiu`, // bcrypt
		`$y$j9T$salt$digest`, // yescrypt never reaches this path
		`$7$C6..../....SodiumChloride$kBGj9fHznVYFQMEn/qDCfrDevf9YDtcDdKvEqHJLV8D`, // scrypt
		`invalid`,
		``,
	} {
		if legacyCryptVerify("anything", hash) {
			t.Errorf("legacyCryptVerify accepted an unsupported hash: %q", hash)
		}
	}
}
