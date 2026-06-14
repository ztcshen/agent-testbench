package main

import (
	"path/filepath"
	"strings"
)

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

func stringMapFromAny(value any) map[string]string {
	out := map[string]string{}
	switch typed := value.(type) {
	case map[string]string:
		for key, value := range typed {
			if strings.TrimSpace(key) != "" {
				out[strings.TrimSpace(key)] = strings.TrimSpace(value)
			}
		}
	case map[string]any:
		for key, value := range typed {
			if strings.TrimSpace(key) != "" {
				out[strings.TrimSpace(key)] = strings.TrimSpace(valueString(value))
			}
		}
	}
	return out
}

func generatedFileContentMapFromAny(value any) map[string]string {
	out := map[string]string{}
	switch typed := value.(type) {
	case map[string]string:
		for key, value := range typed {
			if clean := cleanGeneratedFileContentPath(key); clean != "" {
				out[clean] = value
			}
		}
	case map[string]any:
		for key, value := range typed {
			if clean := cleanGeneratedFileContentPath(key); clean != "" {
				out[clean] = valueString(value)
			}
		}
	}
	return out
}

func cleanGeneratedFileContentPath(path string) string {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "." || clean == "" {
		return ""
	}
	return clean
}

func stringSliceFromAny(value any) []string {
	values, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]string); ok {
			out := make([]string, 0, len(typed))
			for _, item := range typed {
				if strings.TrimSpace(item) != "" {
					out = append(out, strings.TrimSpace(item))
				}
			}
			return out
		}
		return nil
	}
	out := make([]string, 0, len(values))
	for _, item := range values {
		if value := strings.TrimSpace(valueString(item)); value != "" {
			out = append(out, value)
		}
	}
	return out
}
