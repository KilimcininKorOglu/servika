// Package credentials manages FTP and MySQL database accounts.
package credentials

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// RandomPassword returns a URL-safe alphanumeric password, using 20 characters by default.
func RandomPassword(length int) string {
	if length <= 0 {
		length = 20
	}
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, length)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

// ValidPassword reports whether a password is safe for line-oriented system commands.
func ValidPassword(password string) bool {
	return !strings.ContainsAny(password, "\r\n\x00")
}

// FTPCreate inserts an FTP account and stores its password as cleartext for Pure-FTPd MYSQLCrypt.
func FTPCreate(db *sql.DB, domainID int64, systemUser, password string, uidN, gidN int) error {
	home := "/home/" + systemUser
	_, err := db.Exec(
		`INSERT INTO ftp_accounts(domain_id, username, password_md5, home_dir, uid_n, gid_n, status)
		 VALUES(?,?,?,?,?,?, 'active')
		 ON DUPLICATE KEY UPDATE password_md5=VALUES(password_md5), home_dir=VALUES(home_dir), uid_n=VALUES(uid_n), gid_n=VALUES(gid_n), status='active'`,
		domainID, systemUser, password, home, uidN, gidN)
	return err
}

// FTPUpdatePassword updates an existing FTP account password.
func FTPUpdatePassword(db *sql.DB, systemUser, password string) error {
	_, err := db.Exec(
		`UPDATE ftp_accounts SET password_md5=? WHERE username=?`,
		password, systemUser)
	return err
}

// FTPDelete explicitly removes an FTP account even though domain deletion cascades.
func FTPDelete(db *sql.DB, systemUser string) error {
	_, err := db.Exec(`DELETE FROM ftp_accounts WHERE username=?`, systemUser)
	return err
}

var (
	mysqlIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
	mysqlPasswordPattern   = regexp.MustCompile(`^[ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789]{1,255}$`)
	mysqlSuffixPattern     = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)
)

// ValidCustomerDBIdentifier reports whether a database identifier is safe and namespaced to a domain user.
func ValidCustomerDBIdentifier(systemUser, identifier string) bool {
	return mysqlIdentifierPattern.MatchString(systemUser) &&
		mysqlIdentifierPattern.MatchString(identifier) &&
		strings.HasPrefix(identifier, systemUser+"_")
}

// ValidDBSuffix reports whether a customer-provided database/user suffix is safe before the panel
// prepends the `<system_user>_` prefix. Only lowercase letters, digits, and underscore, 1-32 chars.
// The combined length (prefix + suffix) is additionally validated with mysqlIdentifierPattern.
func ValidDBSuffix(suffix string) bool {
	return mysqlSuffixPattern.MatchString(suffix)
}

// StrongPassword reports whether a customer-chosen database password is strong enough: at least
// 12 characters and a mix of letters and digits. The returned reason is English for API display.
func StrongPassword(password string) (bool, string) {
	if !ValidPassword(password) {
		return false, "password contains invalid characters (line breaks or control chars)"
	}
	if len([]rune(password)) < 12 {
		return false, "password must be at least 12 characters"
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return false, "password must contain both letters and digits"
	}
	return true, ""
}

func escapeSQLString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `'`, `\'`)
}

// ErrInvalidMySQLCredentials indicates that a database name, user, or password is unsafe for SQL construction.
var ErrInvalidMySQLCredentials = errors.New("invalid MySQL credentials")

func validateMySQLCredentials(dbName, dbUser, dbPass string) error {
	if !mysqlIdentifierPattern.MatchString(dbName) {
		return fmt.Errorf("%w: database name", ErrInvalidMySQLCredentials)
	}
	if !mysqlIdentifierPattern.MatchString(dbUser) {
		return fmt.Errorf("%w: database user", ErrInvalidMySQLCredentials)
	}
	if !mysqlPasswordPattern.MatchString(dbPass) {
		return fmt.Errorf("%w: database password", ErrInvalidMySQLCredentials)
	}
	return nil
}

// MySQLCreateDB creates a MariaDB database and user, grants access, and records the account.
func MySQLCreateDB(db *sql.DB, domainID int64, dbName, dbUser, dbPass string) error {
	if err := validateMySQLCredentials(dbName, dbUser, dbPass); err != nil {
		return err
	}
	// Create the MariaDB database and user through root socket authentication.
	stmts := []string{
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;", dbName),
		fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s';", dbUser, escapeSQLString(dbPass)),
		fmt.Sprintf("ALTER USER '%s'@'localhost' IDENTIFIED BY '%s';", dbUser, escapeSQLString(dbPass)),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost';", dbName, dbUser),
		"FLUSH PRIVILEGES;",
	}
	sql := strings.Join(stmts, " ")
	if out, err := exec.Command("mysql", "-e", sql).CombinedOutput(); err != nil {
		return fmt.Errorf("mysql exec: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Record the account in the panel database.
	_, err := db.Exec(
		`INSERT INTO db_accounts(domain_id, db_name, db_user, db_pass_plain, db_host)
		 VALUES(?,?,?,?, 'localhost')`,
		domainID, dbName, dbUser, dbPass)
	return err
}

// MySQLCreateDBForUser creates a database and grants access to an EXISTING database user without
// touching that user's password (so other databases sharing the user are not broken). A new
// db_accounts row is inserted for this domain+database using the existing user's stored password
// (needed for phpMyAdmin single sign-on). The caller MUST first verify that dbUser belongs to this
// domain (ownership + prefix check).
func MySQLCreateDBForUser(db *sql.DB, domainID int64, dbName, dbUser string) error {
	if !mysqlIdentifierPattern.MatchString(dbName) || !mysqlIdentifierPattern.MatchString(dbUser) {
		return fmt.Errorf("%w: database name or user", ErrInvalidMySQLCredentials)
	}
	var pass string
	if err := db.QueryRow(
		`SELECT db_pass_plain FROM db_accounts WHERE db_user=? LIMIT 1`, dbUser).Scan(&pass); err != nil {
		return fmt.Errorf("existing user password not found: %w", err)
	}
	// Create the database and grant the existing user access. No CREATE/ALTER USER statement, so
	// the user's password is preserved.
	stmts := []string{
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;", dbName),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost';", dbName, dbUser),
		"FLUSH PRIVILEGES;",
	}
	if out, err := exec.Command("mysql", "-e", strings.Join(stmts, " ")).CombinedOutput(); err != nil {
		return fmt.Errorf("mysql exec: %s: %w", strings.TrimSpace(string(out)), err)
	}
	_, err := db.Exec(
		`INSERT INTO db_accounts(domain_id, db_name, db_user, db_pass_plain, db_host)
		 VALUES(?,?,?,?, 'localhost')`,
		domainID, dbName, dbUser, pass)
	return err
}

// MySQLDropDB removes a database and user, then deletes the account metadata.
func MySQLDropDB(db *sql.DB, dbName, dbUser string) error {
	if !mysqlIdentifierPattern.MatchString(dbName) {
		return fmt.Errorf("%w: database name", ErrInvalidMySQLCredentials)
	}
	if !mysqlIdentifierPattern.MatchString(dbUser) {
		return fmt.Errorf("%w: database user", ErrInvalidMySQLCredentials)
	}
	stmts := []string{
		fmt.Sprintf("DROP DATABASE IF EXISTS `%s`;", dbName),
		fmt.Sprintf("DROP USER IF EXISTS '%s'@'localhost';", dbUser),
		"FLUSH PRIVILEGES;",
	}
	if out, err := exec.Command("mysql", "-e", strings.Join(stmts, " ")).CombinedOutput(); err != nil {
		return fmt.Errorf("mysql drop: %s: %w", strings.TrimSpace(string(out)), err)
	}
	_, err := db.Exec(`DELETE FROM db_accounts WHERE db_name=?`, dbName)
	return err
}

// MySQLDropDBKeepUser drops only the database and its metadata row, leaving the user intact. Use
// this for single-database deletion when the user is shared across other databases (existing-user
// mode), so the sharing databases keep their access.
func MySQLDropDBKeepUser(db *sql.DB, dbName string) error {
	if !mysqlIdentifierPattern.MatchString(dbName) {
		return fmt.Errorf("%w: database name", ErrInvalidMySQLCredentials)
	}
	if out, err := exec.Command("mysql", "-e",
		fmt.Sprintf("DROP DATABASE IF EXISTS `%s`;", dbName)).CombinedOutput(); err != nil {
		return fmt.Errorf("mysql drop: %s: %w", strings.TrimSpace(string(out)), err)
	}
	_, err := db.Exec(`DELETE FROM db_accounts WHERE db_name=?`, dbName)
	return err
}

// MySQLDropAllForDomain removes every database account belonging to a deleted domain.
func MySQLDropAllForDomain(db *sql.DB, domainID int64) error {
	rows, err := db.Query(`SELECT db_name, db_user FROM db_accounts WHERE domain_id=?`, domainID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var dbName, dbUser string
		if err := rows.Scan(&dbName, &dbUser); err != nil {
			continue
		}
		_ = MySQLDropDB(db, dbName, dbUser)
	}
	return nil
}

// SyncSSHPassword synchronizes the system account password with the FTP password.
// The FTP password is kept as cleartext in ftp_accounts.password_md5 for Pure-FTPd MYSQLCrypt.
func SyncSSHPassword(db *sql.DB, systemUser string) error {
	if !strings.HasPrefix(systemUser, "c_") {
		return fmt.Errorf("security: system user must have the c_ prefix")
	}
	var password string
	if err := db.QueryRow(
		`SELECT password_md5 FROM ftp_accounts WHERE username=? AND status='active'`,
		systemUser).Scan(&password); err != nil {
		return fmt.Errorf("read FTP password: %w", err)
	}
	if strings.TrimSpace(password) == "" {
		return fmt.Errorf("FTP password is empty")
	}
	if !ValidPassword(password) {
		return fmt.Errorf("security: FTP password contains invalid characters")
	}
	cmd := exec.Command("chpasswd")
	cmd.Stdin = strings.NewReader(systemUser + ":" + password)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("chpasswd: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// LockSSHPassword locks the system password when SSH is disabled.
func LockSSHPassword(systemUser string) error {
	if !strings.HasPrefix(systemUser, "c_") {
		return fmt.Errorf("security: system user must have the c_ prefix")
	}
	out, err := exec.Command("passwd", "-l", systemUser).CombinedOutput()
	if err != nil {
		return fmt.Errorf("passwd -l: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
