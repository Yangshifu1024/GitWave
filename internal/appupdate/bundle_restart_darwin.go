//go:build darwin && !ios

package appupdate

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/unix"
)

const macHandoffArgument = "--gitwave-update-handoff="
const macHandoffTimeout = 30 * time.Second

func (b *macBundle) relaunch(parent context.Context, version, expectedHash string, expected os.FileInfo, args []string) (func(bool) error, error) {
	ctx, cancel := context.WithTimeout(parent, macHandoffTimeout)
	defer cancel()
	if !sameMacDirectory(b.path, expected) || !sameMacDirectory(filepath.Dir(b.path), b.parent) {
		return nil, errMacInstallationChanged
	}
	if err := b.verifyMacUpdate(ctx, b.path, version); err != nil {
		return nil, err
	}
	hash, err := macCodeHash(ctx, b.path)
	if err != nil {
		return nil, err
	}
	if hash != expectedHash {
		return nil, errMacInstallationChanged
	}
	dir, err := os.MkdirTemp("/tmp", "gwu-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(dir)
	address := filepath.Join(dir, "ready")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: address, Net: "unix"})
	if err != nil {
		return nil, err
	}
	defer listener.Close()
	deadline, _ := ctx.Deadline()
	if err := listener.SetDeadline(deadline); err != nil {
		return nil, err
	}
	stopListener := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stopListener()
	if !sameMacDirectory(b.path, expected) {
		return nil, errMacInstallationChanged
	}
	spawnArgs := append([]string{macHandoffArgument + address}, args...)
	process, err := spawnVerifiedMacExecutable(macExecutable(b.path), expectedHash, spawnArgs)
	if err != nil {
		return nil, err
	}
	go func() {
		_, _ = process.Wait()
		_ = listener.Close()
	}()
	keep := false
	defer func() {
		if !keep {
			_ = process.Kill()
		}
	}()
	conn, err := listener.AcceptUnix()
	if err != nil {
		return nil, errors.Join(errors.New("the replacement application did not start"), ctx.Err(), err)
	}
	defer func() {
		if !keep {
			_ = conn.Close()
		}
	}()
	pid, err := macPeerPID(conn)
	if err != nil || pid != process.Pid {
		return nil, errors.New("the replacement application did not provide a valid process identity")
	}
	peerHash, err := macProcessCodeHash(pid)
	if err != nil || peerHash != expectedHash {
		return nil, errors.Join(errors.New("the launched process does not match the verified replacement"), err)
	}
	if !sameMacDirectory(b.path, expected) {
		return nil, errMacInstallationChanged
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	stopConn := context.AfterFunc(ctx, func() { _ = conn.Close() })
	var ready [1]byte
	_, readErr := io.ReadFull(conn, ready[:])
	stopConn()
	if readErr != nil || ready[0] != 1 || ctx.Err() != nil {
		return nil, errors.Join(errors.New("the replacement application did not confirm startup"), ctx.Err(), readErr)
	}
	keep = true
	return func(quit bool) error {
		defer conn.Close()
		if quit {
			_, err := conn.Write([]byte{1})
			return err
		}
		return nil
	}, nil
}

func macProcessCodeHash(pid int) (string, error) {
	// XNU's CS_OPS_CDHASH reads the CodeDirectory selected by exec for this PID.
	symbol, err := purego.Dlsym(purego.RTLD_DEFAULT, "csops")
	if err != nil {
		return "", err
	}
	var csops func(int32, uint32, *[20]byte, uintptr) int32
	purego.RegisterFunc(&csops, symbol)
	var hash [20]byte
	const codeDirectoryHashOperation = 5
	if csops(int32(pid), codeDirectoryHashOperation, &hash, uintptr(len(hash))) != 0 {
		return "", errors.New("could not verify the launched process's code hash")
	}
	return hex.EncodeToString(hash[:]), nil
}

func macCodeHash(ctx context.Context, bundle string) (string, error) {
	output, err := macTool(ctx, "/usr/bin/codesign", "-d", "--verbose=4", "--", bundle)
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(output, "\n") {
		if value, ok := strings.CutPrefix(line, "CDHash="); ok {
			decoded, err := hex.DecodeString(value)
			if err == nil && len(decoded) == 20 {
				return value, nil
			}
		}
	}
	return "", errors.New("could not read the verified application's code hash")
}

func macPeerPID(conn *net.UnixConn) (int, error) {
	fd, err := conn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var pid int
	var peerErr error
	err = fd.Control(func(raw uintptr) {
		peer, err := unix.GetsockoptXucred(int(raw), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if err != nil || peer.Uid != uint32(os.Geteuid()) {
			peerErr = errors.New("unexpected update connection owner")
			return
		}
		pid, peerErr = unix.GetsockoptInt(int(raw), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
	})
	if pid <= 0 {
		peerErr = errors.Join(peerErr, errors.New("missing update process ID"))
	}
	return pid, errors.Join(err, peerErr)
}

func WaitForUpdateParent() error {
	if len(os.Args) < 2 || !strings.HasPrefix(os.Args[1], macHandoffArgument) {
		return nil
	}
	address := strings.TrimPrefix(os.Args[1], macHandoffArgument)
	os.Args = append(os.Args[:1], os.Args[2:]...)
	directory := filepath.Dir(address)
	if filepath.Base(address) != "ready" || filepath.Dir(directory) != "/tmp" || !strings.HasPrefix(filepath.Base(directory), "gwu-") {
		return errors.New("invalid update handoff socket")
	}
	connection, err := net.DialTimeout("unix", address, macHandoffTimeout)
	if err != nil {
		return err
	}
	conn := connection.(*net.UnixConn)
	defer conn.Close()
	parent, err := macPeerPID(conn)
	if err != nil || parent == os.Getpid() {
		return errors.New("the update handoff did not provide a valid parent process")
	}
	queue, err := unix.Kqueue()
	if err != nil {
		return err
	}
	defer unix.Close(queue)
	change := unix.Kevent_t{Ident: uint64(parent), Filter: unix.EVFILT_PROC,
		Flags: unix.EV_ADD | unix.EV_ONESHOT, Fflags: unix.NOTE_EXIT}
	if _, err := unix.Kevent(queue, []unix.Kevent_t{change}, nil, nil); err != nil {
		return err
	}
	if err := conn.SetDeadline(time.Now().Add(macHandoffTimeout)); err != nil {
		return err
	}
	if _, err := conn.Write([]byte{1}); err != nil {
		return err
	}
	var quit [1]byte
	if _, err := io.ReadFull(conn, quit[:]); err != nil || quit[0] != 1 {
		return errors.Join(errors.New("the previous application canceled the restart"), err)
	}
	deadline := time.Now().Add(macHandoffTimeout)
	var events [1]unix.Kevent_t
	for time.Now().Before(deadline) {
		timeout := unix.NsecToTimespec(time.Until(deadline).Nanoseconds())
		n, err := unix.Kevent(queue, nil, events[:], &timeout)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if n == 1 && events[0].Fflags&unix.NOTE_EXIT != 0 {
			return nil
		}
		break
	}
	return errors.New("the previous GitWave process did not exit")
}
