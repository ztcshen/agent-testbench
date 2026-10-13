package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
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
	// Studio bindings are explicit dependencies, so a missing exported value
	// must fail before sending an incorrectly rendered request.
	if runtime != nil {
		doc := workflowStepDocumentFromBundle(bundle.TemplateConfigs, workflowID, plan.StepID, plan.ID)
		if err := validateStepGroupRuntimeParameters(doc, plan.Overrides); err != nil {
			return "", "", err
		}
		meta := mapFromAny(doc["workflowStudio"])
		for alias := range stringSetFromAny(meta["managedInputs"]) {
			if value, exists := plan.Overrides[alias]; !exists || value == nil || strings.TrimSpace(valueString(value)) == "" {
				return "", "", fmt.Errorf("step %s is missing the selected upstream value %s", plan.StepID, alias)
			}
		}
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
		Metadata: workflowStepGroupRunMetadata(bundle.TemplateConfigs, workflowID, plan),
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
