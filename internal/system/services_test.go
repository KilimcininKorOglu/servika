package system

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withSystemctlRunner(t *testing.T, runner func(...string) string) {
	t.Helper()
	previous := runSystemctl
	runSystemctl = runner
	t.Cleanup(func() { runSystemctl = previous })
}

func TestServiceActionRejectsNonAllowlistedUnit(t *testing.T) {
	called := false
	withSystemctlRunner(t, func(...string) string {
		called = true
		return "active\n"
	})

	request := httptest.NewRequest(http.MethodPost, "/system/service-action", strings.NewReader(`{"unit":"sshd","action":"restart"}`))
	response := httptest.NewRecorder()
	ServiceAction(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("ServiceAction() status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if called {
		t.Fatal("ServiceAction() invoked systemctl for a non-allowlisted unit")
	}
}

func TestServiceActionRejectsUnsupportedAction(t *testing.T) {
	called := false
	withSystemctlRunner(t, func(...string) string {
		called = true
		return "active\n"
	})

	request := httptest.NewRequest(http.MethodPost, "/system/service-action", strings.NewReader(`{"unit":"nginx","action":"stop"}`))
	response := httptest.NewRecorder()
	ServiceAction(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("ServiceAction() status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if called {
		t.Fatal("ServiceAction() invoked systemctl for an unsupported action")
	}
}

func TestServiceActionFallsBackToRestartWhenReloadIsUnsupported(t *testing.T) {
	var calls [][]string
	withSystemctlRunner(t, func(args ...string) string {
		calls = append(calls, append([]string(nil), args...))
		if len(args) > 0 && args[0] == "is-active" {
			return "active\n"
		}
		return ""
	})

	request := httptest.NewRequest(http.MethodPost, "/system/service-action", strings.NewReader(`{"unit":"mariadb","action":"reload"}`))
	response := httptest.NewRecorder()
	ServiceAction(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("ServiceAction() status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if len(calls) != 2 || strings.Join(calls[0], " ") != "restart mariadb" || strings.Join(calls[1], " ") != "is-active mariadb" {
		t.Fatalf("ServiceAction() calls = %v, want restart followed by status check", calls)
	}
	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Action != "restart" {
		t.Fatalf("ServiceAction() action = %q, want restart", body.Action)
	}
}

func TestServiceActionDoesNotExposeSystemctlOutput(t *testing.T) {
	withSystemctlRunner(t, func(args ...string) string {
		if len(args) > 0 && args[0] == "is-active" {
			return "failed\n"
		}
		return "sensitive systemctl detail\n"
	})

	request := httptest.NewRequest(http.MethodPost, "/system/service-action", strings.NewReader(`{"unit":"nginx","action":"restart"}`))
	response := httptest.NewRecorder()
	ServiceAction(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("ServiceAction() status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if strings.Contains(response.Body.String(), "sensitive systemctl detail") || strings.Contains(response.Body.String(), `"status":"failed"`) {
		t.Fatalf("ServiceAction() exposed command details: %s", response.Body.String())
	}
}

func TestServiceStatusesMapsUnknownUnitToAbsent(t *testing.T) {
	withSystemctlRunner(t, func(args ...string) string {
		if len(args) == 2 && args[1] == "nginx" {
			return "unknown\n"
		}
		return "active\n"
	})

	response := httptest.NewRecorder()
	ServiceStatuses(response, httptest.NewRequest(http.MethodGet, "/system/services", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("ServiceStatuses() status = %d, want %d", response.Code, http.StatusOK)
	}
	var statuses []serviceStatus
	if err := json.NewDecoder(response.Body).Decode(&statuses); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(statuses) == 0 || statuses[0].Status != "absent" {
		t.Fatalf("ServiceStatuses() first status = %q, want absent", statuses[0].Status)
	}
}
