package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func postReportMap(endpoint string, payload map[string]any) (map[string]any, error) {
	return postReportMapWithContext(context.Background(), endpoint, payload)
}

func postReportMapWithContext(ctx context.Context, endpoint string, payload map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: close report response body: %v\n", closeErr)
		}
	}()
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, err
	}
	result["httpStatus"] = response.StatusCode
	return result, nil
}

func workflowStepMissingInputs(step map[string]any, contextValues map[string]any) []string {
	missing := []string{}
	for _, rawInput := range listFromReportAny(step["inputs"]) {
		item := mapFromReportAny(rawInput)
		name := valueString(item["name"])
		if name == "" || !workflowInputRequired(item) {
			continue
		}
		value, ok := contextValues[name]
		if !ok || value == nil {
			missing = append(missing, name)
		}
	}
	return missing
}

func workflowInputRequired(item map[string]any) bool {
	raw, ok := item["required"]
	if !ok {
		return true
	}
	return boolFromReportAny(raw)
}

func workflowExportedValues(step map[string]any, result map[string]any) map[string]any {
	out := map[string]any{}
	for _, rawExport := range listFromReportAny(step["exports"]) {
		item := mapFromReportAny(rawExport)
		name := valueString(item["name"])
		if name == "" {
			continue
		}
		value := workflowValueAtPath(workflowExportRoot(result, valueString(item["from"])), valueString(item["path"]))
		if value != nil {
			out[name] = value
		}
	}
	return out
}

func workflowExportRoot(result map[string]any, source string) any {
	resultBlock := mapFromReportAny(result["result"])
	request := mapFromReportAny(resultBlock["request"])
	response := mapFromReportAny(resultBlock["response"])
	responseBody := rawJSONObject(valueString(response["body"]))
	switch source {
	case "request", "requestBody":
		return firstReportValue(request, "body")
	case "requestQuery":
		return firstReportValue(request, "query")
	case "responseHeaders":
		return firstReportValue(response, "headers")
	case "response", "responseBody", "":
		return responseBody
	default:
		return responseBody
	}
}

func workflowValueAtPath(root any, path string) any {
	if strings.TrimSpace(path) == "" {
		return root
	}
	current := root
	for _, part := range strings.Split(path, ".") {
		switch typed := current.(type) {
		case map[string]any:
			current = typed[part]
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return nil
			}
			current = typed[index]
		default:
			return nil
		}
		if current == nil {
			return nil
		}
	}
	return current
}
