package gitcli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"gitwave.work/internal/gittool"
)

type lfsPushUpdate struct {
	tip, previous string
}

func readLFSPushUpdates(input []byte) ([]lfsPushUpdate, error) {
	var updates []lfsPushUpdate
	for line := range strings.SplitSeq(strings.TrimSuffix(string(input), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 4 || !validOID(fields[1]) || !validOID(fields[3]) || len(fields[1]) != len(fields[3]) {
			return nil, fmt.Errorf("invalid update from the Git pre-push hook")
		}
		if strings.Trim(fields[1], "0") == "" {
			continue
		}
		updates = append(updates, lfsPushUpdate{tip: fields[1], previous: fields[3]})
	}
	return updates, nil
}

// Collect the small Git blobs in the exact revisions Git proposes to send.
// Using Git's object filter keeps large ordinary blobs out of the batch.
func (s lfsPushSpec) pushObjects(ctx context.Context, updates []lfsPushUpdate) ([]lfsPointer, error) {
	if len(updates) == 0 {
		return nil, nil
	}
	var revisions strings.Builder
	seen, bases := make(map[string]bool), make(map[string]bool)
	for _, update := range updates {
		if !seen[update.tip] {
			fmt.Fprintln(&revisions, update.tip)
			seen[update.tip] = true
		}
		if strings.Trim(update.previous, "0") != "" {
			bases[update.previous] = true
		}
	}
	// Git can advertise a remote tip absent from the local object store.
	// Ignore only missing exclusions, after validating every positive tip.
	for oid := range bases {
		fmt.Fprintf(&revisions, "^%s\n", oid)
	}
	revList := s.objectCommand(ctx, "rev-list", "--objects", "--no-object-names", "--ignore-missing",
		"--filter=blob:limit=1024", "--filter=object:type=blob", "--stdin", "--")
	revList.Stdin = strings.NewReader(revisions.String())
	stdout, err := revList.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var diagnostic gitDiagnostic
	revList.Stderr = &diagnostic
	if err := revList.Start(); err != nil {
		return nil, err
	}
	completed := false
	defer func() {
		if !completed {
			_ = revList.Process.Kill()
			_ = revList.Wait()
		}
	}()
	const batchSize = 256
	scanner := bufio.NewScanner(stdout)
	var batch []string
	found := make(map[string]lfsPointer)
	for scanner.Scan() {
		oid := scanner.Text()
		if !validOID(oid) {
			return nil, fmt.Errorf("invalid object in the selected Git history")
		}
		batch = append(batch, oid)
		if len(batch) == batchSize {
			if err := s.collectLFSPointers(ctx, batch, found); err != nil {
				return nil, err
			}
			batch = batch[:0]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	err = revList.Wait()
	completed = true
	if err != nil {
		return nil, fmt.Errorf("scan Git history for LFS objects: %w (%s)", err, strings.TrimSpace(diagnostic.buffer.String()))
	}
	if err := s.collectLFSPointers(ctx, batch, found); err != nil {
		return nil, err
	}
	var objects []lfsPointer
	for _, entry := range found {
		objects = append(objects, entry)
	}
	slices.SortFunc(objects, func(a, b lfsPointer) int { return strings.Compare(a.oid, b.oid) })
	return objects, nil
}

func (s lfsPushSpec) collectLFSPointers(ctx context.Context, batch []string, found map[string]lfsPointer) error {
	if len(batch) == 0 {
		return nil
	}
	// An explicitly provided commit or tag can appear despite the filter.
	// Check types and sizes before opening a second batch with blob data.
	input := strings.Join(batch, "\n") + "\n"
	check := s.objectCommand(ctx, "cat-file", "--batch-check")
	check.Stdin = strings.NewReader(input)
	headers, err := readGitOutput(ctx, check, int64(len(batch)*192), "read LFS candidates")
	if err != nil {
		return err
	}
	var candidates []string
	records := strings.Split(strings.TrimSuffix(string(headers), "\n"), "\n")
	if len(records) != len(batch) {
		return fmt.Errorf("incomplete Git object information before LFS push")
	}
	for i, record := range records {
		fields := strings.Fields(record)
		if len(fields) != 3 || fields[0] != batch[i] {
			return fmt.Errorf("missing or invalid Git object before LFS push")
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 {
			return fmt.Errorf("invalid Git object size before LFS push")
		}
		if fields[1] == "blob" && size < lfsPointerLimit {
			candidates = append(candidates, batch[i])
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	read := s.objectCommand(ctx, "cat-file", "--batch")
	read.Stdin = strings.NewReader(strings.Join(candidates, "\n") + "\n")
	output, err := readGitOutput(ctx, read, int64(len(candidates)*(lfsPointerLimit+160)), "read possible LFS pointers")
	if err != nil {
		return err
	}
	for _, oid := range candidates {
		header, rest, ok := bytes.Cut(output, []byte{'\n'})
		fields := strings.Fields(string(header))
		if !ok || len(fields) != 3 || fields[0] != oid || fields[1] != "blob" {
			return fmt.Errorf("invalid Git blob in LFS scan")
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || size >= lfsPointerLimit || len(rest) <= size || rest[size] != '\n' {
			return fmt.Errorf("truncated or invalid Git blob in LFS scan")
		}
		if pointer, ok := parseLFSPointer(rest[:size]); ok && pointer.size > 0 {
			if prior, exists := found[pointer.oid]; exists && prior.size != pointer.size {
				return fmt.Errorf("inconsistent sizes for LFS object %s", pointer.oid)
			}
			found[pointer.oid] = pointer
		}
		output = rest[size+1:]
	}
	if len(output) != 0 {
		return fmt.Errorf("unexpected data in the Git LFS scan")
	}
	return nil
}

func (s lfsPushSpec) objectCommand(ctx context.Context, args ...string) *exec.Cmd {
	options := []string{"--no-replace-objects", "--git-dir=" + s.Stage}
	// Keep children in the Git push's process group for cancellation.
	cmd := exec.CommandContext(ctx, s.Git, append(options, args...)...)
	cmd.Dir = s.Stage
	cmd.Env = append(gittool.Environment(isolatedLFSConfig(), s.Git, ""), "GIT_OBJECT_DIRECTORY="+s.GitObjects)
	return cmd
}

func readLFSPushInput(in io.Reader) ([]byte, error) {
	const maxInput = 8 << 20
	data, err := io.ReadAll(io.LimitReader(in, maxInput+1))
	if len(data) > maxInput {
		return nil, fmt.Errorf("the Git pre-push update list exceeds the size limit")
	}
	return data, err
}
