package controlplane

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func applyAPICaseEquivalentBodyPatch(body any, patchJSON string) error {
	var operations []apiCaseJSONPatchOperation
	if err := json.Unmarshal([]byte(patchJSON), &operations); err != nil {
		return fmt.Errorf("decode patchJson: %w", err)
	}
	for _, operation := range operations {
		segments, err := parseAPICaseJSONPath(operation.Path)
		if err != nil {
			return err
		}
		if len(segments) != 1 || segments[0].Index != nil {
			continue
		}
		candidates := equivalentJSONFieldNames(segments[0].Key)
		if len(candidates) == 0 {
			continue
		}
		applyEquivalentJSONFieldPatch(body, candidates, operation)
	}
	return nil
}

func equivalentJSONFieldNames(key string) map[string]bool {
	parts := strings.Split(strings.TrimSpace(key), "_")
	candidates := map[string]bool{}
	for index := 0; index < len(parts); index++ {
		aliasParts := parts[index:]
		if index > 0 && len(aliasParts) < 2 {
			continue
		}
		name := lowerCamelName(aliasParts)
		if name != "" {
			candidates[name] = true
		}
		identifierParts := append([]string(nil), aliasParts...)
		if len(identifierParts) > 0 && identifierParts[len(identifierParts)-1] == "no" {
			identifierParts[len(identifierParts)-1] = "id"
			if name := lowerCamelName(identifierParts); name != "" {
				candidates[name] = true
			}
		}
	}
	delete(candidates, strings.TrimSpace(key))
	return candidates
}

func lowerCamelName(parts []string) string {
	var out strings.Builder
	for index, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if index == 0 && out.Len() == 0 {
			out.WriteString(strings.ToLower(part))
			continue
		}
		out.WriteString(strings.ToUpper(part[:1]))
		if len(part) > 1 {
			out.WriteString(strings.ToLower(part[1:]))
		}
	}
	return out.String()
}

func applyEquivalentJSONFieldPatch(value any, candidates map[string]bool, operation apiCaseJSONPatchOperation) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if candidates[key] {
				switch strings.ToLower(strings.TrimSpace(operation.Op)) {
				case "add", "replace":
					typed[key] = operation.Value
				case "remove":
					delete(typed, key)
				}
				continue
			}
			applyEquivalentJSONFieldPatch(child, candidates, operation)
		}
	case []any:
		for _, child := range typed {
			applyEquivalentJSONFieldPatch(child, candidates, operation)
		}
	}
}

func applyAPICaseExpectedJSON(request *caseHTTPRequest, expectedJSON string) error {
	expectedJSON = strings.TrimSpace(expectedJSON)
	if expectedJSON == "" || expectedJSON == "{}" {
		return nil
	}
	var parsed struct {
		ExpectedHTTPCodes []int    `json:"expectedHttpCodes"`
		ResponseContains  []string `json:"expectedResponseContains"`
	}
	if err := json.Unmarshal([]byte(expectedJSON), &parsed); err != nil {
		return fmt.Errorf("decode api case expectedJson: %w", err)
	}
	if len(parsed.ExpectedHTTPCodes) > 0 {
		request.expectedHTTPCodes = parsed.ExpectedHTTPCodes
	}
	if len(parsed.ResponseContains) > 0 {
		request.expectedResponse = parsed.ResponseContains
	}
	return nil
}

type apiCaseJSONPatchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

type apiCaseJSONPathSegment struct {
	Key   string
	Index *int
}

func applyAPICaseJSONPatch(body any, patchJSON string) (any, error) {
	var operations []apiCaseJSONPatchOperation
	if err := json.Unmarshal([]byte(patchJSON), &operations); err != nil {
		return nil, fmt.Errorf("decode patchJson: %w", err)
	}
	next := body
	for _, operation := range operations {
		operation.Value = renderCaseExecutionValue(operation.Value, nil)
		segments, err := parseAPICaseJSONPath(operation.Path)
		if err != nil {
			return nil, err
		}
		if len(segments) == 0 {
			switch strings.ToLower(strings.TrimSpace(operation.Op)) {
			case "add", "replace":
				next = operation.Value
			case "remove":
				next = nil
			default:
				return nil, fmt.Errorf("unsupported patch op %q", operation.Op)
			}
			continue
		}
		var patchErr error
		next, patchErr = applyAPICaseJSONPatchOperation(next, segments, operation)
		if patchErr != nil {
			return nil, patchErr
		}
	}
	return next, nil
}

func parseAPICaseJSONPath(path string) ([]apiCaseJSONPathSegment, error) {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "$")
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return nil, nil
	}
	parts := strings.Split(path, ".")
	segments := make([]apiCaseJSONPathSegment, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		for part != "" {
			bracket := strings.Index(part, "[")
			if bracket < 0 {
				segments = append(segments, apiCaseJSONPathSegment{Key: part})
				break
			}
			if bracket > 0 {
				segments = append(segments, apiCaseJSONPathSegment{Key: part[:bracket]})
			}
			closeBracket := strings.Index(part[bracket:], "]")
			if closeBracket < 0 {
				return nil, fmt.Errorf("invalid patch path %q", path)
			}
			indexText := part[bracket+1 : bracket+closeBracket]
			index, err := strconv.Atoi(strings.TrimSpace(indexText))
			if err != nil || index < 0 {
				return nil, fmt.Errorf("invalid patch array index %q", path)
			}
			segments = append(segments, apiCaseJSONPathSegment{Index: &index})
			part = part[bracket+closeBracket+1:]
			part = strings.TrimPrefix(part, ".")
		}
	}
	return segments, nil
}

func applyAPICaseJSONPatchOperation(root any, segments []apiCaseJSONPathSegment, operation apiCaseJSONPatchOperation) (any, error) {
	parent := root
	for _, segment := range segments[:len(segments)-1] {
		next, ok := apiCaseJSONPathChild(parent, segment)
		if !ok {
			return root, fmt.Errorf("patch path not found: %s", operation.Path)
		}
		parent = next
	}
	last := segments[len(segments)-1]
	op := strings.ToLower(strings.TrimSpace(operation.Op))
	if last.Index != nil {
		array, ok := parent.([]any)
		if !ok {
			return root, fmt.Errorf("patch path is not an array: %s", operation.Path)
		}
		index := *last.Index
		if index < 0 || index >= len(array) {
			return root, fmt.Errorf("patch array index out of range: %s", operation.Path)
		}
		switch op {
		case "add", "replace":
			array[index] = operation.Value
		case "remove":
			copy(array[index:], array[index+1:])
			array[len(array)-1] = nil
			array = array[:len(array)-1]
			if len(segments) == 1 {
				return array, nil
			}
			assignAPICaseJSONPathChild(root, segments[:len(segments)-1], array)
			return root, nil
		default:
			return root, fmt.Errorf("unsupported patch op %q", operation.Op)
		}
		return root, nil
	}
	object, ok := parent.(map[string]any)
	if !ok {
		return root, fmt.Errorf("patch path is not an object: %s", operation.Path)
	}
	switch op {
	case "add", "replace":
		object[last.Key] = operation.Value
	case "remove":
		delete(object, last.Key)
	default:
		return root, fmt.Errorf("unsupported patch op %q", operation.Op)
	}
	return root, nil
}

func apiCaseJSONPathChild(parent any, segment apiCaseJSONPathSegment) (any, bool) {
	if segment.Index != nil {
		array, ok := parent.([]any)
		if !ok || *segment.Index < 0 || *segment.Index >= len(array) {
			return nil, false
		}
		return array[*segment.Index], true
	}
	object, ok := parent.(map[string]any)
	if !ok {
		return nil, false
	}
	value, ok := object[segment.Key]
	return value, ok
}

func assignAPICaseJSONPathChild(root any, segments []apiCaseJSONPathSegment, value any) {
	if len(segments) == 0 {
		return
	}
	parent := root
	for _, segment := range segments[:len(segments)-1] {
		next, ok := apiCaseJSONPathChild(parent, segment)
		if !ok {
			return
		}
		parent = next
	}
	last := segments[len(segments)-1]
	if last.Index != nil {
		array, ok := parent.([]any)
		if !ok || *last.Index < 0 || *last.Index >= len(array) {
			return
		}
		array[*last.Index] = value
		return
	}
	if object, ok := parent.(map[string]any); ok {
		object[last.Key] = value
	}
}
