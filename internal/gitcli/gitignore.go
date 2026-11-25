package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const gitignorePath = ".gitignore"

type GitignoreEditor struct{ file repositoryEditor }

func (r *Repository) OpenGitignore(ctx context.Context) (*GitignoreEditor, error) {
	if r.WorkTree == "" {
		return nil, fmt.Errorf("editing .gitignore requires a working copy")
	}
	if err := r.checkIndexCheckout(ctx); err != nil {
		return nil, err
	}
	directory, err := bindEditorDirectory(r.WorkTree)
	if err != nil {
		return nil, err
	}
	file, err := directory.read(r, gitignorePath)
	if err != nil {
		return nil, err
	}
	return &GitignoreEditor{file: file}, nil
}

func (e *GitignoreEditor) Text() string { return string(e.file.before.data) }

func (e *GitignoreEditor) Save(ctx context.Context, text string) error {
	text = finishGitignoreLine(text)
	index, err := e.file.repo.lockCheckoutIndex(ctx)
	if err != nil {
		return err
	}
	defer index.close()
	return e.save(ctx, text, func() error {
		if !index.ownsLock() {
			return fmt.Errorf("the Git index lock changed while saving .gitignore")
		}
		return nil
	})
}

func (e *GitignoreEditor) save(ctx context.Context, text string, check func() error) error {
	return e.file.save(ctx, text, e.file.before.mode, func() error {
		if err := e.file.repo.checkIndexCheckout(ctx); err != nil {
			return err
		}
		return check()
	})
}

func finishGitignoreLine(text string) string {
	if text != "" && !strings.HasSuffix(text, "\n") {
		if strings.Contains(text, "\r\n") && !strings.HasSuffix(text, "\r") {
			return text + "\r\n"
		}
		return text + "\n"
	}
	return text
}

type IgnoreScope int

const (
	IgnoreFile IgnoreScope = iota
	IgnoreDirectory
	IgnoreExtension
)

type IgnoreOption struct {
	Scope   IgnoreScope
	Pattern string
}

func GitignoreOptions(path string) []IgnoreOption {
	if !validIndexPath(path) || path == gitignorePath || strings.ContainsAny(path, "\r\n") || !utf8.ValidString(path) {
		return nil
	}
	options := []IgnoreOption{{Scope: IgnoreFile, Pattern: "/" + gitignoreLiteral(path)}}
	name := path
	if slash := strings.LastIndexByte(path, '/'); slash >= 0 {
		options = append(options, IgnoreOption{Scope: IgnoreDirectory, Pattern: "/" + gitignoreLiteral(path[:slash]) + "/"})
		name = path[slash+1:]
	}
	if dot := strings.LastIndexByte(name, '.'); dot > 0 {
		options = append(options, IgnoreOption{Scope: IgnoreExtension, Pattern: "*" + gitignoreLiteral(name[dot:])})
	}
	return options
}

func gitignoreLiteral(path string) string {
	var pattern strings.Builder
	for _, char := range path {
		if strings.ContainsRune("\\*?[] ", char) {
			pattern.WriteByte('\\')
		}
		pattern.WriteRune(char)
	}
	return pattern.String()
}

// IgnoreUntracked appends one rule for the exact untracked file selected in Changes.
func (r *Repository) IgnoreUntracked(ctx context.Context, path string, scope IgnoreScope) (bool, error) {
	var pattern string
	for _, option := range GitignoreOptions(path) {
		if option.Scope == scope {
			pattern = option.Pattern
			break
		}
	}
	if pattern == "" {
		return false, fmt.Errorf("the selected path cannot be ignored with that scope")
	}
	index, err := r.lockCheckoutIndex(ctx)
	if err != nil {
		return false, err
	}
	defer index.close()
	ignored, err := r.isUntrackedForIgnore(ctx, path, "--ignored")
	if err != nil || ignored {
		return false, err
	}
	editor, err := r.OpenGitignore(ctx)
	if err != nil {
		return false, err
	}
	lineEnd := "\n"
	if strings.Contains(editor.Text(), "\r\n") {
		lineEnd = "\r\n"
	}
	next := finishGitignoreLine(editor.Text()) + pattern + lineEnd
	err = editor.save(ctx, next, func() error {
		if !index.ownsLock() {
			return fmt.Errorf("the Git index lock changed while saving .gitignore")
		}
		unmerged, err := r.readLimited(ctx, "", 4096, "ls-files", "--unmerged", "--", gitignorePath)
		if err != nil {
			return err
		}
		if len(unmerged) != 0 {
			return fmt.Errorf("resolve the .gitignore conflict before adding a rule")
		}
		visible, err := r.isUntrackedForIgnore(ctx, path)
		if err != nil {
			return err
		}
		if !visible {
			return fmt.Errorf("%q is no longer an untracked file shown in Changes", path)
		}
		if strings.Contains(path, "/") {
			if err := r.previewIgnoreRule(ctx, path, next); err != nil {
				return err
			}
		}
		if !index.ownsLock() {
			return fmt.Errorf("the Git index lock changed while saving .gitignore")
		}
		return nil
	})
	return err == nil, err
}

func (r *Repository) previewIgnoreRule(ctx context.Context, path, contents string) error {
	// A nearer .gitignore can override the new root rule. Let Git check
	// the proposed file alongside copies of the original ancestor rules.
	git, err := r.workingGit(ctx, "")
	if err != nil {
		return err
	}
	defer git.close()
	if err := git.isolateWorkTree(); err != nil {
		return err
	}
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return err
	}
	defer root.Close()
	staged, err := os.OpenRoot(git.workTree)
	if err != nil {
		return err
	}
	defer staged.Close()
	if err := git.snapshotWorktreeMetadata(ctx, root, staged, []string{path}, nil, gitignorePath); err != nil {
		return err
	}
	if err := staged.MkdirAll(filepath.Dir(filepath.FromSlash(path)), 0o700); err != nil {
		return err
	}
	if err := staged.WriteFile(gitignorePath, []byte(contents), 0o600); err != nil {
		return err
	}
	_, err = git.read(ctx, 4096, "check-ignore", "--no-index", "--quiet", "--", path)
	if gitExitCode(err, 1) {
		return fmt.Errorf("a lower-level .gitignore overrides this rule for %q", path)
	}
	if err != nil {
		return err
	}
	for parent := filepath.Dir(filepath.FromSlash(path)); parent != "."; parent = filepath.Dir(parent) {
		if err := verifyIgnoreRules(root, staged, filepath.Join(parent, gitignorePath)); err != nil {
			return err
		}
	}
	return nil
}

func verifyIgnoreRules(root, staged *os.Root, rule string) error {
	preview, err := readGitStorageSnapshot(staged, rule, repositoryEditorLimit)
	if err != nil {
		return err
	}
	parent, err := openWorktreeParent(root, rule, false)
	if err != nil {
		return err
	}
	defer parent.Close()
	name := filepath.Base(rule)
	info, err := parent.Lstat(name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	var live gitStorageSnapshot
	if info != nil && info.Mode().IsRegular() {
		live, err = readGitStorageSnapshot(parent, name, repositoryEditorLimit)
		if err != nil {
			return err
		}
	}
	if (preview.mode == 0) != (live.mode == 0) || !bytes.Equal(preview.data, live.data) {
		return fmt.Errorf("%s changed during the ignore rule preview", rule)
	}
	return nil
}

func (r *Repository) isUntrackedForIgnore(ctx context.Context, path string, options ...string) (bool, error) {
	args := append([]string{"--literal-pathspecs", "ls-files", "--others", "--exclude-standard", "-z"}, options...)
	data, err := r.readLimited(ctx, "", 1<<20, append(args, "--", path)...)
	return string(data) == path+"\x00", err
}
