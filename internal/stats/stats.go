// Package stats provides read-only per-domain traffic analysis from nginx access logs.
package stats

import (
	"bufio"
	"database/sql"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

// Handlers provides domain traffic statistics HTTP handlers.
type Handlers struct {
	DB *sql.DB
}

// KV represents a named aggregate count.
type KV struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Day represents the request count for one day.
type Day struct {
	Date    string `json:"date"`
	Request int    `json:"request"`
}

// Summary contains aggregated traffic statistics for a domain.
type Summary struct {
	DomainName       string         `json:"domain_name"`
	HasLog           bool           `json:"has_log"`
	TotalRequests    int            `json:"total_requests"`
	TotalBandwidthMB float64        `json:"total_bandwidth_mb"`
	UniqueIP         int            `json:"unique_ip"`
	BotRatio         int            `json:"bot_ratio"` // Percentage.
	StatusGroup      map[string]int `json:"status_group"`
	TopPaths         []KV           `json:"top_paths"`
	TopIP            []KV           `json:"top_ip"`
	AggStatus        []KV           `json:"agg_status"`
	Daily            []Day          `json:"daily"`
	LastRequests     []string       `json:"last_requests"`
}

// reLog parses the combined log format: IP - - [date] "METHOD path proto" status bytes "ref" "ua".
var reLog = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^:]+):[^\]]+\] "(\S+) (\S+)[^"]*" (\d{3}) (\d+|-) "[^"]*" "([^"]*)"`)

const maxLines = 200000

func topN(m map[string]int, n int) []KV {
	out := make([]KV, 0, len(m))
	for k, v := range m {
		out = append(out, KV{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// Show returns aggregated nginx access log statistics for a domain.
func (h *Handlers) Show(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var domainName string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT domain_name FROM domains WHERE id=?`, id).Scan(&domainName); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	summary := Summary{DomainName: domainName, StatusGroup: map[string]int{"2xx": 0, "3xx": 0, "4xx": 0, "5xx": 0}}

	logPath := "/var/log/nginx/" + domainName + ".access.log"
	file, err := os.Open(logPath)
	if err != nil {
		httpx.WriteJSON(w, http.StatusOK, summary) // Return an empty summary when no log exists.
		return
	}
	defer func() { _ = file.Close() }()
	summary.HasLog = true

	if err := summarizeLog(file, &summary); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not read access log")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, summary)
}

func summarizeLog(reader io.Reader, summary *Summary) error {
	paths := map[string]int{}
	ips := map[string]int{}
	statuses := map[string]int{}
	days := map[string]int{}
	var recentRequests []string
	var totalBytes int64
	botKeys := []string{"bot", "spider", "crawl", "slurp", "bingpreview", "facebookexternal", "curl", "wget", "python", "go-http"}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	lineCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		matches := reLog.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		lineCount++
		if lineCount > maxLines {
			break
		}
		ip, date, method, path, statusCode, byteCount, userAgent := matches[1], matches[2], matches[3], matches[4], matches[5], matches[6], matches[7]
		summary.TotalRequests++
		ips[ip]++
		// Normalize the path by removing its query string.
		if i := strings.IndexByte(path, '?'); i >= 0 {
			path = path[:i]
		}
		if len(path) > 80 {
			path = path[:80]
		}
		paths[method+" "+path]++
		statuses[statusCode]++
		switch statusCode[0] {
		case '2':
			summary.StatusGroup["2xx"]++
		case '3':
			summary.StatusGroup["3xx"]++
		case '4':
			summary.StatusGroup["4xx"]++
		case '5':
			summary.StatusGroup["5xx"]++
		}
		days[date]++
		if byteCount != "-" {
			if parsedBytes, parseErr := strconv.ParseInt(byteCount, 10, 64); parseErr == nil {
				totalBytes += parsedBytes
			}
		}
		lowerUserAgent := strings.ToLower(userAgent)
		for _, botKey := range botKeys {
			if strings.Contains(lowerUserAgent, botKey) {
				summary.BotRatio++ // Count bot requests before converting to a percentage.
				break
			}
		}
		if len(recentRequests) < 40 {
			recentRequests = append(recentRequests, statusCode+" "+method+" "+path+" ("+ip+")")
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	summary.UniqueIP = len(ips)
	summary.TotalBandwidthMB = float64(totalBytes) / (1024 * 1024)
	if summary.TotalRequests > 0 {
		summary.BotRatio = summary.BotRatio * 100 / summary.TotalRequests
	}
	summary.TopPaths = topN(paths, 10)
	summary.TopIP = topN(ips, 10)
	summary.AggStatus = topN(statuses, 8)
	// Reverse the captured requests and return the newest 20 from the last 40.
	for i := len(recentRequests) - 1; i >= 0 && len(summary.LastRequests) < 20; i-- {
		summary.LastRequests = append(summary.LastRequests, recentRequests[i])
	}
	// Return the last seven days sorted by day name.
	dayKeys := make([]string, 0, len(days))
	for day := range days {
		dayKeys = append(dayKeys, day)
	}
	sort.Strings(dayKeys)
	if len(dayKeys) > 7 {
		dayKeys = dayKeys[len(dayKeys)-7:]
	}
	for _, day := range dayKeys {
		summary.Daily = append(summary.Daily, Day{day, days[day]})
	}
	return nil
}
