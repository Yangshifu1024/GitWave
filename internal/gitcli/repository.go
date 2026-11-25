package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"gitwave.work/internal/gittool"
	"gitwave.work/internal/process"
)

var ErrNotRepositoryRoot = errors.New("not a Git repository root")
var errGitOutputLimit = errors.New("Git output exceeds the limit")

// Repository distinguishes the worktree's own Git directory from the
// directory whose refs and objects it shares with other worktrees.
type Repository struct {
	WorkTree                  string
	GitDir                    string
	CommonDir                 string
	git                       string
	gitDirInfo, commonDirInfo os.FileInfo
	workingMoveCache          *atomic.Pointer[workingMoveScan]
	stagedMoveCache           *atomic.Pointer[stagedMoveScan]
}

func findGit() (string, error) {
	return gittool.Git()
}

// Init creates an empty working repository, or opens an existing one so a
// retry after a Workspace save failure can finish registering it.
func Init(ctx context.Context, path string) (*Repository, error) {
	if path == "" {
		return nil, fmt.Errorf("select a directory for the new repository")
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	repo, err := OpenRoot(ctx, target)
	if err == nil {
		if repo.WorkTree == "" {
			return nil, fmt.Errorf("the selected directory is already a bare repository: %q", target)
		}
		return repo, nil
	}
	if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, ErrNotRepositoryRoot) {
		return nil, err
	}
	if _, err := os.Lstat(filepath.Join(target, ".git")); err == nil {
		return nil, fmt.Errorf("the selected directory already contains Git metadata: %q", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	git, err := findGit()
	if err != nil {
		return nil, err
	}
	dir, target, err := initLocation(ctx, git, target)
	if err != nil {
		return nil, err
	}
	// Older Git already defaults to file refs and ignores this config key.
	options := []string{"-c", "init.defaultRefFormat=files"}
	if _, err := readGit(ctx, git, dir, options, "", 1<<20,
		"init", "--quiet", "--initial-branch=main", "--object-format=sha1", "--", target); err != nil {
		return nil, err
	}
	return OpenRoot(ctx, target)
}

func initLocation(ctx context.Context, git, target string) (dir, path string, err error) {
	// Resolve the existing part of the path to check Git metadata even
	// when the destination and several of its parents do not yet exist.
	dir, path, err = resolveExistingPath(target)
	if err != nil {
		return "", "", err
	}
	for _, flag := range []string{"--absolute-git-dir", "--git-common-dir"} {
		metadata, err := readPath(ctx, git, dir, flag)
		if isNotRepository(err) {
			break
		}
		if err != nil {
			return "", "", err
		}
		gitDir, err := os.Stat(metadata)
		if err != nil {
			return "", "", err
		}
		for ancestor := dir; ; ancestor = filepath.Dir(ancestor) {
			info, err := os.Stat(ancestor)
			if err != nil {
				return "", "", err
			}
			if os.SameFile(gitDir, info) {
				return "", "", fmt.Errorf("the destination is inside Git metadata: %q", target)
			}
			if filepath.Dir(ancestor) == ancestor {
				break
			}
		}
	}
	return dir, path, nil
}

func resolveExistingPath(path string) (ancestorPath, resolvedPath string, err error) {
	tail := ""
	for ancestor := path; ; ancestor = filepath.Dir(ancestor) {
		resolved, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			return resolved, filepath.Join(resolved, tail), nil
		}
		if !errors.Is(err, os.ErrNotExist) || filepath.Dir(ancestor) == ancestor {
			return "", "", err
		}
		tail = filepath.Join(filepath.Base(ancestor), tail)
	}
}

func Open(ctx context.Context, path string) (*Repository, error) {
	git, err := findGit()
	if err != nil {
		return nil, err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	workTree, err := readPath(ctx, git, path, "--show-toplevel")
	root := workTree
	if err != nil {
		bare, probeErr := readGit(ctx, git, path, nil, "", 0, "rev-parse", "--is-bare-repository")
		if probeErr != nil || strings.TrimSuffix(string(bare), "\n") != "true" {
			return nil, err
		}
		root = path
	}
	gitDir, err := readPath(ctx, git, root, "--absolute-git-dir")
	if err != nil {
		return nil, err
	}
	commonDir, err := readPath(ctx, git, root, "--git-common-dir")
	if err != nil {
		return nil, err
	}
	return openedRepository(workTree, gitDir, commonDir, git)
}

func openedRepository(workTree, gitDir, commonDir, git string) (*Repository, error) {
	info, err := openRepositoryDirectory(gitDir)
	if err != nil {
		return nil, err
	}
	commonInfo := info
	if commonDir != gitDir {
		commonInfo, err = openRepositoryDirectory(commonDir)
		if err != nil {
			return nil, err
		}
	}
	return &Repository{WorkTree: workTree, GitDir: gitDir, CommonDir: commonDir, git: git,
		gitDirInfo: info, commonDirInfo: commonInfo, workingMoveCache: new(atomic.Pointer[workingMoveScan]),
		stagedMoveCache: new(atomic.Pointer[stagedMoveScan])}, nil
}

func openRepositoryDirectory(path string) (os.FileInfo, error) {
	dir, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	// File.Stat captures the open directory's identity even when its
	// pathname is later reused. Windows path Stat resolves it lazily.
	info, err := dir.Stat()
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("Git metadata path %q is no longer a directory", path)
	}
	return info, nil
}

// SameCheckout compares the opened checkout identities, including a Git
// directory that may have been replaced at the same filesystem path.
func (r *Repository) SameCheckout(other *Repository) bool {
	return r.WorkTree == other.WorkTree && r.GitDir == other.GitDir && r.CommonDir == other.CommonDir &&
		os.SameFile(r.gitDirInfo, other.gitDirInfo) && os.SameFile(r.commonDirInfo, other.commonDirInfo)
}

func (r *Repository) Path() string {
	if r.WorkTree != "" {
		return r.WorkTree
	}
	return r.GitDir
}

// OpenRoot requires the exact root selected by the caller. An invalid
// nested repository must never silently resolve to an ancestor.
func OpenRoot(ctx context.Context, path string) (*Repository, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("repository root requires an absolute path")
	}
	root, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	r, err := Open(ctx, root)
	if err != nil {
		if isNotRepository(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotRepositoryRoot, path)
		}
		return nil, err
	}
	if r.Path() != root {
		selected, selectErr := os.Stat(root)
		actual, actualErr := os.Stat(r.Path())
		if selectErr != nil || actualErr != nil || !os.SameFile(selected, actual) {
			return nil, fmt.Errorf("%w: %s", ErrNotRepositoryRoot, path)
		}
	}
	return r, nil
}

func isNotRepository(err error) bool {
	var exit *exec.ExitError
	return errors.As(err, &exit) && strings.HasPrefix(string(exit.Stderr), "fatal: not a git repository")
}

func readPath(ctx context.Context, git, dir, flag string) (string, error) {
	// Ask for one path per invocation so a newline inside a path is unambiguous.
	output, err := readGit(ctx, git, dir, nil, "", 0, "rev-parse", "--path-format=absolute", flag)
	if err != nil {
		return "", err
	}
	path := strings.TrimSuffix(string(output), "\n")
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("Git did not return an absolute path for %s", flag)
	}
	return filepath.EvalSymlinks(path)
}

func (r *Repository) read(ctx context.Context, input string, args ...string) ([]byte, error) {
	return r.readLimited(ctx, input, 0, args...)
}

func (r *Repository) readLimited(ctx context.Context, input string, limit int64, args ...string) ([]byte, error) {
	return readGitOutput(ctx, r.command(ctx, input, args...), limit, args[0])
}

func (r *Repository) command(ctx context.Context, input string, args ...string) *exec.Cmd {
	// Keep reads tied to the selected worktree even if its Git directory is moved.
	options := []string{"--git-dir=" + r.GitDir}
	if r.WorkTree != "" {
		options = append(options, "--work-tree="+r.WorkTree)
	}
	return gitCommand(ctx, r.git, r.Path(), options, input, args...)
}

func readGit(ctx context.Context, git, dir string, options []string, input string, limit int64, args ...string) ([]byte, error) {
	return readGitOutput(ctx, gitCommand(ctx, git, dir, options, input, args...), limit, args[0])
}

func gitCommand(ctx context.Context, git, dir string, options []string, input string, args ...string) *exec.Cmd {
	argv := []string{"-C", dir, "--no-optional-locks", "--no-replace-objects", "-c", "core.fsmonitor=false"}
	argv = append(argv, options...)
	cmd := exec.CommandContext(ctx, git, append(argv, args...)...)
	process.Hide(cmd)
	cmd.Stdin = strings.NewReader(input)
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = gittool.Environment(gitEnvironment(), git, "")
	return cmd
}

func gitEnvironment() []string {
	env := slices.DeleteFunc(os.Environ(), func(entry string) bool {
		name, _, _ := strings.Cut(entry, "=")
		name = strings.ToUpper(name)
		switch name {
		case "GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM", "GIT_CONFIG_NOSYSTEM":
			return false
		case "LC_ALL":
			return true
		default:
			return strings.HasPrefix(name, "GIT_")
		}
	})
	// Also block transport on Git versions without GIT_NO_LAZY_FETCH.
	return append(env, "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "GIT_NO_LAZY_FETCH=1", "GIT_ALLOW_PROTOCOL=")
}

func readGitOutput(ctx context.Context, cmd *exec.Cmd, limit int64, action string) ([]byte, error) {
	output, err := gitOutput(cmd, limit)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", action, err)
	}
	return output, nil
}

func gitOutput(cmd *exec.Cmd, limit int64) ([]byte, error) {
	if limit == 0 {
		return cmd.Output()
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	// Keep draining stderr while retaining only the beginning for diagnostics.
	var stderr gitDiagnostic
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, limit+1))
	if readErr != nil || int64(len(output)) > limit {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if readErr != nil {
			return nil, readErr
		}
		return nil, errGitOutputLimit
	}
	if err := cmd.Wait(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			exit.Stderr = stderr.buffer.Bytes()
		}
		return nil, err
	}
	return output, nil
}

type gitDiagnostic struct {
	buffer bytes.Buffer
}

func (d *gitDiagnostic) Write(p []byte) (int, error) {
	keep := min(len(p), 32<<10-d.buffer.Len())
	_, _ = d.buffer.Write(p[:keep])
	return len(p), nil
}
