package sshclient

import (
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenSSHLoopbackProbeUsesTheLoadedAgentIdentity(t *testing.T) {
	address, user := os.Getenv("GITWAVE_TEST_SSH_ADDRESS"), os.Getenv("GITWAVE_TEST_SSH_USER")
	if address == "" || user == "" {
		t.Skip("enable an isolated OpenSSH loopback fixture")
	}
	host, port, err := probeAddress(address)
	ip, ipErr := netip.ParseAddr(host)
	if err != nil || ipErr != nil || !ip.IsLoopback() || port == "" {
		t.Fatal("the OpenSSH fixture must be on a loopback address and explicit port")
	}
	result, err := Probe(t.Context(), address, user)
	if err != nil || !result.Success || !strings.Contains(result.Message, "fixture has no shell") {
		t.Fatalf("loaded agent key was not confirmed by OpenSSH: %+v, %v", result, err)
	}
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "missing-agent"))
	result, err = Probe(t.Context(), address, user)
	if err != nil || result.Success || !strings.Contains(strings.ToLower(result.Message), "permission denied") {
		t.Fatalf("missing agent key was not reported as an authentication failure: %+v, %v", result, err)
	}
}
