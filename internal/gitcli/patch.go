package gitcli

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Read the entire bounded patch for accurate counts, keeping only the prefix
// that can be rendered at the requested preview budget.
func parsePatch(file *FileDiff, patch string, byteLimit, lineLimit, charLimit int) (bool, error) {
	var old, newLine, oldRemaining, newRemaining int
	var retainedLines, retainedBytes int
	truncated, collecting := false, true
	for patch != "" {
		line, rest, newline := strings.Cut(patch, "\n")
		patch = rest
		if strings.HasPrefix(line, "@@ ") {
			if oldRemaining != 0 || newRemaining != 0 {
				return false, fmt.Errorf("incomplete Git diff hunk")
			}
			hunk, err := parseHunk(line)
			if err != nil {
				return false, err
			}
			old, newLine = hunk.OldStart, hunk.NewStart
			oldRemaining, newRemaining = hunk.OldLines, hunk.NewLines
			if collecting {
				file.Hunks = append(file.Hunks, hunk)
			}
			continue
		}
		if (oldRemaining == 0 && newRemaining == 0) || strings.HasPrefix(line, "\\ ") {
			continue
		}
		if line == "" {
			return false, fmt.Errorf("invalid Git diff line")
		}
		var row DiffLine
		switch line[0] {
		case ' ':
			row.Kind, row.Old, row.New = "context", old, newLine
			old++
			newLine++
			oldRemaining--
			newRemaining--
		case '-':
			row.Kind, row.Old = "removed", old
			old++
			oldRemaining--
			file.Deletions++
		case '+':
			row.Kind, row.New = "added", newLine
			newLine++
			newRemaining--
			file.Additions++
		default:
			return false, fmt.Errorf("invalid Git diff line")
		}
		if oldRemaining < 0 || newRemaining < 0 {
			return false, fmt.Errorf("invalid Git diff hunk length")
		}
		if !collecting {
			continue
		}
		content := line[1:]
		size := len(content)
		if newline && !strings.HasPrefix(rest, "\\ ") {
			size++
		}
		if retainedLines >= lineLimit || retainedBytes+size > byteLimit {
			truncated, collecting = true, false
			continue
		}
		retainedLines++
		retainedBytes += size
		var clipped bool
		row.Content, clipped = previewLineText(content, charLimit)
		truncated = truncated || clipped
		hunk := &file.Hunks[len(file.Hunks)-1]
		hunk.Lines = append(hunk.Lines, row)
	}
	if oldRemaining != 0 || newRemaining != 0 {
		return false, fmt.Errorf("incomplete Git diff hunk")
	}
	return truncated, nil
}

func previewLineText(content string, charLimit int) (string, bool) {
	content = lossyGitText(strings.TrimRight(content, "\r\n"))
	if utf8.RuneCountInString(content) <= charLimit {
		return content, false
	}
	for offset := range content {
		if charLimit == 0 {
			return content[:offset], true
		}
		charLimit--
	}
	return content, false
}

func lossyGitText(text string) string {
	if utf8.ValidString(text) {
		return text
	}
	// Match Rust's replacement of each invalid UTF-8 sequence, including
	// an incomplete multi-byte prefix before EOF or a bad continuation.
	var output strings.Builder
	for text != "" {
		r, width := utf8.DecodeRuneInString(text)
		if r == utf8.RuneError && width == 1 {
			for n := 1; n <= len(text); n++ {
				if utf8.FullRuneInString(text[:n]) {
					break
				}
				width = n
			}
		}
		output.WriteRune(r)
		text = text[width:]
	}
	return output.String()
}

func parseHunk(header string) (DiffHunk, error) {
	fields := strings.Fields(header)
	if len(fields) < 4 || fields[3] != "@@" {
		return DiffHunk{}, fmt.Errorf("invalid Git diff hunk")
	}
	old, oldLength, err := parseRange(fields[1], "-")
	if err != nil {
		return DiffHunk{}, err
	}
	newLine, newLength, err := parseRange(fields[2], "+")
	if err != nil {
		return DiffHunk{}, err
	}
	return DiffHunk{OldStart: old, OldLines: oldLength, NewStart: newLine, NewLines: newLength}, nil
}

func parseRange(field, prefix string) (int, int, error) {
	value, ok := strings.CutPrefix(field, prefix)
	start, length, explicit := strings.Cut(value, ",")
	if !explicit {
		length = "1"
	}
	n, startErr := strconv.Atoi(start)
	count, lengthErr := strconv.Atoi(length)
	if !ok || startErr != nil || lengthErr != nil || n < 0 || count < 0 {
		return 0, 0, fmt.Errorf("invalid Git diff hunk range")
	}
	return n, count, nil
}
