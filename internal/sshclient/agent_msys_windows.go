package sshclient

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// Git Bash and Cygwin encode their AF_LOCAL endpoints as Windows files.
func dialWindowsAgentSocket(ctx context.Context, path string) (io.ReadWriteCloser, error) {
	conn, socketErr := dialAgentSocket(ctx, path)
	if socketErr == nil || ctx.Err() != nil {
		return conn, socketErr
	}
	sshAdd, err := systemCommand("ssh-add")
	if err != nil {
		return nil, socketErr
	}
	bin := filepath.Dir(sshAdd)
	id := filepath.Join(bin, "id.exe")
	if _, err := os.Stat(id); err != nil {
		return nil, socketErr
	}
	if strings.HasPrefix(path, "/") {
		path, err = msysAgentValue(ctx, filepath.Join(bin, "cygpath.exe"), "-aw", "--", path)
		if err != nil {
			return nil, socketErr
		}
	}
	if !filepath.IsAbs(path) || strings.HasPrefix(filepath.VolumeName(path), `\`) {
		return nil, socketErr
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 128 {
		return nil, socketErr
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, socketErr
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 129))
	if err != nil || len(data) > 128 {
		return nil, socketErr
	}
	port, cookie, err := msysAgentAddress(data)
	if err != nil {
		return nil, socketErr
	}
	var credentials [12]byte
	binary.LittleEndian.PutUint32(credentials[:4], uint32(os.Getpid()))
	for i, flag := range []string{"-u", "-g"} {
		value, err := msysAgentValue(ctx, id, flag)
		if err != nil {
			return nil, fmt.Errorf("read the MSYS SSH agent user: %w", err)
		}
		number, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return nil, errors.New("MSYS returned an invalid user or group ID")
		}
		binary.LittleEndian.PutUint32(credentials[4+i*4:], uint32(number))
	}
	return connectMSYSAgent(ctx, port, cookie, credentials)
}

func msysAgentValue(ctx context.Context, executable string, args ...string) (string, error) {
	out, diagnostic, err := run(ctx, agentTimeout, executable, args...)
	return strings.TrimRight(out, "\r\n"), withDiagnostic(err, diagnostic)
}

func msysAgentAddress(data []byte) (uint16, [16]byte, error) {
	var cookie [16]byte
	description, ok := strings.CutPrefix(strings.TrimSuffix(string(data), "\x00"), "!<socket >")
	fields := strings.Fields(description)
	if !ok || len(fields) != 3 || fields[1] != "s" {
		return 0, cookie, errAgentReply
	}
	port, err := strconv.ParseUint(fields[0], 10, 16)
	if err != nil || port == 0 {
		return 0, cookie, errAgentReply
	}
	parts := strings.Split(fields[2], "-")
	if len(parts) != 4 {
		return 0, cookie, errAgentReply
	}
	for i, part := range parts {
		number, err := strconv.ParseUint(part, 16, 32)
		if err != nil || len(part) != 8 {
			return 0, cookie, errAgentReply
		}
		binary.LittleEndian.PutUint32(cookie[i*4:], uint32(number))
	}
	// The Windows descriptor stores the port from sockaddr_in in network order.
	return bits.ReverseBytes16(uint16(port)), cookie, nil
}

func connectMSYSAgent(ctx context.Context, port uint16, cookie [16]byte, credentials [12]byte) (_ io.ReadWriteCloser, err error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))))
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, windows.WSAECONNREFUSED) {
		return nil, ErrAgentUnavailable
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	if _, err := conn.Write(cookie[:]); err != nil {
		return nil, err
	}
	var echoed [16]byte
	if _, err := io.ReadFull(conn, echoed[:]); err != nil {
		return nil, err
	}
	if !bytes.Equal(cookie[:], echoed[:]) {
		return nil, errors.New("MSYS SSH agent returned an unexpected socket cookie")
	}
	if _, err := conn.Write(credentials[:]); err != nil {
		return nil, err
	}
	var peer [12]byte
	if _, err := io.ReadFull(conn, peer[:]); err != nil {
		return nil, err
	}
	return conn, nil
}
