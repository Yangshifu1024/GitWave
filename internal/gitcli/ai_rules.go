package gitcli

import (
	"context"
	"strings"
	"unicode/utf8"
)

// AIRules reads the selected worktree's optional AI rules. Missing, invalid
// or oversize files are ignored because the rules are advisory.
func (r *Repository) AIRules(ctx context.Context) string {
	if r.WorkTree == "" {
		return ""
	}
	const maxBytes = 1 << 20
	const maxRunes = 8000
	file, err := r.workFile(ctx, ".gitwave/AI.md", maxBytes)
	if err != nil || !file.mode.IsRegular() || !utf8.Valid(file.data) {
		return ""
	}
	rules := strings.TrimSpace(string(file.data))
	count := 0
	for i := range rules {
		if count == maxRunes {
			return rules[:i] + "\n[rules truncated due to size]"
		}
		count++
	}
	return rules
}
