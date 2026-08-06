package provisioner

import (
	"database/sql"
	"log"
	"os"
	"strings"
)

// webmailMarker identifies a vhost that already serves webmail. A test keeps it
// in step with the block itself.
const webmailMarker = "location ^~ /webmail/"

// normalVhostMarker identifies a vhost rendered from the ordinary template. The
// suspended, redirect-only and operator-written shapes do not carry the deny
// blocks, and none of them can hold the webmail block either, so a vhost without
// this marker must not be re-rendered: it would never gain the block and the
// repair would reload nginx for nothing on every boot.
const normalVhostMarker = "# ---- Deny CGI and interpreter scripts ----"

// healWebmailVhostsOnStartup writes the `/webmail/` block into the vhosts of
// domains that already existed before webmail moved onto the customer's own
// name.
//
// The block is computed on every render (see webmailBlock), but existing vhost
// files are not rewritten on their own: without this pass a customer would have
// to wait for the next certificate renewal or PHP version change before webmail
// answered on their own domain.
//
// Only vhosts that would actually gain the block are re-rendered, so the pass
// costs one small file read per domain when there is nothing to do. The ordinary
// render path is used, which keeps its nginx validation and rollback.
func healWebmailVhostsOnStartup() {
	if packageDB == nil || !webmailInstalled() {
		return // no Roundcube on this host, so there is nothing to point at
	}
	rows, err := packageDB.Query(
		`SELECT id, system_user, domain_name, parent_domain_id FROM domains ORDER BY id`)
	if err != nil {
		log.Printf("webmail vhost repair: could not list domains: %v", err)
		return
	}
	type domain struct {
		id         int64
		systemUser string
		domainName string
		parentID   sql.NullInt64
	}
	var domains []domain
	for rows.Next() {
		var item domain
		if err := rows.Scan(&item.id, &item.systemUser, &item.domainName, &item.parentID); err != nil {
			log.Printf("webmail vhost repair: could not read domain row: %v", err)
			continue
		}
		domains = append(domains, item)
	}
	rowsErr := rows.Err()
	_ = rows.Close()
	if rowsErr != nil {
		log.Printf("webmail vhost repair: domain iteration failed: %v", rowsErr)
		return
	}

	updated, failed := 0, 0
	for _, item := range domains {
		configPath := "/etc/nginx/conf.d/dom_" + item.systemUser + ".conf"
		if item.parentID.Valid {
			configPath = addonVhostConfigPath(item.systemUser, item.domainName)
		}
		// #nosec G304 -- path is a fixed system/config path, a server-internal temp/archive path, or built from a validated identifier; tenant file reads go through safeio (openat2), not this call.
		content, err := os.ReadFile(configPath)
		if err != nil {
			continue // no vhost yet; the first render will carry the block
		}
		body := string(content)
		if strings.Contains(body, webmailMarker) {
			continue // already serving webmail
		}
		// Webmail is only rendered onto the TLS vhost, and only into the ordinary
		// shape. Anything else can never gain the block.
		if !strings.Contains(body, "listen 443 ssl") || !strings.Contains(body, normalVhostMarker) {
			continue
		}
		if err := RerenderVhost(packageDB, item.id); err != nil {
			log.Printf("webmail vhost repair: %s vhost update failed: %v", item.domainName, err)
			failed++
			continue
		}
		updated++
	}
	if updated > 0 {
		log.Printf("webmail vhost repair: /webmail/ added to %d domain vhosts", updated)
	}
	if failed > 0 {
		log.Printf("webmail vhost repair: %d of %d domains failed, retry scheduled for next startup", failed, len(domains))
	}
}
