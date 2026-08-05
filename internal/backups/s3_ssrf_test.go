package backups

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The S3 endpoint is customer-configurable: the backup-destination routes are
// CustomerScope. Without a guard a destination pointed at the cloud metadata
// address or a loopback service turns the panel into an SSRF proxy, and the
// response body comes back in the connection-test error.
func TestS3RequestsRefuseAnInternalEndpoint(t *testing.T) {
	internal := []string{
		"https://127.0.0.1/",
		"https://169.254.169.254/",
		"https://10.0.0.5/",
		"https://192.168.1.10/",
		"https://172.16.0.1/",
		"https://[::1]/",
	}
	for _, endpoint := range internal {
		t.Run(endpoint, func(t *testing.T) {
			d := &Destination{Type: "s3", Endpoint: endpoint, Bucket: "b", Region: "us-east-1", PathStyle: true}
			err := testS3Connection(context.Background(), d)
			if err == nil {
				t.Fatalf("endpoint %s was accepted", endpoint)
			}
			if !strings.Contains(err.Error(), "not permitted") {
				t.Errorf("error %q does not name the reason, so the operator cannot tell it from a network fault", err)
			}
		})
	}
}

// The upfront lookup can be answered with a public address and the connection
// then made to a private one. The dialer must refuse it regardless.
func TestS3DialerRefusesAnInternalAddressAfterResolution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	request, err := http.NewRequest(http.MethodHead, server.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	// Straight to the client, skipping requireExternalEndpoint, so only the
	// dialer stands between the request and the loopback listener.
	response, err := s3HTTPClient.Do(request)
	if err == nil {
		_ = response.Body.Close()
		t.Fatal("the dialer connected to a loopback listener")
	}
	if !strings.Contains(err.Error(), "not permitted") {
		t.Errorf("error %q is not the guard's refusal", err)
	}
}

// A public endpoint must still be reachable, or the guard has broken the
// feature it protects.
func TestS3GuardAllowsAPublicEndpoint(t *testing.T) {
	request, err := http.NewRequest(http.MethodHead, "https://s3.us-east-1.amazonaws.com/bucket/key", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if err := requireExternalEndpoint(request); err != nil {
		if _, lookupErr := net.LookupIP("s3.us-east-1.amazonaws.com"); lookupErr != nil {
			t.Skipf("no DNS available here: %v", lookupErr)
		}
		t.Errorf("requireExternalEndpoint rejected a public S3 endpoint: %v", err)
	}
}
