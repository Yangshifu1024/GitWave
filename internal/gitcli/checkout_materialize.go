package gitcli

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

func (g *workingGit) removedCheckoutGitlinks(ctx context.Context, old, next string) ([]string, error) {
	if old == next {
		return nil, nil
	}
	output, err := g.read(ctx, 64<<20, "diff-tree", "-r", "--raw", "--no-renames", "--no-commit-id", "--ignore-submodules=none", "-z", old, next, "--")
	if err != nil || len(output) == 0 {
		return nil, err
	}
	var removed []string
	records := strings.Split(strings.TrimSuffix(string(output), "\x00"), "\x00")
	if len(records)%2 != 0 {
		return nil, fmt.Errorf("invalid Git checkout change list")
	}
	for i := 0; i < len(records); i += 2 {
		fields := strings.Fields(records[i])
		name := records[i+1]
		if len(fields) != 5 || !strings.HasPrefix(fields[0], ":") || !validIndexPath(name) {
			return nil, fmt.Errorf("invalid Git checkout change at %q", name)
		}
		if fields[0] == ":160000" && fields[1] != "160000" {
			removed = append(removed, name)
		}
	}
	return removed, nil
}

// Git can replace a gitlink with a blob by deleting the existing directory,
// even when checkout is told not to recurse into submodules.
func (g *workingGit) protectCheckoutGitlinks(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	root, err := os.OpenRoot(g.repo.WorkTree)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, name := range paths {
		local := filepath.FromSlash(name)
		actual, err := root.Lstat(local)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !actual.IsDir() {
			continue
		}
		dir, err := root.Open(local)
		if err != nil {
			return err
		}
		names, readErr := dir.Readdirnames(1)
		dir.Close()
		if len(names) != 0 {
			return fmt.Errorf("checkout would replace a nonempty submodule directory at %q", name)
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
	}
	return nil
}

// Prepare target blobs away from the live worktree. Some versions of Git
// exit successfully after reporting that they could not write all files.
func (g *workingGit) materializeCheckoutTarget(ctx context.Context, oid string, affected []string) error {
	if oid == "" {
		return nil
	}
	out, err := g.read(ctx, 128<<20, "ls-tree", "-r", "-z", oid, "--")
	if err != nil {
		return err
	}
	changed := make(map[string]bool, len(affected))
	for _, path := range affected {
		changed[path] = true
	}
	var allPaths, files []string
	attributes := make(map[string]bool)
	for record := range strings.SplitSeq(strings.TrimSuffix(string(out), "\x00"), "\x00") {
		if record == "" {
			continue
		}
		info, name, found := strings.Cut(record, "\t")
		fields := strings.Fields(info)
		if !found || !validIndexPath(name) || len(fields) != 3 || !validOID(fields[2]) {
			return fmt.Errorf("invalid path or entry in checkout tree: %q", name)
		}
		allPaths = append(allPaths, name)
		if changed[name] && filepath.Base(name) == ".gitattributes" {
			attributes[name] = fields[0] == "100644" || fields[0] == "100755"
		}
		if changed[name] && fields[1] == "blob" {
			files = append(files, name)
		}
	}
	root, err := os.OpenRoot(g.repo.WorkTree)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := checkCheckoutAliases(root, allPaths); err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	shadow, err := g.repo.workingGit(ctx, oid)
	if err != nil {
		return err
	}
	defer shadow.close()
	if err := shadow.isolateWorkTree(); err != nil {
		return err
	}
	shadow.index = filepath.Join(shadow.dir, "index")
	shadow.settings = append(shadow.settings, g.settings...)
	shadow.lfsEnvironment = g.lfsEnvironment
	shadow.frozenAttributes = g.frozenAttributes
	target, err := os.OpenRoot(shadow.workTree)
	if err != nil {
		return err
	}
	defer target.Close()
	if err := shadow.snapshotWorktreeMetadata(ctx, root, target, files, nil, ".gitattributes"); err != nil {
		return err
	}
	for _, name := range affected {
		if filepath.Base(name) == ".gitattributes" && !attributes[name] {
			// Git checkout-index falls back to worktree attributes when
			// the index has no file. A real checkout removes this file.
			if err := target.Remove(filepath.FromSlash(name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	if err := shadow.runCheckoutCommand(ctx, "", "read-tree", oid); err != nil {
		return err
	}
	return shadow.runCheckoutCommand(ctx, nulPaths(files), "checkout-index", "--temp", "--ignore-skip-worktree-bits", "-z", "--stdin")
}

func checkCheckoutAliases(root *os.Root, paths []string) error {
	fold := cases.Fold()
	candidate := func(s string) string { return norm.NFC.String(fold.String(s)) }
	if _, _, found := findCheckoutAlias(paths, candidate); !found {
		return nil
	}
	caseInsensitive, normalizes, err := probeCheckoutNames(root)
	if err != nil {
		return err
	}
	normalize := func(s string) string {
		if caseInsensitive {
			s = fold.String(s)
		}
		if normalizes {
			s = norm.NFC.String(s)
		}
		return s
	}
	if a, b, found := findCheckoutAlias(paths, normalize); found {
		return fmt.Errorf("this working filesystem cannot represent both %q and %q", a, b)
	}
	return nil
}

func findCheckoutAlias(paths []string, key func(string) string) (string, string, bool) {
	seen := make(map[string]string, len(paths))
	for _, name := range paths {
		for current := name; current != "."; current = path.Dir(current) {
			folded := key(current)
			if previous, ok := seen[folded]; ok {
				if previous != current {
					return previous, current, true
				}
				break
			}
			seen[folded] = current
		}
	}
	return "", "", false
}

func probeCheckoutNames(root *os.Root) (caseInsensitive, normalizes bool, err error) {
	temp := ".gitwave-name-check-" + rand.Text()
	if err := root.Mkdir(temp, 0o700); err != nil {
		return false, false, err
	}
	caseName := filepath.Join(temp, "case")
	unicodeName := filepath.Join(temp, "é")
	defer func() {
		_ = root.Remove(caseName)
		_ = root.Remove(unicodeName)
		_ = root.Remove(temp)
	}()
	for _, file := range []string{caseName, unicodeName} {
		if err := root.WriteFile(file, nil, 0o600); err != nil {
			return false, false, err
		}
	}
	_, caseErr := root.Lstat(filepath.Join(temp, "CASE"))
	if caseErr != nil && !errors.Is(caseErr, fs.ErrNotExist) {
		return false, false, caseErr
	}
	_, unicodeErr := root.Lstat(filepath.Join(temp, "e\u0301"))
	if unicodeErr != nil && !errors.Is(unicodeErr, fs.ErrNotExist) {
		return false, false, unicodeErr
	}
	return caseErr == nil, unicodeErr == nil, nil
}
