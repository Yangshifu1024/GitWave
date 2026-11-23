package main

import (
	"slices"
	"testing"
)

func TestDiffRowsPairsUnequalChanges(t *testing.T) {
	hunk := diffHunk{
		OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 3,
		Lines: []diffLine{
			{Kind: "removed", Content: "old"},
			{Kind: "added", Content: "new"},
			{Kind: "added", Content: "more"},
			{Kind: "context", Content: "same"},
		},
	}
	hunks := []diffHunk{hunk}
	split := diffRows(hunks, true)
	if len(split) != 4 || split[1].left.Content != "old" || split[1].right.Content != "new" ||
		split[2].left != nil || split[2].right.Content != "more" {
		t.Fatalf("split rows: %#v", split)
	}
	var order []string
	for _, row := range diffRows(hunks, false)[1:] {
		line := row.right
		if row.show == "left" {
			line = row.left
		}
		order = append(order, line.Content)
	}
	if !slices.Equal(order, []string{"old", "new", "more", "same"}) {
		t.Fatalf("unified order: %v", order)
	}
}

func TestChangedRangeKeepsUnicodeCharactersWhole(t *testing.T) {
	first, second := "文件🙂old世界", "文件🙂新版世界"
	from, to := changedRange(first, second)
	if got := first[from:to]; got != "old" {
		t.Fatalf("old range: %q", got)
	}
	from, to = changedRange(second, first)
	if got := second[from:to]; got != "新版" {
		t.Fatalf("new range: %q", got)
	}
}
