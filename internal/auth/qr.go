// Package auth — 2FA QR code generation (pure Go, no cgo).
package auth

import (
	"encoding/base64"

	qrcode "github.com/skip2/go-qrcode"
)

// TOTPQRDataURI encodes an otpauth:// URI as a 256px QR PNG and returns a base64
// data-URI ("data:image/png;base64,..."). Pure Go (skip2/go-qrcode), no cgo required.
// Note: the URI includes the secret; this function does NOT log the secret.
func TOTPQRDataURI(uri string) (string, error) {
	png, err := qrcode.Encode(uri, qrcode.Medium, 256)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}
