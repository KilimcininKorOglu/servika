package provisioner

import (
	"strings"
	"testing"
)

// TestBuildSecurityHeadersProtectsPreviewViaCSP verifies clickjacking protection
// moved off X-Frame-Options onto an enforced CSP frame-ancestors directive, so the
// panel origin can iframe-preview the tenant site.
func TestBuildSecurityHeadersProtectsPreviewViaCSP(t *testing.T) {
	got := buildSecurityHeaders(VhostOpts{})
	if strings.Contains(got, "X-Frame-Options") {
		t.Error("security headers must not emit X-Frame-Options")
	}
	if !strings.Contains(got, `add_header Content-Security-Policy "frame-ancestors 'self'`) {
		t.Errorf("security headers must enforce frame-ancestors CSP, got:\n%s", got)
	}
}

// TestFramePolicyHeaderFoldsUpgrade confirms upgrade-insecure-requests is folded
// into the enforced frame policy only when HTTPS upgrade is requested.
func TestFramePolicyHeaderFoldsUpgrade(t *testing.T) {
	if strings.Contains(framePolicyHeader("    ", false), "upgrade-insecure-requests") {
		t.Error("frame policy must not upgrade when HTTPS upgrade is disabled")
	}
	if !strings.Contains(framePolicyHeader("    ", true), "upgrade-insecure-requests") {
		t.Error("frame policy must fold in upgrade-insecure-requests when requested")
	}
}

// TestPanelFrameAncestorsAlwaysAllowsSelf ensures the allowlist degrades safely to
// 'self' when no DB handle or public IP is configured.
func TestPanelFrameAncestorsAlwaysAllowsSelf(t *testing.T) {
	t.Setenv("SERVIKA_PUBLIC_IPV4", "")
	if got := panelFrameAncestors(); !strings.HasPrefix(got, "'self'") {
		t.Errorf("frame-ancestors allowlist must start with 'self', got %q", got)
	}
}
