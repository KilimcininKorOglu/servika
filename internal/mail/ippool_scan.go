package mail

import (
	"context"
	"database/sql"
	"log"
	"net"
	"strings"
	"time"
)

// Blocklist scanning for the outbound addresses.
//
// This runs in the background rather than when a screen is opened. A blocklist
// answers over DNS from someone else's infrastructure, and a panel request that
// waited on eight of them would hang for as long as the slowest one; the answer
// also changes on the scale of hours, so measuring it per request buys nothing.

const (
	// dnsblQueryTimeout bounds one blocklist query. A listed address answers at
	// once; an unreachable zone answers never.
	dnsblQueryTimeout = 4 * time.Second
	// dnsblScanInterval is how often the pool is rescanned. A delisting takes
	// hours to propagate, so scanning faster only adds queries.
	dnsblScanInterval = time.Hour
)

// StartPoolScanner rescans the pool on a timer.
func StartPoolScanner(db *sql.DB) {
	go func() {
		// The pool is usually empty on a fresh install and a scan then does
		// nothing, so there is no reason to compete with the rest of the boot.
		time.Sleep(2 * time.Minute)
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			if err := ScanPool(ctx, db); err != nil {
				log.Printf("mail address pool scan: %v", err)
			}
			cancel()
			time.Sleep(dnsblScanInterval)
		}
	}()
}

// ScanPool checks every pool address against the configured blocklists and
// refreshes its reverse DNS.
//
// The zones are the ones already configured for rejecting inbound mail. Using a
// different list would mean an operator's own sending addresses were judged by
// blocklists they had not chosen to trust.
func ScanPool(ctx context.Context, db *sql.DB) error {
	settings, err := ReadServerSettings(ctx, db)
	if err != nil {
		return err
	}
	zones := strings.Fields(settings.DNSBLZones)

	rows, err := db.QueryContext(ctx, `SELECT id, ip FROM mail_ip_pool ORDER BY id`)
	if err != nil {
		return err
	}
	type poolRow struct {
		id int64
		ip string
	}
	var pool []poolRow
	for rows.Next() {
		var row poolRow
		if err := rows.Scan(&row.id, &row.ip); err != nil {
			_ = rows.Close()
			return err
		}
		pool = append(pool, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, row := range pool {
		ptrName, ptrOK := lookupPTR(ctx, row.ip)
		hits := dnsblHits(ctx, row.ip, zones)
		if _, err := db.ExecContext(ctx,
			`UPDATE mail_ip_pool
			    SET ptr_name=?, ptr_ok=?, dnsbl_listed=?, dnsbl_zones=?, last_scan_at=NOW()
			  WHERE id=?`,
			ptrName, boolToInt(ptrOK), boolToInt(len(hits) > 0), strings.Join(hits, " "), row.id); err != nil {
			// One address failing must not stop the rest: the others still need
			// their state refreshed, and the failure is visible in the log.
			log.Printf("mail address pool scan for %d: %v", row.id, err)
		}
	}
	return nil
}

// dnsblHits returns the zones that list the address.
//
// A blocklist answers by resolving the reversed address under its zone, so a
// name that resolves means listed and one that does not means clean. A lookup
// that fails for any other reason is NOT counted as a hit: reporting an
// unreachable blocklist as a listing would have an operator chasing a delisting
// that was never needed.
func dnsblHits(ctx context.Context, value string, zones []string) []string {
	if len(zones) == 0 {
		return nil
	}
	reversed := reverseIPv4(value)
	if reversed == "" {
		return nil // only IPv4 blocklists are queried this way
	}
	resolver := poolResolver()
	var hits []string
	for _, zone := range zones {
		query, cancel := context.WithTimeout(ctx, dnsblQueryTimeout)
		addresses, err := resolver.LookupHost(query, reversed+"."+zone)
		cancel()
		if err == nil && len(addresses) > 0 {
			hits = append(hits, zone)
		}
	}
	return hits
}

// reverseIPv4 turns 203.0.113.5 into 5.113.0.203, which is how a blocklist zone
// is queried. Anything that is not IPv4 returns empty.
func reverseIPv4(value string) string {
	ip := net.ParseIP(value)
	if ip == nil {
		return ""
	}
	four := ip.To4()
	if four == nil {
		return ""
	}
	parts := strings.Split(four.String(), ".")
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, ".")
}
