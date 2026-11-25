package gitcli

import (
	"fmt"
	"strconv"
	"strings"
)

const lfsAttributeValues = "filter=lfs diff=lfs merge=lfs -text"
const attributesBOM = "\xef\xbb\xbf"
const attributesLineLimit = 2048

type attributeToken struct {
	value      string
	start, end int
}

type lfsAttributeRule struct {
	pattern string
	attrs   []attributeToken
}

func attrBlank(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n'
}

func nextAttributeToken(text string, start int) attributeToken {
	for start < len(text) && attrBlank(text[start]) {
		start++
	}
	end := start
	for end < len(text) && !attrBlank(text[end]) {
		end++
	}
	return attributeToken{text[start:end], start, end}
}

func parseLFSAttributeRule(line string) (lfsAttributeRule, bool) {
	// Git ignores invalid lines. Their bytes belong to the user.
	if len(strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")) >= attributesLineLimit || strings.ContainsRune(line, 0) {
		return lfsAttributeRule{}, false
	}
	pattern := nextAttributeToken(line, 0)
	if pattern.value == "" || pattern.value[0] == '#' {
		return lfsAttributeRule{}, false
	}
	if pattern.value[0] == '"' {
		end := pattern.start + 1
		for end < len(line) && line[end] != '"' {
			if line[end] == '\\' {
				end++
				if end >= len(line) || !strings.ContainsRune("abfnrtv\\\"0123", rune(line[end])) {
					return lfsAttributeRule{}, false
				}
			}
			end++
		}
		if end == len(line) {
			return lfsAttributeRule{}, false
		}
		value, err := strconv.Unquote(line[pattern.start : end+1])
		if err != nil || strings.ContainsAny(value, "\x00\r\n") {
			return lfsAttributeRule{}, false
		}
		pattern.value, pattern.end = value, end+1
	}
	if pattern.value == "" || strings.HasPrefix(pattern.value, "!") || strings.HasPrefix(pattern.value, "[attr]") {
		return lfsAttributeRule{}, false
	}
	rule := lfsAttributeRule{pattern: pattern.value}
	for start := pattern.end; start < len(line); {
		token := nextAttributeToken(line, start)
		if token.value == "" {
			break
		}
		if !validAttribute(token.value) {
			return lfsAttributeRule{}, false
		}
		rule.attrs = append(rule.attrs, token)
		start = token.end
	}
	return rule, true
}

func validAttribute(token string) bool {
	name, _, _ := strings.Cut(token, "=")
	if len(name) > 0 && (name[0] == '-' || name[0] == '!') {
		name = name[1:]
	}
	if name == "" || name[0] == '-' || strings.HasPrefix(name, "builtin_") {
		return false
	}
	for _, ch := range name {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || strings.ContainsRune("-_.", ch)) {
			return false
		}
	}
	return true
}

func lfsPatterns(data string) []string {
	var patterns []string
	seen := make(map[string]bool)
	active := make(map[string]bool)
	for line := range strings.SplitSeq(strings.TrimPrefix(data, attributesBOM), "\n") {
		rule, ok := parseLFSAttributeRule(line)
		if !ok {
			continue
		}
		for _, attr := range rule.attrs {
			name, _, _ := strings.Cut(attr.value, "=")
			if name != "filter" && name != "!filter" && name != "-filter" {
				continue
			}
			active[rule.pattern] = attr.value == "filter=lfs"
			if active[rule.pattern] && !seen[rule.pattern] {
				seen[rule.pattern] = true
				patterns = append(patterns, rule.pattern)
			}
		}
	}
	remaining := patterns[:0]
	for _, pattern := range patterns {
		if active[pattern] {
			remaining = append(remaining, pattern)
		}
	}
	return remaining
}

func encodeLFSPattern(pattern string) (string, error) {
	if pattern == "" || strings.ContainsAny(pattern, "\x00\r\n") ||
		strings.HasPrefix(pattern, "!") || strings.HasPrefix(pattern, "[attr]") {
		return "", fmt.Errorf("enter a valid Git attributes pattern")
	}
	if strings.HasSuffix(pattern, "/") {
		return "", fmt.Errorf("use /** to track files inside a directory")
	}
	if strings.ContainsAny(pattern, " \t\"") || strings.HasPrefix(pattern, "#") {
		escape := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\t", "\\t")
		pattern = "\"" + escape.Replace(pattern) + "\""
	}
	if len(pattern)+1+len(lfsAttributeValues) >= attributesLineLimit {
		return "", fmt.Errorf("the Git attributes pattern is too long")
	}
	return pattern, nil
}

func appendLFSRule(data, pattern string) string {
	newline := "\n"
	if i := strings.LastIndexByte(data, '\n'); i > 0 && data[i-1] == '\r' {
		newline = "\r\n"
	}
	if data != "" && data != attributesBOM && !strings.HasSuffix(data, "\n") {
		data += newline
	}
	return data + pattern + " " + lfsAttributeValues + newline
}

func removeLFSRules(data, pattern string) string {
	var next strings.Builder
	if strings.HasPrefix(data, attributesBOM) {
		next.WriteString(attributesBOM)
		data = strings.TrimPrefix(data, attributesBOM)
	}
	for original := range strings.SplitAfterSeq(data, "\n") {
		rule, ok := parseLFSAttributeRule(original)
		if !ok || rule.pattern != pattern {
			next.WriteString(original)
			continue
		}
		filter := false
		for _, attr := range rule.attrs {
			if attr.value == "filter=lfs" {
				filter = true
				break
			}
		}
		if !filter {
			next.WriteString(original)
			continue
		}
		var edited strings.Builder
		end := 0
		kept := false
		for i, attr := range rule.attrs {
			// Remove -text only when it is part of Git LFS's generated sequence.
			generatedText := attr.value == "-text" && i >= 3 && rule.attrs[i-3].value == "filter=lfs" &&
				rule.attrs[i-2].value == "diff=lfs" && rule.attrs[i-1].value == "merge=lfs"
			if attr.value == "filter=lfs" || attr.value == "diff=lfs" || attr.value == "merge=lfs" ||
				generatedText {
				cut := attr.start
				for cut > end && attrBlank(original[cut-1]) {
					cut--
				}
				edited.WriteString(original[end:cut])
			} else {
				edited.WriteString(original[end:attr.end])
				kept = true
			}
			end = attr.end
		}
		if kept {
			edited.WriteString(original[end:])
			next.WriteString(edited.String())
		}
	}
	return next.String()
}
