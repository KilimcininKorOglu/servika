// Package diskusage measures directory sizes with du, bounding what one
// customer request can cost the whole server.
//
// Every caller here is reachable from a CustomerScope route, and the panel runs
// as root, so a scan is NOT accounted against the tenant's cgroup I/O limit
// (internal/resourcelimit). Measuring by hand, as each caller used to, gave a
// customer three ways to turn a page into unbounded disk I/O: no deadline, so a
// multi-million-inode tree pinned a disk until it finished; no cache, so a
// held-down refresh started a fresh full-tree scan per request; and no
// coalescing, so concurrent requests for the same path each started their own.
//
// Routing every measurement through this package fixes all three at once, and
// gives the subprocess an explicit environment instead of the panel's, which
// carries SERVIKA_JWT_SECRET and SERVIKA_SECRET_KEY.
package diskusage

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// measureTimeout bounds a single du run.
const measureTimeout = 30 * time.Second

// cacheTTL is how long a measurement is reused. A home directory's size does not
// change meaningfully inside a minute, and this is what keeps a refresh from
// costing a scan.
const cacheTTL = 60 * time.Second

// cacheSweepAt is the entry count above which expired entries are dropped. The
// cache is keyed by path, so it is bounded by the number of tenants; the sweep
// only stops a long-lived process from holding entries for deleted directories.
const cacheSweepAt = 512

// commandEnv is the subprocess environment. du needs nothing from the panel's
// own environment, and inheriting it would hand a customer-triggered child the
// panel's JWT and encryption keys.
var commandEnv = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}

var errNoSize = errors.New("du produced no size")

type result struct {
	bytes int64
	err   error
}

type entry struct {
	result
	at time.Time
}

// call is one in-flight measurement. Callers arriving while it runs wait on done
// rather than starting a second du for the same path.
type call struct {
	done chan struct{}
	result
}

var (
	mu       sync.Mutex
	cache    = map[string]entry{}
	inflight = map[string]*call{}

	// Test seams. now lets a test age the cache without sleeping, and measureFn
	// lets it count how many real measurements a set of calls produced.
	now       = time.Now
	measureFn = measure
)

// Bytes returns the apparent size of path in bytes.
//
// The measurement deliberately does not inherit ctx. It runs under its own
// deadline, so a caller that gives up (a client disconnecting mid-request)
// neither kills the run other callers are waiting on nor leaves the cache
// unpopulated. ctx bounds only how long THIS caller waits for the answer.
//
// A du that exits non-zero is an error even when it printed a total: a partial
// total under-reports usage, and these numbers drive quota display.
func Bytes(ctx context.Context, path string) (int64, error) {
	mu.Lock()
	if e, ok := cache[path]; ok && now().Sub(e.at) < cacheTTL {
		mu.Unlock()
		return e.bytes, e.err
	}
	if c, ok := inflight[path]; ok {
		mu.Unlock()
		return c.wait(ctx)
	}
	c := &call{done: make(chan struct{})}
	inflight[path] = c
	mu.Unlock()

	go func() {
		c.result = measureFn(path)
		mu.Lock()
		cache[path] = entry{result: c.result, at: now()}
		delete(inflight, path)
		sweepLocked()
		mu.Unlock()
		close(c.done)
	}()
	return c.wait(ctx)
}

// wait blocks until the shared measurement finishes or this caller's context
// ends, whichever comes first.
func (c *call) wait(ctx context.Context) (int64, error) {
	select {
	case <-c.done:
		return c.bytes, c.err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// sweepLocked drops expired entries once the cache grows past cacheSweepAt. The
// caller holds mu.
func sweepLocked() {
	if len(cache) < cacheSweepAt {
		return
	}
	cutoff := now().Add(-cacheTTL)
	for path, e := range cache {
		if e.at.Before(cutoff) {
			delete(cache, path)
		}
	}
}

// measure runs du once. It reports apparent size in bytes (-sb) for every
// caller, so the same tree cannot be reported as two different sizes by two
// endpoints, which is what the old mix of -sb, -sk and -sm produced.
func measure(path string) result {
	ctx, cancel := context.WithTimeout(context.Background(), measureTimeout)
	defer cancel()
	// #nosec G204 G702 -- fixed binary with separate args (no shell); callers validate the path before measuring.
	command := exec.CommandContext(ctx, "du", "-sb", path)
	command.Env = commandEnv
	out, err := command.Output()
	if err != nil {
		return result{err: fmt.Errorf("du %s: %w", path, err)}
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return result{err: errNoSize}
	}
	size, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return result{err: fmt.Errorf("du %s: %w", path, err)}
	}
	return result{bytes: size}
}
