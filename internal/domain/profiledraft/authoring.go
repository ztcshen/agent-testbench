// Package profiledraft contains profile authoring helpers for import planners.
package profiledraft

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

func CompactTags(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
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

func CompactJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func SortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func Slug(value string) string {
	value = splitCamelCase(value)
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugPattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "item"
	}
	return value
}

func splitCamelCase(value string) string {
	var builder strings.Builder
	var previous rune
	for index, ch := range value {
		if index > 0 && previous >= 'a' && previous <= 'z' && ch >= 'A' && ch <= 'Z' {
			builder.WriteByte('-')
		}
		builder.WriteRune(ch)
		previous = ch
	}
	return builder.String()
}
