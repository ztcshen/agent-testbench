package main

import (
	"path/filepath"
	"strings"
)

func composeHostSourceLooksLikePath(source string) bool {
	return strings.HasPrefix(source, ".") ||
		strings.HasPrefix(source, "/") ||
		strings.HasPrefix(source, "~") ||
		strings.HasPrefix(source, "$") ||
		strings.HasPrefix(source, "${")
}

func extractSandboxComposePaths(value string) []string {
	out := []string{}
	for _, field := range strings.FieldsFunc(value, func(r rune) bool {
		return r == '"' || r == '\'' || r == ',' || r == '[' || r == ']' || r == ' ' || r == '\t'
	}) {
		field = strings.TrimSpace(field)
		if !strings.HasPrefix(field, "/sandbox/compose/") {
			continue
		}
		out = append(out, filepath.Clean(strings.TrimPrefix(field, "/sandbox/")))
	}
	return out
}

func dedupeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func leadingSpaceCount(value string) int {
	count := 0
	for _, r := range value {
		if r != ' ' {
			break
		}
		count++
	}
	return count
}
