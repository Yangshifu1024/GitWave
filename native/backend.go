package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

type repository struct {
	Branch   string
	Head     string
	Branches []branch
}

type branch struct {
	Name     string
	Kind     string
	Current  bool   `json:"is_current"`
	Tip      string `json:"last_commit_sha"`
	Upstream string
}

type commitRef struct{ Name, Kind string }

type commit struct {
	SHA     string
	Author  string
	Email   string `json:"author_email"`
	Time    int64
	Message string `json:"message_summary"`
	Lane    int
	Parents []string
	Refs    []commitRef
}

type commitDetails struct {
	SHA     string
	Author  string
	Email   string `json:"author_email"`
	Time    int64
	Message string `json:"message_full"`
	Parents []string
}

type historyPage struct {
	Commits   []commit
	Cursor    string `json:"next_cursor"`
	More      bool   `json:"has_more"`
	Truncated bool   `json:"snapshot_truncated"`
	Size      int    `json:"snapshot_size"`
}

type fileDiff struct {
	Path      string
	OldSHA    string `json:"old_sha"`
	NewSHA    string `json:"new_sha"`
	Additions int
	Deletions int
	Hunks     []diffHunk
}

type diffHunk struct {
	OldStart int `json:"old_start"`
	OldLines int `json:"old_lines"`
	NewStart int `json:"new_start"`
	NewLines int `json:"new_lines"`
	Lines    []diffLine
}

type diffLine struct {
	Kind, Content string
	Old           int `json:"old_line_no"`
	New           int `json:"new_line_no"`
}

type preview struct {
	Diff      struct{ Files []fileDiff }
	Truncated bool
	TooLarge  bool `json:"too_large"`
}

type request struct {
	Method   string `json:"method"`
	SHA      string `json:"sha,omitempty"`
	OID      string `json:"oid,omitempty"`
	Path     string `json:"path,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
	Filter   string `json:"filter,omitempty"`
	Expanded bool   `json:"expanded,omitempty"`
}

type response struct {
	Result json.RawMessage
	Error  *backendError
}

type backendError struct{ Code, Message string }

func (err *backendError) Error() string { return err.Message }

// One child per selected repository keeps libgit2's history snapshot alive.
// Concurrent callers share a single stream, so each request owns its reply.
type backend struct {
	callMu  sync.Mutex
	send    *json.Encoder
	receive *json.Decoder
	cancel  context.CancelFunc
}

func startBackend(ctx context.Context, path string) (*backend, error) {
	executable := os.Getenv("GITWAVE_NATIVE_CORE")
	if executable == "" {
		app, err := os.Executable()
		if err != nil {
			return nil, err
		}
		name := "gitwave-native-core"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		executable = filepath.Join(filepath.Dir(app), name)
	}
	childCtx, cancel := context.WithCancel(ctx)
	process := exec.CommandContext(childCtx, executable, path)
	input, err := process.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	output, err := process.StdoutPipe()
	if err != nil {
		cancel()
		input.Close()
		return nil, err
	}
	process.Stderr = os.Stderr
	if err := process.Start(); err != nil {
		cancel()
		input.Close()
		output.Close()
		return nil, fmt.Errorf("start GitWave core: %w", err)
	}
	client := &backend{
		send: json.NewEncoder(input), receive: json.NewDecoder(output),
		cancel: cancel,
	}
	go func() {
		<-childCtx.Done()
		client.callMu.Lock()
		defer client.callMu.Unlock()
		_ = process.Wait()
	}()
	return client, nil
}

func (b *backend) call(ctx context.Context, req request, to any) error {
	b.callMu.Lock()
	defer b.callMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := b.send.Encode(req); err != nil {
		return fmt.Errorf("send to GitWave core: %w", err)
	}
	var reply response
	if err := b.receive.Decode(&reply); err != nil {
		return fmt.Errorf("read from GitWave core: %w", err)
	}
	if reply.Error != nil {
		return reply.Error
	}
	return json.Unmarshal(reply.Result, to)
}

func (b *backend) fetchHistory(ctx context.Context, req request) (historyPage, bool, error) {
	var page historyPage
	err := b.call(ctx, req, &page)
	var remoteErr *backendError
	if req.Cursor == "" || !errors.As(err, &remoteErr) || remoteErr.Code != "git.history_cursor_expired" {
		return page, req.Cursor == "", err
	}

	req.Cursor = ""
	if err := b.call(ctx, req, &page); err != nil || !page.More {
		return page, true, err
	}
	// The virtual list doesn't rearm load-more if page one still has the
	// same rows. Continue the original request against the fresh cursor.
	req.Cursor = page.Cursor
	var next historyPage
	if err := b.call(ctx, req, &next); err != nil {
		return page, true, err
	}
	next.Commits = append(page.Commits, next.Commits...)
	return next, true, nil
}

func (b *backend) close() { b.cancel() }
