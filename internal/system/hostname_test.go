package system

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateHostname(t *testing.T) {
	valid := map[string]string{
		"panel.example.com":     "panel.example.com",
		" WEB-01.Example.COM. ": "web-01.example.com",
		"server-1":              "server-1",
	}
	for input, want := range valid {
		got, err := validateHostname(input)
		if err != nil || got != want {
			t.Fatalf("%q: got=%q err=%v", input, got, err)
		}
	}
	for _, input := range []string{"", "localhost", "127.0.0.1", "-server", "server-", "a..b", "server_1", strings.Repeat("a", 64)} {
		if _, err := validateHostname(input); err == nil {
			t.Fatalf("%q should have been rejected", input)
		}
	}
}

func TestHostnameSave(t *testing.T) {
	tmp := t.TempDir()
	oldHostname, oldCloud, oldNM, oldApplier := hostnameFile, cloudInitFile, networkMgrFile, hostnameApplier
	t.Cleanup(func() {
		hostnameFile, cloudInitFile, networkMgrFile, hostnameApplier = oldHostname, oldCloud, oldNM, oldApplier
	})
	hostnameFile = filepath.Join(tmp, "etc/hostname")
	cloudInitFile = filepath.Join(tmp, "etc/cloud/cloud.cfg.d/99-servika-hostname.cfg")
	networkMgrFile = filepath.Join(tmp, "etc/NetworkManager/conf.d/99-servika-hostname.conf")
	var applied string
	hostnameApplier = func(_ context.Context, name string) error { applied = name; return nil }

	req := httptest.NewRequest(http.MethodPut, "/system/hostname", strings.NewReader(`{"hostname":"Panel.Example.COM"}`))
	rec := httptest.NewRecorder()
	HostnameSave(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if applied != "panel.example.com" {
		t.Fatalf("applied=%q", applied)
	}
	for path, part := range map[string]string{
		hostnameFile:   "panel.example.com\n",
		cloudInitFile:  "preserve_hostname: true",
		networkMgrFile: "hostname-mode=none",
	} {
		b, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(b), part) {
			t.Fatalf("%s: content=%q err=%v", path, b, err)
		}
	}
}

func TestHostnameSaveRejectsInvalid(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/system/hostname", strings.NewReader(`{"hostname":"bad_name"}`))
	rec := httptest.NewRecorder()
	HostnameSave(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
