package domains

import (
	"context"
	"database/sql"
)

// SeedIfEmpty: used to add 4 demo domains to an empty table.
// Now a NO-OP: a fresh install ships with ZERO domains (no demo site).
// This also closes the bug where demos regenerated when the table emptied after deleting domains.
func SeedIfEmpty(ctx context.Context, db *sql.DB, ipv4 string) error {
	_ = ctx
	_ = db
	_ = ipv4
	return nil
}
