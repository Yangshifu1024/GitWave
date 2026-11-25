package gitcli

import (
	"fmt"
	"strings"
	"testing"
)

const branchHeaders = "# branch.oid abc\x00# branch.head main\x00"

func TestParsesRawPathsAndBranchTracking(t *testing.T) {
	const rename = "2 R. N... 100644 100644 100644 abc abc R100 new\nname\x00old\tname\x00"
	const upstream = "# branch.upstream origin/main\x00# branch.ab +2 -1\x00"
	status, err := parseStatus(branchHeaders+upstream+"# branch.future value\x00"+rename+"? 字\n.txt\x00", true)
	if err != nil || status.Upstream != "origin/main" || status.Ahead != 2 || status.Behind != 1 {
		t.Fatalf("parsed status = %+v, err = %v", status, err)
	}
	if len(status.Changes) != 2 || status.Changes[0].Path != "new\nname" || status.Changes[0].PreviousPath != "old\tname" || status.Changes[1].Path != "字\n.txt" {
		t.Fatalf("raw paths were changed: %+v", status.Changes)
	}
}

func TestRejectsIncompleteStatus(t *testing.T) {
	for _, output := range []string{
		"",
		branchHeaders + "? no terminator",
		branchHeaders + "# branch.ab +2 -x\x00",
		branchHeaders + "2 R. N... 100644 100644 100644 abc abc R100 missing-original\x00",
	} {
		if _, err := parseStatus(output, true); err == nil {
			t.Errorf("accepted incomplete status %q", output)
		}
	}
}

func TestStatusTracksTheCheckedOutBranch(t *testing.T) {
	for _, format := range []string{"files", "reftable"} {
		t.Run(format, func(t *testing.T) {
			dir := newFixture(t, "--ref-format="+format)
			fixtureGit(t, dir, "switch", "-c", "remote-side")
			fixtureGit(t, dir, "commit", "--allow-empty", "-m", "Remote change")
			remoteTip := gitValue(t, dir, "rev-parse", "HEAD")
			fixtureGit(t, dir, "switch", "main")
			fixtureGit(t, dir, "commit", "--allow-empty", "-m", "First local change")
			fixtureGit(t, dir, "commit", "--allow-empty", "-m", "Second local change")
			head := gitValue(t, dir, "rev-parse", "HEAD")
			fixtureGit(t, dir, "remote", "add", "origin", "https://example.invalid/repo.git")
			fixtureGit(t, dir, "update-ref", "refs/remotes/origin/main", remoteTip)
			fixtureGit(t, dir, "branch", "--set-upstream-to=origin/main")
			repo := openFixture(t, dir)
			if format == "files" {
				// An unrelated missing object must not prevent Changes from opening.
				writeFixture(t, repo.CommonDir, "refs/tags/unavailable", strings.Repeat("1", len(head))+"\n")
			}
			check := func(upstream string, ahead, behind int) {
				t.Helper()
				status := readFixtureStatus(t, repo)
				if status.HeadOID != head || status.HeadRef != "refs/heads/main" || status.Branch != "main" ||
					status.Upstream != upstream || status.Ahead != ahead || status.Behind != behind || len(status.Changes) != 0 {
					t.Fatalf("unexpected checkout status: %+v", status)
				}
			}
			check("origin/main", 2, 1)
			fixtureGit(t, dir, "config", "remote.origin.fetch", "+refs/heads/*:refs/tracking/origin/*")
			fixtureGit(t, dir, "update-ref", "refs/tracking/origin/main", remoteTip)
			check("refs/tracking/origin/main", 2, 1)
			fixtureGit(t, dir, "update-ref", "-d", "refs/tracking/origin/main")
			check("refs/tracking/origin/main", 0, 0)
			broken := statusCommitWithMissingParent(t, dir)
			fixtureGit(t, dir, "update-ref", "refs/tracking/origin/main", broken)
			check("refs/tracking/origin/main", 0, 0)
			fixtureGit(t, dir, "update-ref", "-d", "refs/remotes/origin/main")
			fixtureGit(t, dir, "config", "remote.origin.fetch", "+refs/heads/main:refs/remotes")
			fixtureGit(t, dir, "update-ref", "refs/remotes", broken)
			if status, err := repo.Status(t.Context()); err == nil {
				t.Fatalf("accepted corrupt upstream at the refs/remotes namespace itself: %+v", status)
			}
			fixtureGit(t, dir, "update-ref", "-d", "refs/remotes")

			fixtureGit(t, dir, "switch", "--detach", head)
			if status := readFixtureStatus(t, repo); status.HeadOID != head || status.HeadRef != "" || status.Branch != "(detached)" || status.Upstream != "" ||
				status.Ahead != 0 || status.Behind != 0 || len(status.Changes) != 0 {
				t.Fatalf("working status followed a branch in detached HEAD: %+v", status)
			}
		})
	}
}

func TestStatusKeepsUnbornHeadSeparateFromPrefixedRefs(t *testing.T) {
	dir := newFixture(t)
	const child = "next/deep/child"
	fixtureGit(t, dir, "update-ref", "refs/heads/"+child, statusCommitWithMissingParent(t, dir))
	fixtureGit(t, dir, "config", "branch."+child+".remote", ".")
	fixtureGit(t, dir, "config", "branch."+child+".merge", "refs/heads/main")
	fixtureGit(t, dir, "symbolic-ref", "HEAD", "refs/heads/next")
	status := readFixtureStatus(t, openFixture(t, dir))
	if status.HeadOID != "(initial)" || status.HeadRef != "refs/heads/next" || status.Branch != "next" || status.Upstream != "" {
		t.Fatalf("working status followed a descendant of an unborn branch: %+v", status)
	}
}

func statusCommitWithMissingParent(t *testing.T, dir string) string {
	t.Helper()
	tree := gitValue(t, dir, "rev-parse", "HEAD^{tree}")
	input := fmt.Sprintf("tree %s\nparent %s\nauthor Fixture <test@example.invalid> 10 +0000\ncommitter Fixture <test@example.invalid> 10 +0000\n\nMissing ancestor\n", tree, strings.Repeat("1", len(tree)))
	command := fixtureCommand(t, dir, "hash-object", "-w", "-t", "commit", "--stdin")
	command.Stdin = strings.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("create commit with missing parent: %v\n%s", err, output)
	}
	return strings.TrimSpace(string(output))
}
