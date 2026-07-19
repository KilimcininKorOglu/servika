// Package firewall provides a panel-managed nftables firewall (inet servika_fw).
//
// Security design:
//   - The table policy is ACCEPT, so no traffic is blocked when no rule exists.
//   - "ct state established,related accept" comes first, so active sessions, including SSH,
//     are never interrupted. Rules affect only new connections and avoid immediate lockout.
//   - Critical ports for SSH, web, panel, and DNS cannot be closed.
//   - "nft -c" validates rules before applying them, following the nginx -t pattern.
//   - Rules persist in /etc/nftables/servika_fw.nft and Reapply runs at panel startup.
package firewall

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

const (
	tableName = "servika_fw"
	rulesFile = "/etc/nftables/servika_fw.nft"
)

// Protected ports cannot be closed without disrupting the server, panel, or hosted sites.
var protectedPorts = map[int]bool{
	22:   true, // SSH management access.
	80:   true, // Customer sites over HTTP.
	443:  true, // Customer sites over HTTPS.
	8080: true, // Panel API.
	8443: true, // Panel UI.
	53:   true, // DNS through named.
}

// firewallTemplates contains ready-to-use rule sets that close commonly exposed ports.
// Templates must never include critical ports from protectedPorts.
type templateRule struct {
	Type, Protocol, Description string
	Port                        int
}

var firewallTemplates = map[string][]templateRule{
	"close_mysql": {
		{"close", "tcp", "Template: MySQL closed to external access", 3306},
	},
	"close_ftp": {
		{"close", "tcp", "Template: FTP closed to external access", 21},
	},
	"close_mail": {
		{"close", "tcp", "Template: SMTP closed", 25},
		{"close", "tcp", "Template: SMTPS closed", 465},
		{"close", "tcp", "Template: Submission closed", 587},
		{"close", "tcp", "Template: POP3 closed", 110},
		{"close", "tcp", "Template: IMAP closed", 143},
	},
	"close_rpc": {
		{"close", "tcp", "Template: rpcbind closed", 111},
		{"close", "tcp", "Template: NFS closed", 2049},
	},
}

// Handlers provides HTTP handlers for firewall rule operations.
type Handlers struct{ DB *sql.DB }

// Rule describes a persisted firewall rule.
type Rule struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	IP          string `json:"ip"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at"`
}

// GET /firewall returns rules and protected ports.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, type, ip, port, protocol, description, enabled, DATE_FORMAT(created_at,'%Y-%m-%d %H:%i')
		 FROM firewall_rules ORDER BY id DESC`)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "rules could not be listed")
		return
	}
	defer func() { _ = rows.Close() }()
	out := []Rule{}
	for rows.Next() {
		var rule Rule
		var enabled int
		if err := rows.Scan(&rule.ID, &rule.Type, &rule.IP, &rule.Port, &rule.Protocol, &rule.Description, &enabled, &rule.CreatedAt); err == nil {
			rule.Enabled = enabled == 1
			out = append(out, rule)
		}
	}
	_ = rows.Err()
	protected := make([]int, 0, len(protectedPorts))
	for p := range protectedPorts {
		protected = append(protected, p)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"rules": out, "protected_ports": protected})
}

// POST /firewall  {type, ip, port, protocol, description}
func (h *Handlers) Add(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type        string `json:"type"`
		IP          string `json:"ip"`
		Port        int    `json:"port"`
		Protocol    string `json:"protocol"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	req.IP = strings.TrimSpace(req.IP)
	req.Protocol = strings.ToLower(strings.TrimSpace(req.Protocol))
	if req.Protocol == "" {
		req.Protocol = "tcp"
	}
	if req.Protocol != "tcp" && req.Protocol != "udp" {
		httpx.WriteError(w, http.StatusBadRequest, "protocol must be tcp or udp")
		return
	}
	if req.Port < 0 || req.Port > 65535 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid port (0-65535)")
		return
	}

	switch req.Type {
	case "banned", "whitelist":
		if !validIP(req.IP) {
			httpx.WriteError(w, http.StatusBadRequest, "enter a valid IP address or CIDR (for example, 1.2.3.4 or 1.2.3.0/24)")
			return
		}
	case "close":
		if req.Port == 0 {
			httpx.WriteError(w, http.StatusBadRequest, "specify a port to close")
			return
		}
		if protectedPorts[req.Port] {
			httpx.WriteError(w, http.StatusBadRequest,
				fmt.Sprintf("port %d is critical for SSH, web, panel, or DNS access and cannot be closed", req.Port))
			return
		}
		req.IP = "" // Closing a port blocks everyone.
	default:
		httpx.WriteError(w, http.StatusBadRequest, "type must be banned, whitelist, or closed")
		return
	}
	// A banned may block one IP from a critical port to stop an attacker. Active sessions
	// remain protected by established-accept. Combining all ports with a critical management IP
	// is risky, so the UI warns. A port-zero banned does not override an earlier whitelist rule.

	res, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO firewall_rules (type, ip, port, protocol, description, enabled) VALUES (?,?,?,?,?,1)`,
		req.Type, req.IP, req.Port, req.Protocol, req.Description)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "rule could not be added")
		return
	}
	nid, _ := res.LastInsertId()

	if err := h.rebuild(); err != nil {
		// Roll back the record when applying the rules fails.
		_, _ = h.DB.Exec(`DELETE FROM firewall_rules WHERE id=?`, nid)
		_ = h.rebuild()
		httpx.WriteError(w, http.StatusInternalServerError, "firewall rules could not be applied")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": nid})
}

// POST /firewall/template applies a predefined rule set idempotently.
func (h *Handlers) Template(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Template string `json:"template"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rules, ok := firewallTemplates[req.Template]
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "unknown template")
		return
	}
	added := 0
	for _, rule := range rules {
		if protectedPorts[rule.Port] { // Skip critical ports even though templates must not contain them.
			continue
		}
		var count int
		_ = h.DB.QueryRow(`SELECT COUNT(*) FROM firewall_rules WHERE type=? AND port=? AND protocol=? AND ip=''`,
			rule.Type, rule.Port, rule.Protocol).Scan(&count)
		if count > 0 { // Skip existing rules to preserve idempotency.
			continue
		}
		if _, err := h.DB.ExecContext(r.Context(),
			`INSERT INTO firewall_rules (type, ip, port, protocol, description, enabled) VALUES (?,'',?,?,?,1)`,
			rule.Type, rule.Port, rule.Protocol, rule.Description); err == nil {
			added++
		}
	}
	if err := h.rebuild(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "firewall rules could not be applied")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "added": added})
}

// DELETE /firewall/{id}
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM firewall_rules WHERE id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "rule could not be deleted")
		return
	}
	if err := h.rebuild(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "firewall rules could not be updated")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /firewall/{id}/status updates the active state.
func (h *Handlers) Status(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req struct {
		Enabled bool `json:"enabled"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	enabled := 0
	if req.Enabled {
		enabled = 1
	}
	if _, err := h.DB.ExecContext(r.Context(), `UPDATE firewall_rules SET enabled=? WHERE id=?`, enabled, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "rule could not be updated")
		return
	}
	if err := h.rebuild(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "firewall rules could not be updated")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// rebuild generates an nft ruleset from active rules, validates it, applies it, and persists it.
func (h *Handlers) rebuild() error {
	rows, err := h.DB.Query(`SELECT type, ip, port, protocol FROM firewall_rules WHERE enabled=1 ORDER BY
		FIELD(type,'whitelist','close','banned'), id`)
	if err != nil {
		return err
	}
	var allowlisted, closed, banned []string
	// A port-specific whitelist rule puts that port into allowlist mode.
	// A "proto dport P drop" rule follows the corresponding "ip saddr X dport P accept" rules,
	// allowing only listed IP addresses to access that port.
	// Sort "proto/port" keys for deterministic output.
	restrictedPorts := map[string]bool{}
	for rows.Next() {
		var ruleType, ip, proto string
		var port int
		if err := rows.Scan(&ruleType, &ip, &port, &proto); err != nil {
			continue
		}
		switch ruleType {
		case "whitelist":
			allowlisted = append(allowlisted, "\t\t"+saddr(ip)+dport(proto, port)+"accept")
			if port > 0 { // A port-specific permission enables allowlist mode for that port.
				restrictedPorts[proto+"/"+strconv.Itoa(port)] = true
			}
		case "close":
			closed = append(closed, "\t\t"+proto+" dport "+strconv.Itoa(port)+" drop")
		case "banned":
			banned = append(banned, "\t\t"+saddr(ip)+dport(proto, port)+"drop")
		}
	}
	_ = rows.Err()
	_ = rows.Close()

	// Allowlist drops come after permitted IP accepts and before close or banned rules.
	// Since established,related and lo come first, active sessions and SSH are not interrupted.
	restrictedKeys := make([]string, 0, len(restrictedPorts))
	for key := range restrictedPorts {
		restrictedKeys = append(restrictedKeys, key)
	}
	sort.Strings(restrictedKeys)
	var restricted []string
	for _, key := range restrictedKeys {
		if i := strings.IndexByte(key, '/'); i > 0 {
			restricted = append(restricted, "\t\t"+key[:i]+" dport "+key[i+1:]+" drop")
		}
	}

	var b bytes.Buffer
	// Replace atomically and idempotently by ensuring, deleting, and rebuilding the table.
	fmt.Fprintf(&b, "table inet %s {}\n", tableName)
	fmt.Fprintf(&b, "delete table inet %s\n", tableName)
	fmt.Fprintf(&b, "table inet %s {\n", tableName)
	b.WriteString("\tchain input {\n")
	b.WriteString("\t\ttype filter hook input priority filter; policy accept;\n")
	b.WriteString("\t\tct state established,related accept\n")
	b.WriteString("\t\tiif \"lo\" accept\n")
	// Ordering matters: whitelist accepts, allowlist drops, closed-port drops, then banned drops.
	for _, r := range allowlisted {
		fmt.Fprintf(&b, "%s\n", r)
	}
	for _, r := range restricted {
		fmt.Fprintf(&b, "%s\n", r)
	}
	for _, r := range closed {
		fmt.Fprintf(&b, "%s\n", r)
	}
	for _, r := range banned {
		fmt.Fprintf(&b, "%s\n", r)
	}
	b.WriteString("\t}\n}\n")

	ruleset := b.Bytes()
	// 1. Validate so an invalid ruleset is never applied.
	if out, err := nftCheck(ruleset); err != nil {
		return fmt.Errorf("nft validation failed: %s", strings.TrimSpace(out))
	}
	// 2. Apply the ruleset.
	if out, err := nftApply(ruleset); err != nil {
		return fmt.Errorf("nft apply failed: %s", strings.TrimSpace(out))
	}
	// 3. Persist the ruleset so panel startup can reload it after reboot.
	_ = os.MkdirAll("/etc/nftables", 0o755)
	_ = os.WriteFile(rulesFile, ruleset, 0o600)
	return nil
}

// Reapply restores rules from the database when the panel starts after a reboot.
func Reapply(db *sql.DB) error {
	h := &Handlers{DB: db}
	return h.rebuild()
}

// --- Helpers ---

func validIP(s string) bool {
	if s == "" {
		return false
	}
	if net.ParseIP(s) != nil {
		return true
	}
	_, _, err := net.ParseCIDR(s)
	return err == nil
}

// saddr returns an IPv4 or IPv6 source-address expression for an IP or CIDR.
func saddr(ip string) string {
	fam := "ip"
	host := ip
	if before, _, found := strings.Cut(ip, "/"); found {
		host = before
	}
	if p := net.ParseIP(host); p != nil && p.To4() == nil {
		fam = "ip6"
	}
	return fam + " saddr " + ip + " "
}

// dport returns a destination-port expression for a positive port, or empty for all ports.
func dport(proto string, port int) string {
	if port <= 0 {
		return ""
	}
	return proto + " dport " + strconv.Itoa(port) + " "
}

func nftCheck(ruleset []byte) (string, error) {
	cmd := exec.Command("nft", "-c", "-f", "-")
	cmd.Stdin = bytes.NewReader(ruleset)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func nftApply(ruleset []byte) (string, error) {
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = bytes.NewReader(ruleset)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
