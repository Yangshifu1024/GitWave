package ai

import (
	"bytes"
	"regexp"
	"strings"
)

var secretLineMarkers = []string{
	"api_key", "api-key", "apikey", "secret", "password", "passwd", "private_key", "private-key", "privatekey",
	"aws_", "bearer", "authorization", "auth_token", "access_token", "refresh_token", "client_secret",
	"credential", "_token", "token_", "sessiontoken", "accesskeyid", "-----begin",
}

var secretPrefixes = []string{
	"ghp_", "gho_", "ghu_", "ghs_", "ghr_", "github_pat_", "glpat-", "sk-",
	"xoxb-", "xoxp-", "xoxa-", "xoxr-", "xoxs-", "xoxo-", "xoxd-", "akia", "aiza", "npm_",
}

var tokenWhitespace = strings.NewReplacer(" ", "", "\t", "", "\"", "", "'", "")
var embeddedCredential = regexp.MustCompile(`://[^\s/?#@:]+:[^\s/?#@]+@`)

// ContainsPrivateKey also works on complete blobs, where a caller can
// recognize a private key before a diff omits its delimiters.
func ContainsPrivateKey(blob []byte) bool {
	const marker = "PRIVATE KEY-----"
	for len(blob) >= len(marker) {
		i := bytes.IndexAny(blob, "Pp")
		if i < 0 || len(blob)-i < len(marker) {
			break
		}
		if bytes.EqualFold(blob[i:i+len(marker)], []byte(marker)) {
			return true
		}
		blob = blob[i+1:]
	}
	return false
}

const redaction = "[REDACTED]"

// Scrub removes sensitive lines and encoded blocks from prompts. Pass Git
// blobs through ContainsPrivateKey before truncating a patch to a few lines.
func Scrub(text string) string {
	lines := strings.Split(text, "\n")
	redact := make([]bool, len(lines))
	insideKey, encodedRun := false, false
	runStart := 0
	for i := range lines {
		line := strings.TrimSuffix(lines[i], "\r")
		lines[i] = line
		if strings.HasPrefix(line, "diff --git ") {
			insideKey = false
		}
		payload := line
		if len(line) > 0 && strings.ContainsRune("+- ", rune(line[0])) && !strings.HasPrefix(line, "-----") {
			payload = line[1:]
		}
		payload = strings.TrimSpace(payload)
		upper := strings.ToUpper(payload)
		begin := strings.Contains(upper, "-----BEGIN ") && strings.Contains(upper, "PRIVATE KEY-----")
		end := strings.Contains(upper, "-----END ") && strings.Contains(upper, "PRIVATE KEY-----")
		redact[i] = insideKey || begin || end || secretLine(line)
		insideKey = (insideKey || begin) && !end
		switch {
		case resemblesBase64(payload):
			encodedRun = encodedRun || len(payload) >= 32
		case !strings.HasPrefix(line, "@@"):
			if encodedRun {
				fillRedaction(redact[runStart:i])
			}
			runStart, encodedRun = i+1, false
		}
	}
	if encodedRun {
		fillRedaction(redact[runStart:])
	}
	for i := range lines {
		if redact[i] && !(i == len(lines)-1 && lines[i] == "" && strings.HasSuffix(text, "\n")) {
			lines[i] = redaction
		}
	}
	return strings.Join(lines, "\n")
}

func fillRedaction(lines []bool) {
	for i := range lines {
		lines[i] = true
	}
}

func resemblesBase64(payload string) bool {
	for i := range len(payload) {
		b := payload[i]
		if (b < 'A' || b > 'Z') && (b < 'a' || b > 'z') && (b < '0' || b > '9') && b != '+' && b != '/' && b != '=' {
			return false
		}
	}
	return payload != ""
}

func secretLine(line string) bool {
	lower := strings.ToLower(line)
	for _, markers := range [][]string{secretLineMarkers, secretPrefixes} {
		for _, marker := range markers {
			if strings.Contains(lower, marker) {
				return true
			}
		}
	}
	spaceless := tokenWhitespace.Replace(lower)
	return strings.Contains(spaceless, "token=") || strings.Contains(spaceless, "token:") || embeddedCredential.MatchString(line)
}
