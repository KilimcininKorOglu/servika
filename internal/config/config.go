package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config contains the server's environment-derived runtime configuration.
type Config struct {
	ListenAddr  string
	DBDsn       string
	JWTSecret   []byte
	SecretKey   []byte
	JWTLifetime int // Lifetime in seconds.
	Env         string
}

// Load reads and validates runtime configuration from environment variables.
func Load() (*Config, error) {
	// No built-in DSN default: a fallback credential is the same string in every
	// installation, so a server started without its environment file would come up
	// silently on a publicly known account instead of refusing to start.
	dsn := strings.TrimSpace(os.Getenv("SERVIKA_DB_DSN"))
	if dsn == "" {
		return nil, fmt.Errorf("SERVIKA_DB_DSN is required")
	}
	c := &Config{
		ListenAddr:  envOr("SERVIKA_LISTEN", ":8080"),
		DBDsn:       dsn,
		Env:         envOr("SERVIKA_ENV", "production"),
		JWTLifetime: envInt("SERVIKA_JWT_LIFETIME_SEC", 8*3600),
	}
	secret := strings.TrimSpace(os.Getenv("SERVIKA_JWT_SECRET"))
	if len(secret) < 32 {
		return nil, fmt.Errorf("SERVIKA_JWT_SECRET must be at least 32 characters (current: %d)", len(secret))
	}
	secretKey := strings.TrimSpace(os.Getenv("SERVIKA_SECRET_KEY"))
	if len(secretKey) < 32 {
		return nil, fmt.Errorf("SERVIKA_SECRET_KEY must be at least 32 characters (current: %d)", len(secretKey))
	}
	if err := ValidateRuntimePaths(); err != nil {
		return nil, err
	}
	c.JWTSecret = []byte(secret)
	c.SecretKey = []byte(secretKey)
	return c, nil
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
