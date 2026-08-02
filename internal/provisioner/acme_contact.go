package provisioner

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"servika/internal/config"
)

// letsEncryptDirectory is the acme.sh per-CA config directory for the Let's Encrypt
// production endpoint; its ca.conf holds the registered contact address (CA_EMAIL).
const letsEncryptDirectory = "ca/acme-v02.api.letsencrypt.org/directory"

// RunACMEIssue runs an acme.sh command and recovers from a permanently broken account
// contact address.
//
// Root problem: installation accepts any address that merely looks like an email (for
// example admin@test.local, whose TLD is not a public suffix). Let's Encrypt rejects it
// with invalidContact, but acme.sh still persists the value in BOTH account.conf
// (ACCOUNT_EMAIL) and the per-CA ca.conf (CA_EMAIL) even though registration failed.
// acme.sh reads CA_EMAIL first, so every later --issue call reuses the broken address and
// fails again with invalidContact, even for a perfectly valid domain and even when no -m
// flag is passed. Once a host lands in this state it can never obtain a certificate.
//
// Recovery: clear the stored contact in both files, register a contact-less account, and
// retry the command once. This also self-heals hosts already stuck in that state, without
// needing shell access.
func RunACMEIssue(args ...string) ([]byte, error) {
	out, err := acmeCommand(args...).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "invalidContact") {
		return out, err
	}
	log.Printf("acme: invalidContact, clearing the stored account contact and re-registering")
	clearACMEContact()
	_, _ = acmeCommand("--register-account", "--server", "letsencrypt").CombinedOutput()
	return acmeCommand(args...).CombinedOutput()
}

// clearACMEContact empties the persisted acme.sh contact address in account.conf and in
// the Let's Encrypt ca.conf. Both must be cleared: acme.sh resolves the account email from
// the ACCOUNT_EMAIL environment variable, then CA_EMAIL, then account.conf, so clearing
// only one of them leaves the broken address in effect.
func clearACMEContact() {
	home := config.ACMEHome()
	emptyACMEKey(filepath.Join(home, "account.conf"), "ACCOUNT_EMAIL")
	emptyACMEKey(filepath.Join(home, letsEncryptDirectory, "ca.conf"), "CA_EMAIL")
}

func emptyACMEKey(path, key string) {
	// #nosec G304 -- path is a fixed system/config path, a server-internal temp/archive path, or built from a validated identifier; tenant file reads go through safeio (openat2), not this call.
	body, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(body), "\n")
	changed := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), key+"=") {
			lines[i] = key + "=''"
			changed = true
		}
	}
	if !changed {
		return
	}
	// #nosec G703 -- path is built from a validated identifier (systemUser ^c_[A-Za-z0-9_]+$ / validated domainName), a fixed system path, or a server-internal temp path; tenant file-manager paths use safeio (openat2) instead.
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		log.Printf("acme: clear %s in %s: %v", key, path, err)
	}
}

// ACMECommand exposes an acme.sh command with the panel's environment for callers outside
// this package.
func ACMECommand(args ...string) *exec.Cmd { return acmeCommand(args...) }
