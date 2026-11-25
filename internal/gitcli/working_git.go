package gitcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gitwave.work/internal/process"
)

// Worktree reads run with a private Git directory so config from the live
// checkout cannot add external filters between the snapshot and the read.
// Git sees the original index and objects, with optional index writes disabled.
type workingGit struct {
	repo              *Repository
	dir               string
	workTree          string
	privateObjects    bool
	groupCancellation bool
	settings          []string
	index             string
	oidSize           int
	frozenAttributes  bool
	shallowBoundaries map[string]bool
	localLFS          localLFSConfig
	lfsEnvironment    []string
	lfsStageDir       string
	lfsCheckoutScope  *lfsPathScope
}

func (r *Repository) workingGit(ctx context.Context, head string) (*workingGit, error) {
	return r.newWorkingGit(ctx, head, true)
}

func (r *Repository) newWorkingGit(ctx context.Context, head string, useRepositoryIndex bool) (*workingGit, error) {
	if head != "" && !validOID(head) {
		return nil, fmt.Errorf("invalid working copy HEAD")
	}
	format := "sha1"
	if len(head) == 64 {
		format = "sha256"
	} else if head == "" {
		configured, err := r.read(ctx, "", "rev-parse", "--show-object-format")
		if err != nil {
			return nil, err
		}
		format = strings.TrimSpace(string(configured))
		if format != "sha1" && format != "sha256" {
			return nil, fmt.Errorf("unsupported Git object format")
		}
	}
	settings, localLFS, err := r.workingGitSettings(ctx)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "gitwave-working-copy-*")
	if err != nil {
		return nil, err
	}
	oidSize := 40
	if format == "sha256" {
		oidSize = 64
	}
	git := &workingGit{repo: r, dir: dir, workTree: r.WorkTree, settings: settings, oidSize: oidSize, localLFS: localLFS}
	git.index = filepath.Join(dir, "index")
	if useRepositoryIndex {
		git.index = filepath.Join(r.GitDir, "index")
	}
	if err := git.initialize(ctx, head, format, useRepositoryIndex); err != nil {
		git.close()
		return nil, err
	}
	return git, nil
}

func (r *Repository) workingGitSettings(ctx context.Context) ([]string, localLFSConfig, error) {
	// Read a fixed set of built-in filesystem, attribute and ignore settings.
	// Command options and arbitrary configuration sections stay in the original
	// repository. Passing these values as arguments avoids interpolation in a file.
	const keys = `^(core\.(autocrlf|eol|safecrlf|filemode|symlinks|ignorecase|precomposeunicode|checkstat|trustctime|excludesfile|attributesfile)|filter\.lfs\.(clean|process|smudge)|lfs\.(storage|extension\..+))$`
	output, err := r.readLimited(ctx, "", 64<<10, "config", "--null", "--get-regexp", keys)
	if err != nil && !gitExitCode(err, 1) {
		return nil, localLFSConfig{}, fmt.Errorf("read working copy settings: %w", err)
	}
	var settings []string
	lfs := localLFSConfig{storage: "lfs"}
	var smudge string
	for record := range strings.SplitSeq(strings.TrimSuffix(string(output), "\x00"), "\x00") {
		if record == "" {
			continue
		}
		key, value, present := strings.Cut(record, "\n")
		if !present {
			value = "true"
		}
		switch key {
		case "filter.lfs.clean":
			lfs.clean = value
		case "filter.lfs.process":
			lfs.process = value
		case "filter.lfs.smudge":
			smudge = value
		case "lfs.storage":
			if value != "" {
				lfs.storage = value
			}
		default:
			if strings.HasPrefix(key, "lfs.extension.") {
				lfs.extensions = true
				continue
			}
			if value != "" && !filepath.IsAbs(value) && !strings.HasPrefix(value, "~") {
				switch key {
				case "core.attributesfile":
					if !strings.HasPrefix(value, "%(prefix)/") {
						// Keep parent components for the filesystem to resolve after symlinks.
						value = r.WorkTree + string(os.PathSeparator) + value
					}
				case "core.excludesfile":
					value = filepath.Join(r.WorkTree, value)
				}
			}
			settings = append(settings, "-c", key+"="+value)
		}
	}
	if !filepath.IsAbs(lfs.storage) {
		lfs.storage = filepath.Join(r.CommonDir, lfs.storage)
	}
	lfs.skipSmudge = lfs.standardDriver() != "" && strings.HasSuffix(lfs.process, " --skip") ||
		lfs.process == "" && lfsDriverCommand(smudge, "smudge --skip -- %f") != ""
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GIT_LFS_SKIP_SMUDGE"))) {
	case "1", "true", "on", "yes":
		lfs.skipSmudge = true
	}
	if lfs.standardDriver() != "" {
		version, err := r.optionalRef(ctx, "config", "--local", "--includes", "--get", lfsFormatKey)
		if err != nil {
			return nil, localLFSConfig{}, err
		}
		lfs.unsupportedFormat = version != "" && version != "0"
	}
	return settings, lfs, nil
}

func (g *workingGit) initialize(ctx context.Context, head, format string, useRepositoryIndex bool) error {
	for _, folder := range []string{"refs", "info"} {
		if err := os.Mkdir(filepath.Join(g.dir, folder), 0o700); err != nil {
			return err
		}
	}
	if head == "" {
		head = "ref: refs/heads/working-copy"
	}
	if err := os.WriteFile(filepath.Join(g.dir, "HEAD"), []byte(head+"\n"), 0o600); err != nil {
		return err
	}
	config := "[core]\n\trepositoryformatversion = 0\n"
	if format == "sha256" {
		config = "[core]\n\trepositoryformatversion = 1\n[extensions]\n\tobjectformat = sha256\n"
	}
	if err := os.WriteFile(filepath.Join(g.dir, "config"), []byte(config), 0o600); err != nil {
		return err
	}
	for _, name := range []string{"exclude", "attributes"} {
		path := filepath.Join("info", name)
		if err := copyGitMetadata(filepath.Join(g.repo.CommonDir, path), filepath.Join(g.dir, path), 16<<20); err != nil {
			return err
		}
	}
	if !useRepositoryIndex {
		return nil
	}
	shared, err := g.repo.read(ctx, "", "rev-parse", "--path-format=absolute", "--shared-index-path")
	if err != nil {
		return err
	}
	if path := strings.TrimSuffix(string(shared), "\n"); path != "" {
		if !filepath.IsAbs(path) {
			path = filepath.Join(g.repo.WorkTree, filepath.FromSlash(path))
		}
		if err := copyGitMetadata(path, filepath.Join(g.dir, filepath.Base(path)), 512<<20); err != nil {
			return err
		}
	}
	return nil
}

func copyGitMetadata(source, destination string, limit int64) error {
	input, err := os.OpenFile(source, os.O_RDONLY|workFileOpenFlags, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer input.Close()
	return copyGitRegularFile(input, destination, limit)
}

func copyGitRegularFile(input *os.File, destination string, limit int64) error {
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return fmt.Errorf("invalid or oversized Git metadata")
	}
	writer, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	written, readErr := io.Copy(writer, io.LimitReader(input, limit+1))
	closeErr := writer.Close()
	if written > limit {
		return fmt.Errorf("Git metadata exceeds the read limit")
	}
	return errors.Join(readErr, closeErr)
}

func (g *workingGit) close() { _ = os.RemoveAll(g.dir) }

func (g *workingGit) read(ctx context.Context, limit int64, args ...string) ([]byte, error) {
	return g.run(ctx, "", limit, args...)
}

func (g *workingGit) run(ctx context.Context, input string, limit int64, args ...string) ([]byte, error) {
	return readGitOutput(ctx, g.command(ctx, input, args...), limit, args[0])
}

func (g *workingGit) command(ctx context.Context, input string, args ...string) *exec.Cmd {
	options := []string{"--git-dir=" + g.dir, "--work-tree=" + g.workTree}
	options = append(options, g.settings...)
	cmd := gitCommand(ctx, g.repo.git, g.workTree, options, input, args...)
	if g.groupCancellation {
		process.SetGroupCancellation(cmd)
	}
	objects := filepath.Join(g.repo.CommonDir, "objects")
	if g.privateObjects {
		// Git parses C-style quoted entries in this path list. Quoting keeps
		// a path containing the platform's list separator in one entry.
		cmd.Env = append(cmd.Env, "GIT_ALTERNATE_OBJECT_DIRECTORIES="+quoteGitPath(objects))
		objects = filepath.Join(g.dir, "objects")
	}
	cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_SYSTEM="+os.DevNull, "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_OBJECT_DIRECTORY="+objects, "GIT_INDEX_FILE="+g.index)
	if g.frozenAttributes {
		cmd.Env = append(cmd.Env, "GIT_ATTR_NOSYSTEM=1")
	}
	cmd.Env = append(cmd.Env, g.lfsEnvironment...)
	return cmd
}

var gitPathQuote = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)

func quoteGitPath(path string) string { return `"` + gitPathQuote.Replace(path) + `"` }

// Copy the selected file and its ancestor attributes into the private
// directory. Git can apply built-in conversions without reopening the user's
// mutable path during diff generation.
func (g *workingGit) snapshotFile(ctx context.Context, change Change, limit int64) (workFileContent, error) {
	if err := g.isolateWorkTree(); err != nil {
		return workFileContent{}, err
	}
	copyFile := func(relative string, contents workFileContent) error {
		path := filepath.Join(g.workTree, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if contents.mode&os.ModeSymlink != 0 {
			return os.Symlink(string(contents.data), path)
		}
		return os.WriteFile(path, contents.data, 0o600|(contents.mode.Perm()&0o100))
	}
	path := change.Path
	const attributeBudget = 16 << 20
	remaining := int64(attributeBudget)
	for i := 0; ; {
		attributes := path[:i] + ".gitattributes"
		if attributes != path {
			contents, err := g.repo.workFile(ctx, attributes, remaining)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return workFileContent{}, fmt.Errorf("read diff attributes: %w", err)
			}
			if err == nil && contents.mode.IsRegular() {
				remaining -= int64(len(contents.data))
				if err := copyFile(attributes, contents); err != nil {
					return workFileContent{}, err
				}
			}
		}
		separator := strings.IndexByte(path[i:], '/')
		if separator == -1 {
			break
		}
		i += separator + 1
	}
	if change.WorkTree == 'D' {
		return workFileContent{}, nil
	}
	contents, err := g.repo.workFile(ctx, path, limit)
	if err != nil {
		return contents, err
	}
	if err := copyFile(path, contents); err != nil {
		return contents, err
	}
	return contents, nil
}

func (g *workingGit) isolateWorkTree() error {
	g.workTree = filepath.Join(g.dir, "worktree")
	return os.Mkdir(g.workTree, 0o700)
}
