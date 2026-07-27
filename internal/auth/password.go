package auth

import (
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Password layer for panel accounts.
//
// THERE ARE TWO SEPARATE PASSWORD WORLDS; they must never be mixed:
//
//   - root (id=1): the password lives in /etc/shadow, not in the panel DB.
//     Verification is verifyRootPassword (yescrypt) and changes go through
//     chpasswd. This path was DELIBERATELY LEFT UNTOUCHED when moving to
//     multi-user support: it is the only way to keep the risk of locking
//     yourself out of the panel at zero.
//
//   - reseller / customer accounts: their passwords are stored in
//     users.password_hash with bcrypt. These accounts have no matching Unix
//     user on the system; they only open a panel session.
//
// IsRootUser is the single decision point for this split.

// bcryptCost is 12, the same level as the existing root hash ($2a$12$...).
const bcryptCost = 12

// PasswordMinLength is the minimum length for new passwords. Kept identical to
// the 8-character rule already enforced by the ChangePassword endpoint.
const PasswordMinLength = 8

// ErrPasswordTooShort is returned when a new password is under the minimum.
var ErrPasswordTooShort = errors.New("password must be at least 8 characters")

// IsRootUser reports whether this username is the system root account. When
// true the password is read from / written to /etc/shadow, not the users table.
func IsRootUser(username string) bool {
	return strings.EqualFold(strings.TrimSpace(username), "root")
}

// HashPassword bcrypt-hashes a panel account password.
func HashPassword(password string) (string, error) {
	if len(password) < PasswordMinLength {
		return "", ErrPasswordTooShort
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// PasswordMatches compares a users.password_hash against the given password.
//
// bcrypt.CompareHashAndPassword is already constant-time. An empty hash (an
// account whose password was never set) always returns false, otherwise login
// with an empty password would be possible.
func PasswordMatches(hash, password string) bool {
	if strings.TrimSpace(hash) == "" || password == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
