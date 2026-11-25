package gitcli

import (
	"strings"
	"testing"
)

func TestProxyDiagnosticsSuppressCredentialsFromAHelper(t *testing.T) {
	const diagnostic = "a remote response echoed the unknown password"
	for _, policy := range []struct{ env, config []string }{
		{env: []string{"HTTPS_PROXY=http://private-user@proxy.invalid:8080"}},
		{config: []string{"http://private-user@proxy.invalid:8080"}},
	} {
		got := redactProxyDiagnostic(diagnostic, policy.env, policy.config)
		if !strings.Contains(got, "authenticated proxy") || strings.Contains(got, diagnostic) {
			t.Fatalf("proxy diagnostic = %q", got)
		}
	}
}
