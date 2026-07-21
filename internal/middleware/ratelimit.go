package middleware

// Login brute-force protection — per-IP sliding window + lockout.
//
// RATIONALE: The panel login is the server ROOT password and :8443 is exposed
// to the internet. Without rate limiting an online brute-force attack can
// directly compromise the entire server.
//
// DESIGN:
//   - Only FAILED (401) attempts are counted.
//   - The counter does NOT reset on success: in the 2FA flow a correct password
//     returns 200 + two_fa_required; resetting would let an attacker probe
//     password-only requests to keep the counter at zero and brute-force the
//     TOTP code indefinitely. The counter ages out when the window expires.
//   - Policy: 5 failed attempts in 15 minutes → IP banned for 30 minutes.
//   - Graduated delay: each failed attempt slows the request (capped).
//   - Records are periodically pruned (memory-bloat/DoS prevention).
//
// NOTE: The IP key comes from httpx.ClientIP; that function trusts proxy
// headers ONLY from the local reverse-proxy (nginx) — otherwise a spoofed
// X-Forwarded-For could bypass this limit.

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"servika/internal/httpx"
)

const (
	loginWindow  = 15 * time.Minute // sliding window for counting failures
	loginMaxFail = 5                // allowed failures per window
	loginLock    = 30 * time.Minute // lockout duration when exceeded
	loginMaxLag  = 2 * time.Second  // graduated delay upper bound
)

type loginRecord struct {
	failures []time.Time
	lockedAt time.Time
}

var (
	loginMu  sync.Mutex
	loginMap = map[string]*loginRecord{}
)

func init() { go loginReaper() }

// loginReaper prunes stale records (prevents unbounded memory growth).
func loginReaper() {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		cutoff := now.Add(-(loginWindow + loginLock))
		loginMu.Lock()
		for ip, r := range loginMap {
			emptyAndStale := len(r.failures) == 0 || r.failures[len(r.failures)-1].Before(cutoff)
			if r.lockedAt.Before(now) && emptyAndStale {
				delete(loginMap, ip)
			}
		}
		loginMu.Unlock()
	}
}

// loginStatus trims out-of-window failures; returns (current count, remaining lock time).
func loginStatus(ip string) (int, time.Duration) {
	now := time.Now()
	loginMu.Lock()
	defer loginMu.Unlock()
	r := loginMap[ip]
	if r == nil {
		return 0, 0
	}
	if now.Before(r.lockedAt) {
		return loginMaxFail, r.lockedAt.Sub(now)
	}
	cutoff := now.Add(-loginWindow)
	kept := r.failures[:0]
	for _, t := range r.failures {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	r.failures = kept
	return len(r.failures), 0
}

func loginRecordFail(ip string) {
	now := time.Now()
	loginMu.Lock()
	defer loginMu.Unlock()
	r := loginMap[ip]
	if r == nil {
		r = &loginRecord{}
		loginMap[ip] = r
	}
	r.failures = append(r.failures, now)
	if len(r.failures) >= loginMaxFail {
		r.lockedAt = now.Add(loginLock)
		r.failures = nil
	}
}

// durationText formats remaining seconds into human-readable text.
func durationText(sec int) string {
	if sec < 60 {
		return fmt.Sprintf("%d seconds", sec)
	}
	min := (sec + 59) / 60
	return fmt.Sprintf("%d minutes", min)
}

// statusWriter captures the HTTP status code written by a handler.
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (s *statusWriter) WriteHeader(c int) {
	s.code = c
	s.ResponseWriter.WriteHeader(c)
}

// LoginRateLimit provides brute-force protection on authentication endpoints
// (counts 401 responses per IP).
func LoginRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := httpx.ClientIP(r)
		count, remain := loginStatus(ip)
		if remain > 0 {
			sec := int(remain.Seconds()) + 1
			w.Header().Set("Retry-After", strconv.Itoa(sec))
			httpx.WriteError(w, http.StatusTooManyRequests,
				fmt.Sprintf("too many failed login attempts — try again in %s", durationText(sec)))
			return
		}
		if count > 0 { // graduated slowdown
			d := time.Duration(count) * 250 * time.Millisecond
			if d > loginMaxLag {
				d = loginMaxLag
			}
			time.Sleep(d)
		}
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(sw, r)
		if sw.code == http.StatusUnauthorized {
			loginRecordFail(ip)
		}
	})
}
