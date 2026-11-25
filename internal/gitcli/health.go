package gitcli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	staleBranchAge       = 30 * 24 * time.Hour
	healthLargeFileBytes = 1 << 20
	healthFileLimit      = 5
	healthEntryLimit     = 50_000
	healthDateAttempts   = 128
)

type HealthReport struct {
	Unpushed          *int        `json:"unpushed"`
	ConflictResidue   []string    `json:"conflict_residue"`
	DirtyFiles        int         `json:"dirty_files"`
	SubmoduleWarning  string      `json:"submodule_warning,omitempty"`
	StaleBranches     []string    `json:"stale_branches"`
	LargeFiles        []LargeFile `json:"large_files"`
	LargeFilesLimited bool        `json:"large_files_limited,omitempty"`
	BranchCount       int         `json:"branch_count"`
	TagCount          int         `json:"tag_count"`
}

type LargeFile struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
}

// Health reads local metrics for the selected checkout without updating its index.
func (r *Repository) Health(ctx context.Context) (HealthReport, error) {
	report := HealthReport{ConflictResidue: []string{}, StaleBranches: []string{}, LargeFiles: []LargeFile{}}
	if err := r.collectRefHealth(ctx, &report, time.Now().Add(-staleBranchAge).Unix()); err != nil {
		return HealthReport{}, err
	}
	if r.WorkTree == "" {
		return report, nil
	}
	status, err := r.healthWorkingStatus(ctx)
	if err != nil {
		return HealthReport{}, err
	}
	report.DirtyFiles = len(status.Changes)
	report.SubmoduleWarning = status.SubmoduleWarning
	report.LargeFiles, report.LargeFilesLimited, err = scanHealthFiles(ctx, r.WorkTree, healthEntryLimit)
	return report, err
}

func (r *Repository) collectRefHealth(ctx context.Context, report *HealthReport, cutoff int64) error {
	head, err := r.optionalRef(ctx, "symbolic-ref", "-q", "HEAD")
	if err != nil {
		return err
	}
	// The object-independent format also lists refs whose objects are missing.
	output, err := r.readLimited(ctx, "", 32<<20, "for-each-ref", "--include-root-refs", "--sort=refname",
		"--format=%(refname)%00%(objectname)%00%(symref)%00%(upstream)",
		"refs/heads", "refs/tags", "CHERRY_PICK_HEAD", "REVERT_HEAD", "REBASE_HEAD")
	if err != nil {
		return err
	}
	type branchTip struct{ name, oid string }
	var tips []branchTip
	oids := make(map[string]bool)
	rootRefs := make(map[string]bool)
	var current, upstream string
	for line := range strings.SplitSeq(strings.TrimSuffix(string(output), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\x00")
		if len(fields) != 4 || !validOID(fields[1]) {
			return fmt.Errorf("invalid Git reference in health report")
		}
		name, oid, symbolic, tracking := fields[0], fields[1], fields[2], fields[3]
		if strings.HasPrefix(name, "refs/tags/") {
			report.TagCount++
			continue
		}
		if !strings.HasPrefix(name, "refs/heads/") {
			rootRefs[name] = true
			continue
		}
		if !utf8.ValidString(name) {
			continue
		}
		report.BranchCount++
		if name == head {
			current, upstream = oid, tracking
		} else if symbolic == "" {
			tips = append(tips, branchTip{strings.TrimPrefix(name, "refs/heads/"), oid})
		}
		oids[oid] = true
	}
	for _, name := range []string{"MERGE_HEAD", "REVERT_HEAD", "CHERRY_PICK_HEAD", "REBASE_HEAD", "rebase-merge", "rebase-apply", "BISECT_LOG"} {
		if _, err := os.Stat(filepath.Join(r.GitDir, name)); err == nil || rootRefs[name] {
			report.ConflictResidue = append(report.ConflictResidue, name)
		}
	}
	dates, err := r.healthCommitDates(ctx, oids)
	if err != nil {
		return err
	}
	for _, tip := range tips {
		if date, ok := dates[tip.oid]; ok && date < cutoff {
			report.StaleBranches = append(report.StaleBranches, tip.name)
		}
	}
	if _, ok := dates[current]; !ok || upstream == "" {
		return nil
	}
	tip, err := r.optionalRef(ctx, "rev-parse", "--verify", "--quiet", upstream+"^{commit}")
	if err != nil || !validOID(tip) {
		return err
	}
	output, err = r.readLimited(ctx, "", 128, "rev-list", "--count", tip+".."+current, "--")
	if err != nil {
		// An incomplete graph leaves the count unavailable, as with a missing upstream.
		return ctx.Err()
	}
	ahead, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil || ahead < 0 {
		return fmt.Errorf("invalid Git ahead count in health report")
	}
	report.Unpushed = &ahead
	return nil
}

func (r *Repository) healthCommitDates(ctx context.Context, oids map[string]bool) (map[string]int64, error) {
	ids := slices.Sorted(maps.Keys(oids))
	dates := make(map[string]int64, len(ids))
	attempts := healthDateAttempts
	return dates, r.readHealthDates(ctx, ids, dates, &attempts)
}

func (r *Repository) readHealthDates(ctx context.Context, ids []string, dates map[string]int64, attempts *int) error {
	if len(ids) == 0 || *attempts == 0 {
		return ctx.Err()
	}
	*attempts -= 1
	output, err := r.readLimited(ctx, strings.Join(ids, "\n")+"\n", 32<<20, "log", "--ignore-missing", "--no-walk=unsorted", "--stdin",
		"--no-show-signature", "--no-notes", "--no-decorate", "--no-patch", "--color=never", "-z", "--format=%H%x00%ct", "--")
	if gitExitCode(err, 128) {
		// Some damaged object files abort Git even with --ignore-missing.
		// Isolate those tips with a bounded number of read attempts.
		if len(ids) == 1 {
			return nil
		}
		mid := len(ids) / 2
		if err := r.readHealthDates(ctx, ids[:mid], dates, attempts); err != nil {
			return err
		}
		return r.readHealthDates(ctx, ids[mid:], dates, attempts)
	}
	if err != nil || len(output) == 0 {
		return err
	}
	fields := strings.Split(string(output), "\x00")
	if len(fields)%2 != 1 || fields[len(fields)-1] != "" {
		return fmt.Errorf("incomplete Git commit data in health report")
	}
	for i := 0; i+1 < len(fields); i += 2 {
		oid := fields[i]
		date, err := strconv.ParseInt(fields[i+1], 10, 64)
		if _, found := slices.BinarySearch(ids, oid); err == nil && found {
			dates[oid] = date
		}
	}
	return nil
}

func (r *Repository) healthWorkingStatus(ctx context.Context) (Status, error) {
	head, err := r.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return Status{}, err
	}
	return r.workingStatus(ctx, head, "--untracked-files=all", "--no-renames", "--branch")
}

func scanHealthFiles(ctx context.Context, path string, budget int) ([]LargeFile, bool, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, false, err
	}
	defer root.Close()
	large := make([]LargeFile, 0, healthFileLimit+1)
	dirs := []string{"."}
	for len(dirs) > 0 && budget > 0 {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		var dir string
		dir, dirs = dirs[len(dirs)-1], dirs[:len(dirs)-1]
		file, err := root.OpenFile(dir, os.O_RDONLY|workFileOpenFlags, 0)
		if err != nil {
			if dir == "." {
				return nil, false, err
			}
			continue
		}
		for budget > 0 && ctx.Err() == nil {
			entries, err := file.ReadDir(min(128, budget))
			budget -= len(entries)
			for _, entry := range entries {
				name := filepath.Join(dir, entry.Name())
				if entry.IsDir() {
					if !skipHealthDirectory(entry.Name()) {
						dirs = append(dirs, name)
					}
					continue
				}
				if !entry.Type().IsRegular() {
					continue
				}
				info, err := root.Lstat(name)
				if err == nil && info.Mode().IsRegular() && info.Size() >= healthLargeFileBytes {
					large = append(large, LargeFile{Path: strings.ToValidUTF8(name, "\uFFFD"), SizeBytes: info.Size()})
					slices.SortFunc(large, func(a, b LargeFile) int {
						if size := cmp.Compare(b.SizeBytes, a.SizeBytes); size != 0 {
							return size
						}
						return cmp.Compare(a.Path, b.Path)
					})
					large = large[:min(len(large), healthFileLimit)]
				}
			}
			if err != nil || len(entries) == 0 {
				if err != nil && !errors.Is(err, io.EOF) && dir == "." {
					file.Close()
					return nil, false, err
				}
				break
			}
		}
		file.Close()
	}
	return large, budget == 0, ctx.Err()
}

func skipHealthDirectory(name string) bool {
	switch strings.ToLower(name) {
	case ".git", "node_modules", "target", "dist", "build", "vendor", ".venv", "__pycache__":
		return true
	default:
		return false
	}
}
