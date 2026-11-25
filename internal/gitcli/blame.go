package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxBlameLines  = 250_000
	maxBlameOutput = 32 << 20
)

var ErrBlameTooLarge = errors.New("file exceeds the blame preview limit")

type BlameLine struct {
	LineNo      int    `json:"line_no"`
	SHA         string `json:"sha"`
	Author      string `json:"author"`
	AuthorEmail string `json:"author_email"`
	Time        int64  `json:"time"`
	Content     string `json:"content"`
}

// Blame annotates the same immutable blob used by the selected file
// version, even if the worktree or HEAD changes while Git calculates it.
func (r *Repository) Blame(ctx context.Context, commit Commit, path string, deleted bool) ([]BlameLine, error) {
	if !validOID(commit.SHA) {
		return nil, fmt.Errorf("invalid Git commit ID")
	}
	if !validFilePath(path) {
		return nil, fmt.Errorf("invalid Git file path")
	}
	revision := commit.SHA
	if deleted {
		var err error
		revision, err = r.firstParent(ctx, commit)
		if err != nil {
			return nil, fmt.Errorf("read parent for blame: %w", err)
		}
		if revision == "" {
			return nil, fmt.Errorf("no parent for deleted file")
		}
	}
	object := revision + ":" + path
	sizeOutput, err := r.read(ctx, "", "cat-file", "-s", object)
	if err != nil {
		return nil, fmt.Errorf("read file size for blame: %w", err)
	}
	size, err := strconv.Atoi(strings.TrimSpace(string(sizeOutput)))
	if err != nil || size < 0 {
		return nil, fmt.Errorf("invalid Git file size")
	}
	if size > expandedBytes {
		return nil, ErrBlameTooLarge
	}
	blob, err := r.readLimited(ctx, "", expandedBytes, "cat-file", "blob", object)
	if err != nil {
		return nil, blameReadError("read file for blame", err)
	}
	if len(blob) == 0 {
		return nil, nil
	}
	lineCount := bytes.Count(blob, []byte{'\n'})
	if blob[len(blob)-1] != '\n' {
		lineCount++
	}
	if lineCount > maxBlameLines {
		return nil, ErrBlameTooLarge
	}
	lines := blameContent(lossyGitText(string(blob)), lineCount)
	output, err := r.readLimited(ctx, "", maxBlameOutput, "blame", "--incremental", "--root", "--no-textconv",
		"--diff-algorithm=myers", "--no-indent-heuristic", "--no-ignore-revs-file", revision, "--", path)
	if err != nil {
		return nil, blameReadError("read blame", err)
	}
	oids, err := assignBlame(ctx, string(output), lines)
	if err != nil {
		return nil, err
	}

	// Git blame applies the mailmap. Read original author fields and
	// committer timestamps from the commit objects, as the old view did.
	type author struct {
		name, email string
		time        int64
	}
	authors := make(map[string]author, len(oids))
	remaining := int64(maxBlameOutput)
	for batch := range slices.Chunk(oids, 300) {
		const columns = 4
		if remaining == 0 {
			return nil, ErrBlameTooLarge
		}
		fields, err := r.readCommitFields(ctx, batch, min(expandedBytes, remaining), "%H%x00%an%x00%ae%x00%ct", columns)
		if err != nil {
			return nil, blameReadError("read blame commits", err)
		}
		for i, oid := range batch {
			row := fields[i*columns : (i+1)*columns]
			stamp, err := strconv.ParseInt(row[3], 10, 64)
			if row[0] != oid || err != nil {
				return nil, fmt.Errorf("invalid Git blame commit data")
			}
			remaining -= int64(len(row[0]) + len(row[1]) + len(row[2]) + len(row[3]) + columns)
			authors[oid] = author{name: row[1], email: row[2], time: stamp}
		}
	}
	for i := range lines {
		attribution := authors[lines[i].SHA]
		if utf8.ValidString(attribution.name) {
			lines[i].Author = attribution.name
		}
		if utf8.ValidString(attribution.email) {
			lines[i].AuthorEmail = attribution.email
		}
		lines[i].Time = attribution.time
	}
	return lines, ctx.Err()
}

func blameReadError(operation string, err error) error {
	if errors.Is(err, errGitOutputLimit) {
		return ErrBlameTooLarge
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func blameContent(content string, count int) []BlameLine {
	lines := make([]BlameLine, 0, count)
	for content != "" {
		line, rest, newline := strings.Cut(content, "\n")
		if newline {
			line = strings.TrimSuffix(line, "\r")
		}
		lines = append(lines, BlameLine{LineNo: len(lines) + 1, Content: line})
		content = rest
	}
	return lines
}

// Each incremental group ends with a filename line. Groups may arrive
// out of order, and repeated commits may omit their metadata.
func assignBlame(ctx context.Context, output string, lines []BlameLine) ([]string, error) {
	var oids []string
	seen := make(map[string]bool)
	assigned := 0
	for output != "" {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		header, next, ok := strings.Cut(output, "\n")
		fields := strings.Fields(header)
		if !ok || len(fields) != 4 || !validOID(fields[0]) {
			return nil, fmt.Errorf("invalid Git blame group")
		}
		oid := fields[0]
		start, startErr := strconv.Atoi(fields[2])
		count, countErr := strconv.Atoi(fields[3])
		if startErr != nil || countErr != nil || start < 1 || start > len(lines) ||
			count < 1 || count > len(lines)-start+1 {
			return nil, fmt.Errorf("invalid Git blame range")
		}
		terminated := false
		for next != "" {
			var line string
			line, next, ok = strings.Cut(next, "\n")
			if !ok {
				break
			}
			if strings.HasPrefix(line, "filename ") {
				terminated = true
				break
			}
		}
		if !terminated {
			return nil, fmt.Errorf("incomplete Git blame group")
		}
		output = next
		for i := start - 1; i < start-1+count; i++ {
			if lines[i].SHA != "" {
				return nil, fmt.Errorf("overlapping Git blame ranges")
			}
			lines[i].SHA = oid
		}
		assigned += count
		if !seen[oid] {
			seen[oid] = true
			oids = append(oids, oid)
		}
	}
	if assigned != len(lines) {
		return nil, fmt.Errorf("incomplete Git blame")
	}
	return oids, nil
}
