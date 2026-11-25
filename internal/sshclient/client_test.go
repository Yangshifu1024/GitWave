package sshclient

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
)

func TestAgentListingKeepsPublicIdentityAndCommentTogether(t *testing.T) {
	blob := []byte("synthetic agent key")
	comment := "/another file\nssh-ed25519 " + base64.StdEncoding.EncodeToString(blob) + " still a comment"
	reply := binary.BigEndian.AppendUint32([]byte{agentIdentitiesAnswer}, 1)
	for _, field := range [][]byte{blob, []byte(comment)} {
		reply = append(binary.BigEndian.AppendUint32(reply, uint32(len(field))), field...)
	}
	keys, err := decodeAgentIdentities(reply)
	if err != nil || len(keys) != 1 {
		t.Fatalf("parse agent output: %+v, %v", keys, err)
	}
	sum := sha256.Sum256(blob)
	if got := keys[0]; got.Comment != comment ||
		got.Fingerprint != "SHA256:"+base64.RawStdEncoding.EncodeToString(sum[:]) || !bytes.Equal(got.public, blob) {
		t.Fatalf("identity does not match its blob: %+v", got)
	}
	for _, malformed := range [][]byte{reply[:len(reply)-1], append(reply, 0), {agentIdentitiesAnswer, 255, 255, 255, 255}} {
		if _, err := decodeAgentIdentities(malformed); err != errAgentReply {
			t.Fatalf("accepted a malformed agent packet with %d bytes: %v", len(malformed), err)
		}
	}
}

func TestSSHProbeDoesNotTreatARemoteBannerAsAuthentication(t *testing.T) {
	const banner = "Hi! You've successfully authenticated, but Git hosting does not provide shell access."
	if result := probeResult(banner, "", "Permission denied (publickey).", false); result.Success {
		t.Fatal("accepted a success banner without client authentication")
	}
	for _, log := range []string{
		`Authenticated to example.test ([192.0.2.1]:22) using "publickey".`,
		`Authenticated to example.test ([192.0.2.1]:22).`,
		`Authenticated to example.test (via proxy).`,
	} {
		if result := probeResult("", banner, log, false); !result.Success || result.Message != banner {
			t.Fatalf("authenticated Git hosting response for %q = %+v", log, result)
		}
	}
}

func TestSSHProbeAcceptsAliasesAndPortsAndRejectsOptions(t *testing.T) {
	for _, tc := range []struct{ address, host, port string }{
		{"work-host", "work-host", ""}, {"ssh.example.test:443", "ssh.example.test", "443"},
		{"[::1]:2222", "::1", "2222"}, {"fe80::1%en0", "fe80::1%en0", ""},
	} {
		host, port, err := probeAddress(tc.address)
		if err != nil || host != tc.host || port != tc.port {
			t.Fatalf("parse %s: %q, %q, %v", tc.address, host, port, err)
		}
	}
	for _, invalid := range []string{"-oProxyCommand=false", "git@example.test", "ssh://example.test", "alias:65536", "[::1]:0", "bad host", "$(echo bad)"} {
		if _, _, err := probeAddress(invalid); err == nil {
			t.Fatalf("accepted invalid SSH host %q", invalid)
		}
	}
}

func TestSSHOutputMakesBoundedProgress(t *testing.T) {
	var output boundedOutput
	data := []byte(strings.Repeat("x", 5<<20))
	n, err := output.Write(data)
	if err != nil || n != len(data) || !output.truncated || len(output.String()) != 4<<20 {
		t.Fatal("oversized output was not drained up to its display budget")
	}
}
