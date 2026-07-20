//go:build tools

package tools

import (
	_ "golang.org/x/crypto/bcrypt" // build dependency for scripts/seed_admin.go
)
