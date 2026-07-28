// Package secret provides authenticated at-rest encryption for sensitive
// values (remote backup credentials, tokens) stored in the panel database.
// The key is supplied from SERVIKA_SECRET_KEY, held outside the database, so a
// leaked database dump does not expose the plaintext secrets.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// prefix marks a value produced by Encrypt. A stored value without this prefix
// is treated as legacy plaintext by Decrypt, so records written before
// encryption was introduced keep working until their next write.
const prefix = "enc:v1:"

var (
	gcm            cipher.AEAD
	errNotInit     = errors.New("secret: not initialized")
	errShortCipher = errors.New("secret: ciphertext too short")
)

// Init derives a 256-bit AES-GCM key from the supplied secret and prepares the
// package for Encrypt/Decrypt. It must be called once at startup before any
// encryption or decryption. The key is hashed with SHA-256 so any key length
// accepted by config maps to a valid AES-256 key.
func Init(key []byte) error {
	if len(key) == 0 {
		return errors.New("secret: empty key")
	}
	sum := sha256.Sum256(key)
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return fmt.Errorf("secret: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("secret: new gcm: %w", err)
	}
	gcm = aead
	return nil
}

// IsEncrypted reports whether value was produced by Encrypt/EncryptWith (i.e.
// carries the encryption prefix). Callers use it to reject user-supplied input
// that already looks like ciphertext, which would otherwise turn a later reveal
// into a decryption oracle for another record's secret.
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, prefix)
}

// Encrypt seals plaintext with AES-256-GCM and returns a prefixed, base64
// value safe to store in a text column. A random nonce is prepended to the
// ciphertext so each call produces a distinct output.
func Encrypt(plaintext string) (string, error) {
	return EncryptWith(plaintext, "")
}

// EncryptWith is Encrypt with additional authenticated data (AAD) bound into
// the seal. The AAD is authenticated but not stored, so a ciphertext produced
// for one AAD (e.g. a specific database user) cannot be moved to another record
// and decrypted under a different AAD — Open fails. Pass "" for no binding
// (equivalent to Encrypt).
func EncryptWith(plaintext, aad string) (string, error) {
	if gcm == nil {
		return "", errNotInit
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("secret: nonce: %w", err)
	}
	var extra []byte
	if aad != "" {
		extra = []byte(aad)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), extra)
	return prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. A value without the encryption prefix is returned
// unchanged as legacy plaintext, so pre-encryption records remain readable.
func Decrypt(value string) (string, error) {
	return DecryptWith(value, "")
}

// DecryptWith reverses EncryptWith. The same AAD passed to EncryptWith must be
// supplied or Open fails. A value without the encryption prefix is returned
// unchanged as legacy plaintext (AAD ignored), so pre-encryption records remain
// readable.
func DecryptWith(value, aad string) (string, error) {
	if !strings.HasPrefix(value, prefix) {
		return value, nil // legacy plaintext written before encryption existed
	}
	if gcm == nil {
		return "", errNotInit
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil {
		return "", fmt.Errorf("secret: decode: %w", err)
	}
	if len(raw) < gcm.NonceSize() {
		return "", errShortCipher
	}
	var extra []byte
	if aad != "" {
		extra = []byte(aad)
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, extra)
	if err != nil {
		return "", fmt.Errorf("secret: open: %w", err)
	}
	return string(plaintext), nil
}
