package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"gitwave.work/internal/gittool"
	"gitwave.work/internal/process"
)

const lfsHookLimit = 1 << 20

var lfsHookNames = []string{"pre-push", "post-checkout", "post-commit", "post-merge"}

type LFSInstallResult struct {
	ExternalHooks bool
}

func (r *Repository) lfsExecutable(ctx context.Context) (string, error) {
	if path, bundled := gittool.BundledLFS(r.git); bundled {
		executable, err := exec.LookPath(path)
		if err != nil {
			return "", fmt.Errorf("find bundled Git LFS: %w", err)
		}
		return executable, nil
	}
	gitExecPath, err := r.readLimited(ctx, "", 16<<10, "--exec-path")
	var lookups []string
	if err == nil {
		if path := gitPathLine(gitExecPath); filepath.IsAbs(path) {
			lookups = append(lookups, filepath.Join(path, "git-lfs"))
			if runtime.GOOS == "windows" && filepath.Base(path) == "git-core" && filepath.Base(filepath.Dir(path)) == "libexec" {
				// Git for Windows can ship Git LFS beside the native Git binary.
				lookups = append(lookups, filepath.Join(path, "..", "..", "bin", "git-lfs"))
			}
		}
	}
	lookups = append(lookups, "git-lfs", filepath.Join(filepath.Dir(r.git), "git-lfs"))
	for _, lookup := range lookups {
		if executable, err := exec.LookPath(lookup); err == nil && filepath.IsAbs(executable) {
			return executable, nil
		}
	}
	return "", fmt.Errorf("Git LFS was not found on PATH")
}

func (r *Repository) runLocalLFS(ctx context.Context, executable, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	process.Hide(cmd)
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = gittool.Environment(isolatedLFSConfig(), r.git, executable)
	return readGitOutput(ctx, cmd, 64<<10, "lfs "+args[0])
}

func isolatedLFSConfig() []string {
	return append(gitEnvironment(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
}

// InstallLFS prepares the official CLI's result in a scratch repository. In
// particular, Git LFS can reject an existing hook after it has written filters.
func (r *Repository) InstallLFS(ctx context.Context) (LFSInstallResult, error) {
	if r.WorkTree == "" {
		return LFSInstallResult{}, fmt.Errorf("Git LFS requires a working copy")
	}
	executable, err := r.lfsExecutable(ctx)
	if err != nil {
		return LFSInstallResult{}, err
	}
	var result LFSInstallResult
	err = r.withRefEdit(ctx, func() error {
		var err error
		result, err = r.installLFS(ctx, executable)
		return err
	})
	return result, err
}

type lfsInstallFile struct {
	root   *os.Root
	name   string
	limit  int64
	before gitStorageSnapshot
	staged gitStorageSnapshot
	lock   *gitStorageLock
}

func (f *lfsInstallFile) verify() error {
	current, err := readGitStorageSnapshot(f.root, f.name, f.limit)
	if err != nil {
		return err
	}
	if !current.same(f.before) {
		return fmt.Errorf("%s changed while preparing Git LFS", f.name)
	}
	return nil
}

func (r *Repository) installLFS(ctx context.Context, executable string) (LFSInstallResult, error) {
	common, err := os.OpenRoot(r.CommonDir)
	if err != nil {
		return LFSInstallResult{}, err
	}
	defer common.Close()
	config := lfsInstallFile{root: common, name: "config", limit: remoteConfigLimit}
	config.lock, err = lockGitStorageFile(common, config.name)
	if err != nil {
		return LFSInstallResult{}, err
	}
	defer config.lock.close()
	config.before, err = readGitStorageSnapshot(common, config.name, config.limit)
	if err != nil {
		return LFSInstallResult{}, err
	}
	filters, err := r.lfsConfig(ctx)
	if err != nil {
		return LFSInstallResult{}, err
	}
	hookRootPath, hookDir, err := r.lfsHookLocation(ctx)
	if err != nil {
		return LFSInstallResult{}, err
	}
	result := LFSInstallResult{ExternalHooks: hookRootPath == ""}
	var files []*lfsInstallFile
	if !result.ExternalHooks {
		root, err := os.OpenRoot(hookRootPath)
		if err != nil {
			return result, err
		}
		defer root.Close()
		for _, hook := range lfsHookNames {
			name := filepath.Join(hookDir, hook)
			file := &lfsInstallFile{root: root, name: name, limit: lfsHookLimit}
			parent, err := openWorktreeParent(root, name, false)
			if err == nil {
				file.before, err = readGitStorageSnapshot(parent, hook, file.limit)
				if parent != root {
					parent.Close()
				}
				if err != nil {
					return result, err
				}
			} else if !errors.Is(err, fs.ErrNotExist) {
				return result, err
			}
			files = append(files, file)
		}
	}

	scratch, err := os.MkdirTemp("", "gitwave-lfs-install-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(scratch)
	if err := r.stageLFSInstallation(ctx, executable, scratch, filters, &config, files); err != nil {
		return result, err
	}

	for _, file := range files {
		parent, err := openWorktreeParent(file.root, file.name, true)
		if err != nil {
			return result, err
		}
		if parent != file.root {
			parent.Close()
		}
		file.lock, err = lockGitStorageFile(file.root, file.name)
		if err != nil {
			return result, err
		}
		defer file.lock.close()
	}
	files = append(files, &config)
	verify := func() error {
		if err := r.checkIndexCheckout(ctx); err != nil {
			return err
		}
		if err := r.verifyGitStorageLocks(ctx, common, config.lock); err != nil {
			return err
		}
		if !result.ExternalHooks {
			opened, firstErr := files[0].root.Stat(".")
			live, secondErr := os.Stat(hookRootPath)
			if firstErr != nil || secondErr != nil || !sameFileIdentity(opened, live) {
				return fmt.Errorf("the hooks directory moved while preparing Git LFS")
			}
		}
		currentRoot, currentDir, err := r.lfsHookLocation(ctx)
		if err != nil {
			return err
		}
		currentFilters, err := r.lfsConfig(ctx)
		if err != nil {
			return err
		}
		if currentRoot != hookRootPath || currentDir != hookDir || !maps.EqualFunc(filters.local, currentFilters.local, slices.Equal) ||
			!maps.EqualFunc(filters.worktree, currentFilters.worktree, slices.Equal) {
			return fmt.Errorf("the effective Git configuration changed while preparing Git LFS")
		}
		return nil
	}
	return result, publishLFSInstallation(files, verify)
}

func (r *Repository) lfsHookLocation(ctx context.Context) (rootPath, relative string, err error) {
	data, err := r.readLimited(ctx, "", 16<<10, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
	if err != nil {
		return "", "", err
	}
	hooks := gitPathLine(data)
	if !filepath.IsAbs(hooks) {
		return "", "", fmt.Errorf("Git did not return an absolute hooks directory")
	}
	for _, root := range []string{r.CommonDir, r.WorkTree} {
		rel, err := filepath.Rel(root, hooks)
		if err == nil && filepath.IsLocal(rel) {
			return root, rel, nil
		}
	}
	return "", hooks, nil
}

func gitPathLine(data []byte) string {
	path := strings.TrimSuffix(string(data), "\n")
	if runtime.GOOS == "windows" {
		path = strings.TrimSuffix(path, "\r")
	}
	return path
}

func (r *Repository) stageLFSInstallation(ctx context.Context, executable, scratch string, filters lfsConfig,
	config *lfsInstallFile, hooks []*lfsInstallFile) error {
	runGit := func(args ...string) ([]byte, error) {
		cmd := gitCommand(ctx, r.git, scratch, nil, "", args...)
		cmd.Env = gittool.Environment(isolatedLFSConfig(), r.git, "")
		return readGitOutput(ctx, cmd, 64<<10, args[0])
	}
	if _, err := runGit("init", "--quiet", "--template="); err != nil {
		return err
	}
	scratchConfig := filepath.Join(scratch, ".git", "config")
	hookDir := filepath.Join(scratch, ".git", "hooks")
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		return err
	}
	for _, file := range hooks {
		if file.before.mode != 0 {
			if err := os.WriteFile(filepath.Join(hookDir, filepath.Base(file.name)), file.before.data, file.before.mode.Perm()); err != nil {
				return err
			}
		}
	}
	for _, key := range lfsConfigKeys {
		for _, value := range filters.local[key] {
			if _, err := runGit("config", "--file", scratchConfig, "--add", key, value); err != nil {
				return err
			}
		}
	}
	args := []string{"install", "--local"}
	if len(hooks) == 0 {
		args = append(args, "--skip-repo")
	}
	if _, err := r.runLocalLFS(ctx, executable, scratch, args...); err != nil {
		return err
	}
	overlay := filepath.Join(scratch, "config-overlay")
	for _, key := range lfsConfigKeys {
		value, err := runGit("config", "--file", scratchConfig, "--null", "--get", key)
		if key == lfsFormatKey && gitExitCode(err, 1) {
			continue // Git LFS versions before 3.8 do not write this key.
		}
		if err != nil {
			return fmt.Errorf("Git LFS did not configure %s: %w", key, err)
		}
		next := strings.TrimSuffix(string(value), "\x00")
		if override := filters.worktree[key]; len(override) > 0 && override[len(override)-1] != next {
			return fmt.Errorf("%s is overridden in the worktree configuration", key)
		}
		if prior := filters.local[key]; len(prior) > 0 && prior[len(prior)-1] == next {
			continue
		}
		if _, err := runGit("config", "--file", overlay, "--add", key, next); err != nil {
			return err
		}
	}
	data, err := os.ReadFile(overlay)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	// Append the CLI's assignments after all local includes, preserving their
	// contents and the relative locations of the Git config files.
	config.staged = config.before
	if len(data) > 0 {
		config.staged.data = slices.Concat(config.before.data, []byte{'\n', '\n'}, data)
		if int64(len(config.staged.data)) > config.limit {
			return fmt.Errorf("the updated Git configuration exceeds the file size limit")
		}
	}
	private, err := os.OpenRoot(hookDir)
	if err != nil {
		return err
	}
	defer private.Close()
	for _, file := range hooks {
		file.staged, err = readGitStorageSnapshot(private, filepath.Base(file.name), file.limit)
		if err != nil {
			return err
		}
		if file.staged.mode == 0 {
			return fmt.Errorf("Git LFS did not install %s", filepath.Base(file.name))
		}
		if bytes.Equal(file.before.data, file.staged.data) {
			file.staged.mode = file.before.mode
		}
	}
	return nil
}

func publishLFSInstallation(files []*lfsInstallFile, verify func() error) error {
	var changed []*lfsInstallFile
	for _, file := range files {
		if !file.staged.same(file.before) {
			if err := file.lock.prepare(file.staged.data, file.staged.mode); err != nil {
				return err
			}
			changed = append(changed, file)
		}
	}
	if err := verify(); err != nil {
		return err
	}
	for _, file := range files {
		parent, err := file.lock.parent.Stat(".")
		if err != nil {
			return err
		}
		if err := checkDiscardParent(file.root, file.name, parent); err != nil {
			return err
		}
		if err := file.verify(); err != nil {
			return err
		}
		if !file.lock.owns() {
			return fmt.Errorf("the lock for %s changed while preparing Git LFS", file.name)
		}
	}
	for i, file := range changed {
		if err := file.lock.publish(); err != nil {
			for _, previous := range slices.Backward(changed[:i]) {
				err = errors.Join(err, restoreGitStorageFile(previous.root, previous.name, previous.before, previous.staged.data, previous.limit))
			}
			return err
		}
	}
	return nil
}
