package sshclient

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"syscall"
)

// RFC 9987, sections 5.4 and 5.5.
const (
	agentSuccess           = 6
	agentRequestIdentities = 11
	agentIdentitiesAnswer  = 12
	agentRemoveIdentity    = 18
	agentMaxReply          = 4 << 20
	agentMaxIdentities     = 2048
)

var errAgentReply = errors.New("ssh-agent returned an invalid response")

func List(ctx context.Context) (Listing, error) {
	var listing Listing
	err := useAgent(ctx, func(conn io.ReadWriter) error {
		listing.Running = true
		var err error
		listing.Keys, err = agentIdentities(conn)
		return err
	})
	if errors.Is(err, ErrAgentUnavailable) {
		return Listing{}, nil
	}
	return listing, err
}

// Remove resolves the fingerprint and sends the public blob on the same connection.
func Remove(ctx context.Context, fingerprint string) error {
	return useAgent(ctx, func(conn io.ReadWriter) error {
		keys, err := agentIdentities(conn)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(keys, func(key Key) bool { return key.Fingerprint == fingerprint })
		if i < 0 {
			return ErrKeyChanged
		}
		public := keys[i].public
		request := binary.BigEndian.AppendUint32([]byte{agentRemoveIdentity}, uint32(len(public)))
		reply, err := exchangeAgentMessage(conn, append(request, public...))
		if err != nil {
			return err
		}
		if len(reply) != 1 || reply[0] != agentSuccess {
			return errors.New("ssh-agent did not remove the selected key")
		}
		return nil
	})
}

func useAgent(parent context.Context, action func(io.ReadWriter) error) error {
	ctx, cancel := context.WithTimeout(parent, agentTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	conn, err := dialAgent(ctx)
	if ctx.Err() != nil {
		if conn != nil {
			conn.Close()
		}
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	err = action(conn)
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func dialAgentSocket(ctx context.Context, path string) (io.ReadWriteCloser, error) {
	if path == "" {
		return nil, ErrAgentUnavailable
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", path)
	switch {
	case errors.Is(err, os.ErrNotExist), errors.Is(err, syscall.ECONNREFUSED):
		return nil, ErrAgentUnavailable
	case err != nil:
		return nil, fmt.Errorf("connect to ssh-agent: %w", err)
	default:
		return conn, nil
	}
}

func agentIdentities(conn io.ReadWriter) ([]Key, error) {
	reply, err := exchangeAgentMessage(conn, []byte{agentRequestIdentities})
	if err != nil {
		return nil, err
	}
	return decodeAgentIdentities(reply)
}

func decodeAgentIdentities(reply []byte) ([]Key, error) {
	if len(reply) < 5 || reply[0] != agentIdentitiesAnswer {
		return nil, errAgentReply
	}
	count := binary.BigEndian.Uint32(reply[1:5])
	data := reply[5:]
	if count > agentMaxIdentities || count > uint32(len(data)/9) {
		return nil, errAgentReply
	}
	keys := make([]Key, 0, count)
	seen := make(map[[sha256.Size]byte]bool, count)
	for range count {
		public, rest, okPublic := agentString(data)
		comment, next, okComment := agentString(rest)
		if !okPublic || len(public) == 0 || !okComment {
			return nil, errAgentReply
		}
		digest := sha256.Sum256(public)
		if !seen[digest] {
			keys = append(keys, Key{Fingerprint: "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:]),
				Comment: displayText(string(comment)), public: public})
			seen[digest] = true
		}
		data = next
	}
	if len(data) != 0 {
		return nil, errAgentReply
	}
	return keys, nil
}

func agentString(data []byte) (value, rest []byte, ok bool) {
	if len(data) < 4 {
		return nil, nil, false
	}
	size := binary.BigEndian.Uint32(data[:4])
	data = data[4:]
	if size > uint32(len(data)) {
		return nil, nil, false
	}
	return data[:size], data[size:], true
}

func exchangeAgentMessage(conn io.ReadWriter, request []byte) ([]byte, error) {
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(request)))
	if _, err := conn.Write(append(header[:], request...)); err != nil {
		return nil, fmt.Errorf("send ssh-agent request: %w", err)
	}
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, fmt.Errorf("read ssh-agent response: %w", err)
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > agentMaxReply {
		return nil, errAgentReply
	}
	reply := make([]byte, size)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return nil, fmt.Errorf("read ssh-agent response: %w", err)
	}
	return reply, nil
}
