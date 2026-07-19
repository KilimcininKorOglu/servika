//go:build ignore

// One-time administrator seeding:
//
//	go run scripts/seed_admin.go -dsn '...' -username admin -password '...'
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	dsn := flag.String("dsn", "", "MySQL DSN")
	user := flag.String("username", "admin", "administrator username")
	pass := flag.String("password", "", "administrator password (defaults to SERVIKA_SEED_PASSWORD)")
	email := flag.String("email", "admin@local", "email address")
	flag.Parse()

	if *pass == "" {
		*pass = os.Getenv("SERVIKA_SEED_PASSWORD")
	}
	if *dsn == "" || *pass == "" {
		log.Fatalf("DSN and password are required")
	}

	db, err := sql.Open("mysql", *dsn)
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping: %v", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(*pass), 12)
	if err != nil {
		log.Fatalf("bcrypt: %v", err)
	}

	res, err := db.Exec(
		`INSERT INTO users(username, email, password_hash, role, full_name, status)
		 VALUES(?,?,?, 'admin', 'System Administrator', 'active')
		 ON DUPLICATE KEY UPDATE password_hash=VALUES(password_hash), role='admin', status='active'`,
		*user, *email, string(hash))
	if err != nil {
		log.Fatalf("insert: %v", err)
	}
	aff, _ := res.RowsAffected()
	fmt.Printf("administrator seeded (username=%s, rowsAffected=%d)\n", *user, aff)
}
