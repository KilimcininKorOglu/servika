package subdomain

import "testing"

// The decision is extracted because the handlers that use it cannot be reached
// from a test: both read the request's parent domain from the database and one
// of them writes an nginx configuration. What has to be provable is the rule
// itself, which is a claim about three values and nothing else.
func TestAVersionIsLockedOnlyWhenTheTenantRunsItsOwnFPM(t *testing.T) {
	for _, tc := range []struct {
		name            string
		tenantFPMActive bool
		parentVersion   string
		requested       string
		want            bool
	}{
		// A shared master serves a socket per version, so every account on one can
		// have its subdomains on whatever version is installed.
		{name: "a shared master imposes nothing", parentVersion: "8.1", requested: "7.4"},
		// One master, one interpreter. A pool inside it scopes the document root,
		// not the version.
		{
			name:            "a tenant master cannot serve a second version",
			tenantFPMActive: true, parentVersion: "8.1", requested: "7.4", want: true,
		},
		// Asking for what is already running is not a change and must not be
		// refused, or a subdomain could not be created at all on such an account.
		{
			name:            "the parent's own version is always allowed",
			tenantFPMActive: true, parentVersion: "8.1", requested: "8.1",
		},
		{
			name:            "whitespace does not make it a different version",
			tenantFPMActive: true, parentVersion: "8.1", requested: "  8.1  ",
		},
		// An omitted version means "leave it alone", which every path treats as the
		// parent's. Refusing it would break the ordinary create.
		{
			name:            "an omitted version is not a request",
			tenantFPMActive: true, parentVersion: "8.1",
		},
		// Both sides normalise to the default, so they name the same runtime. The
		// comparison is provisioner.SamePHPVersion, which the pool eligibility
		// check uses too, so the refusal here cannot disagree with what the
		// provisioner would have done.
		{
			name:            "two unrecognised versions name the same default",
			tenantFPMActive: true, parentVersion: "not-a-version", requested: "also-not",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := phpVersionLocked(tc.tenantFPMActive, tc.parentVersion, tc.requested); got != tc.want {
				t.Errorf("phpVersionLocked(%v, %q, %q) = %v, want %v",
					tc.tenantFPMActive, tc.parentVersion, tc.requested, got, tc.want)
			}
		})
	}
}
