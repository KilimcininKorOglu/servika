package autoconfig

import (
	"encoding/xml"
	"errors"
	"net/mail"
	"strings"
)

// Thunderbird's format, from the Mozilla autoconfig ConfigFileFormat spec: a
// clientConfig root carrying one emailProvider, whose incomingServer/
// outgoingServer children each name a type, hostname, port, socketType,
// username and authentication.

type clientConfig struct {
	XMLName  xml.Name      `xml:"clientConfig"`
	Version  string        `xml:"version,attr"`
	Provider emailProvider `xml:"emailProvider"`
}

type emailProvider struct {
	ID               string         `xml:"id,attr"`
	Domain           string         `xml:"domain"`
	DisplayName      string         `xml:"displayName"`
	DisplayShortName string         `xml:"displayShortName"`
	Incoming         []serverConfig `xml:"incomingServer"`
	Outgoing         []serverConfig `xml:"outgoingServer"`
}

type serverConfig struct {
	Type           string `xml:"type,attr"`
	Hostname       string `xml:"hostname"`
	Port           int    `xml:"port"`
	SocketType     string `xml:"socketType"`
	Username       string `xml:"username"`
	Authentication string `xml:"authentication"`
}

// Outlook's POX format. The two namespaces differ on purpose and are part of the
// contract: the envelope carries the responseschema/2006 namespace and the
// payload inside it carries outlook/responseschema/2006a. Outlook rejects a
// response that puts the payload in the envelope's namespace.
const (
	autodiscoverRequestNS  = "http://schemas.microsoft.com/exchange/autodiscover/outlook/requestschema/2006"
	autodiscoverResponseNS = "http://schemas.microsoft.com/exchange/autodiscover/responseschema/2006"
	autodiscoverPayloadNS  = "http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a"
)

type autodiscoverResponse struct {
	XMLName  xml.Name         `xml:"Autodiscover"`
	Xmlns    string           `xml:"xmlns,attr"`
	Response autodiscoverBody `xml:"Response"`
}

type autodiscoverBody struct {
	Xmlns   string              `xml:"xmlns,attr"`
	Account autodiscoverAccount `xml:"Account"`
}

type autodiscoverAccount struct {
	AccountType string                 `xml:"AccountType"`
	Action      string                 `xml:"Action"`
	Protocols   []autodiscoverProtocol `xml:"Protocol"`
}

// SSL="on" means the connection is encrypted, not that it uses implicit TLS.
// Submission on 587 and IMAP on 143 both negotiate STARTTLS, and Postfix's
// submission service is configured with smtpd_tls_security_level=encrypt, so an
// unencrypted session is refused rather than merely discouraged.
type autodiscoverProtocol struct {
	Type         string `xml:"Type"`
	Server       string `xml:"Server"`
	Port         int    `xml:"Port"`
	LoginName    string `xml:"LoginName"`
	SPA          string `xml:"SPA"`
	SSL          string `xml:"SSL"`
	AuthRequired string `xml:"AuthRequired"`
}

// autodiscoverRequest is only ever decoded, never produced.
type autodiscoverRequest struct {
	XMLName xml.Name `xml:"Autodiscover"`
	Request struct {
		EMailAddress string `xml:"EMailAddress"`
	} `xml:"Request"`
}

var errNoAddress = errors.New("no email address in the request")

// requestedAddress pulls the address out of Outlook's POST body.
//
// The address is validated with net/mail before it goes anywhere near the
// response, so a body carrying markup or control characters is rejected outright
// rather than being escaped and returned. The namespace is checked too: a body
// that is not an Autodiscover request is not one this endpoint should answer.
func requestedAddress(body []byte) (string, error) {
	var request autodiscoverRequest
	if err := xml.Unmarshal(body, &request); err != nil {
		return "", err
	}
	// Some clients send the namespace over https rather than http; both appear in
	// the wild, and neither changes what the request means.
	space := strings.TrimPrefix(request.XMLName.Space, "https://")
	space = strings.TrimPrefix(space, "http://")
	if space != strings.TrimPrefix(autodiscoverRequestNS, "http://") {
		return "", errors.New("not an autodiscover request")
	}

	raw := strings.TrimSpace(request.Request.EMailAddress)
	if raw == "" {
		return "", errNoAddress
	}
	parsed, err := mail.ParseAddress(raw)
	if err != nil {
		return "", err
	}
	// ParseAddress accepts a display name too ("Name <a@b>"); only the address
	// itself may be echoed back, and it must be a bare mailbox.
	if parsed.Name != "" || parsed.Address != raw {
		return "", errors.New("the email address was not a bare mailbox")
	}
	if strings.Count(parsed.Address, "@") != 1 {
		return "", errors.New("the email address had no single domain part")
	}
	return parsed.Address, nil
}
