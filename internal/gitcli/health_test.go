package gitcli

import (
	"bytes"
	"compress/zlib"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestHealthReadsReftableOperationStateFromTheSelectedWorktree(t *testing.T) {
	main := newFixture(t, "--object-format=sha256", "--ref-format=reftable")
	linked := filepath.Join(t.TempDir(), "linked")
	fixtureGit(t, main, "worktree", "add", "--detach", "--", linked)
	fixtureGit(t, linked, "update-ref", "CHERRY_PICK_HEAD", "HEAD")
	fixtureGit(t, linked, "update-ref", "REBASE_HEAD", "HEAD")
	report, err := openFixture(t, linked).Health(t.Context())
	if err != nil || !slices.Equal(report.ConflictResidue, []string{"CHERRY_PICK_HEAD", "REBASE_HEAD"}) ||
		report.BranchCount != 1 || report.Unpushed != nil {
		t.Fatalf("linked checkout health: %+v, %v", report, err)
	}
	other, err := openFixture(t, main).Health(t.Context())
	if err != nil || len(other.ConflictResidue) != 0 {
		t.Fatalf("main checkout inherited another worktree's operation: %+v, %v", other, err)
	}
}

func TestHealthKeepsOtherMetricsWhenBranchObjectsAreMissingOrMalformed(t *testing.T) {
	dir := newFixture(t)
	writeFixture(t, dir, ".git/refs/heads/lost", strings.Repeat("f", 40)+"\n")
	cmd := fixtureCommand(t, dir, "hash-object", "--literally", "-w", "-t", "commit", "--stdin")
	cmd.Stdin = strings.NewReader("malformed object\n")
	oid, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, ".git/refs/heads/broken", string(oid))
	unreadable := strings.Repeat("e", 40)
	if err := os.MkdirAll(filepath.Join(dir, ".git", "objects", unreadable[:2]), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, ".git/objects/"+unreadable[:2]+"/"+unreadable[2:], "not a compressed Git object")
	writeFixture(t, dir, ".git/refs/heads/unreadable", unreadable+"\n")
	truncated := strings.Repeat("b", 40)
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err := writer.Write([]byte("commit 500\x00tree " + strings.Repeat("0", 40) + "\n" + strings.Repeat("a", 454))); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git", "objects", truncated[:2]), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, ".git/objects/"+truncated[:2]+"/"+truncated[2:], string(compressed.Bytes()[:compressed.Len()-5]))
	writeFixture(t, dir, ".git/refs/heads/truncated", truncated+"\n")
	report, err := openFixture(t, dir).Health(t.Context())
	if err != nil || report.BranchCount != 5 || len(report.StaleBranches) != 0 || report.DirtyFiles != 0 {
		t.Fatalf("health with a dangling branch: %+v, %v", report, err)
	}
}

func TestHealthIncludesDirtySubmodulesWithoutUsingTheirFilters(t *testing.T) {
	parent, source := newFixture(t), newFixture(t)
	fixtureGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", "--", source, "child")
	fixtureGit(t, parent, "commit", "-m", "Add submodule")
	child := filepath.Join(parent, "child")
	writeFixture(t, child, "mixed.txt", "modified content\n")
	repo := openFixture(t, parent)
	report, err := repo.Health(t.Context())
	if err != nil || report.DirtyFiles != 1 {
		t.Fatalf("health with a dirty submodule: %+v, %v", report, err)
	}
	fixtureGit(t, child, "config", "filter.outside.clean", "echo ! > filter-ran; cat")
	fixtureGit(t, child, "config", "filter.outside.required", "true")
	writeFixture(t, child, ".gitattributes", "mixed.txt filter=outside\n")
	report, err = repo.Health(t.Context())
	if err != nil || report.DirtyFiles != 1 {
		t.Fatalf("health with a custom submodule filter: %+v, %v", report, err)
	}
	if _, err := os.Stat(filepath.Join(child, "filter-ran")); !os.IsNotExist(err) {
		t.Fatalf("health executed a configured filter: %v", err)
	}
}

func TestHealthFileScanKeepsSortedTopFiveAtBudget(t *testing.T) {
	root := t.TempDir()
	for i, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		path := filepath.Join(root, name)
		writeFixture(t, root, name, "")
		if err := os.Truncate(path, int64(healthLargeFileBytes+i)); err != nil {
			t.Fatal(err)
		}
	}
	files, limited, err := scanHealthFiles(t.Context(), root, 7)
	if err != nil || !limited || len(files) != healthFileLimit {
		t.Fatalf("file scan at budget: %+v, %v", files, err)
	}
	for i := 1; i < len(files); i++ {
		if files[i].SizeBytes >= files[i-1].SizeBytes {
			t.Fatalf("unsorted files at the visit budget: %+v", files)
		}
	}
	files, limited, err = scanHealthFiles(t.Context(), root, healthEntryLimit)
	if err != nil || limited || len(files) != healthFileLimit || files[0].Path != "h" || files[4].Path != "d" {
		t.Fatalf("largest scanned files: %+v, %v", files, err)
	}
}

func TestHealthDoesNotScanLinkedDirectories(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	writeFixture(t, outside, "external", "")
	if err := os.Truncate(filepath.Join(outside, "external"), healthLargeFileBytes+1); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "linked")); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	files, limited, err := scanHealthFiles(t.Context(), dir, healthEntryLimit)
	if err != nil || limited || len(files) != 0 {
		t.Fatalf("a linked directory was scanned: %+v, %v", files, err)
	}
}
