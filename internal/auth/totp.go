package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// RFC 6238 TOTP (HMAC-SHA1, 6 digits, 30s period), no external dependency.

// TOTPGenerateSecret generates a 160-bit random base32 secret (no padding).
// Returns an error when the system RNG fails — swallowing the error and returning
// a predictable (all-zero) secret would completely defeat 2FA.
func TOTPGenerateSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.EncodeToString(b), "="), nil
}

func hotp(secret string, counter uint64) (string, bool) {
	s := strings.ToUpper(strings.TrimSpace(secret))
	if m := len(s) % 8; m != 0 {
		s += strings.Repeat("=", 8-m)
	}
	key, err := base32.StdEncoding.DecodeString(s)
	if err != nil || len(key) == 0 {
		return "", false
	}
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	val := (uint32(sum[off]&0x7f) << 24) |
		(uint32(sum[off+1]) << 16) |
		(uint32(sum[off+2]) << 8) |
		uint32(sum[off+3])
	return fmt.Sprintf("%06d", val%1000000), true
}

// validStep verifies the code in the +/-1 window with CONSTANT-TIME comparison
// and returns the accepted 30s time-step. Steps <= minStep are REJECTED
// (replay protection: prevents reuse of the same code). When ok is false the
// step is 0.
func validStep(secret, code string, minStep int64) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != 6 || secret == "" {
		return 0, false
	}
	t := time.Now().Unix() / 30
	for _, c := range []int64{t - 1, t, t + 1} {
		if c <= minStep {
			continue
		}
		if v, ok := hotp(secret, uint64(c)); ok && subtle.ConstantTimeCompare([]byte(v), []byte(code)) == 1 {
			return c, true
		}
	}
	return 0, false
}

// TOTPVerify performs backward-compatible, replay-unprotected verification
// (used for 2FA disable flow).
func TOTPVerify(secret, code string) bool {
	_, ok := validStep(secret, code, -1)
	return ok
}

// TOTPVerifyStep performs replay-protected verification for login/enable;
// accepts codes matching a step AFTER lastStep and returns the matched step
// (caller must persist it to the database).
func TOTPVerifyStep(secret, code string, lastStep int64) (int64, bool) {
	return validStep(secret, code, lastStep)
}

// TOTPURI builds the otpauth:// URI read by authenticator apps (for QR).
func TOTPURI(secret, account, issuer string) string {
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", "6")
	v.Set("period", "30")
	return fmt.Sprintf("otpauth://totp/%s:%s?%s",
		url.PathEscape(issuer), url.PathEscape(account), v.Encode())
}
