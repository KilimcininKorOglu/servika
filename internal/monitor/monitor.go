// Package monitor provides process monitoring and domain HTTP health probes.
package monitor

import (
	"crypto/tls"
	"database/sql"
	"errors"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"servika/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type Process struct {
	PID     int     `json:"pid"`
	User    string  `json:"user"`
	CPU     float64 `json:"cpu_percent"`
	Mem     float64 `json:"mem_percent"`
	Command string  `json:"command"`
}

type Handlers struct {
	DB *sql.DB
}

// GET /system/processes?n=15&sort=cpu|mem
func Processes(w http.ResponseWriter, r *http.Request) {
	n := 15
	if s := r.URL.Query().Get("n"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 && v <= 100 {
			n = v
		}
	}
	sortBy := r.URL.Query().Get("sort")
	sortFlag := "-pcpu"
	if sortBy == "mem" {
		sortFlag = "-pmem"
	}

	cmd := exec.Command("ps", "-eo", "pid,user:32,pcpu,pmem,args", "--no-headers", "--sort="+sortFlag)
	out, err := cmd.Output()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to read process list")
		return
	}
	lines := strings.Split(string(out), "\n")
	procs := make([]Process, 0, n)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		cpu, _ := strconv.ParseFloat(f[2], 64)
		mem, _ := strconv.ParseFloat(f[3], 64)
		command := strings.Join(f[4:], " ")
		if len(command) > 120 {
			command = command[:120] + "…"
		}
		procs = append(procs, Process{PID: pid, User: f[1], CPU: cpu, Mem: mem, Command: command})
		if len(procs) >= n {
			break
		}
	}
	httpx.WriteJSON(w, http.StatusOK, procs)
}

type SSLInfo struct {
	Valid         bool   `json:"valid"`
	EndDate       string `json:"end_date"`
	RemainingDays int    `json:"remaining_days"`
	Issuer        string `json:"issuer,omitempty"`
	SubjectName   string `json:"subject,omitempty"`
}

type DomainHealth struct {
	URL            string   `json:"url"`
	StatusCode     int      `json:"status_code"`
	ResponseTimeMS float64  `json:"response_time_ms"`
	Reachable      bool     `json:"reachable"`
	Error          string   `json:"error,omitempty"`
	Scheme         string   `json:"scheme"` // "http" | "https"
	SSL            *SSLInfo `json:"ssl,omitempty"`
	Size           int64    `json:"size_byte"`
	Server         string   `json:"server,omitempty"`
}

// GET /domains/{id}/health
// Health tries HTTPS first, falls back to HTTP, and reads SSL details from the certificate.
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var domainName, ipv4 string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT domain_name, ipv4 FROM domains WHERE id=?`, id).Scan(&domainName, &ipv4)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to read domain")
		return
	}

	res := probe("https://" + domainName)
	res.Scheme = "https"
	if !res.Reachable {
		// Fall back to HTTP when HTTPS fails.
		alt := probe("http://" + domainName)
		if alt.Reachable {
			alt.Scheme = "http"
			httpx.WriteJSON(w, http.StatusOK, alt)
			return
		}
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

func probe(targetURL string) DomainHealth {
	res := DomainHealth{URL: targetURL}

	tlsCfg := &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}
	tr := &http.Transport{
		TLSClientConfig:       tlsCfg,
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: 6 * time.Second,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   8 * time.Second,
		// Follow up to five redirects.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	start := time.Now()
	req, _ := http.NewRequest("GET", targetURL, nil)
	req.Header.Set("User-Agent", "Servika-Monitor/1.0")
	req.Header.Set("Accept", "text/html,*/*")
	resp, err := client.Do(req)
	res.ResponseTimeMS = float64(time.Since(start).Microseconds()) / 1000.0
	if err != nil {
		res.Error = "request failed"
		return res
	}
	defer func() { _ = resp.Body.Close() }()
	res.Reachable = true
	res.StatusCode = resp.StatusCode
	res.Server = resp.Header.Get("Server")
	res.Size = resp.ContentLength

	// Read SSL certificate information when a TLS connection exists.
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		c := resp.TLS.PeerCertificates[0]
		now := time.Now()
		remainingDays := int(c.NotAfter.Sub(now).Hours() / 24)
		res.SSL = &SSLInfo{
			Valid:         now.Before(c.NotAfter) && now.After(c.NotBefore),
			EndDate:       c.NotAfter.Format("2006-01-02"),
			RemainingDays: remainingDays,
			Issuer:        c.Issuer.CommonName,
			SubjectName:   c.Subject.CommonName,
		}
	}
	return res
}
