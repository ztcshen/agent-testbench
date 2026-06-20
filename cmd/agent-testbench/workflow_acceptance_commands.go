package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

func workflowAcceptanceURL(serverURL string, apiPath string) string {
	return strings.TrimRight(strings.TrimSpace(serverURL), "/") + apiPath
}

func postWorkflowAcceptanceJSON(ctx context.Context, endpoint string, payload map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return doWorkflowAcceptanceJSON(req)
}

func fetchWorkflowAcceptanceJSON(ctx context.Context, endpoint string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	return doWorkflowAcceptanceJSON(req)
}

func doWorkflowAcceptanceJSON(req *http.Request) (map[string]any, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: close workflow acceptance response body: %v\n", closeErr)
		}
	}()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return payload, fmt.Errorf("%s %s failed with http status %d: %s", req.Method, req.URL.String(), resp.StatusCode, valueString(payload["error"]))
	}
	return payload, nil
}
