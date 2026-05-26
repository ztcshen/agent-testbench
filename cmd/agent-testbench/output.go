package main

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"strings"

	"agent-testbench/internal/store"
)

func jsonObjectFromAny(value any) map[string]any {
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func mustCompactJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func jsonObjectString(raw string) map[string]any {
	var out map[string]any
	if err := json.Unmarshal([]byte(stringDefault(raw, "{}")), &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func jsonArrayString(raw string) []any {
	var out []any
	if err := json.Unmarshal([]byte(stringDefault(raw, "[]")), &out); err != nil || out == nil {
		return []any{}
	}
	return out
}

func stringDefault(value string, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func rawJSONListFromAny(value any) []json.RawMessage {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]json.RawMessage, 0, len(values))
	for _, item := range values {
		raw, err := json.Marshal(item)
		if err == nil {
			out = append(out, raw)
		}
	}
	return out
}

func writeIndentedJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func compactRawJSON(raw json.RawMessage) (string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	compact, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(compact), nil
}

func compactJSONValue(value any) (string, error) {
	compact, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(compact), nil
}

func reportPill(label string, value string) string {
	return `<span class="pill"><span class="small">` + html.EscapeString(label) + `</span> ` + html.EscapeString(value) + `</span>`
}

func statusText(ok bool) string {
	if ok {
		return store.StatusPassed
	}
	return store.StatusFailed
}

func mapFromReportAny(value any) map[string]any {
	typed, ok := value.(map[string]any)
	if !ok || typed == nil {
		return map[string]any{}
	}
	return typed
}

func listFromReportAny(value any) []any {
	switch typed := value.(type) {
	case []any:
		if typed == nil {
			return []any{}
		}
		return typed
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return []any{}
	}
}

func rawJSONObject(value string) map[string]any {
	out := map[string]any{}
	if strings.TrimSpace(value) == "" {
		return out
	}
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return map[string]any{}
	}
	return out
}

func cloneMap(value map[string]any) map[string]any {
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func mergeReportMap(target map[string]any, key string, values map[string]any) {
	next := mapFromReportAny(target[key])
	if len(next) == 0 {
		next = map[string]any{}
	}
	for itemKey, itemValue := range values {
		next[itemKey] = itemValue
	}
	target[key] = next
}

func firstReportValue(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return nil
}

func intSliceFromReportAny(value any) []int {
	switch typed := value.(type) {
	case []any:
		out := make([]int, 0, len(typed))
		for _, item := range typed {
			if number := intFromReportAny(item); number > 0 {
				out = append(out, number)
			}
		}
		return out
	case []int:
		return typed
	default:
		return nil
	}
}

func intFromReportAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		out, err := typed.Int64()
		if err != nil {
			return 0
		}
		return int(out)
	default:
		return 0
	}
}

func valueString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		return fmt.Sprint(value)
	}
}

func boolFromReportAny(value any) bool {
	typed, ok := value.(bool)
	return ok && typed
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func truncateReportText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	if limit <= 1 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func safeReportID(value string) string {
	var b strings.Builder
	for _, item := range value {
		if item >= 'a' && item <= 'z' || item >= 'A' && item <= 'Z' || item >= '0' && item <= '9' || item == '.' || item == '_' || item == '-' {
			b.WriteRune(item)
			continue
		}
		b.WriteByte('_')
	}
	if b.Len() == 0 {
		return "item"
	}
	return b.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func readJSONFile(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func compactJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
