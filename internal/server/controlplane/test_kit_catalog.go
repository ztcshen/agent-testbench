package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/store"
)

func findAPICase(items []profile.APICase, id string) (profile.APICase, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return profile.APICase{}, false
}

func findRunnableAPICase(ctx context.Context, bundle profile.Bundle, runtime store.Store, id string, payload map[string]any) (runnableAPICase, bool) {
	if item, ok := findAPICase(bundle.APICases, id); ok {
		item.CasePath = resolveBundleAPICasePath(ctx, runtime, bundle, item.CasePath)
		execution := findCaseExecutionConfig(ctx, runtime, id, payload)
		caseBaseURL := item.BaseURL
		if runtime != nil {
			if catalogItem, catalog, ok := findCatalogAPICase(ctx, runtime, id); ok {
				catalogCase := profileAPICaseFromCatalog(catalogItem)
				catalogCase.CasePath = resolveCatalogAPICasePath(ctx, runtime, catalog.ProfileID, catalogItem.CasePath)
				catalogCase.PayloadTemplateJSON = apiCasePayloadTemplateJSON(catalogCase.PayloadTemplateJSON, catalogRequestTemplateJSON(catalog, catalogItem.RequestTemplateID))
				item = mergeRunnableAPICaseModel(item, catalogCase)
				caseBaseURL = firstNonEmpty(caseBaseURL, catalogItem.BaseURL)
				if execution == nil {
					execution = deriveCaseExecutionConfigFromCatalog(catalog, catalogItem)
				}
			}
		}
		return runnableAPICase{Case: item, Execution: execution, CaseBaseURL: caseBaseURL}, true
	}
	if runtime == nil {
		return runnableAPICase{}, false
	}
	catalog, err := runtime.GetProfileCatalog(ctx)
	if err != nil {
		return runnableAPICase{}, false
	}
	for _, item := range catalog.APICases {
		if item.ID == id {
			apiCase := profileAPICaseFromCatalog(item)
			apiCase.CasePath = resolveCatalogAPICasePath(ctx, runtime, catalog.ProfileID, item.CasePath)
			apiCase.PayloadTemplateJSON = apiCasePayloadTemplateJSON(apiCase.PayloadTemplateJSON, catalogRequestTemplateJSON(catalog, item.RequestTemplateID))
			execution := findCaseExecutionConfigFromCatalog(catalog, id, payload)
			if execution == nil {
				execution = deriveCaseExecutionConfigFromCatalog(catalog, item)
			}
			return runnableAPICase{Case: apiCase, Execution: execution, CaseBaseURL: item.BaseURL}, true
		}
	}
	return runnableAPICase{}, false
}

func findCatalogAPICase(ctx context.Context, runtime store.Store, id string) (store.CatalogAPICase, store.ProfileCatalog, bool) {
	catalog, err := runtime.GetProfileCatalog(ctx)
	if err != nil {
		return store.CatalogAPICase{}, store.ProfileCatalog{}, false
	}
	for _, item := range catalog.APICases {
		if item.ID == id {
			return item, catalog, true
		}
	}
	return store.CatalogAPICase{}, store.ProfileCatalog{}, false
}

func catalogRequestTemplateJSON(catalog store.ProfileCatalog, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	for _, item := range catalog.RequestTemplates {
		if item.ID == id {
			return item.TemplateJSON
		}
	}
	return ""
}

func apiCasePayloadTemplateJSON(value string, requestTemplateJSON string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "{}" {
		return requestTemplateJSON
	}
	return value
}

func mergeRunnableAPICaseModel(base profile.APICase, enriched profile.APICase) profile.APICase {
	if strings.TrimSpace(base.DisplayName) == "" {
		base.DisplayName = enriched.DisplayName
	}
	if strings.TrimSpace(base.Description) == "" {
		base.Description = enriched.Description
	}
	if strings.TrimSpace(base.NodeID) == "" {
		base.NodeID = enriched.NodeID
	}
	if strings.TrimSpace(base.CaseType) == "" {
		base.CaseType = enriched.CaseType
	}
	if strings.TrimSpace(base.Scenario) == "" {
		base.Scenario = enriched.Scenario
	}
	if len(base.Tags) == 0 {
		base.Tags = enriched.Tags
	}
	if strings.TrimSpace(base.Priority) == "" {
		base.Priority = enriched.Priority
	}
	if strings.TrimSpace(base.Owner) == "" {
		base.Owner = enriched.Owner
	}
	if strings.TrimSpace(base.PayloadTemplateJSON) == "" || strings.TrimSpace(base.PayloadTemplateJSON) == "{}" {
		base.PayloadTemplateJSON = enriched.PayloadTemplateJSON
	}
	if strings.TrimSpace(base.RequestTemplateID) == "" {
		base.RequestTemplateID = enriched.RequestTemplateID
	}
	if strings.TrimSpace(base.PatchJSON) == "" {
		base.PatchJSON = enriched.PatchJSON
	}
	if strings.TrimSpace(base.RenderMode) == "" {
		base.RenderMode = enriched.RenderMode
	}
	if strings.TrimSpace(base.ExpectedJSON) == "" {
		base.ExpectedJSON = enriched.ExpectedJSON
	}
	if strings.TrimSpace(base.CasePath) == "" {
		base.CasePath = enriched.CasePath
	}
	if strings.TrimSpace(base.BaseURL) == "" {
		base.BaseURL = enriched.BaseURL
	}
	if strings.TrimSpace(base.EvidenceDir) == "" {
		base.EvidenceDir = enriched.EvidenceDir
	}
	if base.TimeoutSeconds == 0 {
		base.TimeoutSeconds = enriched.TimeoutSeconds
	}
	if len(base.DefaultOverrides) == 0 {
		base.DefaultOverrides = enriched.DefaultOverrides
	}
	return base
}

func profileAPICaseFromCatalog(item store.CatalogAPICase) profile.APICase {
	return profile.APICase{
		ID:                   item.ID,
		DisplayName:          item.DisplayName,
		Description:          item.Description,
		NodeID:               item.NodeID,
		CaseType:             item.CaseType,
		Scenario:             item.Scenario,
		Tags:                 item.Tags,
		Priority:             item.Priority,
		Owner:                item.Owner,
		PayloadTemplateJSON:  item.PayloadTemplateJSON,
		RequestTemplateID:    item.RequestTemplateID,
		PatchJSON:            item.PatchJSON,
		RenderMode:           item.RenderMode,
		ExpectedJSON:         item.ExpectedJSON,
		RequiredForAdmission: item.RequiredForAdmission,
		Status:               item.Status,
		SortOrder:            item.SortOrder,
		CasePath:             item.CasePath,
		SourceKind:           item.SourceKind,
		SourcePath:           item.SourcePath,
		ExecutorID:           item.ExecutorID,
		BaseURL:              item.BaseURL,
		EvidenceDir:          item.EvidenceDir,
		TimeoutSeconds:       item.TimeoutSeconds,
		DefaultOverrides:     mapFromAny(jsonObject(item.DefaultOverridesJSON)),
	}
}

func deriveCaseExecutionConfigFromCatalog(catalog store.ProfileCatalog, item store.CatalogAPICase) *caseExecutionConfig {
	if execution := deriveCaseExecutionConfigFromSiblingConfig(catalog, item); execution != nil {
		return execution
	}
	templateID := strings.TrimSpace(item.RequestTemplateID)
	if templateID == "" {
		return nil
	}
	var selected store.CatalogRequestTemplate
	for _, template := range catalog.RequestTemplates {
		if template.ID == templateID && activeCatalogStatus(template.Status) {
			selected = template
			break
		}
	}
	if strings.TrimSpace(selected.ID) == "" {
		return nil
	}
	method := strings.ToUpper(firstNonEmpty(selected.Method, "GET"))
	execution := caseExecutionConfig{
		Method: strings.ToUpper(method),
		NodeID: firstNonEmpty(item.NodeID, selected.NodeID),
		Path:   firstNonEmpty(selected.Path, "/"),
	}
	if strings.TrimSpace(selected.TemplateJSON) != "" {
		var body any
		if json.Unmarshal([]byte(selected.TemplateJSON), &body) == nil {
			if method == http.MethodGet || method == http.MethodHead {
				execution.Query = mapFromAny(body)
			} else {
				execution.Body = body
			}
		}
	}
	if expected := expectedConfigFromAPICase(item.ExpectedJSON); expected != nil {
		execution.ExpectedHTTPCodes = expected.ExpectedHTTPCodes
		execution.ExpectedResponse = expected.ExpectedResponse
		execution.RequireRequestID = expected.RequireRequestID
	}
	return &execution
}

func deriveCaseExecutionConfigFromSiblingConfig(catalog store.ProfileCatalog, item store.CatalogAPICase) *caseExecutionConfig {
	caseNodeByID := map[string]string{}
	for _, apiCase := range catalog.APICases {
		caseNodeByID[apiCase.ID] = apiCase.NodeID
	}
	for _, config := range catalog.TemplateConfigs {
		if config.Status != "" && config.Status != "active" {
			continue
		}
		var parsed caseExecutionTemplateConfig
		if json.Unmarshal([]byte(config.ConfigJSON), &parsed) != nil {
			continue
		}
		if strings.TrimSpace(config.NodeID) != "" && config.NodeID != item.NodeID && caseNodeByID[parsed.CaseID] != item.NodeID {
			continue
		}
		next := parsed.CaseExecution
		if next.Method == "" && next.Path == "" && next.NodeID == "" {
			continue
		}
		if strings.TrimSpace(config.NodeID) == "" && caseNodeByID[parsed.CaseID] != item.NodeID {
			continue
		}
		cloned := cloneCaseExecutionConfig(next)
		if expected := expectedConfigFromAPICase(item.ExpectedJSON); expected != nil {
			cloned.ExpectedHTTPCodes = expected.ExpectedHTTPCodes
			cloned.ExpectedResponse = expected.ExpectedResponse
			cloned.RequireRequestID = expected.RequireRequestID
		}
		return &cloned
	}
	return nil
}

func cloneCaseExecutionConfig(input caseExecutionConfig) caseExecutionConfig {
	raw, err := json.Marshal(input)
	if err != nil {
		return input
	}
	var out caseExecutionConfig
	if json.Unmarshal(raw, &out) != nil {
		return input
	}
	return out
}

func expectedConfigFromAPICase(expectedJSON string) *caseExecutionConfig {
	expectedJSON = strings.TrimSpace(expectedJSON)
	if expectedJSON == "" || expectedJSON == "{}" {
		return nil
	}
	var parsed struct {
		ExpectedHTTPCodes []int    `json:"expectedHttpCodes"`
		ResponseContains  []string `json:"expectedResponseContains"`
		RequireRequestID  bool     `json:"requireRequestId"`
	}
	if json.Unmarshal([]byte(expectedJSON), &parsed) != nil {
		return nil
	}
	return &caseExecutionConfig{
		ExpectedHTTPCodes: parsed.ExpectedHTTPCodes,
		ExpectedResponse:  parsed.ResponseContains,
		RequireRequestID:  parsed.RequireRequestID,
	}
}

func resolveBundleAPICasePath(ctx context.Context, runtime store.Store, bundle profile.Bundle, casePath string) string {
	casePath = strings.TrimSpace(casePath)
	if casePath == "" || filepath.IsAbs(casePath) || fileExists(casePath) {
		return casePath
	}
	if strings.TrimSpace(bundle.BaseDir) != "" {
		candidate := resolveProfilePath(bundle.BaseDir, filepath.FromSlash(casePath))
		if fileExists(candidate) {
			return candidate
		}
	}
	return resolveCatalogAPICasePath(ctx, runtime, bundle.ID, casePath)
}

func resolveCatalogAPICasePath(ctx context.Context, runtime store.Store, profileID string, casePath string) string {
	casePath = strings.TrimSpace(casePath)
	if casePath == "" || filepath.IsAbs(casePath) || fileExists(casePath) || runtime == nil {
		return casePath
	}
	index, err := runtime.GetProfileIndex(ctx, strings.TrimSpace(profileID))
	if err != nil || strings.TrimSpace(index.BundlePath) == "" {
		return casePath
	}
	candidate := filepath.Join(index.BundlePath, filepath.FromSlash(casePath))
	if fileExists(candidate) {
		return candidate
	}
	return casePath
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func findCaseExecutionConfig(ctx context.Context, runtime store.Store, caseID string, payload map[string]any) *caseExecutionConfig {
	template := findCaseExecutionTemplateConfig(ctx, runtime, caseID, payload)
	if template == nil {
		return nil
	}
	return &template.CaseExecution
}

func findCaseExecutionTemplateConfig(ctx context.Context, runtime store.Store, caseID string, payload map[string]any) *caseExecutionTemplateConfig {
	if runtime == nil {
		return nil
	}
	catalog, err := runtime.GetProfileCatalog(ctx)
	if err != nil {
		return nil
	}
	return findCaseExecutionTemplateConfigFromCatalog(catalog, caseID, payload)
}

func findCaseExecutionConfigFromCatalog(catalog store.ProfileCatalog, caseID string, payload map[string]any) *caseExecutionConfig {
	template := findCaseExecutionTemplateConfigFromCatalog(catalog, caseID, payload)
	if template == nil {
		return nil
	}
	return &template.CaseExecution
}

func findCaseExecutionTemplateConfigFromCatalog(catalog store.ProfileCatalog, caseID string, payload map[string]any) *caseExecutionTemplateConfig {
	workflowID := valueString(payload["workflowId"])
	stepID := valueString(payload["stepId"])
	var defaultValue *caseExecutionTemplateConfig
	for _, config := range catalog.TemplateConfigs {
		if config.Status != "" && config.Status != "active" {
			continue
		}
		var parsed caseExecutionTemplateConfig
		if err := json.Unmarshal([]byte(config.ConfigJSON), &parsed); err != nil {
			continue
		}
		next := parsed.CaseExecution
		if next.Method == "" && next.Path == "" && next.NodeID == "" {
			continue
		}
		if workflowID != "" && stepID != "" && config.WorkflowID == workflowID && config.ScopeID == stepID {
			return &parsed
		}
		if parsed.CaseID != caseID {
			continue
		}
		if defaultValue == nil {
			defaultValue = &parsed
		}
	}
	return defaultValue
}

func testKitCaseIDs(value any) []string {
	switch typed := value.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if id := valueString(item); id != "" {
				out = append(out, id)
			}
		}
		return out
	case []string:
		return typed
	default:
		return nil
	}
}

func boolValue(value any) bool {
	typed, _ := value.(bool)
	return typed
}
