package dns

import (
	"context"
	"strings"
	"testing"
)

func TestDNSCommandUsesExplicitArgumentsAndEnvironment(t *testing.T) {
	t.Setenv("SERVIKA_JWT_SECRET", "must-not-leak")
	command := dnsCommand(context.Background(), "dig", "+short", "@127.0.0.1", "example.com", "DNSKEY")
	wantArgs := []string{"dig", "+short", "@127.0.0.1", "example.com", "DNSKEY"}
	if strings.Join(command.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("dnsCommand() args = %q, want %q", command.Args, wantArgs)
	}
	environment := strings.Join(command.Env, "\n")
	if strings.Contains(environment, "SERVIKA_JWT_SECRET") {
		t.Fatal("dnsCommand() inherited a panel secret")
	}
	if !strings.Contains(environment, "PATH=/usr/sbin:/usr/bin:/sbin:/bin") {
		t.Fatalf("dnsCommand() environment = %q", command.Env)
	}
}
