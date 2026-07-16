package controlplane

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

func renderCaseExecutionValue(value any, overrides map[string]any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = renderCaseExecutionValue(item, overrides)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, renderCaseExecutionValue(item, overrides))
		}
		return out
	case string:
		return renderCaseString(typed, overrides)
	default:
		return value
	}
}

func renderCaseString(value string, overrides map[string]any) string {
	rendered := strings.ReplaceAll(value, "${AUTO_SERIAL}", serialValue("GEN"))
	rendered = strings.ReplaceAll(rendered, "${AUTO_RT_ORDER_ID}", serialValue("RT"))
	cursor := 0
	for cursor < len(rendered) {
		start := strings.Index(rendered[cursor:], "{{")
		if start >= 0 {
			start += cursor
		}
		if start < 0 {
			break
		}
		end := strings.Index(rendered[start+2:], "}}")
		if end < 0 {
			break
		}
		end += start + 2
		token := rendered[start+2 : end]
		replacement, ok := renderCaseToken(token, overrides)
		if !ok {
			cursor = end + 2
			continue
		}
		rendered = rendered[:start] + replacement + rendered[end+2:]
		cursor = start + len(replacement)
	}
	return rendered
}

func renderCaseToken(token string, overrides map[string]any) (string, bool) {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(token, "override:") {
		body := strings.TrimPrefix(token, "override:")
		key, defaultValue, _ := strings.Cut(body, "|")
		if value := strings.TrimSpace(valueString(overrides[strings.TrimSpace(key)])); value != "" {
			return renderDefaultValue(value), true
		}
		return renderDefaultValue(defaultValue), true
	}
	if strings.HasPrefix(token, "serial:") {
		return serialValue(strings.TrimPrefix(token, "serial:")), true
	}
	if token == "now:datetime" {
		return time.Now().UTC().Format("2006-01-02 15:04:05"), true
	}
	return "", false
}

func renderDefaultValue(value string) string {
	if strings.HasPrefix(value, "serial:") {
		return serialValue(strings.TrimPrefix(value, "serial:"))
	}
	if value == "now:datetime" {
		return time.Now().UTC().Format("2006-01-02 15:04:05")
	}
	return value
}

func serialValue(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "GEN"
	}
	counter := atomic.AddUint64(&caseSerialCounter, 1) % 1000000
	return fmt.Sprintf("%s%s%06d", prefix, time.Now().UTC().Format("20060102150405"), counter)
}

func headerStrings(headers map[string]any) map[string]string {
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		if strings.TrimSpace(key) != "" {
			out[key] = valueString(value)
		}
	}
	return out
}

func responseHeaders(headers http.Header) map[string]string {
	out := make(map[string]string, len(headers))
	for key, values := range headers {
		out[key] = strings.Join(values, ", ")
	}
	return out
}

func expectedHTTPCode(status int, expected []int) bool {
	if len(expected) == 0 {
		return status >= 200 && status < 300
	}
	for _, value := range expected {
		if status == value {
			return true
		}
	}
	return false
}

const (
	defaultTestKitTimeoutSeconds = 90
	maxTestKitTimeoutSeconds     = 600
)

func testKitTimeout(payload map[string]any, configuredSeconds int) (time.Duration, error) {
	seconds := int64(configuredSeconds)
	if raw, ok := payload[apiFieldTimeoutSeconds]; ok && raw != nil && strings.TrimSpace(valueString(raw)) != "" {
		parsed, err := testKitTimeoutSeconds(raw)
		if err != nil {
			return 0, err
		}
		seconds = parsed
		if seconds == 0 {
			seconds = defaultTestKitTimeoutSeconds
		}
	} else if seconds == 0 {
		seconds = defaultTestKitTimeoutSeconds
	}
	if seconds < 0 || seconds > maxTestKitTimeoutSeconds {
		return 0, fmt.Errorf("timeoutSeconds must be between 0 and %d", maxTestKitTimeoutSeconds)
	}
	return time.Duration(seconds) * time.Second, nil
}

func testKitTimeoutSeconds(value any) (int64, error) {
	switch typed := value.(type) {
	case int:
		return int64(typed), nil
	case int8:
		return int64(typed), nil
	case int16:
		return int64(typed), nil
	case int32:
		return int64(typed), nil
	case int64:
		return typed, nil
	case uint:
		if uint64(typed) > math.MaxInt64 {
			return 0, fmt.Errorf("timeoutSeconds is outside the supported integer range")
		}
		return int64(typed), nil
	case uint8:
		return int64(typed), nil
	case uint16:
		return int64(typed), nil
	case uint32:
		return int64(typed), nil
	case uint64:
		if typed > math.MaxInt64 {
			return 0, fmt.Errorf("timeoutSeconds is outside the supported integer range")
		}
		return int64(typed), nil
	case json.Number:
		seconds, err := typed.Int64()
		if err != nil {
			return 0, fmt.Errorf("timeoutSeconds must be an integer: %w", err)
		}
		return seconds, nil
	case float32:
		return testKitFloatTimeoutSeconds(float64(typed))
	case float64:
		return testKitFloatTimeoutSeconds(typed)
	default:
		return 0, fmt.Errorf("timeoutSeconds must be an integer")
	}
}

func testKitFloatTimeoutSeconds(value float64) (int64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value != math.Trunc(value) || value < math.MinInt64 || value > math.MaxInt64 {
		return 0, fmt.Errorf("timeoutSeconds must be an integer")
	}
	return int64(value), nil
}

func failedCaseExecution(caseID string, reason string) caseExecutionResult {
	return caseExecutionResult{
		ok:            false,
		failureReason: reason,
		result: map[string]any{
			"request":  map[string]any{"caseId": caseID},
			"response": map[string]any{"body": "{}"},
		},
	}
}
