package sshclient

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type ProbeResult struct {
	Success bool
	Message string
}

// Probe uses the same OpenSSH configuration as Git. The command runs only
// when requested and cannot ask for a password or forward the local agent.
func Probe(ctx context.Context, address, user string) (ProbeResult, error) {
	user = strings.TrimSpace(user)
	host, port, err := probeAddress(address)
	if err != nil {
		return ProbeResult{}, err
	}
	if len(user) > 256 || !validName(user, ".-_+@\\") {
		return ProbeResult{}, errors.New("enter a valid SSH user")
	}
	dir, err := os.MkdirTemp("", "gitwave-ssh-probe-")
	if err != nil {
		return ProbeResult{}, err
	}
	defer os.RemoveAll(dir)
	logPath := filepath.Join(dir, "connection.log")
	args := []string{"-T", "-a", "-x", "-l", user, "-E", logPath,
		"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=10",
		"-o", "ClearAllForwardings=yes", "-o", "ControlPath=none", "-o", "PermitLocalCommand=no", "-o", "RemoteCommand=none",
		"-o", "LogLevel=VERBOSE"}
	if port != "" {
		args = append(args, "-p", port)
	}
	stdout, stderr, err := run(ctx, 30*time.Second, "ssh", append(args, "--", host)...)
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return ProbeResult{}, withDiagnostic(err, stderr)
		}
	}
	log, logErr := readClientLog(logPath)
	if logErr != nil && !errors.Is(logErr, os.ErrNotExist) {
		return ProbeResult{}, logErr
	}
	result := probeResult(stdout, stderr, log, err == nil)
	if !result.Success && result.Message == "" && err != nil {
		result.Message = err.Error()
	}
	return result, nil
}

func probeAddress(address string) (host, port string, err error) {
	host = strings.TrimSpace(address)
	if parsed, service, splitErr := net.SplitHostPort(host); splitErr == nil {
		n, parseErr := strconv.ParseUint(service, 10, 16)
		if parseErr != nil || n == 0 {
			return "", "", errors.New("enter a valid SSH port")
		}
		host, port = parsed, strconv.FormatUint(n, 10)
	} else if unwrapped, ok := strings.CutPrefix(host, "["); ok {
		if bare, ended := strings.CutSuffix(unwrapped, "]"); ended && strings.Contains(bare, ":") {
			host = bare
		}
	}
	if len(host) > 1024 || !validName(host, "._-:%") {
		return "", "", errors.New("enter a valid SSH host")
	}
	if strings.ContainsAny(host, ":%") {
		addr, err := netip.ParseAddr(host)
		if err != nil || !addr.Is6() || (addr.Zone() != "" && !validName(addr.Zone(), "._-")) {
			return "", "", errors.New("enter a valid SSH host")
		}
	}
	return host, port, nil
}

func validName(value, extra string) bool {
	if value == "" || strings.HasPrefix(value, "-") {
		return false
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune(extra, r) {
			return false
		}
	}
	return true
}

func readClientLog(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	b, err := io.ReadAll(io.LimitReader(file, 1<<20))
	return string(b), err
}

func probeResult(stdout, stderr, log string, exitedSuccessfully bool) ProbeResult {
	result := ProbeResult{Success: exitedSuccessfully}
	var lastDiagnostic string
	for line := range strings.SplitSeq(log, "\n") {
		line = strings.TrimSpace(line)
		if peer, authenticated := strings.CutPrefix(line, "Authenticated to "); authenticated &&
			(strings.Contains(peer, `) using "`) || (strings.Contains(peer, " (") && strings.HasSuffix(peer, ")."))) {
			// OpenSSH 8.1 omitted the authentication method from this client log line.
			result.Success = true
		}
		if line != "" && !strings.HasPrefix(line, "debug") && !strings.HasPrefix(line, "OpenSSH_") {
			lastDiagnostic = line
		}
	}
	result.Message = displayText(stdout)
	if result.Message == "" {
		result.Message = displayText(stderr)
	}
	if result.Message == "" {
		result.Message = displayText(lastDiagnostic)
	}
	return result
}
