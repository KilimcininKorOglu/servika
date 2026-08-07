package mail

import (
	"context"
	"strings"
	"time"
)

// Reason codes the verification step can answer with. They are codes rather than
// sentences because the API is English and the panel ships twelve languages.
const (
	ReasonAuthFailed = "auth_failed"
	// The provider switched password-based IMAP off entirely. No password will
	// ever work, so the customer needs to be told that rather than left retyping
	// one they believe is wrong.
	ReasonBasicAuthDisabled = "basic_auth_disabled"
	// The account itself is fine but needs a purpose-made password.
	ReasonAppPasswordRequired = "app_password_required"
)

// verifyBudget bounds one login attempt, including the connection.
const verifyBudget = 30 * time.Second

// providerHints name the hosts that refuse an ordinary password, and what the
// customer has to do instead.
//
// Without this the panel would report "authentication failed" for a password
// that is perfectly correct, and the customer would spend the afternoon
// retyping it.
var providerHints = []struct {
	markers []string
	code    string
}{
	{
		// Microsoft removed Basic Authentication for IMAP; an app password
		// cannot be created either.
		markers: []string{"office365", "outlook.", "outlook.com", "hotmail", "live.com", "msn."},
		code:    ReasonBasicAuthDisabled,
	},
	{
		markers: []string{"gmail", "googlemail", "google.com"},
		code:    ReasonAppPasswordRequired,
	},
	{
		markers: []string{"yahoo", "ymail", "rocketmail", "yandex", "aol."},
		code:    ReasonAppPasswordRequired,
	},
}

// providerHint returns the code describing why a well-known provider will
// refuse, or empty when the host is not one of them.
func providerHint(host, username string) string {
	haystack := strings.ToLower(host + " " + username)
	for _, hint := range providerHints {
		for _, marker := range hint.markers {
			if strings.Contains(haystack, marker) {
				return hint.code
			}
		}
	}
	return ""
}

// VerifyLogin signs in to the remote server before any copying starts.
//
// A migration runs for hours, and a wrong password discovered at the end wastes
// all of it. This is also where a customer learns that their provider will never
// accept the password they are holding, instead of concluding the panel is
// broken.
func VerifyLogin(ctx context.Context, host string, port int, security, username, password string) (bool, string) {
	ctx, cancel := context.WithTimeout(ctx, verifyBudget)
	defer cancel()

	client, err := dialIMAP(ctx, host, port, security)
	if err != nil {
		return false, reasonFor(err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Login(username, password).Wait(); err != nil {
		// A provider that cannot accept a password at all reports the same
		// "authentication failed" as a typo, so the host decides which of the
		// two the customer is actually looking at.
		if code := providerHint(host, username); code != "" {
			return false, code
		}
		return false, ReasonAuthFailed
	}
	return true, ""
}
