package mail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Everything a remote autoconfig document says is hostile input, and the domain
// itself comes from a form, so anything that is not a plain hostname has to be
// refused before it reaches a URL or a dial.
func TestDiscoverableDomainRefusesAnythingButAHostname(t *testing.T) {
	for _, domain := range []string{
		"", "localhost", "example .com", "example.com/../evil",
		"example.com\r\nHost: evil.test", "example.com:993", "EXAMPLE.COM",
		"exa mple.com", "example.com?x=1", strings.Repeat("a", 254) + ".com",
	} {
		if isDiscoverableDomain(domain) {
			t.Errorf("%q was accepted as a hostname", domain)
		}
	}
}

// The refusal above is only meaningful if ordinary names still pass.
func TestDiscoverableDomainAcceptsOrdinaryNames(t *testing.T) {
	for _, domain := range []string{"example.com", "mail.example.co.uk", "imap-1.example.com"} {
		if !isDiscoverableDomain(domain) {
			t.Errorf("%q was refused", domain)
		}
	}
}

func TestSocketTypeMapsOntoThePanelsVocabulary(t *testing.T) {
	cases := map[string]string{
		"SSL": SecuritySSL, "ssl": SecuritySSL,
		"STARTTLS": SecuritySTARTTLS, "plain": SecurityPlain,
		"": "", "TLS": "",
	}
	for socketType, want := range cases {
		if got := socketSecurity(socketType); got != want {
			t.Errorf("socketSecurity(%q) = %q, want %q", socketType, got, want)
		}
	}
}

// A server the provider published must not be offered a second time as a guess,
// and the published entry must be the one that survives.
func TestDuplicateCandidatesCollapseOntoTheBestSource(t *testing.T) {
	list := dedupeCandidates([]Candidate{
		{Host: "imap.example.com", Port: 993, Security: SecuritySSL, Source: SourceGuess},
		{Host: "imap.example.com", Port: 993, Security: SecuritySSL, Source: SourceAutoconfig},
		{Host: "mail.example.com", Port: 143, Security: SecuritySTARTTLS, Source: SourceGuess},
	})
	if len(list) != 2 {
		t.Fatalf("got %d candidates, want 2: %+v", len(list), list)
	}
	if list[0].Source != SourceAutoconfig {
		t.Errorf("the published entry did not win: %+v", list[0])
	}
}

// Microsoft refuses a password outright and Google wants a purpose-made one.
// Reporting both as "authentication failed" sends the customer off retyping a
// password that is not the problem.
func TestWellKnownProvidersAreNamedBeforeThePasswordIsBlamed(t *testing.T) {
	cases := map[string]string{
		"outlook.office365.com": ReasonBasicAuthDisabled,
		"imap-mail.outlook.com": ReasonBasicAuthDisabled,
		"imap.gmail.com":        ReasonAppPasswordRequired,
		"imap.mail.yahoo.com":   ReasonAppPasswordRequired,
		"imap.yandex.com":       ReasonAppPasswordRequired,
	}
	for host, want := range cases {
		if got := providerHint(host, "someone@"+host); got != want {
			t.Errorf("providerHint(%q) = %q, want %q", host, got, want)
		}
	}
}

// The other direction: an ordinary host must not collect a hint, or every failed
// login would be blamed on a provider policy that does not exist.
func TestAnOrdinaryHostGetsNoProviderHint(t *testing.T) {
	for _, host := range []string{"imap.example.com", "mail.kucukfirma.com.tr", "mx.acme.co"} {
		if got := providerHint(host, "info@"+host); got != "" {
			t.Errorf("providerHint(%q) = %q, want no hint", host, got)
		}
	}
}

// The document is fetched from a host the customer named, so the parser has to
// take only what it understands and refuse the rest.
func TestAutoconfigParsingTakesOnlyUsableImapEntries(t *testing.T) {
	// httptest listens on loopback, which the SSRF guard refuses by design.
	t.Setenv("SERVIKA_ALLOW_PRIVATE_TARGETS", "1")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<clientConfig><emailProvider id="example.com">
      <incomingServer type="pop3">
        <hostname>pop.example.com</hostname><port>995</port><socketType>SSL</socketType>
      </incomingServer>
      <incomingServer type="imap">
        <hostname>imap.example.com</hostname><port>993</port><socketType>SSL</socketType>
      </incomingServer>
      <incomingServer type="imap">
        <hostname>evil.example.com/../x</hostname><port>993</port><socketType>SSL</socketType>
      </incomingServer>
      <incomingServer type="imap">
        <hostname>weird.example.com</hostname><port>0</port><socketType>SSL</socketType>
      </incomingServer>
      <incomingServer type="imap">
        <hostname>legacy.example.com</hostname><port>143</port><socketType>UNKNOWN</socketType>
      </incomingServer>
    </emailProvider></clientConfig>`))
	}))
	defer server.Close()

	list := fetchClientConfig(context.Background(), server.URL, SourceAutoconfig)
	if len(list) != 1 {
		t.Fatalf("got %d candidates, want only the usable IMAP entry: %+v", len(list), list)
	}
	got := list[0]
	if got.Host != "imap.example.com" || got.Port != 993 || got.Security != SecuritySSL || got.Source != SourceAutoconfig {
		t.Errorf("candidate = %+v", got)
	}
}

// Only the domain being asked about has any standing to say which server holds
// its mail, because the answer decides where the customer will type their old
// password. A redirect would let another host supply that answer under the
// domain's name.
func TestAutoconfigWillNotTakeSettingsFromARedirect(t *testing.T) {
	t.Setenv("SERVIKA_ALLOW_PRIVATE_TARGETS", "1")

	const document = `<clientConfig><emailProvider id="example.com">
      <incomingServer type="imap">
        <hostname>imap.example.com</hostname><port>993</port><socketType>SSL</socketType>
      </incomingServer>
    </emailProvider></clientConfig>`

	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(document))
	}))
	defer elsewhere.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusFound)
	}))
	defer redirector.Close()

	if list := fetchClientConfig(context.Background(), redirector.URL, SourceAutoconfig); len(list) != 0 {
		t.Errorf("a redirect supplied %d candidate(s): %+v", len(list), list)
	}
	// The same document served directly still counts, or the refusal above would
	// be free and the whole fetch would be dead.
	if list := fetchClientConfig(context.Background(), elsewhere.URL, SourceAutoconfig); len(list) != 1 {
		t.Errorf("the document served directly produced %d candidates, want 1", len(list))
	}
}

// A document that is not XML at all, or is enormous, must produce nothing rather
// than an error the customer cannot act on.
func TestAutoconfigParsingSurvivesRubbish(t *testing.T) {
	t.Setenv("SERVIKA_ALLOW_PRIVATE_TARGETS", "1")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><body>404 not found</body></html>"))
	}))
	defer server.Close()

	if list := fetchClientConfig(context.Background(), server.URL, SourceAutoconfig); len(list) != 0 {
		t.Errorf("got %+v, want nothing", list)
	}
}

// An address without a usable domain must not start a sweep of DNS lookups and
// TCP connections.
func TestDiscoveryRefusesAnAddressWithoutADomain(t *testing.T) {
	for _, address := range []string{"", "not-an-address", "user@", "user@localhost"} {
		if list := DiscoverCandidates(context.Background(), address); len(list) != 0 {
			t.Errorf("%q produced %+v", address, list)
		}
	}
}

// The conventional hosts are worth one connection each even when a domain
// publishes nothing, which is the common case for a small mail server.
func TestGuessesCoverTheConventionalHosts(t *testing.T) {
	list := fromGuesses(context.Background(), "example.com", "info@example.com")
	seen := map[string]bool{}
	for _, candidate := range list {
		seen[candidate.Host] = true
	}
	for _, host := range []string{"mail.example.com", "imap.example.com", "example.com"} {
		if !seen[host] {
			t.Errorf("%q was not guessed", host)
		}
	}
}
