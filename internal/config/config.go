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
	JWTLifetime int // Lifetime in seconds.
	Env         string
}

// Load reads and validates runtime configuration from environment variables.
func Load() (*Config, error) {
	c := &Config{
		ListenAddr:  envOr("SERVIKA_LISTEN", ":8080"),
		DBDsn:       envOr("SERVIKA_DB_DSN", "panel:panelpw@unix(/var/lib/mysql/mysql.sock)/panel?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci"),
		Env:         envOr("SERVIKA_ENV", "production"),
		JWTLifetime: envInt("SERVIKA_JWT_LIFETIME_SEC", 8*3600),
	}
	secret := strings.TrimSpace(os.Getenv("SERVIKA_JWT_SECRET"))
	if len(secret) < 32 {
		return nil, fmt.Errorf("SERVIKA_JWT_SECRET must be at least 32 characters (current: %d)", len(secret))
	}
	if err := ValidateRuntimePaths(); err != nil {
		return nil, err
	}
	c.JWTSecret = []byte(secret)
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
