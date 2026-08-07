package mail

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"servika/internal/netguard"
)

// Where a candidate came from. The screen shows this so an operator can tell a
// server the old provider actually published from one the panel guessed.
const (
	SourceAutoconfig = "autoconfig"
	SourceISPDB      = "ispdb"
	SourceSRV        = "srv"
	SourceMX         = "mx"
	SourceGuess      = "guess"
)

// Candidate is one server the old mailbox might live on.
type Candidate struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Security string `json:"security"`
	Source   string `json:"source"`
	// Responds is set by the probe: the panel offers a server that answered
	// first, because a published record can outlive the server it names.
	Responds bool `json:"responds"`
}

// discoveryBudget bounds the whole sweep. Several lookups run in parallel and
// any of them can hang, so the customer waits for this at worst rather than for
// the slowest DNS server on the internet.
const discoveryBudget = 20 * time.Second

// autoconfigLimit caps a fetched document. The file comes from a host the
// customer named, so it is hostile input and must not be read into memory
// without a ceiling.
const autoconfigLimit = 256 << 10

// discoveryClient fetches autoconfig documents.
//
// The hostnames are derived from an address the customer typed, so the dialler
// carries the SSRF guard; it runs per connection, which also covers redirects.
var discoveryClient = &http.Client{
	Timeout: 8 * time.Second,
	Transport: &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, Control: netguard.DialControl}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
		DisableKeepAlives:   true,
	},
}

// clientConfig is the part of the Thunderbird autoconfig format the panel uses.
type clientConfig struct {
	Provider struct {
		Incoming []struct {
			Type       string `xml:"type,attr"`
			Hostname   string `xml:"hostname"`
			Port       int    `xml:"port"`
			SocketType string `xml:"socketType"`
		} `xml:"incomingServer"`
	} `xml:"emailProvider"`
}

// socketSecurity maps the autoconfig vocabulary onto the panel's.
func socketSecurity(socketType string) string {
	switch strings.ToUpper(strings.TrimSpace(socketType)) {
	case "SSL":
		return SecuritySSL
	case "STARTTLS":
		return SecuritySTARTTLS
	case "PLAIN", "NONE":
		return SecurityPlain
	default:
		return ""
	}
}

// DiscoverCandidates collects every server worth trying for an address.
//
// The sources are tried together rather than in sequence: a provider that
// publishes autoconfig is the best answer, but plenty of small hosts publish
// nothing at all, and the guesses cost one TCP connection each.
func DiscoverCandidates(ctx context.Context, address string) []Candidate {
	domain := addressDomain(address)
	if domain == "" || !isDiscoverableDomain(domain) {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, discoveryBudget)
	defer cancel()

	var (
		mu    sync.Mutex
		found []Candidate
	)
	add := func(list ...Candidate) {
		mu.Lock()
		found = append(found, list...)
		mu.Unlock()
	}

	var wg sync.WaitGroup
	for _, source := range []func(context.Context, string, string) []Candidate{
		fromAutoconfig, fromISPDB, fromSRV, fromMX, fromGuesses,
	} {
		wg.Add(1)
		go func(collect func(context.Context, string, string) []Candidate) {
			defer wg.Done()
			add(collect(ctx, domain, address)...)
		}(source)
	}
	wg.Wait()

	return probeCandidates(ctx, dedupeCandidates(found))
}

// isDiscoverableDomain refuses anything that is not a plain hostname, so a
// crafted address cannot turn into a URL path or an extra header.
func isDiscoverableDomain(domain string) bool {
	if len(domain) == 0 || len(domain) > 253 || !strings.Contains(domain, ".") {
		return false
	}
	for _, r := range domain {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-':
		default:
			return false
		}
	}
	return true
}

// fromAutoconfig reads the two places a domain can publish its own settings.
func fromAutoconfig(ctx context.Context, domain, address string) []Candidate {
	query := url.Values{"emailaddress": {address}}.Encode()
	urls := []string{
		"https://autoconfig." + domain + "/mail/config-v1.1.xml?" + query,
		"https://" + domain + "/.well-known/autoconfig/mail/config-v1.1.xml?" + query,
	}
	for _, endpoint := range urls {
		if list := fetchClientConfig(ctx, endpoint, SourceAutoconfig); len(list) > 0 {
			return list
		}
	}
	return nil
}

// fromISPDB asks Mozilla's database, which answers for most large providers
// that publish nothing themselves.
func fromISPDB(ctx context.Context, domain, _ string) []Candidate {
	return fetchClientConfig(ctx, "https://autoconfig.thunderbird.net/v1.1/"+domain, SourceISPDB)
}

func fetchClientConfig(ctx context.Context, endpoint, source string) []Candidate {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}
	response, err := discoveryClient.Do(request)
	if err != nil {
		return nil
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil
	}

	var config clientConfig
	if err := xml.NewDecoder(io.LimitReader(response.Body, autoconfigLimit)).Decode(&config); err != nil {
		return nil
	}

	var list []Candidate
	for _, server := range config.Provider.Incoming {
		if !strings.EqualFold(server.Type, "imap") {
			continue
		}
		security := socketSecurity(server.SocketType)
		if security == "" || !validPort(server.Port) || !isDiscoverableDomain(strings.ToLower(server.Hostname)) {
			continue
		}
		list = append(list, Candidate{
			Host: strings.ToLower(server.Hostname), Port: server.Port,
			Security: security, Source: source,
		})
	}
	return list
}

// fromSRV reads the service records RFC 6186 defines for mail clients.
func fromSRV(ctx context.Context, domain, _ string) []Candidate {
	var list []Candidate
	for _, record := range []struct {
		service  string
		security string
	}{
		{"_imaps._tcp.", SecuritySSL},
		{"_imap._tcp.", SecuritySTARTTLS},
	} {
		_, entries, err := net.DefaultResolver.LookupSRV(ctx, "", "", record.service+domain)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			host := strings.ToLower(strings.TrimSuffix(entry.Target, "."))
			if !isDiscoverableDomain(host) || !validPort(int(entry.Port)) {
				continue
			}
			list = append(list, Candidate{Host: host, Port: int(entry.Port), Security: record.security, Source: SourceSRV})
		}
	}
	return list
}

// fromMX offers the domain's mail exchanger. It is often the same machine that
// serves IMAP on a small host, and it is wrong often enough that it is only ever
// one candidate among several.
func fromMX(ctx context.Context, domain, _ string) []Candidate {
	records, err := net.DefaultResolver.LookupMX(ctx, domain)
	if err != nil || len(records) == 0 {
		return nil
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].Pref < records[j].Pref })
	host := strings.ToLower(strings.TrimSuffix(records[0].Host, "."))
	if !isDiscoverableDomain(host) {
		return nil
	}
	return []Candidate{{Host: host, Port: 993, Security: SecuritySSL, Source: SourceMX}}
}

// fromGuesses covers the hosts that are conventional enough to be worth a
// connection even when nothing is published.
func fromGuesses(_ context.Context, domain, _ string) []Candidate {
	var list []Candidate
	for _, host := range []string{"mail." + domain, "imap." + domain, domain} {
		list = append(list,
			Candidate{Host: host, Port: 993, Security: SecuritySSL, Source: SourceGuess},
			Candidate{Host: host, Port: 143, Security: SecuritySTARTTLS, Source: SourceGuess},
		)
	}
	return list
}

func validPort(port int) bool { return port > 0 && port < 65536 }

// dedupeCandidates keeps the first mention of each host/port/security, so a
// server that autoconfig published is not offered again as a guess.
func dedupeCandidates(list []Candidate) []Candidate {
	rank := map[string]int{SourceAutoconfig: 0, SourceISPDB: 1, SourceSRV: 2, SourceMX: 3, SourceGuess: 4}
	sort.SliceStable(list, func(i, j int) bool { return rank[list[i].Source] < rank[list[j].Source] })

	seen := make(map[string]struct{}, len(list))
	out := make([]Candidate, 0, len(list))
	for _, candidate := range list {
		key := fmt.Sprintf("%s|%d|%s", candidate.Host, candidate.Port, candidate.Security)
		if _, done := seen[key]; done {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

// probeCandidates opens each candidate to find out which ones actually answer.
//
// A published record can name a server that was decommissioned years ago, and a
// guess can be right, so the panel reports what responded rather than what was
// advertised. Probes run together because the slow ones are the dead ones.
func probeCandidates(ctx context.Context, list []Candidate) []Candidate {
	var wg sync.WaitGroup
	for index := range list {
		wg.Add(1)
		go func(at int) {
			defer wg.Done()
			client, err := dialIMAP(ctx, list[at].Host, list[at].Port, list[at].Security)
			if err != nil {
				return
			}
			_ = client.Close()
			list[at].Responds = true
		}(index)
	}
	wg.Wait()

	sort.SliceStable(list, func(i, j int) bool { return list[i].Responds && !list[j].Responds })
	return list
}
