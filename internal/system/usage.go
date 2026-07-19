package system

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"servika/internal/httpx"
	"servika/internal/resourcelimit"
)

const PanelVersion = "Servika 0.2.0"

type CPUUsage struct {
	Percent float64 `json:"percent"`
	Cores   int     `json:"cores"`
	Load1   float64 `json:"load_1m"`
	Load5   float64 `json:"load_5m"`
	Load15  float64 `json:"load_15m"`
}

type MemUsage struct {
	TotalKB int64   `json:"total_kb"`
	UsedKB  int64   `json:"used_kb"`
	FreeKB  int64   `json:"free_kb"`
	Percent float64 `json:"percent"`
}

type SwapUsage struct {
	TotalKB int64   `json:"total_kb"`
	UsedKB  int64   `json:"used_kb"`
	Percent float64 `json:"percent"`
}

type DiskUsage struct {
	TotalBytes uint64  `json:"total_byte"`
	UsedBytes  uint64  `json:"used_byte"`
	FreeBytes  uint64  `json:"free_byte"`
	Percent    float64 `json:"percent"`
	Mount      string  `json:"mount"`
	FS         string  `json:"fs,omitempty"`
}

type NetworkUsage struct {
	Interface    string `json:"interface"`
	RxBytes      int64  `json:"rx_bytes_sec"`
	TxBytes      int64  `json:"tx_bytes_sec"`
	RxTotalBytes int64  `json:"rx_total_byte"`
	TxTotalBytes int64  `json:"tx_total_byte"`
}

type ServiceStat struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Enabled bool   `json:"enabled"`
}

type SystemInfo struct {
	Hostname     string `json:"hostname"`
	IP           string `json:"ip"`
	OSName       string `json:"os_name"`
	Kernel       string `json:"kernel"`
	Arch         string `json:"arch"`
	CPUModel     string `json:"cpu_model"`
	CPUCores     int    `json:"cpu_cores"`
	PanelVersion string `json:"panel_version"`
}

type Usage struct {
	System        SystemInfo    `json:"system"`
	CPU           CPUUsage      `json:"cpu"`
	Memory        MemUsage      `json:"memory"`
	Swap          SwapUsage     `json:"swap"`
	Disk          DiskUsage     `json:"disk"`
	Disks         []DiskUsage   `json:"disks"`
	Network       NetworkUsage  `json:"network"`
	Services      []ServiceStat `json:"services"`
	UptimeSeconds int64         `json:"uptime_sec"`

	// QuotaRebootRequired is true when disk quota enforcement is INACTIVE (fs noquota /
	// uqnoenforce). A yellow warning banner on the dashboard reads this flag.
	QuotaRebootRequired bool `json:"quota_reboot_required"`
}

type cpuStat struct{ total, idle uint64 }

// scanLines visits each scanned line until the visitor requests a stop.
func scanLines(r io.Reader, visit func(string) bool) error {
	s := bufio.NewScanner(r)
	for s.Scan() {
		if !visit(s.Text()) {
			break
		}
	}
	return s.Err()
}

func readCPUStat() (cpuStat, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuStat{}, err
	}
	defer func() { _ = f.Close() }()
	var stat cpuStat
	found := false
	if err := scanLines(f, func(line string) bool {
		if !strings.HasPrefix(line, "cpu ") {
			return true
		}
		fields := strings.Fields(line)
		for i, fld := range fields[1:] {
			n, _ := strconv.ParseUint(fld, 10, 64)
			stat.total += n
			if i == 3 {
				stat.idle = n
			}
		}
		found = true
		return false
	}); err != nil {
		return cpuStat{}, err
	}
	if !found {
		return cpuStat{}, fmt.Errorf("CPU line not found")
	}
	return stat, nil
}

func ReadCPU() (CPUUsage, error) {
	s1, err := readCPUStat()
	if err != nil {
		return CPUUsage{}, err
	}
	time.Sleep(150 * time.Millisecond)
	s2, err := readCPUStat()
	if err != nil {
		return CPUUsage{}, err
	}
	totalDelta := float64(s2.total - s1.total)
	idleDelta := float64(s2.idle - s1.idle)
	pct := 0.0
	if totalDelta > 0 {
		pct = 100.0 * (totalDelta - idleDelta) / totalDelta
	}
	var load1, load5, load15 float64
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		f := strings.Fields(string(data))
		if len(f) >= 3 {
			load1, _ = strconv.ParseFloat(f[0], 64)
			load5, _ = strconv.ParseFloat(f[1], 64)
			load15, _ = strconv.ParseFloat(f[2], 64)
		}
	}
	return CPUUsage{
		Percent: round2(pct),
		Cores:   runtime.NumCPU(),
		Load1:   load1, Load5: load5, Load15: load15,
	}, nil
}

func ReadMem() (MemUsage, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return MemUsage{}, err
	}
	defer func() { _ = f.Close() }()
	var total, free, buffers, cached, available int64
	if err := scanLines(f, func(line string) bool {
		var v int64
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			_, _ = fmt.Sscanf(line, "MemTotal: %d kB", &v)
			total = v
		case strings.HasPrefix(line, "MemFree:"):
			_, _ = fmt.Sscanf(line, "MemFree: %d kB", &v)
			free = v
		case strings.HasPrefix(line, "Buffers:"):
			_, _ = fmt.Sscanf(line, "Buffers: %d kB", &v)
			buffers = v
		case strings.HasPrefix(line, "Cached:"):
			_, _ = fmt.Sscanf(line, "Cached: %d kB", &v)
			cached = v
		case strings.HasPrefix(line, "MemAvailable:"):
			_, _ = fmt.Sscanf(line, "MemAvailable: %d kB", &v)
			available = v
		}
		return true
	}); err != nil {
		return MemUsage{}, err
	}
	used := total - available
	if used <= 0 {
		used = total - free - buffers - cached
	}
	pct := 0.0
	if total > 0 {
		pct = 100.0 * float64(used) / float64(total)
	}
	return MemUsage{
		TotalKB: total,
		UsedKB:  used,
		FreeKB:  total - used,
		Percent: round2(pct),
	}, nil
}

func ReadSwap() SwapUsage {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return SwapUsage{}
	}
	defer func() { _ = f.Close() }()
	var total, free int64
	if err := scanLines(f, func(line string) bool {
		var v int64
		switch {
		case strings.HasPrefix(line, "SwapTotal:"):
			_, _ = fmt.Sscanf(line, "SwapTotal: %d kB", &v)
			total = v
		case strings.HasPrefix(line, "SwapFree:"):
			_, _ = fmt.Sscanf(line, "SwapFree: %d kB", &v)
			free = v
		}
		return true
	}); err != nil {
		return SwapUsage{}
	}
	used := total - free
	pct := 0.0
	if total > 0 {
		pct = 100.0 * float64(used) / float64(total)
	}
	return SwapUsage{TotalKB: total, UsedKB: used, Percent: round2(pct)}
}

func ReadDisk(mount string) (DiskUsage, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(mount, &st); err != nil {
		return DiskUsage{}, err
	}
	total := st.Blocks * uint64(st.Bsize)
	free := st.Bavail * uint64(st.Bsize)
	used := total - free
	pct := 0.0
	if total > 0 {
		pct = 100.0 * float64(used) / float64(total)
	}
	return DiskUsage{
		TotalBytes: total,
		UsedBytes:  used,
		FreeBytes:  free,
		Percent:    round2(pct),
		Mount:      mount,
	}, nil
}

func ReadDisks() []DiskUsage {
	skipFS := map[string]bool{
		"proc": true, "sysfs": true, "devtmpfs": true, "tmpfs": true,
		"devpts": true, "cgroup": true, "cgroup2": true, "pstore": true,
		"bpf": true, "tracefs": true, "debugfs": true, "configfs": true,
		"securityfs": true, "fusectl": true, "mqueue": true, "hugetlbfs": true,
		"binfmt_misc": true, "autofs": true, "rpc_pipefs": true, "nfsd": true,
		"selinuxfs": true, "fuse.gvfsd-fuse": true, "overlay": true, "squashfs": true,
		"ramfs": true,
	}
	skipPrefix := []string{"/proc", "/sys", "/dev", "/run", "/var/lib/docker", "/var/lib/containers", "/snap", "/home/jails"}
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	seen := map[string]bool{}
	seenDev := map[string]bool{} // Track duplicate /dev devices created by bind mounts.
	out := []DiskUsage{}
	if err := scanLines(f, func(line string) bool {
		parts := strings.Fields(line)
		if len(parts) < 3 {
			return true
		}
		dev := parts[0]
		mount := parts[1]
		fs := parts[2]
		if skipFS[fs] {
			return true
		}
		for _, p := range skipPrefix {
			if strings.HasPrefix(mount, p+"/") || mount == p {
				return true
			}
		}
		if seen[mount] {
			return true
		}
		seen[mount] = true
		// Remove duplicate bind-mounted devices. Jail home mounts share the root device and
		// would report its size again, so list each physical block device only once.
		if strings.HasPrefix(dev, "/dev/") {
			if seenDev[dev] {
				return true
			}
			seenDev[dev] = true
		}
		d, err := ReadDisk(mount)
		if err != nil {
			return true
		}
		d.FS = fs
		out = append(out, d)
		return true
	}); err != nil {
		return nil
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mount < out[j].Mount })
	return out
}

var (
	networkMu       sync.Mutex
	previousNetwork = map[string]networkSnapshot{}
)

type networkSnapshot struct {
	rx, tx int64
	t      time.Time
}

func ReadNetwork() NetworkUsage {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return NetworkUsage{}
	}
	defer func() { _ = f.Close() }()
	type rec struct {
		name   string
		rx, tx int64
	}
	var stats []rec
	if err := scanLines(f, func(line string) bool {
		name, after, found := strings.Cut(line, ":")
		if !found {
			return true
		}
		name = strings.TrimSpace(name)
		if name == "lo" || strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "br-") ||
			strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "tap") ||
			strings.HasPrefix(name, "tun") || strings.HasPrefix(name, "wg") ||
			strings.HasPrefix(name, "virbr") || strings.HasPrefix(name, "vnet") {
			return true
		}
		fld := strings.Fields(after)
		if len(fld) < 9 {
			return true
		}
		rx, _ := strconv.ParseInt(fld[0], 10, 64)
		tx, _ := strconv.ParseInt(fld[8], 10, 64)
		stats = append(stats, rec{name: name, rx: rx, tx: tx})
		return true
	}); err != nil {
		return NetworkUsage{}
	}
	if len(stats) == 0 {
		return NetworkUsage{}
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].rx > stats[j].rx })
	primary := stats[0]
	now := time.Now()
	networkMu.Lock()
	defer networkMu.Unlock()
	prev, existed := previousNetwork[primary.name]
	previousNetwork[primary.name] = networkSnapshot{rx: primary.rx, tx: primary.tx, t: now}
	var rxRate, txRate int64
	if existed {
		dt := now.Sub(prev.t).Seconds()
		if dt > 0 {
			rxRate = int64(float64(primary.rx-prev.rx) / dt)
			txRate = int64(float64(primary.tx-prev.tx) / dt)
			if rxRate < 0 {
				rxRate = 0
			}
			if txRate < 0 {
				txRate = 0
			}
		}
	}
	return NetworkUsage{
		Interface: primary.name, RxBytes: rxRate, TxBytes: txRate,
		RxTotalBytes: primary.rx, TxTotalBytes: primary.tx,
	}
}

func ReadUptime() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	f := strings.Fields(string(data))
	if len(f) > 0 {
		sec, _ := strconv.ParseFloat(f[0], 64)
		return int64(sec)
	}
	return 0
}

func round2(f float64) float64 {
	return float64(int(f*100)) / 100
}

func primaryIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		n := iface.Name
		if strings.HasPrefix(n, "veth") || strings.HasPrefix(n, "br-") ||
			strings.HasPrefix(n, "docker") || strings.HasPrefix(n, "tap") ||
			strings.HasPrefix(n, "tun") || strings.HasPrefix(n, "wg") ||
			strings.HasPrefix(n, "virbr") || strings.HasPrefix(n, "vnet") {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok {
				ip := ipnet.IP.To4()
				if ip == nil {
					continue
				}
				if ip.IsPrivate() || ip.IsLoopback() {
					continue
				}
				return ip.String()
			}
		}
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok {
				if ip := ipnet.IP.To4(); ip != nil && !ip.IsLoopback() {
					return ip.String()
				}
			}
		}
	}
	return ""
}

var (
	infoCache   *SystemInfo
	infoCacheMu sync.Mutex
)

func ReadInfo() SystemInfo {
	infoCacheMu.Lock()
	defer infoCacheMu.Unlock()
	if infoCache != nil {
		c := *infoCache
		c.IP = primaryIP()
		return c
	}
	info := SystemInfo{PanelVersion: PanelVersion, CPUCores: runtime.NumCPU(), Arch: runtime.GOARCH}
	info.Hostname, _ = os.Hostname()
	info.IP = primaryIP()
	if output, err := exec.Command("uname", "-r").Output(); err == nil {
		info.Kernel = strings.TrimSpace(string(output))
	}
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if v, found := strings.CutPrefix(line, "PRETTY_NAME="); found {
				v = strings.Trim(v, "\"'")
				info.OSName = v
				break
			}
		}
	}
	if f, err := os.Open("/proc/cpuinfo"); err == nil {
		if err := scanLines(f, func(line string) bool {
			if strings.HasPrefix(line, "model name") {
				if _, after, found := strings.Cut(line, ":"); found {
					info.CPUModel = strings.TrimSpace(after)
					return false
				}
			}
			return true
		}); err != nil {
			info.CPUModel = ""
		}
		_ = f.Close()
	}
	cached := info
	infoCache = &cached
	return info
}

var serviceList = []struct{ name, label string }{
	{"servika", "Panel"},
	{"nginx", "Nginx"},
	{"mariadb", "MariaDB"},
	{"pure-ftpd", "FTP"},
	{"named", "DNS (BIND)"},
	{"php74-php-fpm", "PHP 7.4-FPM"},
	{"php82-php-fpm", "PHP 8.2-FPM"},
	{"php83-php-fpm", "PHP 8.3-FPM"},
	{"php84-php-fpm", "PHP 8.4-FPM"},
	{"crond", "Cron"},
	{"sshd", "SSH"},
	{"firewalld", "Firewalld"},
}

func ReadServices() []ServiceStat {
	out := make([]ServiceStat, 0, len(serviceList))
	type res struct {
		i       int
		enabled bool
	}
	ch := make(chan res, len(serviceList))
	for i, s := range serviceList {
		go func(i int, name string) {
			cmd := exec.Command("systemctl", "is-active", name)
			b, _ := cmd.Output()
			ch <- res{i: i, enabled: strings.TrimSpace(string(b)) == "active"}
		}(i, s.name)
	}
	mat := make(map[int]bool)
	for range len(serviceList) {
		r := <-ch
		mat[r.i] = r.enabled
	}
	for i, s := range serviceList {
		enabled := mat[i]
		if !enabled && (s.name == "pure-ftpd" || strings.HasPrefix(s.name, "php")) {
			cmd := exec.Command("systemctl", "list-unit-files", s.name+".service", "--no-legend")
			b, _ := cmd.Output()
			if len(strings.TrimSpace(string(b))) == 0 {
				continue
			}
		}
		out = append(out, ServiceStat{Name: s.name, Label: s.label, Enabled: enabled})
	}
	return out
}

func Handler(w http.ResponseWriter, r *http.Request) {
	cpu, _ := ReadCPU()
	mem, _ := ReadMem()
	disk, _ := ReadDisk("/")

	var disks []DiskUsage
	var network NetworkUsage
	var services []ServiceStat
	var swap SwapUsage
	var info SystemInfo

	var quotaReboot bool

	var wg sync.WaitGroup
	wg.Add(6)
	go func() { defer wg.Done(); disks = ReadDisks() }()
	go func() { defer wg.Done(); network = ReadNetwork() }()
	go func() { defer wg.Done(); services = ReadServices() }()
	go func() { defer wg.Done(); swap = ReadSwap() }()
	go func() { defer wg.Done(); info = ReadInfo() }()
	go func() { defer wg.Done(); quotaReboot = resourcelimit.QuotaRebootRequired() }()
	wg.Wait()

	httpx.WriteJSON(w, http.StatusOK, Usage{
		System: info, CPU: cpu, Memory: mem, Swap: swap,
		Disk: disk, Disks: disks, Network: network,
		Services: services, UptimeSeconds: ReadUptime(),
		QuotaRebootRequired: quotaReboot,
	})
}
