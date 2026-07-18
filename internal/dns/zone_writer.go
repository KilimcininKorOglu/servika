package dns

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

const (
	// ZoneDir is the directory containing generated BIND zone files.
	ZoneDir = "/var/named"
	// NamedConfInclude is the generated BIND zone include file.
	NamedConfInclude = "/etc/named/servika-zones.conf"
)

// fqdn ensures target domain names for NS, MX, CNAME, SRV, and PTR records end with a dot.
// This prevents BIND from interpreting them as relative names and appending the zone name.
func fqdn(rtype, value string) string {
	t := strings.ToUpper(strings.TrimSpace(rtype))
	d := strings.TrimSpace(value)
	if t == "NS" || t == "MX" || t == "CNAME" || t == "SRV" || t == "PTR" {
		if !strings.HasSuffix(d, ".") {
			d = d + "."
		}
	}
	return d
}

func txtQuote(value string) string {
	data := []byte(value)
	if len(data) == 0 {
		return `""`
	}
	segments := make([]string, 0, (len(data)+254)/255)
	for len(data) > 0 {
		length := 255
		if len(data) < length {
			length = len(data)
		}
		var escaped strings.Builder
		for _, b := range data[:length] {
			switch {
			case b == '\\' || b == '"':
				escaped.WriteByte('\\')
				escaped.WriteByte(b)
			case b < 32 || b == 127:
				fmt.Fprintf(&escaped, `\%03d`, b)
			default:
				escaped.WriteByte(b)
			}
		}
		segments = append(segments, `"`+escaped.String()+`"`)
		data = data[length:]
	}
	return strings.Join(segments, " ")
}

func rdata(recordType, value string) string {
	if strings.EqualFold(strings.TrimSpace(recordType), "TXT") {
		return txtQuote(value)
	}
	return fqdn(recordType, value)
}

var zoneTmpl = template.Must(template.New("z").Funcs(template.FuncMap{
	"rdata":   rdata,
	"soaHost": soaHost,
	"soaMail": soaMail,
}).Parse(`$TTL {{.SOA.TTL}}
@   IN  SOA {{soaHost .SOA.PrimaryNS}} {{soaMail .SOA.Hostmaster}} (
    {{.Serial}}  ; serial
    {{.SOA.Refresh}}  ; refresh
    {{.SOA.Retry}}  ; retry
    {{.SOA.Expire}}  ; expire
    {{.SOA.Minimum}}  ; minimum
)
{{range .Records}}{{.Name}}	{{.TTL}}	IN	{{.Type}}	{{if or (eq .Type "MX") (eq .Type "SRV")}}{{.Priority}} {{end}}{{rdata .Type .Value}}
{{end}}`))

func soaHost(primaryNS string) string {
	primaryNS = strings.TrimSpace(primaryNS)
	if primaryNS == "" {
		return "."
	}
	if !strings.HasSuffix(primaryNS, ".") {
		primaryNS += "."
	}
	return primaryNS
}

func soaMail(hostmaster string) string {
	hostmaster = strings.TrimSpace(hostmaster)
	if i := strings.Index(hostmaster, "@"); i >= 0 {
		hostmaster = hostmaster[:i] + "." + hostmaster[i+1:]
	}
	if hostmaster == "" {
		return "."
	}
	if !strings.HasSuffix(hostmaster, ".") {
		hostmaster += "."
	}
	return hostmaster
}

type zoneCtx struct {
	DomainName string
	Serial     string
	SOA        SOA
	Records    []Record
}

// WriteZone renders and validates the BIND zone file for a domain.
func WriteZone(ctx context.Context, db *sql.DB, domainID int64) error {
	var domainName string
	if err := db.QueryRowContext(ctx, `SELECT domain_name FROM domains WHERE id=?`, domainID).Scan(&domainName); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, domain_id, name, type, value, ttl, priority, enabled,
		   DATE_FORMAT(created_at,'%Y-%m-%d %H:%i') FROM dns_records
		 WHERE domain_id=? AND enabled=1 ORDER BY type, name`, domainID)
	if err != nil {
		return err
	}
	defer rows.Close()
	records := make([]Record, 0)
	for rows.Next() {
		record, err := scan(rows)
		if err == nil {
			records = append(records, record)
		}
	}
	if len(records) == 0 {
		return nil
	}

	// serial: yyyymmddHH + sec (second granularity, for the 10-digit max DNS standard)
	// Format: yyyymmddNN where NN is HH (00-23). On a rewrite within the same hour BIND may keep the old cache;
	// in that case named.run.log warns but it is rare in prod.
	serial := time.Now().UTC().Format("2006010215")

	soa := LoadSOA(ctx, db, domainID, domainName)
	var buf bytes.Buffer
	if err := zoneTmpl.Execute(&buf, zoneCtx{DomainName: domainName, Serial: serial, SOA: soa, Records: records}); err != nil {
		return err
	}

	if err := os.MkdirAll(ZoneDir, 0750); err != nil {
		return err
	}
	zonePath := filepath.Join(ZoneDir, domainName+".zone")
	tmpPath := zonePath + ".tmp"
	if err := os.WriteFile(tmpPath, buf.Bytes(), 0640); err != nil {
		return err
	}
	if out, err := exec.Command("named-checkzone", domainName, tmpPath).CombinedOutput(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("named-checkzone: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if err := os.Rename(tmpPath, zonePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	_, _ = exec.Command("chown", "named:named", zonePath).CombinedOutput()
	_, _ = exec.Command("restorecon", zonePath).CombinedOutput()

	if err := updateZoneIncludes(ctx, db); err != nil {
		return err
	}
	reloadNamed()
	return nil
}

// reloadNamed reloads BIND and falls back to progressively stronger service actions.
func reloadNamed() {
	if err := exec.Command("rndc", "reload").Run(); err == nil {
		return
	}
	if err := exec.Command("systemctl", "reload", "named").Run(); err == nil {
		return
	}
	_ = exec.Command("systemctl", "restart", "named").Run()
}

func updateZoneIncludes(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT d.domain_name FROM domains d
	  WHERE EXISTS (SELECT 1 FROM dns_records r WHERE r.domain_id=d.id AND r.enabled=1)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var sb strings.Builder
	sb.WriteString("// Automatically generated\n")
	for rows.Next() {
		var domainName string
		if err := rows.Scan(&domainName); err == nil {
			fmt.Fprintf(&sb, `zone "%s" { type master; file "%s/%s.zone"; allow-query { any; }; };
`, domainName, ZoneDir, domainName)
		}
	}
	return os.WriteFile(NamedConfInclude, []byte(sb.String()), 0644)
}

// DeleteZone removes a domain's BIND zone file and reloads the zone list.
func DeleteZone(ctx context.Context, db *sql.DB, domainName string) error {
	_ = os.Remove(filepath.Join(ZoneDir, domainName+".zone"))
	_ = updateZoneIncludes(ctx, db)
	reloadNamed()
	return nil
}
