package gitcli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitwave.work/internal/ai"
)

const (
	aiPatchBudget = 12_000
	aiBlobLimit   = 2 << 20
	aiFilesLimit  = 128
)

var (
	ErrNoAIStagedChanges = errors.New("stage files before generating a commit message")
	ErrAIStagedChanged   = errors.New("HEAD or the staged changes changed while generating the message")
)

type AICommitSnapshot struct {
	Context string
	head    string
	headRef string
	staged  [32]byte
}

// AICommitContext reads the captured index and its HEAD in an isolated
// worktree. Only staged patches whose complete blobs can be inspected are
// eligible to leave the machine.
func (r *Repository) AICommitContext(ctx context.Context, shown Status) (AICommitSnapshot, error) {
	if r.WorkTree == "" {
		return AICommitSnapshot{}, ErrNoAIStagedChanges
	}
	head := shown.HeadOID
	if head == "(initial)" {
		head = ""
	}
	git, raw, err := r.aiStagedIndex(ctx, head, shown.HeadRef)
	if err != nil {
		return AICommitSnapshot{}, err
	}
	defer git.close()
	deltas, err := parseTreeDeltas(string(raw))
	if err != nil {
		return AICommitSnapshot{}, err
	}
	if !matchesShownAIIndex(deltas, shown.Changes) {
		return AICommitSnapshot{}, ErrAIStagedChanged
	}
	if len(deltas) == 0 {
		return AICommitSnapshot{}, ErrNoAIStagedChanges
	}

	var prompt strings.Builder
	if head != "" {
		r.appendAICommitHistory(ctx, &prompt, head)
	}
	prompt.WriteString("\nStaged changes:\n")
	diff, err := writeAISafePatch(ctx, git.read, deltas, aiStagedArgs(), nil, "Staged diff")
	if err != nil {
		return AICommitSnapshot{}, err
	}
	prompt.WriteString(diff)
	return AICommitSnapshot{Context: prompt.String(), head: head, headRef: shown.HeadRef, staged: sha256.Sum256(raw)}, nil
}

type aiPatchRead func(context.Context, int64, ...string) ([]byte, error)

func writeAISafePatch(ctx context.Context, read aiPatchRead, deltas []treeDelta, args, revisions []string, heading string) (string, error) {
	var prompt, patch strings.Builder
	omitted := 0
	truncated := len(deltas) > aiFilesLimit
	for _, delta := range deltas[:min(aiFilesLimit, len(deltas))] {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if !aiBlobSafe(ctx, read, delta.file.OldSHA, delta.oldMode) ||
			!aiBlobSafe(ctx, read, delta.file.NewSHA, delta.newMode) {
			omitted++
			continue
		}
		if patch.Len() >= aiPatchBudget {
			truncated = true
			if prompt.Len() < 24_000 {
				fmt.Fprintf(&prompt, "- %q\n", delta.file.Path)
			}
			continue
		}
		paths := append(append([]string(nil), revisions...), "--", ":(literal)"+delta.file.Path, ":(exclude,literal)"+delta.file.Path+"/")
		content, err := read(ctx, 4<<20, diffPatchArgs(args, paths)...)
		if err != nil {
			if errors.Is(err, errGitOutputLimit) {
				truncated = true
				if prompt.Len() < 24_000 {
					fmt.Fprintf(&prompt, "- %q\n", delta.file.Path)
				}
				continue
			}
			return "", err
		}
		// Inspect the full blobs above, then scrub the complete file patch
		// before cutting at a line boundary. A cut may hide a secret marker.
		diff := string(content)
		if prompt.Len() < 24_000 {
			file := delta.file
			if _, err := parsePatch(&file, diff, 0, 0, 0); err != nil {
				return "", err
			}
			fmt.Fprintf(&prompt, "- %q (+%d -%d)\n", file.Path, file.Additions, file.Deletions)
		} else {
			truncated = true
		}
		safe := ai.Scrub(diff)
		if len(safe) > aiPatchBudget-patch.Len() {
			safe = safe[:aiPatchBudget-patch.Len()]
			if end := strings.LastIndexByte(safe, '\n'); end >= 0 {
				safe = safe[:end+1]
			} else {
				safe = ""
			}
			truncated = true
		}
		patch.WriteString(safe)
	}
	if omitted > 0 {
		fmt.Fprintf(&prompt, "[%d file(s) omitted from AI context]\n", omitted)
	}
	if strings.TrimSpace(patch.String()) != "" {
		prompt.WriteString("\n" + heading + " (unified format, may be truncated):\n")
		prompt.WriteString(patch.String())
	}
	if truncated {
		prompt.WriteString("\n[diff truncated due to size]\n")
	}
	return prompt.String(), nil
}

func matchesShownAIIndex(deltas []treeDelta, changes []Change) bool {
	shown := make(map[string]IndexStage)
	for _, change := range changes {
		if change.Kind == 'u' {
			return false
		}
		if change.Kind == '?' || change.Kind == '!' || change.Index == '.' {
			continue
		}
		oid := change.IndexOID
		if change.IndexMode == "000000" {
			oid = ""
		}
		shown[change.Path] = IndexStage{OID: oid, Mode: change.IndexMode}
		if change.Index == 'R' {
			shown[change.PreviousPath] = IndexStage{Mode: "000000"}
		}
	}
	if len(shown) != len(deltas) {
		return false
	}
	for _, delta := range deltas {
		if shown[delta.file.Path] != (IndexStage{OID: delta.file.NewSHA, Mode: delta.newMode}) {
			return false
		}
	}
	return true
}

func (r *Repository) appendAICommitHistory(ctx context.Context, prompt *strings.Builder, head string) {
	args := []string{"log", "--no-show-signature", "--no-notes", "--no-patch", "--no-decorate", "--encoding=UTF-8"}
	if recent, err := r.readLimited(ctx, "", 8_000, append(args, "-8", "--format=- %s", head, "--")...); err == nil {
		prompt.WriteString("Recent commits (newest first):\n")
		prompt.WriteString(string(recent))
	}
	if recent, err := r.readLimited(ctx, "", 12_000, append(args, "-3", "--format=---%n%B%n---", head, "--")...); err == nil {
		prompt.WriteString("\nLast 3 commit messages on this branch (style reference):\n")
		prompt.WriteString(string(recent))
	}
}

func (r *Repository) AICommitSubject(ctx context.Context, oid string) string {
	if !validOID(oid) {
		return ""
	}
	raw, err := r.readLimited(ctx, "", 4096, "log", "-1", "--no-notes", "--no-show-signature", "--no-patch",
		"--no-decorate", "--encoding=UTF-8", "--format=%s", oid, "--")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func aiStagedArgs() []string {
	return []string{"-c", "diff.suppressBlankEmpty=false", "diff", "--cached", "--no-ext-diff", "--no-textconv",
		"--no-color", "--no-renames", "--ignore-submodules=dirty", "--submodule=short", "-O" + os.DevNull}
}

func (r *Repository) aiStagedIndex(ctx context.Context, head, ref string) (*workingGit, []byte, error) {
	if err := r.checkAIHead(ctx, head, ref); err != nil {
		return nil, nil, err
	}
	git, err := r.workingGit(ctx, head)
	if err != nil {
		return nil, nil, err
	}
	raw, err := git.captureAIStagedIndex(ctx)
	if err != nil {
		git.close()
		return nil, nil, err
	}
	if err := r.checkAIHead(ctx, head, ref); err != nil {
		git.close()
		return nil, nil, err
	}
	return git, raw, nil
}

func (r *Repository) checkAIHead(ctx context.Context, head, ref string) error {
	current, err := r.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return err
	}
	branch, err := r.optionalRef(ctx, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		return err
	}
	if current != head || branch != ref {
		return ErrAIStagedChanged
	}
	return nil
}

func (g *workingGit) captureAIStagedIndex(ctx context.Context) ([]byte, error) {
	if err := g.isolateWorkTree(); err != nil {
		return nil, err
	}
	index := filepath.Join(g.dir, "ai-index")
	if err := copyGitMetadata(g.index, index, 512<<20); err != nil {
		return nil, err
	}
	g.index = index
	return g.read(ctx, 4<<20, append(aiStagedArgs(), "--raw", "--no-abbrev", "-z", "--")...)
}

func aiBlobSafe(ctx context.Context, read aiPatchRead, oid, mode string) bool {
	if oid == "" || mode == "160000" {
		return true
	}
	if !validOID(oid) {
		return false
	}
	blob, err := read(ctx, aiBlobLimit, "cat-file", "blob", oid)
	return err == nil && !bytes.ContainsRune(blob, 0) && !ai.ContainsPrivateKey(blob)
}

func (r *Repository) CheckAICommitContext(ctx context.Context, snapshot AICommitSnapshot) error {
	git, raw, err := r.aiStagedIndex(ctx, snapshot.head, snapshot.headRef)
	if err != nil {
		return err
	}
	git.close()
	if sha256.Sum256(raw) != snapshot.staged {
		return ErrAIStagedChanged
	}
	return nil
}
