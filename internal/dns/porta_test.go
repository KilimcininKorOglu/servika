package dns

import "testing"

func TestParseBindZoneBasics(t *testing.T) {
	zone := `$ORIGIN example.com.
$TTL 3600
@	IN	SOA	ns1.example.com. admin.example.com. (
			2026010101 ; serial
			3600 ; refresh
			900 ; retry
			1209600 ; expire
			3600 ; minimum
			)
@	3600	IN	A	192.0.2.10
www	IN	CNAME	example.com.
@	IN	MX	0 mail.example.com.
@	IN	TXT	"v=spf1 -all" ; comment
_sip._tcp	IN	SRV	10 5 5060 sip.example.com.
`
	records, soa := parseBindZone(zone, "example.com")
	if soa == nil {
		t.Fatal("SOA was not parsed")
	}
	if soa.Hostmaster != "admin@example.com" {
		t.Fatalf("hostmaster = %q, want admin@example.com", soa.Hostmaster)
	}
	byKey := map[string]Record{}
	for _, r := range records {
		byKey[r.Type+"|"+r.Name] = r
	}
	if r, ok := byKey["MX|@"]; !ok || r.Priority != 0 || r.Value != "mail.example.com" {
		t.Fatalf("MX priority-0 not parsed correctly: %+v", r)
	}
	if r, ok := byKey["TXT|@"]; !ok || r.Value != "v=spf1 -all" {
		t.Fatalf("TXT quote/comment not handled: %+v", r)
	}
	if r, ok := byKey["SRV|_sip._tcp"]; !ok || r.Priority != 10 || r.Value != "5 5060 sip.example.com" {
		t.Fatalf("SRV not parsed correctly: %+v", r)
	}
	if r, ok := byKey["CNAME|www"]; !ok || r.Value != "example.com" {
		t.Fatalf("CNAME trailing dot not trimmed: %+v", r)
	}
}

func TestRenderBindZoneRoundTrip(t *testing.T) {
	soa := defaultSOA("example.com", "")
	in := []Record{
		{Name: "@", Type: "A", Value: "192.0.2.10", TTL: 3600},
		{Name: "@", Type: "MX", Value: "mail.example.com", TTL: 3600, Priority: 0},
		{Name: "www", Type: "CNAME", Value: "example.com", TTL: 3600},
	}
	zone := renderBindZone("example.com", soa, in)
	out, _ := parseBindZone(zone, "example.com")
	if len(out) != len(in) {
		t.Fatalf("round-trip changed record count: got %d, want %d\n%s", len(out), len(in), zone)
	}
}
