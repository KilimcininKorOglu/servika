package mail

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

// Sender allow and block lists.
//
// Rspamd scores every message and a score is the right answer most of the time.
// What it cannot do is take an instruction, so a supplier whose mail keeps
// landing in spam, or a sender an operator has decided never to accept again,
// had no expression in the panel. These lists are that instruction, applied as
// Rspamd multimaps that add a large negative or positive score.

// FilterEntry is one list row.
type FilterEntry struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`       // allow | block
	MatchType string `json:"match_type"` // address | domain | ip
	Value     string `json:"value"`
	Note      string `json:"note,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

const (
	filterMapDir  = "/etc/rspamd/local.d/servika-maps"
	filterConfig  = "/etc/rspamd/local.d/multimap.conf"
	maxFilterNote = 255
	// maxFilterEntries bounds one list. Rspamd reads these maps into memory and
	// the panel renders them in one page; past this the answer is a different
	// tool, not a longer list.
	maxFilterEntries = 5000
)

var (
	filterApplyMu sync.Mutex
	// Deliberately stricter than the RFCs: everything here is typed into a form
	// by an operator and then written into a configuration file that a daemon
	// parses, so the useful question is "is this the ordinary shape", not "could
	// this conceivably be legal".
	filterAddressPattern = regexp.MustCompile(`^[a-z0-9._%+-]{1,64}@[a-z0-9.-]{1,253}\.[a-z]{2,}$`)
	filterDomainPattern  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
)

// FilterListGet returns every entry. GET /admin/mail/filters
func (h *Handlers) FilterListGet(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, kind, match_type, value, note, DATE_FORMAT(created_at,'%Y-%m-%d %H:%i')
		   FROM mail_filter_list ORDER BY kind, match_type, value`)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not read the filter lists")
		return
	}
	defer func() { _ = rows.Close() }()
	out := make([]FilterEntry, 0)
	for rows.Next() {
		var entry FilterEntry
		if err := rows.Scan(&entry.ID, &entry.Kind, &entry.MatchType, &entry.Value,
			&entry.Note, &entry.CreatedAt); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "could not read the filter lists")
			return
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not read the filter lists")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// FilterListCreate adds an entry and applies the lists. POST /admin/mail/filters
func (h *Handlers) FilterListCreate(w http.ResponseWriter, r *http.Request) {
	var req FilterEntry
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := normalizeFilterEntry(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	var count int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM mail_filter_list`).Scan(&count); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not read the filter lists")
		return
	}
	if count >= maxFilterEntries {
		httpx.WriteError(w, http.StatusBadRequest,
			"the filter lists already hold "+strconv.Itoa(maxFilterEntries)+" entries")
		return
	}

	if _, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO mail_filter_list(kind, match_type, value, note) VALUES(?,?,?,?)`,
		req.Kind, req.MatchType, req.Value, req.Note); err != nil {
		httpx.WriteError(w, http.StatusConflict, "that entry is already in the list")
		return
	}
	if err := ApplyFilterLists(r.Context(), h.DB); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "rspamd rejected the lists and they were rolled back")
		return
	}
	h.audit(r, "mail.filter_list.create", req.Kind+":"+req.Value, true)
	httpx.WriteJSON(w, http.StatusCreated, req)
}

// FilterListDelete removes one entry. DELETE /admin/mail/filters/{fid}
func (h *Handlers) FilterListDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "fid"), 10, 64)
	result, err := h.DB.ExecContext(r.Context(), `DELETE FROM mail_filter_list WHERE id=?`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not delete the entry")
		return
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		httpx.WriteError(w, http.StatusNotFound, "entry not found")
		return
	}
	if err := ApplyFilterLists(r.Context(), h.DB); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "rspamd rejected the lists and they were rolled back")
		return
	}
	h.audit(r, "mail.filter_list.delete", strconv.FormatInt(id, 10), true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// normalizeFilterEntry lower-cases, bounds and checks one entry.
//
// The value ends up on a line of a file a daemon parses, so anything that is not
// the ordinary shape of an address, a domain or an address range is refused
// outright rather than escaped. There is no escaping in a multimap line: a
// value with a space in it simply means something else.
func normalizeFilterEntry(entry *FilterEntry) error {
	entry.Kind = strings.ToLower(strings.TrimSpace(entry.Kind))
	if entry.Kind != "allow" && entry.Kind != "block" {
		return fmt.Errorf("kind must be allow or block")
	}
	entry.MatchType = strings.ToLower(strings.TrimSpace(entry.MatchType))
	entry.Value = strings.ToLower(strings.TrimSpace(entry.Value))
	entry.Note = sanitize(entry.Note, maxFilterNote)

	switch entry.MatchType {
	case "address":
		if !filterAddressPattern.MatchString(entry.Value) {
			return fmt.Errorf("%q is not a valid email address", entry.Value)
		}
	case "domain":
		if !filterDomainPattern.MatchString(entry.Value) {
			return fmt.Errorf("%q is not a valid domain", entry.Value)
		}
	case "ip":
		if err := normalizeFilterIP(entry); err != nil {
			return err
		}
	default:
		return fmt.Errorf("match type must be address, domain or ip")
	}
	return nil
}

// normalizeFilterIP accepts a single address or a CIDR range and rewrites it in
// its canonical form, so the same range typed two ways is one entry.
func normalizeFilterIP(entry *FilterEntry) error {
	if strings.Contains(entry.Value, "/") {
		_, network, err := net.ParseCIDR(entry.Value)
		if err != nil {
			return fmt.Errorf("%q is not a valid address range", entry.Value)
		}
		entry.Value = network.String()
		return nil
	}
	ip := net.ParseIP(entry.Value)
	if ip == nil {
		return fmt.Errorf("%q is not a valid address", entry.Value)
	}
	entry.Value = ip.String()
	return nil
}

// ApplyFilterLists writes the maps and the multimap configuration, then asks
// Rspamd to accept them.
func ApplyFilterLists(ctx context.Context, db *sql.DB) error {
	filterApplyMu.Lock()
	defer filterApplyMu.Unlock()
	if _, err := exec.LookPath("rspamadm"); err != nil {
		return fmt.Errorf("rspamadm is not installed")
	}

	entries, err := readFilterEntries(ctx, db)
	if err != nil {
		return err
	}
	// #nosec G301 -- root-owned system directory the rspamd daemon must traverse; contains no secret material.
	if err := os.MkdirAll(filterMapDir, 0o755); err != nil {
		return err
	}
	for _, group := range filterGroups() {
		body := renderFilterMap(entries, group.kind, group.matchType)
		// #nosec G306 G703 -- fixed system path composed from constants; the rspamd daemon must read it and it holds no secret.
		if err := os.WriteFile(group.path(), body, 0o644); err != nil {
			return err
		}
	}
	return writeRspamdMultimap(renderMultimapConfig())
}

type filterGroup struct {
	kind      string
	matchType string
}

func (g filterGroup) path() string {
	return filterMapDir + "/" + g.kind + "_" + g.matchType + ".map"
}

func (g filterGroup) symbol() string {
	return "SERVIKA_" + strings.ToUpper(g.kind) + "_" + strings.ToUpper(g.matchType)
}

func filterGroups() []filterGroup {
	var groups []filterGroup
	for _, kind := range []string{"allow", "block"} {
		for _, matchType := range []string{"address", "domain", "ip"} {
			groups = append(groups, filterGroup{kind: kind, matchType: matchType})
		}
	}
	return groups
}

func readFilterEntries(ctx context.Context, db *sql.DB) ([]FilterEntry, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT kind, match_type, value FROM mail_filter_list`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var entries []FilterEntry
	for rows.Next() {
		var entry FilterEntry
		if err := rows.Scan(&entry.Kind, &entry.MatchType, &entry.Value); err != nil {
			return nil, err
		}
		// Re-checked on the way out as well as on the way in. The rows may predate
		// a tightening of the rules, or have been edited in the database directly,
		// and a bad line here is a configuration a daemon refuses to start with.
		if err := normalizeFilterEntry(&entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// renderFilterMap produces one map file. The order is sorted so an unchanged
// list produces an unchanged file, and a reload is not triggered by row order.
func renderFilterMap(entries []FilterEntry, kind, matchType string) []byte {
	var values []string
	for _, entry := range entries {
		if entry.Kind == kind && entry.MatchType == matchType {
			values = append(values, entry.Value)
		}
	}
	sort.Strings(values)
	var out bytes.Buffer
	out.WriteString("# Generated by Servika; edit from the panel.\n")
	for _, value := range values {
		out.WriteString(value)
		out.WriteByte('\n')
	}
	return out.Bytes()
}

// renderMultimapConfig wires each map to a symbol and a score.
//
// The rule shapes come from the Rspamd multimap module: type "from" matches the
// sender, the same type with filter "email:domain" matches only its domain part,
// and type "ip" matches the connecting address.
//
// The scores are deliberately far outside the reject threshold in both
// directions. An operator adding an entry here has decided the question, and a
// nudge the rest of the scoring could still outvote would not be that decision.
func renderMultimapConfig() []byte {
	var out bytes.Buffer
	out.WriteString("# Generated by Servika; edit from the panel.\n")
	for _, group := range filterGroups() {
		score := "-20.0"
		if group.kind == "block" {
			score = "20.0"
		}
		ruleType := "from"
		filter := ""
		switch group.matchType {
		case "domain":
			filter = "\n  filter = \"email:domain\";"
		case "ip":
			ruleType = "ip"
		}
		fmt.Fprintf(&out, `
%s {
  type = "%s";%s
  map = "%s";
  symbol = "%s";
  score = %s;
  description = "Servika %s list (%s)";
}
`, group.symbol(), ruleType, filter, group.path(), group.symbol(), score, group.kind, group.matchType)
	}
	return out.Bytes()
}

// writeRspamdMultimap installs the configuration and rolls back when Rspamd
// refuses it, for the same reason the settings file does: a configuration the
// daemon will not start with stops scanning for every domain on the server.
func writeRspamdMultimap(content []byte) error {
	tmp := filterConfig + ".new"
	// #nosec G306 G703 -- fixed system path built from constants; the rspamd daemon must read it and it holds no secret.
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	// #nosec G304 -- fixed system path built from constants, not from any request value.
	previous, previousErr := os.ReadFile(filterConfig)
	if err := os.Rename(tmp, filterConfig); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	rollback := func() {
		if previousErr == nil {
			// #nosec G306 G703 -- restoring the file this function just replaced, at the same fixed system path.
			_ = os.WriteFile(filterConfig, previous, 0o644)
			return
		}
		_ = os.Remove(filterConfig)
	}
	if output, err := exec.Command("rspamadm", "configtest").CombinedOutput(); err != nil {
		rollback()
		return fmt.Errorf("configtest: %s", strings.TrimSpace(string(output)))
	}
	if output, err := exec.Command("systemctl", "reload", "rspamd").CombinedOutput(); err != nil {
		rollback()
		// Reload again so the restored file is the one actually running; without
		// this the daemon keeps serving the configuration that was rejected.
		_, _ = exec.Command("systemctl", "reload", "rspamd").CombinedOutput()
		return fmt.Errorf("reload: %s", strings.TrimSpace(string(output)))
	}
	return nil
}
