package proxy

import (
	"net/url"
	"slices"
	"strings"
	"testing"
)

func TestProxyModesPreserveBypassAndLaunchIntent(t *testing.T) {
	env := map[string]string{
		"http_proxy": "http://launch:8000", "HTTP_PROXY": "http://stale:8000", "NO_PROXY": "direct.example, 127.0.0.1",
	}
	system := windowsProxy("http=plain:80;secure=tls:8080;socks=unused:1080", "*.office.example;<local>;10.0.0.0/8")
	for _, tc := range []struct {
		settings      Settings
		wantVariables bool
		want          [3]string
	}{
		{Settings{Mode: System}, true, [3]string{"http://launch:8000", "http://tls:8080", ""}},
		{Settings{Mode: Manual, ManualURL: "https://manual:8443"}, true, [3]string{"https://manual:8443", "https://manual:8443", "https://manual:8443"}},
		{Settings{Mode: Off}, false, [3]string{"http://launch:8000", "", ""}},
		{Settings{Mode: Manual}, false, [3]string{"http://launch:8000", "", ""}},
	} {
		p := resolve(tc.settings, system, func(name string) string { return env[name] })
		if (len(p.variables) != 0) != tc.wantVariables {
			t.Fatalf("%+v: child variables = %v", tc.settings, p.variables)
		}
		for i, destination := range []string{"http://internet.example", "https://internet.example", "https://child.office.example"} {
			checkRoute(t, p, destination, tc.want[i])
		}
		wantDirect := ""
		if tc.settings.Mode == Manual && tc.settings.ManualURL != "" {
			wantDirect = tc.settings.ManualURL
		}
		checkRoute(t, p, "https://direct.example", wantDirect)
		for _, destination := range []string{"http://127.18.0.1", "http://[::1]:9000"} {
			checkRoute(t, p, destination, "")
		}
		if tc.wantVariables {
			for _, prefix := range []string{"NO_PROXY=", "no_proxy="} {
				if !slices.ContainsFunc(p.variables, func(entry string) bool {
					return strings.HasPrefix(entry, prefix) && strings.Contains(entry, "direct.example") == (wantDirect == "") &&
						strings.Contains(entry, "127.0.0.1") && strings.Contains(entry, "127.0.0.0/8")
				}) {
					t.Fatalf("missing %s bypass in %v", prefix, p.variables)
				}
			}
		}
	}
}

func TestManualProxyIgnoresABroadLaunchBypass(t *testing.T) {
	getenv := func(name string) string {
		if name == "NO_PROXY" {
			return "*"
		}
		return ""
	}
	p := resolve(Settings{Mode: Manual, ManualURL: "http://manual:8080"}, systemProxy{}, getenv)
	checkRoute(t, p, "https://example.invalid", "http://manual:8080")
	checkRoute(t, p, "http://localhost:11434", "")
	if slices.Contains(p.variables, "NO_PROXY=*") {
		t.Fatal("the all-host bypass reached the network subprocess")
	}
}

func checkRoute(t *testing.T, p policy, destination, want string) {
	t.Helper()
	address, err := url.Parse(destination)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.resolve(address)
	if err != nil || (got == nil && want != "") || (got != nil && got.String() != want) {
		t.Fatalf("route to %s = %v, %v; want %q", destination, got, err, want)
	}
}

func TestMacSystemProxySnapshotsKeepExceptions(t *testing.T) {
	p := macProxy(`<dictionary> {
  ExceptionsList : <array> {
    0 : *.corp.example
    1 : 169.254/16
  }
  HTTPEnable : 0
  HTTPProxy : disabled.proxy
  HTTPSProxy : fd00::12
  HTTPSEnable : 1
  HTTPSPort : 8080
  __SCOPED__ : <dictionary> {
    en0 : <dictionary> {
      HTTPEnable : 1
      HTTPProxy : interface-only.proxy
      HTTPSEnable : 0
      ExceptionsList : <array> {
        0 : interface-only.example
      }
    }
  }
}`)
	if p.http != "" || p.https != "http://[fd00::12]:8080" || !slices.Equal(p.bypass, []string{"corp.example", "169.254.0.0/16"}) {
		t.Fatalf("system proxy = %+v", p)
	}
}
