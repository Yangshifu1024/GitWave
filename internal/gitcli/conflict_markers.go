package gitcli

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type ConflictRegion struct{ Start, End int }

// ConflictRegions returns the inclusive line ranges, including unfinished
// regions, in Git's standard and diff3 marker formats.
func ConflictRegions(text string, markerSize int) []ConflictRegion {
	var regions []ConflictRegion
	start, line, size := -1, 0, 0
	var known, separator bool
	for content := range strings.SplitSeq(text, "\n") {
		if n := conflictMarkerLength(content, '<'); n != 0 && (start == -1 || (!known && !separator)) {
			start, size, separator = line, n, false
			known = n == 7 || n == markerSize
		} else if start != -1 {
			if conflictMarkerLength(content, '=') == size && strings.TrimSpace(content[size:]) == "" {
				separator = true
			} else if conflictMarkerLength(content, '>') == size {
				if known || separator {
					regions = append(regions, ConflictRegion{Start: start, End: line})
				}
				start = -1
			}
		}
		line++
	}
	if start != -1 && (known || separator) {
		regions = append(regions, ConflictRegion{Start: start, End: line - 1})
	}
	return regions
}

func conflictMarkerLength(line string, marker byte) int {
	count := 0
	for count < len(line) && line[count] == marker {
		count++
	}
	rest := line[count:]
	first, _ := utf8.DecodeRuneInString(rest)
	if rest == "" || unicode.IsSpace(first) {
		return count
	}
	return 0
}
