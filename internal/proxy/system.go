package proxy

import (
	"net"
	"strconv"
	"strings"
)

func windowsProxy(server, bypass string) systemProxy {
	result := systemProxy{bypass: bypassEntries(bypass)}
	if !strings.Contains(server, "=") {
		result.http = proxyURL(server)
		result.https = result.http
		return result
	}
	for item := range strings.SplitSeq(server, ";") {
		name, address, _ := strings.Cut(item, "=")
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "http":
			result.http = proxyURL(address)
		case "https", "secure":
			result.https = proxyURL(address)
		}
	}
	return result
}

func macProxy(output string) systemProxy {
	settings := make(map[string]string)
	var exceptions []string
	insideExceptions := false
	depth := 0
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "}" {
			depth--
			if depth <= 1 {
				insideExceptions = false
			}
			continue
		}
		key, value, ok := strings.Cut(line, " : ")
		if strings.HasSuffix(line, "<dictionary> {") || strings.HasSuffix(line, "<array> {") {
			if depth == 1 && key == "ExceptionsList" {
				insideExceptions = true
			}
			depth++
			continue
		}
		if !ok {
			continue
		}
		if depth == 2 && insideExceptions {
			exceptions = append(exceptions, bypassEntries(value)...)
		} else if depth == 1 {
			settings[key] = value
		}
	}
	address := func(scheme string) string {
		host, port := settings[scheme+"Proxy"], settings[scheme+"Port"]
		if settings[scheme+"Enable"] != "1" || host == "" {
			return ""
		}
		if port != "" {
			n, err := strconv.Atoi(port)
			if err != nil || n < 1 || n > 65535 {
				return ""
			}
			host = net.JoinHostPort(strings.Trim(strings.TrimSpace(host), "[]"), port)
		}
		return proxyURL(host)
	}
	return systemProxy{http: address("HTTP"), https: address("HTTPS"), bypass: distinctEntries(exceptions)}
}
