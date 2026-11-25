//go:build darwin && !ios

package appupdate

import (
	"context"
	"debug/macho"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

var errMacInstallationChanged = errors.New("the installed GitWave application changed")

type macBundle struct {
	path, id, requirement, originalHash string
	parent, original                    os.FileInfo
}

func InstallerForRunningApp() Installer {
	executable, err := os.Executable()
	if err != nil {
		return nil
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
	if filepath.Ext(bundle) != ".app" || executable != macExecutable(bundle) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	team, err := installedMacTeam(ctx, bundle)
	if err != nil {
		return nil
	}
	installed, err := bindMacBundle(ctx, bundle, DesktopAppID, macApplicationRequirement(team))
	if err != nil {
		return nil
	}
	runningHash, err := macProcessCodeHash(os.Getpid())
	if err != nil || runningHash != installed.originalHash {
		return nil
	}
	return installed.install
}

func macApplicationRequirement(team string) string {
	return `identifier "` + DesktopAppID + `" and anchor apple generic and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = "` + team + `"`
}

func macExecutable(bundle string) string {
	return filepath.Join(bundle, "Contents", "MacOS", "GitWave")
}

func installedMacTeam(ctx context.Context, bundle string) (string, error) {
	output, err := macTool(ctx, "/usr/bin/codesign", "-d", "--verbose=4", "--", bundle)
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(output, "\n") {
		if team, ok := strings.CutPrefix(line, "TeamIdentifier="); ok && validMacTeam(team) {
			return team, nil
		}
	}
	return "", errors.New("this GitWave bundle has no Developer ID team")
}

func validMacTeam(team string) bool {
	return len(team) == 10 && !strings.ContainsFunc(team, func(r rune) bool { return (r < 'A' || r > 'Z') && (r < '0' || r > '9') })
}

func bindMacBundle(ctx context.Context, bundle, id, requirement string) (*macBundle, error) {
	parent, err := os.Stat(filepath.Dir(bundle))
	if err != nil {
		return nil, err
	}
	installed, err := os.Lstat(bundle)
	if err != nil {
		return nil, err
	}
	if !installed.IsDir() {
		return nil, errMacInstallationChanged
	}
	hash, err := macCodeHash(ctx, bundle)
	if err != nil {
		return nil, err
	}
	return &macBundle{path: bundle, id: id, requirement: requirement, originalHash: hash, parent: parent, original: installed}, nil
}

func (b *macBundle) install(ctx context.Context, candidate *Candidate, progress func(int64, int64)) (Restart, error) {
	if candidate == nil {
		return nil, ErrPackage
	}
	artifact, err := ArtifactName(candidate.Version(), MacOSArm64)
	if err != nil || artifact != candidate.artifact {
		return nil, ErrPackage
	}
	dirPath := filepath.Dir(b.path)
	dir, err := os.Open(dirPath)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	parent, err := dir.Stat()
	if err != nil || !os.SameFile(parent, b.parent) || !sameMacDirectory(b.path, b.original) {
		return nil, errMacInstallationChanged
	}
	stage, err := os.MkdirTemp(dirPath, ".gitwave-update-")
	if err != nil {
		return nil, err
	}
	stageInfo, err := os.Lstat(stage)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep && sameMacDirectory(stage, stageInfo) {
			_ = os.RemoveAll(stage)
		}
	}()
	extracted, err := prepareMacBundle(ctx, stage, candidate, progress)
	if err != nil {
		return nil, err
	}
	newBundle := filepath.Join(extracted, macArchiveBundle)
	if err := b.verifyMacUpdate(ctx, newBundle, candidate.Version()); err != nil {
		return nil, err
	}
	newInfo, err := os.Lstat(newBundle)
	if err != nil || !newInfo.IsDir() {
		return nil, ErrPackage
	}
	newHash, err := macCodeHash(ctx, newBundle)
	if err != nil {
		return nil, err
	}
	if err := verifyMacCode(ctx, b.path, b.requirement); err != nil {
		return nil, err
	}
	if liveHash, err := macCodeHash(ctx, b.path); err != nil || liveHash != b.originalHash {
		return nil, errMacInstallationChanged
	}
	backup, err := b.publishMacBundle(ctx, dir, stage, extracted, stageInfo, newInfo, candidate.Version(), newHash)
	keep = backup != ""
	if err != nil {
		return nil, macUpdateRecoveryError(err, backup)
	}
	return func(ctx context.Context) (func(bool) error, error) {
		finish, err := b.relaunch(ctx, candidate.Version(), newHash, newInfo, os.Args[1:])
		if err != nil {
			return nil, macUpdateRecoveryError(err, backup)
		}
		return func(quit bool) error {
			if quit && (!sameMacDirectory(b.path, newInfo) || !sameMacDirectory(dirPath, b.parent)) {
				return macUpdateRecoveryError(errors.Join(errMacInstallationChanged, finish(false)), backup)
			}
			if err := finish(quit); err != nil {
				return macUpdateRecoveryError(err, backup)
			}
			if quit && sameMacDirectory(b.path, newInfo) && sameMacDirectory(stage, stageInfo) && sameMacDirectory(backup, b.original) {
				_ = os.RemoveAll(stage)
			}
			return nil
		}, nil
	}, nil
}

func prepareMacBundle(ctx context.Context, stage string, candidate *Candidate, progress func(int64, int64)) (string, error) {
	input, err := os.Create(filepath.Join(stage, "download.zip"))
	if err != nil {
		return "", err
	}
	defer input.Close()
	if err := candidate.downloadTo(ctx, input, progress); err != nil {
		return "", err
	}
	return extractMacBundle(ctx, stage, input)
}

func extractMacBundle(ctx context.Context, stage string, input *os.File) (string, error) {
	rewritten, err := os.Create(filepath.Join(stage, "verified.zip"))
	if err != nil {
		return "", err
	}
	defer rewritten.Close()
	if err := rewriteMacArchive(ctx, input, rewritten); err != nil {
		return "", err
	}
	if err := rewritten.Close(); err != nil {
		return "", err
	}
	extracted := filepath.Join(stage, "extracted")
	if err := os.Mkdir(extracted, 0o700); err != nil {
		return "", err
	}
	if _, err := macTool(ctx, "/usr/bin/ditto", "-x", "-k", rewritten.Name(), extracted); err != nil {
		return "", err
	}
	return extracted, nil
}

func (b *macBundle) verifyMacUpdate(ctx context.Context, bundle, version string) error {
	if err := verifyMacCode(ctx, bundle, b.requirement); err != nil {
		return err
	}
	for key, want := range map[string]string{"CFBundleIdentifier": b.id, "CFBundleExecutable": "GitWave", "CFBundleShortVersionString": version} {
		got, err := macBundleProperty(ctx, bundle, key)
		if err != nil || got != want {
			return fmt.Errorf("%w: unexpected %s in the replacement application", ErrPackage, key)
		}
	}
	return nil
}

// VerifyMacReleaseBundle checks the same bundle identity and Developer ID requirement as the updater.
func VerifyMacReleaseBundle(ctx context.Context, bundle, version, team string) (string, error) {
	if !validMacTeam(team) {
		return "", errors.New("invalid GitWave Developer ID team")
	}
	expected := macBundle{id: DesktopAppID, requirement: macApplicationRequirement(team)}
	if err := expected.verifyMacUpdate(ctx, bundle, version); err != nil {
		return "", err
	}
	if err := requireMacArm64(macExecutable(bundle)); err != nil {
		return "", err
	}
	return macCodeHash(ctx, bundle)
}

func requireMacArm64(path string) error {
	fat, err := macho.OpenFat(path)
	switch {
	case err == nil:
		defer fat.Close()
		for _, arch := range fat.Arches {
			if arch.Cpu == macho.CpuArm64 {
				return nil
			}
		}
	case errors.Is(err, macho.ErrNotFat):
		thin, err := macho.Open(path)
		if err != nil {
			return err
		}
		defer thin.Close()
		if thin.Cpu == macho.CpuArm64 {
			return nil
		}
	default:
		return err
	}
	return errors.New("the macOS release executable does not support arm64")
}

// VerifyMacUpdateArchive checks a release ZIP through the updater's own extractor.
func VerifyMacUpdateArchive(ctx context.Context, path, version, team, expectedHash string) error {
	input, err := os.Open(path)
	if err != nil {
		return err
	}
	defer input.Close()
	stage, err := os.MkdirTemp("", "gitwave-macos-update-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	extracted, err := extractMacBundle(ctx, stage, input)
	if err != nil {
		return err
	}
	bundle := filepath.Join(extracted, macArchiveBundle)
	hash, err := VerifyMacReleaseBundle(ctx, bundle, version, team)
	if err != nil {
		return err
	}
	if hash != expectedHash {
		return errors.New("the macOS update ZIP contains a different application")
	}
	_, err = macTool(ctx, "/usr/bin/xcrun", "stapler", "validate", bundle)
	return err
}

func macBundleProperty(ctx context.Context, path, key string) (string, error) {
	got, err := macTool(ctx, "/usr/bin/plutil", "-extract", key, "raw", "-o", "-", "--", filepath.Join(path, "Contents", "Info.plist"))
	return strings.TrimSpace(got), err
}

func verifyMacCode(ctx context.Context, bundle, requirement string) error {
	if _, err := macTool(ctx, "/usr/bin/codesign", "--verify", "--strict", "--deep", "--", bundle); err != nil {
		return err
	}
	_, err := macTool(ctx, "/usr/bin/codesign", "--verify", "--strict", "-R="+requirement, "--", bundle)
	return err
}

func macTool(ctx context.Context, tool string, args ...string) (string, error) {
	output, err := exec.CommandContext(ctx, tool, args...).CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("%s: %w: %.400s", filepath.Base(tool), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func (b *macBundle) publishMacBundle(ctx context.Context, parent *os.File, stage, extracted string, stageInfo, newInfo os.FileInfo, version, newHash string) (string, error) {
	if !sameMacDirectory(filepath.Dir(b.path), b.parent) || !sameMacDirectory(stage, stageInfo) {
		return "", errMacInstallationChanged
	}
	name := filepath.Base(b.path)
	old, err := openMacDirectoryAt(parent, name)
	if err != nil {
		return "", err
	}
	defer old.Close()
	if fi, err := old.Stat(); err != nil || !sameMacDirectoryInfo(fi, b.original) {
		return "", errMacInstallationChanged
	}
	if err := unix.Flock(int(old.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return "", err
	}
	stageDir, err := openMacDirectoryAt(parent, filepath.Base(stage))
	if err != nil {
		return "", err
	}
	defer stageDir.Close()
	if info, err := stageDir.Stat(); err != nil || !os.SameFile(info, stageInfo) {
		return "", errMacInstallationChanged
	}
	src, err := openMacDirectoryAt(stageDir, filepath.Base(extracted))
	if err != nil {
		return "", err
	}
	defer src.Close()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !sameMacDirectory(b.path, b.original) || !sameMacDirectory(filepath.Join(extracted, macArchiveBundle), newInfo) {
		return "", errMacInstallationChanged
	}
	srcFD, dstFD := int(src.Fd()), int(parent.Fd())
	backup := filepath.Join(extracted, macArchiveBundle)
	if err := unix.RenameatxNp(srcFD, macArchiveBundle, dstFD, name, unix.RENAME_SWAP); err != nil {
		return "", err
	}
	// Check both moved bundles before releasing the rollback copy to the caller.
	backupVerify := verifyMacCode(ctx, backup, b.requirement)
	installedVerify := b.verifyMacUpdate(ctx, b.path, version)
	backupHash, backupErr := macCodeHash(ctx, backup)
	installedHash, installedErr := macCodeHash(ctx, b.path)
	if !sameMacDirectory(backup, b.original) || !sameMacDirectory(b.path, newInfo) ||
		backupVerify != nil || installedVerify != nil || backupErr != nil || installedErr != nil ||
		backupHash != b.originalHash || installedHash != newHash {
		return backup, errors.Join(errMacInstallationChanged, b.restoreMacBundle(parent, src, backup, newInfo))
	}
	if err := errors.Join(parent.Sync(), src.Sync()); err != nil {
		return backup, errors.Join(fmt.Errorf("sync the installed application: %w", err), b.restoreMacBundle(parent, src, backup, newInfo))
	}
	return backup, nil
}

func (b *macBundle) restoreMacBundle(parent, source *os.File, backup string, replacement os.FileInfo) error {
	if !sameMacDirectory(backup, b.original) || !sameMacDirectory(b.path, replacement) {
		return errMacInstallationChanged
	}
	if err := unix.RenameatxNp(int(source.Fd()), macArchiveBundle, int(parent.Fd()), filepath.Base(b.path), unix.RENAME_SWAP); err != nil {
		return fmt.Errorf("roll back the application replacement: %w", err)
	}
	// A concurrent rename can race the exchange. The caller retains staging
	// after every failed publication, including when both paths were restored.
	if !sameMacDirectory(backup, replacement) || !sameMacDirectory(b.path, b.original) {
		return errMacInstallationChanged
	}
	return errors.Join(parent.Sync(), source.Sync())
}

func openMacDirectoryAt(parent *os.File, name string) (*os.File, error) {
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

func sameMacDirectory(path string, expected os.FileInfo) bool {
	actual, err := os.Lstat(path)
	return err == nil && sameMacDirectoryInfo(actual, expected)
}

func sameMacDirectoryInfo(actual, expected os.FileInfo) bool {
	return actual.IsDir() && os.SameFile(actual, expected)
}

func macUpdateRecoveryError(err error, backup string) error {
	if backup == "" {
		return err
	}
	return fmt.Errorf("%w; the update staging bundle was retained at %s", err, backup)
}
