package mapplanner

import (
	"encoding/json"
	"strings"
)

func jsonObject(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{"raw": raw}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func safeID(value string) string {
	value = strings.TrimSpace(value)
	var out strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
			continue
		}
		out.WriteByte('_')
	}
	if out.Len() == 0 {
		return "unknown"
	}
	return out.String()
}

func stringInList(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
