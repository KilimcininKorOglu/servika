// nameserver.go resolves the shared nameserver pair a zone publishes.
//
// What a hosting provider tells its customer is "point your domain at
// ns1.provider.example / ns2.provider.example". For that to work, the NS
// records inside the customer's zone must name those SHARED nameservers.
//
// The panel used to emit `ns1.<customer domain>` for every domain (vanity
// nameservers). That model requires a separate glue record at the registrar of
// every single domain and is not workable for shared hosting; see
// migrations/0072_shared_nameservers.sql.
//
// Resolution order, first non-empty wins:
//
//  1. the white-label pair of the RESELLER that owns the domain
//     (reseller_nameservers)
//  2. the panel-wide pair (panel_settings.ns1_hostname / ns2_hostname)
//  3. the legacy vanity pair (ns1.<domain>), kept only so that an installation
//     which has configured nothing still writes a zone that has NS records
//
// The pair is never derived from panel_settings.custom_domain. A panel served
// at cloud.provider.example would derive ns1.cloud.provider.example, which
// publishes a nameserver the provider does not own; every customer domain
// pointed at it would then fail to resolve, with nothing on screen to explain
// why. The value is set explicitly, and SuggestedNameservers only offers a
// guess for a human to confirm.
package dns

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
)

// nsHostPattern validates a nameserver hostname. It has to be strict because
// the value is written into a BIND zone file: whitespace, a newline or a
// leading `$` could break a zone line or inject a zone directive (the same
// concern as validRecordFields in dns.go). The final label must be alphabetic,
// which also rejects a bare IP address.
var nsHostPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*\.[a-z]{2,63}$`)

// ValidNSHost reports whether a value is usable as a nameserver hostname.
func ValidNSHost(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return len(value) <= 253 && nsHostPattern.MatchString(value)
}

// NameserverPair resolves the NS pair to publish in a domain's zone.
//
// It returns no error on purpose: zone generation must never write a zone
// without NS records, so every step falls back (see the package comment).
func NameserverPair(ctx context.Context, db *sql.DB, domainID int64, domainName string) (ns1, ns2 string) {
	if first, second, ok := resellerNS(ctx, db, domainID); ok {
		return first, second
	}
	if first, second, ok := panelNS(ctx, db); ok {
		return first, second
	}
	// Last resort: the old vanity behavior, so an unconfigured installation
	// still produces a zone with NS records. The panel warns about it.
	return "ns1." + domainName, "ns2." + domainName
}

// resellerNS returns the white-label pair of the reseller owning the domain.
//
// Ownership chain: domains.customer_id → customers.owner_user_id. A NULL
// owner_user_id means the domain belongs to the admin directly and there is no
// reseller override.
func resellerNS(ctx context.Context, db *sql.DB, domainID int64) (ns1, ns2 string, ok bool) {
	err := db.QueryRowContext(ctx, `
		SELECT rn.ns1, rn.ns2
		  FROM domains d
		  JOIN customers c              ON c.id = d.customer_id
		  JOIN reseller_nameservers rn  ON rn.user_id = c.owner_user_id
		 WHERE d.id = ?`, domainID).Scan(&ns1, &ns2)
	if err != nil || !ValidNSHost(ns1) || !ValidNSHost(ns2) {
		return "", "", false
	}
	return strings.ToLower(strings.TrimSpace(ns1)), strings.ToLower(strings.TrimSpace(ns2)), true
}

// panelNS returns the panel-wide pair, and only an explicitly configured one.
func panelNS(ctx context.Context, db *sql.DB) (ns1, ns2 string, ok bool) {
	var first, second sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT ns1_hostname, ns2_hostname FROM panel_settings WHERE id=1`).
		Scan(&first, &second); err != nil {
		return "", "", false
	}
	a := strings.ToLower(strings.TrimSpace(first.String))
	b := strings.ToLower(strings.TrimSpace(second.String))
	if ValidNSHost(a) && ValidNSHost(b) {
		return a, b, true
	}
	return "", "", false
}

// SuggestedNameservers returns a guess to show an administrator. It is never
// applied on its own.
//
// The panel domain is usually a subdomain (cloud.provider.example), so the
// brand domain is guessed by dropping the first label. When dropping it would
// leave a single label (provider.example → example) the guess is meaningless,
// so the domain is used unchanged. This is a GUESS: nothing is stored until an
// administrator confirms it.
func SuggestedNameservers(ctx context.Context, db *sql.DB) (ns1, ns2 string, ok bool) {
	var custom sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT custom_domain FROM panel_settings WHERE id=1`).Scan(&custom); err != nil {
		return "", "", false
	}
	base := strings.ToLower(strings.TrimSpace(custom.String))
	if !ValidNSHost(base) {
		return "", "", false
	}
	if parts := strings.SplitN(base, ".", 2); len(parts) == 2 && strings.Contains(parts[1], ".") {
		base = parts[1]
	}
	return "ns1." + base, "ns2." + base, true
}

// NameserversConfigured reports whether a real panel-wide pair exists. When it
// is false, zones fall back to the vanity pair and the panel must say so:
// those hostnames cannot be handed to a customer.
func NameserversConfigured(ctx context.Context, db *sql.DB) bool {
	_, _, ok := panelNS(ctx, db)
	return ok
}
