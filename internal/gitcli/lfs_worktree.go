package gitcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type localLFSConfig struct {
	clean, process, storage string
	skipSmudge, extensions  bool
	unsupportedFormat       bool
}

func (c localLFSConfig) supported() bool {
	return c.standardDriver() != "" && !c.extensions && !c.unsupportedFormat
}

func (c localLFSConfig) standardDriver() string {
	if c.process != "" {
		return lfsDriverCommand(strings.TrimSuffix(c.process, " --skip"), "filter-process")
	}
	if driver := lfsDriverCommand(c.clean, "clean -- %f"); driver != "" {
		return driver
	}
	return lfsDriverCommand(c.clean, "clean %f")
}

func lfsDriverCommand(command, action string) string {
	driver, ok := strings.CutSuffix(command, " "+action)
	if !ok {
		return ""
	}
	switch driver {
	case "git-lfs", "git-lfs.exe", "git lfs":
		return driver
	}
	if len(driver) > 2 && (driver[0] == '\'' || driver[0] == '"') && driver[len(driver)-1] == driver[0] {
		if driver[0] == '\'' {
			driver = strings.ReplaceAll(driver, "'\\''", "'")
		}
		driver = driver[1 : len(driver)-1]
	}
	if filepath.IsAbs(driver) {
		switch strings.ToLower(filepath.Base(driver)) {
		case "git-lfs", "git-lfs.exe":
			return driver
		}
	}
	return ""
}

const (
	lfsFilterExecutable = "GITWAVE_LFS_EXECUTABLE"
	lfsFilterStage      = "GITWAVE_LFS_STAGE_DIR"
	lfsFilterCache      = "GITWAVE_LFS_CACHE_DIR"
	lfsFilterTemp       = "GITWAVE_LFS_TEMP_DIR"
	lfsFilterAllowed    = "GITWAVE_LFS_ALLOWED_DIR"
)

// configureLocalLFS installs trusted commands only in the private Git process.
// No commands or extensions from the repository are executed.
func (g *workingGit) configureLocalLFS(write, hydrate bool) error {
	if !g.localLFS.supported() {
		return nil
	}
	if len(g.lfsEnvironment) == 0 {
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		g.settings = append(g.settings,
			"-c", `filter.lfs.clean="$GITWAVE_LFS_EXECUTABLE" `+lfsFilterArg+" clean",
			"-c", `filter.lfs.smudge="$GITWAVE_LFS_EXECUTABLE" `+lfsFilterArg+" smudge %f",
			"-c", "filter.lfs.required=true")
		g.lfsEnvironment = []string{
			lfsFilterExecutable + "=" + filepath.ToSlash(executable),
			lfsFilterTemp + "=" + g.dir,
			lfsFilterCache + "=",
			lfsFilterStage + "=",
			lfsFilterAllowed + "=",
		}
	}
	if write && g.lfsStageDir == "" {
		dir, err := os.MkdirTemp(g.dir, "lfs-stage-")
		if err != nil {
			return err
		}
		g.lfsStageDir = dir
		g.lfsEnvironment = append(g.lfsEnvironment, lfsFilterStage+"="+dir)
	}
	if hydrate && !g.localLFS.skipSmudge {
		g.lfsEnvironment = append(g.lfsEnvironment, lfsFilterCache+"="+g.localLFS.storage)
	}
	return nil
}

func (g *workingGit) restrictLFSCheckout(changed []string, allowed func(string) bool) (*workingGit, error) {
	if allowed == nil || len(g.lfsEnvironment) == 0 {
		return g, nil
	}
	dir, err := os.MkdirTemp(g.dir, "lfs-allowed-")
	if err != nil {
		return nil, err
	}
	for _, name := range changed {
		if allowed(name) {
			if err := os.WriteFile(filepath.Join(dir, lfsPathKey(name)), nil, 0o600); err != nil {
				return nil, err
			}
		}
	}
	target := *g
	target.lfsEnvironment = append(slices.Clone(g.lfsEnvironment), lfsFilterAllowed+"="+dir)
	return &target, nil
}

func (g *workingGit) publishLFSObjects(ctx context.Context) error {
	if g.lfsStageDir == "" {
		return nil
	}
	objects, err := os.ReadDir(g.lfsStageDir)
	if err != nil || len(objects) == 0 {
		return err
	}
	staged, err := os.OpenRoot(g.lfsStageDir)
	if err != nil {
		return err
	}
	defer staged.Close()
	cache, err := g.openLFSStorage(ctx)
	if err != nil {
		return err
	}
	defer cache.Close()
	for _, entry := range objects {
		oid := entry.Name()
		if !validLFSOID(oid) {
			return fmt.Errorf("invalid staged LFS object %q", oid)
		}
		object, err := openLFSRegular(staged, oid)
		if err != nil {
			return err
		}
		err = storeLFSObject(ctx, cache, object, oid)
		object.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (g *workingGit) verifyLocalLFSConfig(ctx context.Context) error {
	if len(g.lfsEnvironment) == 0 {
		return nil
	}
	_, current, err := g.repo.workingGitSettings(ctx)
	if err != nil {
		return err
	}
	if current != g.localLFS {
		return fmt.Errorf("the Git LFS configuration changed during the operation")
	}
	return nil
}

func (g *workingGit) openLFSStorage(ctx context.Context) (*os.Root, error) {
	if err := g.verifyLocalLFSConfig(ctx); err != nil {
		return nil, err
	}
	storage := g.localLFS.storage
	if err := os.MkdirAll(storage, 0o755); err != nil {
		return nil, err
	}
	return os.OpenRoot(storage)
}
