package credentials

import (
	"database/sql"
	"fmt"
	"os/exec"
	"strings"
)

// MySQLChangePassword changes a MariaDB user's password and updates the account metadata.
func MySQLChangePassword(panelDB *sql.DB, dbUser, newPassword string) error {
	if !strings.HasPrefix(dbUser, "c_") {
		return fmt.Errorf("security: database user must have the c_ prefix")
	}
	if !mysqlIdentifierPattern.MatchString(dbUser) {
		return fmt.Errorf("%w: database user", ErrInvalidMySQLCredentials)
	}
	if !ValidPassword(newPassword) {
		return fmt.Errorf("%w: database password", ErrInvalidMySQLCredentials)
	}
	if !mysqlPasswordPattern.MatchString(newPassword) {
		return fmt.Errorf("%w: database password", ErrInvalidMySQLCredentials)
	}
	statements := []string{
		fmt.Sprintf("ALTER USER '%s'@'localhost' IDENTIFIED BY '%s';", dbUser, escapeSQLString(newPassword)),
		"FLUSH PRIVILEGES;",
	}
	out, err := exec.Command("mysql", "-e", strings.Join(statements, " ")).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mysql alter: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if _, err := panelDB.Exec(
		`UPDATE db_accounts SET db_pass_plain=? WHERE db_user=?`,
		newPassword, dbUser); err != nil {
		return fmt.Errorf("metadata: %w", err)
	}
	return nil
}
