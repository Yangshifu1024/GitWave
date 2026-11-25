package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestHistoryOrdersEqualTimestampsAcrossPages(t *testing.T) {
	// This fixture exercises a mixture of packed and loose refs.
	t.Setenv("GIT_DEFAULT_REF_FORMAT", "files")
	useFixtureCommitIdentity(t)
	repo, git := historyRepository(t)
	var stream strings.Builder
	mark := 0
	add := func(branch, message string, parent int, merges ...int) int {
		mark++
		fmt.Fprintf(&stream, "commit refs/heads/%s\nmark :%d\ncommitter GitWave <test@example.invalid> 1700000000 +0000\ndata %d\n%s\n",
			branch, mark, len(message), message)
		if parent != 0 {
			fmt.Fprintf(&stream, "from :%d\n", parent)
		}
		for _, merge := range merges {
			fmt.Fprintf(&stream, "merge :%d\n", merge)
		}
		stream.WriteByte('\n')
		return mark
	}
	root := add("main", "Shared root", 0)
	main := root
	for i := range 269 {
		main = add("main", fmt.Sprintf("Main %d", i), main)
	}
	topic := root
	for i := range 39 {
		topic = add("topic", fmt.Sprintf("Topic %d", i), topic)
	}
	nested := add("topic-long/task", "Nested branch", topic)
	add("main", "Merge topic and nested branch", main, topic, nested)
	add("Alpha", "Alpha tip", root)
	add("Zulu-fork", "Zulu tip", root)
	add("tag-only", "Tag is outside branch history", 0)
	git(stream.String(), "fast-import", "--quiet")
	git("", "tag", "orphan", "tag-only")
	git("", "branch", "-D", "tag-only")
	git("", "pack-refs", "--all")
	git("", "config", "core.ignorecase", "true")
	t.Setenv("GIT_AUTHOR_DATE", "@1700000000 +0000")
	t.Setenv("GIT_COMMITTER_DATE", "@1700000000 +0000")
	tree := git("", "rev-parse", "main^{tree}")
	for _, name := range []string{"A/one", "a-last", "Ä-z", "ä-a"} {
		oid := git("", "commit-tree", tree, "-p", "main~1", "-m", "Loose tip "+name)
		git("", "branch", name, oid)
	}
	git("", "switch", "--detach", "Alpha")
	git("", "commit", "--allow-empty", "-m", "Detached tip")

	ctx := t.Context()
	b, err := startBackend(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := b.fetchHistory(ctx, "", "")
	if err != nil || !first.More || len(first.Commits) != historyPageSize {
		t.Fatalf("first History page: %d commits, more=%v, error=%v", len(first.Commits), first.More, err)
	}
	last, replace, err := b.fetchHistory(ctx, first.Cursor, "")
	// The tag-only commit is outside branch history. The loose tips and
	// detached HEAD are visible.
	if err != nil || replace || last.More || len(first.Commits)+len(last.Commits) != mark+4 {
		t.Fatalf("second History page: %d commits, replace=%v, more=%v, error=%v", len(last.Commits), replace, last.More, err)
	}
	rows := append(first.Commits, last.Commits...)
	firstMessages := []string{"Detached tip", "Loose tip a-last", "Alpha tip", "Loose tip A/one"}
	for i, message := range firstMessages {
		if rows[i].Message != message {
			t.Errorf("History row %d: %q, want %q", i, rows[i].Message, message)
		}
	}
	if rows[len(rows)-1].Message != "Shared root" {
		t.Errorf("last History row: %q", rows[len(rows)-1].Message)
	}
	assertLegacyHistoryRows(t, rows, "5b3c0445d44b17a00fff4234c8ad72f43793001450b6f26ef10eb0374737d034")
}

// Compare the old History order, parent positions, lanes, refs and metadata
// without tying the fixture to a particular Git object hash format.
func assertLegacyHistoryRows(t *testing.T, rows []commit, want string) {
	t.Helper()
	ids := make(map[string]string, len(rows))
	for i, row := range rows {
		ids[row.SHA] = fmt.Sprint(i + 1)
	}
	normalized := slices.Clone(rows)
	for i := range normalized {
		row := &normalized[i]
		row.SHA = ids[row.SHA]
		row.Parents = append([]string(nil), row.Parents...)
		for j := range row.Parents {
			row.Parents[j] = ids[row.Parents[j]]
		}
		if len(row.Refs) == 0 {
			row.Refs = nil
		}
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(encoded)); got != want {
		t.Fatalf("history row fingerprint = %s, want %s", got, want)
	}
}

// Changing only the refs expires a snapshot without changing the
// first 300 rows. The page requested while scrolling must still arrive.
func TestHistoryContinuesAfterRefChanges(t *testing.T) {
	const total = 305
	repo, run := historyRepository(t)
	var stream strings.Builder
	for i := range total {
		fmt.Fprintf(&stream, "commit refs/heads/main\ncommitter GitWave <test@example.invalid> %d +0000\ndata 7\nexample\n\n",
			1700000000+i)
	}
	run(stream.String(), "fast-import", "--quiet")
	ctx := t.Context()
	core, err := startBackend(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	first, replace, err := core.fetchHistory(ctx, "", "")
	if err != nil || !replace || len(first.Commits) != 300 || !first.More || first.Cursor == "" {
		t.Fatalf("first page: %d commits, cursor %q, error %v", len(first.Commits), first.Cursor, err)
	}
	run("", "pack-refs", "--all")
	packed, replace, err := core.fetchHistory(ctx, first.Cursor, "")
	if err != nil || replace || packed.More || len(packed.Commits) != total-historyPageSize {
		t.Fatalf("packing refs expired the cursor: %d commits, replace %v, more %v, error %v",
			len(packed.Commits), replace, packed.More, err)
	}
	run("", "branch", "additional-ref", "HEAD")
	fresh, replace, err := core.fetchHistory(ctx, first.Cursor, "")
	if err != nil || !replace || len(fresh.Commits) != total || fresh.More {
		t.Fatalf("resumed page: %d commits, replace %v, more %v, error %v", len(fresh.Commits), replace, fresh.More, err)
	}
	for i, c := range first.Commits {
		if fresh.Commits[i].SHA != c.SHA {
			t.Fatalf("row %d changed SHA when only its refs changed", i)
		}
	}
}

func TestRefreshHistoryKeepsLoadedDepthAndSkipsUnchangedRefs(t *testing.T) {
	const loaded = 2 * historyPageSize
	repo, git := historyRepository(t)
	var stream strings.Builder
	for i := range loaded + 5 {
		fmt.Fprintf(&stream, "commit refs/heads/main\ncommitter GitWave <test@example.invalid> %d +0000\ndata 7\nexample\n\n",
			1700000000+i)
	}
	git(stream.String(), "fast-import", "--quiet")
	ctx := t.Context()
	b, err := startBackend(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := b.fetchHistory(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := b.fetchHistory(ctx, first.Cursor, "")
	if err != nil || len(second.Commits) != historyPageSize {
		t.Fatalf("second page: %d commits, %v", len(second.Commits), err)
	}
	_, refreshed, err := b.refreshHistory(ctx, "", loaded, false)
	if err != nil || refreshed != nil {
		t.Fatalf("unchanged refs walked again: %v, %v", refreshed != nil, err)
	}
	git("", "commit", "--allow-empty", "-m", "External commit")
	info, refreshed, err := b.refreshHistory(ctx, "", loaded, false)
	if err != nil || refreshed == nil || len(refreshed.Commits) != loaded || !refreshed.More ||
		refreshed.Commits[0].Message != "External commit" || refreshed.Commits[0].SHA != info.Head {
		t.Fatalf("unexpected refreshed History: %v, error %v", refreshed != nil, err)
	}
	last, replace, err := b.fetchHistory(ctx, refreshed.Cursor, "")
	if err != nil || replace || last.More || len(last.Commits) != 6 || last.Commits[0].SHA != second.Commits[len(second.Commits)-1].SHA {
		t.Fatalf("next page after refresh: %d commits, replace=%v, more=%v, error=%v", len(last.Commits), replace, last.More, err)
	}
}

func TestRefreshHistoryDetectsDeepeningWithoutRefChanges(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Shallow parent")
	git("", "commit", "--allow-empty", "-m", "Visible head")
	shallow := filepath.Join(t.TempDir(), "shallow")
	git("", "clone", "--depth=1", "--no-local", repo, shallow)
	ctx := t.Context()
	b, err := startBackend(ctx, shallow)
	if err != nil {
		t.Fatal(err)
	}
	old, _, err := b.fetchHistory(ctx, "", "")
	if err != nil || len(old.Commits) != 1 {
		t.Fatalf("shallow history: %d commits, %v", len(old.Commits), err)
	}
	git("", "-C", shallow, "fetch", "--unshallow")
	_, refreshed, err := b.refreshHistory(ctx, "", 1, false)
	if err != nil || refreshed == nil || len(refreshed.Commits) != 2 || refreshed.Commits[0].SHA != old.Commits[0].SHA ||
		refreshed.Commits[1].Message != "Shallow parent" {
		t.Fatalf("deepened history: %v, %v", refreshed, err)
	}
}

func TestHistorySparseSearchKeepsScanning(t *testing.T) {
	repo, run := historyRepository(t)
	var stream strings.Builder
	for i := range historyScanLimit + 5 {
		message := "subject"
		if i == 0 {
			message += "\n\nNeedle only in the body"
		}
		fmt.Fprintf(&stream, "commit refs/heads/main\ncommitter GitWave <test@example.invalid> %d +0000\ndata %d\n%s\n",
			1700000000+i, len(message), message)
	}
	run(stream.String(), "fast-import", "--quiet")
	ctx := t.Context()
	reader, err := startBackend(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := reader.fetchHistory(ctx, "", "  NEEDLE  ")
	if err != nil || len(first.Commits) != 0 || !first.More || first.Cursor == "" {
		t.Fatalf("first search page: %+v, %v", first, err)
	}
	last, replace, err := reader.fetchHistory(ctx, first.Cursor, "needle")
	if err != nil || replace || last.More || len(last.Commits) != 1 || last.Commits[0].Message != "subject" {
		t.Fatalf("last search page: %+v, replace=%v, %v", last, replace, err)
	}
}

func TestHistoryOpensBareRepository(t *testing.T) {
	repo, run := historyRepository(t)
	run("", "commit", "--allow-empty", "-m", "Bare history")
	hash := run("", "rev-parse", "HEAD")
	bare := t.TempDir()
	run("", "clone", "--bare", "--local", repo, bare)
	ctx := t.Context()
	reader, err := startBackend(ctx, bare)
	if err != nil {
		t.Fatal(err)
	}
	page, _, err := reader.fetchHistory(ctx, "", "")
	if err != nil || len(page.Commits) != 1 || page.Commits[0].SHA != hash {
		t.Fatalf("bare history: %+v, %v", page, err)
	}
}

func TestHistoryIncludesAnnotatedTagAtHeadAndRemoteRef(t *testing.T) {
	useFixtureCommitIdentity(t)
	repo, git := historyRepository(t)
	t.Setenv("GIT_AUTHOR_DATE", "@1700000000 +0000")
	t.Setenv("GIT_COMMITTER_DATE", "@1700000000 +0000")
	git("", "commit", "--allow-empty", "-m", "Root")
	root := git("", "rev-parse", "HEAD")
	tree := git("", "rev-parse", "HEAD^{tree}")
	head := git("", "commit-tree", tree, "-p", root, "-m", "Tagged HEAD")
	remote := git("", "commit-tree", tree, "-p", root, "-m", "Tagged remote tip")
	git("", "tag", "-a", "head-tag", "-m", "HEAD annotation", head)
	git("", "tag", "-a", "remote-tag", "-m", "Remote annotation", remote)
	git("", "update-ref", "refs/remotes/origin/topic", git("", "rev-parse", "refs/tags/remote-tag"))
	git("", "symbolic-ref", "HEAD", "refs/tags/head-tag")

	b, err := startBackend(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	page, _, err := b.fetchHistory(t.Context(), "", "")
	if err != nil || len(page.Commits) != 3 ||
		page.Commits[0].SHA != head || page.Commits[1].SHA != remote || page.Commits[2].SHA != root {
		t.Fatalf("annotated refs: %+v, %v", page.Commits, err)
	}
	assertLegacyHistoryRows(t, page.Commits, "3f53f7451998dfa36a3d694224e26cfe7204a24e4954684a3eb39d46c0a8e0b5")
}

func TestHistoryBranchesTagsAndDetachedHead(t *testing.T) {
	useFixtureCommitIdentity(t)
	repo, run := historyRepository(t)
	ctx := t.Context()
	reader, err := startBackend(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	unborn, err := reader.loadRepository(ctx)
	if err != nil || unborn.Head != "" || !slices.Equal(unborn.Branches, []branch{{Name: "main", Kind: "local", Current: true}}) {
		t.Fatalf("unborn repository: %+v, %v", unborn, err)
	}
	setDate := func(seconds int) {
		stamp := fmt.Sprintf("@%d +0000", 1700000000+seconds)
		t.Setenv("GIT_AUTHOR_DATE", stamp)
		t.Setenv("GIT_COMMITTER_DATE", stamp)
	}
	setDate(0)
	run("", "commit", "--allow-empty", "-m", "根提交", "-m", "A second paragraph")
	root := run("", "rev-parse", "HEAD")
	run("", "tag", "-a", "v1", "-m", "Annotated tag")
	run("", "update-ref", "refs/remotes/origin/main", root)
	run("", "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	run("", "config", "remote.origin.url", "https://example.invalid/repo.git")
	run("", "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	run("", "branch", "--set-upstream-to=origin/main", "main")
	info, err := reader.loadRepository(ctx)
	wantBranches := []branch{
		{Name: "main", Kind: "local", Current: true, Tip: root, Upstream: "origin/main", UpstreamRef: "refs/remotes/origin/main", Time: 1700000000},
		{Name: "origin/main", Kind: "remote", Tip: root, Time: 1700000000},
	}
	if err != nil || info.Branch != "main" || info.Head != root || !slices.Equal(info.Branches, wantBranches) {
		t.Fatalf("repository references: %+v, %v", info, err)
	}
	run("", "branch", "topic")
	setDate(1)
	run("", "commit", "--allow-empty", "-m", "Main child")
	mainChild := run("", "rev-parse", "HEAD")
	run("", "switch", "topic")
	setDate(2)
	run("", "commit", "--allow-empty", "-m", "Topic child")
	topic := run("", "rev-parse", "HEAD")
	run("", "switch", "main")
	setDate(3)
	run("", "merge", "--no-ff", "-m", "Merge topic", "topic")
	merge := run("", "rev-parse", "HEAD")
	run("", "switch", "--detach")
	setDate(4)
	run("", "commit", "--allow-empty", "-m", "Detached commit")
	detached := run("", "rev-parse", "HEAD")
	page, _, err := reader.fetchHistory(ctx, "", "")
	wantOIDs := []string{detached, merge, topic, mainChild, root}
	wantRefs := []commitRef{{"origin/main", "remote_branch"}, {"v1", "tag"}}
	if err != nil || len(page.Commits) != len(wantOIDs) || !slices.Equal(page.Commits[4].Refs, wantRefs) ||
		!slices.Equal(page.Commits[1].Parents, []string{mainChild, topic}) {
		t.Fatalf("detached history: %+v, %v", page, err)
	}
	for i, want := range wantOIDs {
		if page.Commits[i].SHA != want {
			t.Fatalf("commit %d = %s, want %s", i, page.Commits[i].SHA, want)
		}
	}
	details, err := reader.loadCommit(ctx, root)
	if err != nil || details.Message != "根提交\n\nA second paragraph" {
		t.Fatalf("details: %+v, %v", details, err)
	}
	assertLegacyHistoryRows(t, page.Commits, "c121561cd13b69dbb62e40dbd4f3fbab7be637897a9dabec53ad3c0889f7959d")
}

func useFixtureCommitIdentity(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "GitWave")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "GitWave")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.invalid")
}

func historyRepository(t *testing.T) (string, func(string, ...string) string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo := t.TempDir()
	run := func(input string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = repo
		cmd.Stdin = strings.NewReader(input)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	run("", "init", "--template=", "-b", "main")
	run("", "config", "user.name", "GitWave")
	run("", "config", "user.email", "test@example.invalid")
	return repo, run
}
