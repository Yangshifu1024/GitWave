package main

import (
	"context"
	"sync"

	"gitwave.work/internal/gitcli"
)

type repository struct {
	Branch   string
	Head     string
	Shallow  string
	Branches []branch
	Tags     []tagRef
	Remotes  []string
}

type branch struct {
	Name        string
	Kind        string
	Current     bool   `json:"is_current"`
	Tip         string `json:"last_commit_sha"`
	Upstream    string
	UpstreamRef string
	Time        int64
}

type tagRef struct {
	Name, Tip, Object string
	Annotated         bool
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

type commitDetails = gitcli.Commit

type historyPage struct {
	Commits   []commit
	Cursor    string `json:"next_cursor"`
	More      bool   `json:"has_more"`
	Truncated bool   `json:"snapshot_truncated"`
	Size      int    `json:"snapshot_size"`
}

type fileDiff = gitcli.FileDiff
type diffHunk = gitcli.DiffHunk
type diffLine = gitcli.DiffLine
type preview = gitcli.DiffPreview

type backend struct {
	repo      *gitcli.Repository
	historyMu sync.Mutex
	snapshot  *historySnapshot
	historyID uint64
}

func startBackend(ctx context.Context, path string) (*backend, error) {
	repo, err := gitcli.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	return &backend{repo: repo}, nil
}

func startWorkspaceBackend(ctx context.Context, path string) (*backend, error) {
	repo, err := gitcli.OpenRoot(ctx, path)
	if err != nil {
		return nil, err
	}
	return &backend{repo: repo}, nil
}
