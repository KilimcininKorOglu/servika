package mail

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"servika/internal/httpx"
)

// Settings that apply to the whole mail server rather than one domain.
//
// Per-mailbox limits were the only ceiling, so a single compromised account
// under a generous plan could still saturate the server's reputation, and there
// was no way to cap a whole domain or a single sending machine. Message size and
// DNSBL were reachable only by editing Postfix by hand, which the panel then had
// no record of.

// ServerSettings is the whole configuration, as the API speaks it.
type ServerSettings struct {
	// MaxMessageSizeMB is 0 when the panel does not manage the limit, which
	// leaves whatever Postfix is already configured with in place.
	MaxMessageSizeMB int `json:"max_message_size_mb"`
	// Hourly ceilings; 0 means no ceiling.
	DomainSendLimitHour int `json:"domain_send_limit_hour"`
	ClientSendLimitHour int `json:"client_send_limit_hour"`
	// DNSBLZones is the space-separated list of blocklist zones.
	DNSBLZones string `json:"dnsbl_zones"`
}

// dnsblZonePattern is what a blocklist zone may look like. The value goes into a
// Postfix parameter, so anything that is not a hostname is refused rather than
// quoted: a stray space or newline there rewrites the restriction list.
//
// At least one dot is required. A single label is never a blocklist zone, and
// accepting one would let a typo in a space-separated list pass as an extra
// entry that then silently matches nothing.
var dnsblZonePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

const (
	maxMessageSizeMB   = 512
	maxSendLimitHour   = 1000000
	maxDNSBLZoneCount  = 8
	postfixSizeParam   = "message_size_limit"
	postfixClientParam = "smtpd_client_restrictions"
)

// ReadServerSettings returns the stored settings.
func ReadServerSettings(ctx context.Context, db *sql.DB) (ServerSettings, error) {
	var settings ServerSettings
	err := db.QueryRowContext(ctx,
		`SELECT max_message_size_mb, domain_send_limit_hour, client_send_limit_hour, dnsbl_zones
		   FROM mail_server_settings WHERE id = 1`).
		Scan(&settings.MaxMessageSizeMB, &settings.DomainSendLimitHour,
			&settings.ClientSendLimitHour, &settings.DNSBLZones)
	if errors.Is(err, sql.ErrNoRows) {
		return ServerSettings{}, nil
	}
	return settings, err
}

// ServerSettingsGet answers GET /admin/mail/settings.
func (h *Handlers) ServerSettingsGet(w http.ResponseWriter, r *http.Request) {
	settings, err := ReadServerSettings(r.Context(), h.DB)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not read the mail settings")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, settings)
}

// ServerSettingsPut saves and applies them. PUT /admin/mail/settings.
//
// The database is written only after Postfix has accepted the change, so a
// rejected configuration cannot leave the panel showing a setting the server is
// not running.
func (h *Handlers) ServerSettingsPut(w http.ResponseWriter, r *http.Request) {
	var req ServerSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	zones, err := validateServerSettings(&req)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.DNSBLZones = strings.Join(zones, " ")

	if err := applyPostfixSettings(req, zones); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "postfix rejected the settings and they were rolled back")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE mail_server_settings
		    SET max_message_size_mb=?, domain_send_limit_hour=?, client_send_limit_hour=?, dnsbl_zones=?
		  WHERE id = 1`,
		req.MaxMessageSizeMB, req.DomainSendLimitHour, req.ClientSendLimitHour, req.DNSBLZones); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not save the mail settings")
		return
	}
	h.audit(r, "mail.server_settings.update", "", true)
	httpx.WriteJSON(w, http.StatusOK, req)
}

// validateServerSettings bounds every value and returns the parsed DNSBL zones.
//
// The error strings here are the only prose the endpoint produces. They name a
// bound rather than a policy, so the interface can show them verbatim without a
// translation for each one.
func validateServerSettings(req *ServerSettings) ([]string, error) {
	if req.MaxMessageSizeMB < 0 || req.MaxMessageSizeMB > maxMessageSizeMB {
		return nil, fmt.Errorf("message size must be between 0 and %d MB", maxMessageSizeMB)
	}
	if req.DomainSendLimitHour < 0 || req.DomainSendLimitHour > maxSendLimitHour ||
		req.ClientSendLimitHour < 0 || req.ClientSendLimitHour > maxSendLimitHour {
		return nil, fmt.Errorf("send limits must be between 0 and %d", maxSendLimitHour)
	}

	zones := strings.Fields(strings.ToLower(req.DNSBLZones))
	if len(zones) > maxDNSBLZoneCount {
		return nil, fmt.Errorf("at most %d blocklist zones are allowed", maxDNSBLZoneCount)
	}
	for _, zone := range zones {
		if !dnsblZonePattern.MatchString(zone) {
			return nil, fmt.Errorf("%q is not a valid blocklist zone name", zone)
		}
	}
	return zones, nil
}

// postfixCommand is a seam so the tests can exercise the apply logic without a
// Postfix installation.
var postfixCommand = func(name string, args ...string) ([]byte, error) {
	// #nosec G204 -- fixed binary names with separate arguments (no shell); every value has been through validateServerSettings.
	return exec.Command(name, args...).CombinedOutput()
}

// applyPostfixSettings writes the two parameters and reloads.
//
// The previous values are read first and restored when Postfix refuses the
// result, because a main.cf the daemon will not start with takes mail down for
// every domain on the server, not just the one whose settings were being edited.
func applyPostfixSettings(settings ServerSettings, zones []string) error {
	if _, err := exec.LookPath("postconf"); err != nil {
		return fmt.Errorf("postfix is not installed")
	}

	previousSize, sizeErr := readPostfixParam(postfixSizeParam)
	previousClient, clientErr := readPostfixParam(postfixClientParam)

	var changes []string
	if settings.MaxMessageSizeMB > 0 {
		changes = append(changes,
			postfixSizeParam+"="+strconv.Itoa(settings.MaxMessageSizeMB*1024*1024))
	}
	changes = append(changes, postfixClientParam+"="+clientRestrictions(zones))

	for _, change := range changes {
		if out, err := postfixCommand("postconf", "-e", change); err != nil {
			restorePostfixParams(sizeErr, previousSize, clientErr, previousClient)
			return fmt.Errorf("postconf rejected %q: %s", change, strings.TrimSpace(string(out)))
		}
	}
	if out, err := postfixCommand("postfix", "check"); err != nil {
		restorePostfixParams(sizeErr, previousSize, clientErr, previousClient)
		return fmt.Errorf("postfix check failed: %s", strings.TrimSpace(string(out)))
	}
	if out, err := postfixCommand("postfix", "reload"); err != nil {
		restorePostfixParams(sizeErr, previousSize, clientErr, previousClient)
		return fmt.Errorf("postfix reload failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// clientRestrictions builds the smtpd_client_restrictions value.
//
// permit_mynetworks and permit_sasl_authenticated come first so a customer
// sending through submission is never measured against a blocklist that
// describes their home connection rather than a spammer.
func clientRestrictions(zones []string) string {
	parts := []string{"permit_mynetworks", "permit_sasl_authenticated"}
	for _, zone := range zones {
		parts = append(parts, "reject_rbl_client "+zone)
	}
	return strings.Join(parts, ", ")
}

func readPostfixParam(name string) (string, error) {
	out, err := postfixCommand("postconf", "-h", name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// restorePostfixParams puts back what was there before. A parameter that could
// not be read in the first place is removed rather than guessed at, which
// returns Postfix to its built-in default.
func restorePostfixParams(sizeErr error, size string, clientErr error, client string) {
	restore := func(name, value string, readErr error) {
		if readErr != nil {
			_, _ = postfixCommand("postconf", "-X", name)
			return
		}
		_, _ = postfixCommand("postconf", "-e", name+"="+value)
	}
	restore(postfixSizeParam, size, sizeErr)
	restore(postfixClientParam, client, clientErr)
	_, _ = postfixCommand("postfix", "reload")
}
