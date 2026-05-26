package controlplane

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
)

func apiCaseBatchEvidenceOverridesForPlan(plan apiCaseBatchCasePlan, evidencePath string) map[string]any {
	out := apiCaseBatchEvidenceOverrides(evidencePath)
	request, _ := jsonFileObject(filepath.Join(evidencePath, "request.json"))
	response, _ := jsonFileObject(filepath.Join(evidencePath, "response.json"))
	requestBody := apiCaseBatchJSONBody(request)
	responseBody := apiCaseBatchJSONBody(response)
	for _, export := range plan.Exports {
		name := apiCaseBatchOverrideKey(valueString(export["name"]))
		if name == "" {
			continue
		}
		source := strings.ToLower(strings.TrimSpace(valueString(export["from"])))
		path := strings.TrimSpace(valueString(export["path"]))
		var root any
		switch source {
		case "requestbody", "request.body":
			root = requestBody
		case "responsebody", "response.body":
			root = responseBody
		case "request":
			root = request
		case "response":
			root = responseBody
			if root == nil {
				root = response
			}
		default:
			root = responseBody
		}
		value, ok := apiCaseBatchPathValue(root, path)
		if !ok {
			continue
		}
		text := strings.TrimSpace(apiCaseBatchOverrideValueString(value))
		if text != "" {
			out[name] = text
		}
	}
	return out
}

func apiCaseBatchJSONBody(payload map[string]any) any {
	body := strings.TrimSpace(valueString(payload["body"]))
	if body == "" {
		return nil
	}
	var parsed any
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&parsed) != nil {
		return nil
	}
	return parsed
}

func apiCaseBatchPathValue(root any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "$")
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return root, root != nil
	}
	current := root
	for _, part := range strings.Split(path, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch typed := current.(type) {
		case map[string]any:
			value, ok := typed[part]
			if !ok {
				return nil, false
			}
			current = value
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func apiCaseBatchEvidenceOverrides(evidencePath string) map[string]any {
	out := map[string]any{}
	for _, name := range []string{"request.json", "response.json"} {
		payload, _ := jsonFileObject(filepath.Join(evidencePath, name))
		collectAPICaseBatchOverrideFields(out, payload)
		if body := strings.TrimSpace(valueString(payload["body"])); body != "" {
			var parsed any
			decoder := json.NewDecoder(strings.NewReader(body))
			decoder.UseNumber()
			if decoder.Decode(&parsed) == nil {
				collectAPICaseBatchOverrideFields(out, parsed)
			}
		}
	}
	return out
}

func collectAPICaseBatchOverrideFields(out map[string]any, value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			recordAPICaseBatchOverrideField(out, key, item)
			collectAPICaseBatchOverrideFields(out, item)
		}
	case []any:
		for _, item := range typed {
			collectAPICaseBatchOverrideFields(out, item)
		}
	case map[string]string:
		for key, item := range typed {
			recordAPICaseBatchOverrideField(out, key, item)
		}
	}
}

func recordAPICaseBatchOverrideField(out map[string]any, key string, value any) {
	normalized := apiCaseBatchOverrideKey(key)
	if normalized == "" {
		return
	}
	text := strings.TrimSpace(apiCaseBatchOverrideValueString(value))
	if text == "" {
		return
	}
	out[normalized] = text
}

func apiCaseBatchOverrideValueString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		asInt := int64(typed)
		if typed == float64(asInt) {
			return strconv.FormatInt(asInt, 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		asInt := int64(typed)
		if typed == float32(asInt) {
			return strconv.FormatInt(asInt, 10)
		}
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	default:
		return valueString(value)
	}
}

func apiCaseBatchOverrideKey(key string) string {
	return normalizeAPICaseBatchOverrideKey(strings.TrimSpace(key))
}

func normalizeAPICaseBatchOverrideKey(key string) string {
	if key == "" {
		return ""
	}
	runes := []rune(key)
	var out strings.Builder
	var previousUnderscore bool
	for index, char := range runes {
		switch {
		case isAPICaseBatchLower(char):
			out.WriteRune(char)
			previousUnderscore = false
		case isAPICaseBatchUpper(char):
			previous := rune(0)
			if index > 0 {
				previous = runes[index-1]
			}
			next := rune(0)
			if index+1 < len(runes) {
				next = runes[index+1]
			}
			if index > 0 && !previousUnderscore && (isAPICaseBatchLower(previous) || isAPICaseBatchDigit(previous) || isAPICaseBatchLower(next)) {
				out.WriteByte('_')
			}
			out.WriteRune(char + ('a' - 'A'))
			previousUnderscore = false
		case isAPICaseBatchDigit(char):
			out.WriteRune(char)
			previousUnderscore = false
		case char == '_' || char == '-' || char == ' ':
			if out.Len() > 0 && !previousUnderscore {
				out.WriteByte('_')
				previousUnderscore = true
			}
		default:
			return ""
		}
	}
	normalized := strings.Trim(out.String(), "_")
	if normalized == "" {
		return ""
	}
	return normalized
}

func isAPICaseBatchLower(char rune) bool {
	return char >= 'a' && char <= 'z'
}

func isAPICaseBatchUpper(char rune) bool {
	return char >= 'A' && char <= 'Z'
}

func isAPICaseBatchDigit(char rune) bool {
	return char >= '0' && char <= '9'
}
