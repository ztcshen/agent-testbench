package controlplane

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/runner/apicase"
	"agent-testbench/internal/store"
)

func materializeAPICaseBatchExecution(ctx context.Context, bundle profile.Bundle, runtime store.Store, batchRunID string, workflowID string, plan apiCaseBatchCasePlan) (string, string, error) {
	payload := map[string]any{
		"caseId":     plan.ID,
		"stepId":     plan.StepID,
		"workflowId": workflowID,
		"baseUrl":    plan.BaseURL,
		"overrides":  plan.Overrides,
	}
	request, err := buildCaseHTTPRequest(ctx, bundle, runtime, *plan.Execution, plan.BaseURL, payload)
	if err != nil {
		return "", "", err
	}
	if err := applyAPICaseRequestModel(&request, plan.Case); err != nil {
		return "", "", err
	}
	body := mapFromAny(request.body)
	apiCase := apicase.Case{
		ID:    plan.ID,
		Title: firstNonEmpty(plan.DisplayName, plan.ID),
		Request: apicase.Request{
			Method:  request.method,
			Path:    apiCaseBatchRequestPath(request),
			Headers: request.headers,
			Body:    body,
		},
		Assertions: apicase.Assertions{
			ExpectedStatusCodes: request.expectedHTTPCodes,
			ResponseContains:    request.expectedResponse,
		},
	}
	dir := filepath.Join(".runtime", "case-batches", batchRunID, "materialized-cases")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	path := filepath.Join(dir, safeRuntimeLogPathSegment(firstNonEmpty(plan.StepID, plan.ID))+".json")
	raw, err := json.MarshalIndent(apiCase, "", "  ")
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return "", "", err
	}
	return path, request.baseURL, nil
}

func apiCaseBatchRequestPath(request caseHTTPRequest) string {
	baseURL := strings.TrimRight(strings.TrimSpace(request.baseURL), "/")
	fullURL := strings.TrimSpace(request.fullURL)
	if baseURL != "" && strings.HasPrefix(fullURL, baseURL) {
		path := strings.TrimSpace(strings.TrimPrefix(fullURL, baseURL))
		if path != "" {
			return path
		}
	}
	if strings.TrimSpace(request.path) != "" {
		return request.path
	}
	return "/"
}
