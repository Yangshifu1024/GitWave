package gitcli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"gitwave.work/internal/ai"
)

const maxConflictText = 4 << 20

type ConflictPart struct {
	Text string
	Kind string // text, missing, binary, large, symlink or submodule
}

type Conflict struct {
	Path            string
	Ours, Base      ConflictPart
	Theirs, Working ConflictPart
	MarkerSize      int
	repo            *Repository
	shown           Status
	change          Change
	parent          os.FileInfo
	stamp           discardStamp
}

type ConflictChoice uint8

const (
	ConflictText ConflictChoice = iota
	ConflictOurs
	ConflictTheirs
)

func (r *Repository) OpenConflict(ctx context.Context, shown Status, change Change) (*Conflict, error) {
	if r.WorkTree == "" || change.Kind != 'u' || !validIndexPath(change.Path) {
		return nil, fmt.Errorf("select an unresolved file from the working copy")
	}
	c := &Conflict{Path: change.Path, repo: r, shown: shown, change: change}
	if err := c.verifyIndex(ctx); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	local := filepath.FromSlash(change.Path)
	c.parent, c.stamp, err = inspectDiscardFile(ctx, root, local)
	if err != nil {
		return nil, err
	}
	if c.Working, err = c.readWorking(ctx, root, local); err != nil {
		return nil, err
	}
	for i, part := range []*ConflictPart{&c.Base, &c.Ours, &c.Theirs} {
		*part, err = r.readConflictStage(ctx, change.Conflict[i])
		if err != nil {
			return nil, err
		}
	}
	head := shown.HeadOID
	if head == "(initial)" {
		head = ""
	}
	git, err := r.workingGit(ctx, head)
	if err != nil {
		return nil, err
	}
	defer git.close()
	attr, err := git.run(ctx, nulPaths([]string{change.Path}), 64<<10, "check-attr", "-z", "--stdin", "conflict-marker-size")
	if err != nil {
		return nil, err
	}
	fields := strings.Split(string(attr), "\x00")
	if len(fields) == 4 && fields[0] == change.Path && fields[1] == "conflict-marker-size" {
		c.MarkerSize, _ = strconv.Atoi(fields[2])
	}
	return c, nil
}

func (c *Conflict) verifyIndex(ctx context.Context) error {
	current, err := c.repo.Status(ctx)
	if err != nil {
		return err
	}
	if !c.MatchesStatus(current) {
		return fmt.Errorf("the conflict changed on disk: %q", c.change.Path)
	}
	return nil
}

func (c *Conflict) MatchesStatus(current Status) bool {
	return current.HeadRef == c.shown.HeadRef && current.HeadOID == c.shown.HeadOID && current.Commit == c.shown.Commit &&
		slices.Contains(current.Changes, c.change)
}

// AIContext includes only the three captured Git index stages. The
// working file and any draft typed by the user stay out of the prompt.
func (c *Conflict) AIContext(ctx context.Context) (string, error) {
	if err := c.CheckAIContext(ctx); err != nil {
		return "", err
	}
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "Conflict in %q\n", c.Path)
	for _, side := range []struct {
		name string
		part ConflictPart
	}{{"BASE", c.Base}, {"OURS", c.Ours}, {"THEIRS", c.Theirs}} {
		fmt.Fprintf(&prompt, "\n=== %s ===\n", side.name)
		if side.part.Kind != "text" {
			fmt.Fprintf(&prompt, "(%s)\n", side.part.Kind)
			continue
		}
		if ai.ContainsPrivateKey([]byte(side.part.Text)) {
			prompt.WriteString("[omitted private key content]\n")
			continue
		}
		text := ai.Scrub(side.part.Text)
		const maxSideBytes = 12_000
		if len(text) > maxSideBytes {
			end := maxSideBytes
			for end > 0 && !utf8.RuneStart(text[end]) {
				end--
			}
			text = text[:end] + "\n[side truncated due to size]"
		}
		prompt.WriteString(text)
		prompt.WriteByte('\n')
	}
	return prompt.String(), nil
}

func (c *Conflict) CheckAIContext(ctx context.Context) error {
	return c.verifyIndex(ctx)
}
func (c *Conflict) readWorking(ctx context.Context, root *os.Root, local string) (ConflictPart, error) {
	if c.stamp.info == nil {
		return ConflictPart{Kind: "missing"}, nil
	}
	if c.stamp.info.Size() > maxConflictText {
		return ConflictPart{Kind: "large"}, nil
	}
	parent, err := openWorktreeParent(root, local, false)
	if err != nil {
		return ConflictPart{}, err
	}
	if parent != root {
		defer parent.Close()
	}
	name := filepath.Base(local)
	if identity, err := parent.Stat("."); err != nil || !sameFileIdentity(identity, c.parent) {
		return ConflictPart{}, fmt.Errorf("the conflict directory changed while opening %q", local)
	}
	if c.stamp.info.Mode()&os.ModeSymlink != 0 {
		return ConflictPart{Kind: "symlink"}, nil
	}
	file, err := parent.OpenFile(name, os.O_RDONLY|workFileOpenFlags, 0)
	if err != nil {
		return ConflictPart{}, err
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || !sameDiscardMetadata(info, c.stamp.info) {
		return ConflictPart{}, fmt.Errorf("the conflict file changed while opening %q", local)
	}
	data, err := io.ReadAll(io.LimitReader(stageReader{ctx, file}, maxConflictText+1))
	if err != nil {
		return ConflictPart{}, err
	}
	if sha256.Sum256(data) != c.stamp.sum {
		return ConflictPart{}, fmt.Errorf("the conflict file changed while reading %q", local)
	}
	return textConflictPart(data), nil
}

func (r *Repository) readConflictStage(ctx context.Context, stage IndexStage) (ConflictPart, error) {
	switch stage.Mode {
	case "000000":
		return ConflictPart{Kind: "missing"}, nil
	case "160000":
		return ConflictPart{Kind: "submodule", Text: stage.OID}, nil
	case "120000", "100644", "100755":
	default:
		return ConflictPart{}, fmt.Errorf("invalid Git conflict stage")
	}
	if !validOID(stage.OID) {
		return ConflictPart{}, fmt.Errorf("invalid object in Git conflict stage")
	}
	size, err := r.blobSize(ctx, stage.OID)
	if err != nil {
		return ConflictPart{}, err
	}
	if size > maxConflictText {
		return ConflictPart{Kind: "large"}, nil
	}
	data, err := r.readLimited(ctx, "", maxConflictText, "cat-file", "blob", stage.OID)
	if err != nil {
		return ConflictPart{}, err
	}
	part := textConflictPart(data)
	if stage.Mode == "120000" {
		part.Kind = "symlink"
	}
	return part, nil
}

func textConflictPart(data []byte) ConflictPart {
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return ConflictPart{Kind: "binary"}
	}
	return ConflictPart{Kind: "text", Text: string(data)}
}

func (c *Conflict) CanEdit() bool {
	for _, part := range []ConflictPart{c.Working, c.Ours, c.Base, c.Theirs} {
		if part.Kind != "text" && part.Kind != "missing" {
			return false
		}
	}
	return true
}

func (c *Conflict) Resolve(ctx context.Context, choice ConflictChoice, text string) error {
	r := c.repo
	lock, err := r.lockCheckoutIndex(ctx)
	if err != nil {
		return err
	}
	defer lock.close()
	if err := c.verifyIndex(ctx); err != nil {
		return err
	}
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return err
	}
	defer root.Close()
	local := filepath.FromSlash(c.change.Path)
	if err := c.verifyFile(ctx, root, local, c.parent, c.stamp); err != nil {
		return err
	}
	head := c.shown.HeadOID
	if head == "(initial)" {
		head = ""
	}
	git, err := r.workingGit(ctx, head)
	if err != nil {
		return err
	}
	defer git.close()
	git.index = filepath.Join(git.dir, "index")
	git.settings = append(git.settings, "-c", "core.splitIndex=false")
	if err := copyOriginalIndex(lock.root, git.index); err != nil {
		return err
	}
	stage, render, err := c.prepareResolution(ctx, git, root, choice, text)
	if err != nil {
		return err
	}
	finalize := context.WithoutCancel(ctx)
	cmd := r.command(finalize, "", "-c", "core.hooksPath="+git.dir, "update-ref", "--stdin", "-z")
	expected := head
	if expected == "" {
		expected = strings.Repeat("0", git.oidSize)
	}
	installed := false
	var saveErr error
	_, _, err = runReferenceTransaction(cmd, "verify HEAD\x00"+expected+"\x00", func() error {
		if err := c.verifyResolutionStatus(ctx, lock, head); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		saveErr = c.publishFile(finalize, root, local, render, func(parent os.FileInfo, stamp discardStamp) error {
			if stage.OID != "" {
				if err := git.refreshResolvedStats(finalize, []string{c.change.Path}); err != nil {
					return err
				}
			}
			if err := publishIndex(lock.root, lock.file, lock.info, git.index, func() error {
				if err := c.verifyResolutionStatus(finalize, lock, head); err != nil {
					return err
				}
				return c.verifyFile(finalize, root, local, parent, stamp)
			}); err != nil {
				return err
			}
			installed = true
			return nil
		})
		return saveErr
	})
	if installed {
		// This transaction only verifies refs. The completed publication is
		// the index rename while the prepared transaction holds its locks.
		return saveErr
	}
	return err
}

func (c *Conflict) verifyResolutionStatus(ctx context.Context, lock *checkoutIndexLock, head string) error {
	if err := c.repo.checkIndexCheckout(ctx); err != nil {
		return err
	}
	state, err := c.repo.readCommitState(ctx, lock.root)
	if err != nil {
		return err
	}
	if !lock.ownsLock() || state != c.shown.Commit || !c.repo.headMatches(ctx, c.shown.HeadRef, head) {
		return fmt.Errorf("the Git operation changed before saving %q", c.change.Path)
	}
	return nil
}

func (c *Conflict) verifyFile(ctx context.Context, root *os.Root, local string, parent os.FileInfo, stamp discardStamp) error {
	actualParent, current, err := inspectDiscardFile(ctx, root, local)
	if err != nil {
		return err
	}
	if !sameFileIdentity(actualParent, parent) || !current.matches(stamp) {
		return fmt.Errorf("the working file changed on disk: %q", c.change.Path)
	}
	return nil
}
