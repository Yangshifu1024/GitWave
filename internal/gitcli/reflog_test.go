package gitcli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReflogKeepsExactPreviousCommitAfterAnotherRecordWasDeleted(t *testing.T) {
	for _, tc := range []struct{ refs, objects string }{
		{"files", "sha1"}, {"reftable", "sha1"}, {"reftable", "sha256"},
	} {
		t.Run(tc.refs+"-"+tc.objects, func(t *testing.T) {
			dir := newFixture(t, "--ref-format="+tc.refs, "--object-format="+tc.objects)
			repo := openFixture(t, dir)
			base := gitValue(t, dir, "rev-parse", "HEAD")
			fixtureGit(t, dir, "commit", "--allow-empty", "-m", "Second")
			fixtureGit(t, dir, "commit", "--allow-empty", "-m", "Third")
			discarded := gitValue(t, dir, "rev-parse", "HEAD")
			fixtureGit(t, dir, "reset", "--hard", base)
			fixtureGit(t, dir, "reflog", "delete", "refs/heads/main@{1}")
			entries, err := repo.Reflog(t.Context(), "main")
			if err != nil || len(entries) != 3 {
				t.Fatalf("reflog: %+v, %v", entries, err)
			}
			e := entries[0]
			if e.Action != "reset" || e.OldOID != discarded || e.NewOID != base || e.RecoveryOID() != discarded ||
				e.Committer != "GitWave Test" || e.Time == 0 || entries[1].NewOID == discarded {
				t.Fatalf("incorrect recovery target: %+v, older %+v", e, entries[1])
			}
			if head, err := repo.Reflog(t.Context(), "HEAD"); err != nil || len(head) != 4 || head[1].NewOID != discarded {
				t.Fatalf("HEAD reflog was affected: %+v, %v", head, err)
			}
		})
	}
}

func TestReflogReadsLogOnlyReftable(t *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			dir := newFixture(t, "--ref-format=reftable", "--object-format="+format)
			help, _ := fixtureCommand(t, dir, "reflog", "-h").CombinedOutput()
			if !strings.Contains(string(help), "git reflog write") {
				t.Skip("this Git predates reflog write")
			}
			repo := openFixture(t, dir)
			head := gitValue(t, dir, "rev-parse", "HEAD")
			cmd := fixtureCommand(t, dir, "-c", "reftable.autoCompact=false", "reflog", "write", "refs/heads/main", head, head, "log-only event")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("write log-only table: %s, %v", out, err)
			}
			entries, err := repo.Reflog(t.Context(), "main")
			if err != nil || len(entries) != 2 || entries[0].OldOID != head || entries[0].NewOID != head || entries[0].Message != "log-only event" {
				t.Fatalf("log-only table was not included: %+v, %v", entries, err)
			}
		})
	}
}

func TestReflogReadsCompactedLinkedReftable(t *testing.T) {
	dir := newFixture(t, "--ref-format=reftable", "--object-format=sha256")
	linked := filepath.Join(t.TempDir(), "linked")
	fixtureGit(t, dir, "worktree", "add", "-b", "side", linked)
	for i := range 22 {
		fixtureGit(t, linked, "commit", "--allow-empty", "-m", fmt.Sprintf("Linked subject %02d", i))
	}
	fixtureGit(t, linked, "-c", "reftable.blockSize=512", "pack-refs", "--all")
	repo := openFixture(t, linked)
	head, err := repo.Reflog(t.Context(), "HEAD")
	if err != nil || len(head) < 22 || head[0].Message != "commit: Linked subject 21" {
		t.Fatalf("linked HEAD log: %+v, %v", head, err)
	}
	branch, err := repo.Reflog(t.Context(), "refs/heads/side")
	if err != nil || len(branch) != 23 || branch[0].NewOID != head[0].NewOID ||
		branch[0].OldOID != branch[1].NewOID || branch[22].Action != "branch" {
		t.Fatalf("linked branch log: %+v, %v", branch, err)
	}
	main, err := openFixture(t, dir).Reflog(t.Context(), "HEAD")
	if err != nil || len(main) != 1 || main[0].Action != "initial_commit" {
		t.Fatalf("other worktree HEAD log: %+v, %v", main, err)
	}
}

func TestReflogReadsReftableWithMultipleIndexLevels(t *testing.T) {
	dir := newFixture(t, "--ref-format=files")
	repo := openFixture(t, dir)
	head := gitValue(t, dir, "rev-parse", "HEAD")
	log := filepath.Join(repo.CommonDir, "logs", "refs", "heads", "main")
	const count = 320
	file, err := os.OpenFile(log, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	var records strings.Builder
	for i := range count {
		fmt.Fprintf(&records, "%s %s GitWave Test <test@example.invalid> %d +0000\trecord %03d\n", head, head, 1700000000+i, i)
	}
	if _, err := file.WriteString(records.String()); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	migrate := fixtureCommand(t, dir, "-c", "reftable.blockSize=256", "-c", "reftable.restartInterval=1",
		"refs", "migrate", "--ref-format=reftable", "--reflog")
	if out, err := migrate.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "unknown option") || strings.Contains(string(out), "cannot migrate") && strings.Contains(string(out), "reflog") {
			t.Skip("this Git predates reflog migration")
		}
		t.Fatalf("migrate reflog fixture: %s, %v", out, err)
	}
	entries, err := openFixture(t, dir).Reflog(t.Context(), "main")
	if err != nil || len(entries) != count+1 || entries[0].Message != "record 319" || entries[count-1].Message != "record 000" {
		t.Fatalf("multi-level table returned %d records: %v", len(entries), err)
	}
}

func TestReflogIgnoresPartiallyAppendedFileRecord(t *testing.T) {
	dir := newFixture(t, "--ref-format=files")
	repo := openFixture(t, dir)
	log := filepath.Join(repo.GitDir, "logs", "HEAD")
	file, err := os.OpenFile(log, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("truncated reflog data"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := repo.Reflog(t.Context(), "HEAD")
	if err != nil || len(entries) != 1 || entries[0].Action != "initial_commit" || entries[0].OldOID != "" {
		t.Fatalf("partial record changed complete reflog: %+v, %v", entries, err)
	}
}

func TestReflogIncludesFileRecordWithoutMessageSeparator(t *testing.T) {
	dir := newFixture(t, "--ref-format=files")
	repo := openFixture(t, dir)
	base := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "commit", "--allow-empty", "-m", "moved by another client")
	head := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "update-ref", "refs/heads/main", base)
	entries, err := repo.Reflog(t.Context(), "main")
	if err != nil || len(entries) != 3 || entries[0].Message != "" || entries[0].Action != "other" ||
		entries[0].OldOID != head || entries[0].NewOID != base {
		t.Fatalf("reflog without message: %+v, %v", entries, err)
	}
}
