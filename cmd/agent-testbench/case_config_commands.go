package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

type caseConfigUpsertReport struct {
	OK               bool                      `json:"ok"`
	CaseID           string                    `json:"caseId"`
	Created          bool                      `json:"created"`
	Updated          bool                      `json:"updated"`
	Config           caseConfigUpsertConfigRef `json:"config"`
	DefaultOverrides map[string]any            `json:"defaultOverrides,omitempty"`
	SelectedByRunner bool                      `json:"selectedByRunner"`
	Warnings         []string                  `json:"warnings,omitempty"`
}

type caseConfigUpsertConfigRef struct {
	ID string `json:"id"`
}

func runCaseConfig(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing case config command")
	}
	switch args[0] {
	case "upsert":
		return runCaseConfigUpsert(ctx, args[1:])
	default:
		return fmt.Errorf("unknown case config command: %s", args[0])
	}
}

func runCaseConfigUpsert(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case config upsert", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	caseID := flags.String("case", "", "API case id")
	method := flags.String("method", "", "HTTP method")
	path := flags.String("path", "", "Request path")
	bodyJSON := flags.String("body-json", "", "Request body JSON")
	nodeID := flags.String("node-id", "", "Override interface node id")
	configID := flags.String("config-id", "", "Template config id to update")
	authJSON := flags.String("auth-json", "", "Request auth JSON")
	headersJSON := flags.String("headers-json", "", "Request headers JSON object")
	defaultOverridesJSON := flags.String("default-overrides-json", "", "Default request overrides JSON object persisted on the catalog case")
	inputsJSON := flags.String("inputs-json", "", "Workflow input metadata JSON array for this case execution config")
	exportsJSON := flags.String("exports-json", "", "Workflow export metadata JSON array for this case execution config")
	signed := flags.Bool("signed", false, "Enable request signing with the configured auth block")
	traceEndpoint := flags.String("trace-endpoint", "", "Trace endpoint associated with this case execution")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	headers := stringListFlag{}
	defaultOverrides := mapFlag{}
	expectedStatuses := stringListFlag{}
	responseContains := stringListFlag{}
	responseNotContains := stringListFlag{}
	flags.Var(&headers, "header", "Request header as Name=Value; repeat for multiple headers")
	flags.Var(&defaultOverrides, "default-override", "Default override as key=value persisted on the catalog case; repeat for multiple values")
	flags.Var(&expectedStatuses, "expected-status", "Expected HTTP status; repeat for multiple values")
	flags.Var(&responseContains, "response-contains", "Required response fragment; repeat for multiple values")
	flags.Var(&responseNotContains, "response-not-contains", "Forbidden response fragment; repeat for multiple values")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected case config arguments: %s", strings.Join(flags.Args(), " "))
	}
	if strings.TrimSpace(*caseID) == "" {
		return errors.New("--case is required")
	}
	storeDSN, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	runtime, err := openStore(ctx, storeDSN)
	if err != nil {
		return err
	}
	defer closeCLIStore(runtime)
	var report caseConfigUpsertReport
	err = withProfileCatalogWriteLock(storeDSN, func() error {
		var upsertErr error
		report, upsertErr = upsertCaseExecutionConfig(ctx, runtime, caseConfigUpsertOptions{
			CaseID:               *caseID,
			ConfigID:             *configID,
			Method:               *method,
			Path:                 *path,
			BodyJSON:             *bodyJSON,
			NodeID:               *nodeID,
			Headers:              headers.Values(),
			HeadersJSON:          *headersJSON,
			AuthJSON:             *authJSON,
			DefaultOverrides:     defaultOverrides.Values(),
			DefaultOverridesJSON: *defaultOverridesJSON,
			InputsJSON:           *inputsJSON,
			ExportsJSON:          *exportsJSON,
			Signed:               *signed,
			TraceEndpoint:        *traceEndpoint,
			ExpectedStatuses:     expectedStatuses.Values(),
			ResponseContains:     responseContains.Values(),
			ResponseNotContains:  responseNotContains.Values(),
		})
		return upsertErr
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	fmt.Printf("Updated case config: %s\n", report.Config.ID)
	return nil
}

type caseConfigUpsertOptions struct {
	CaseID               string
	ConfigID             string
	Method               string
	Path                 string
	BodyJSON             string
	NodeID               string
	Headers              []string
	HeadersJSON          string
	AuthJSON             string
	DefaultOverrides     map[string]any
	DefaultOverridesJSON string
	InputsJSON           string
	ExportsJSON          string
	Signed               bool
	TraceEndpoint        string
	ExpectedStatuses     []string
	ResponseContains     []string
	ResponseNotContains  []string
}

func upsertCaseExecutionConfig(ctx context.Context, runtime store.Store, options caseConfigUpsertOptions) (caseConfigUpsertReport, error) {
	catalog, err := loadMutableProfileCatalog(ctx, runtime, "")
	if err != nil {
		return caseConfigUpsertReport{}, err
	}
	caseID := strings.TrimSpace(options.CaseID)
	apiCase, ok := findCatalogAPICase(catalog.APICases, caseID)
	if !ok {
		return caseConfigUpsertReport{}, fmt.Errorf("api case not found in Store catalog: %s", caseID)
	}
	body, hasBody, err := parseOptionalJSONValue("body-json", options.BodyJSON)
	if err != nil {
		return caseConfigUpsertReport{}, err
	}
	headers, err := parseHeadersOptions(options.Headers, options.HeadersJSON)
	if err != nil {
		return caseConfigUpsertReport{}, err
	}
	auth, hasAuth, err := parseOptionalJSONObject("auth-json", options.AuthJSON)
	if err != nil {
		return caseConfigUpsertReport{}, err
	}
	defaultOverrides, hasDefaultOverrides, err := parseDefaultOverrideOptions(options.DefaultOverrides, options.DefaultOverridesJSON)
	if err != nil {
		return caseConfigUpsertReport{}, err
	}
	inputs, hasInputs, err := parseOptionalJSONArrayObjects("inputs-json", options.InputsJSON)
	if err != nil {
		return caseConfigUpsertReport{}, err
	}
	exports, hasExports, err := parseOptionalJSONArrayObjects("exports-json", options.ExportsJSON)
	if err != nil {
		return caseConfigUpsertReport{}, err
	}
	statuses, err := parseExpectedStatuses(options.ExpectedStatuses)
	if err != nil {
		return caseConfigUpsertReport{}, err
	}
	configID := strings.TrimSpace(options.ConfigID)
	if configID == "" {
		configID = selectedCaseExecutionTemplateConfigID(catalog, caseID)
	}
	if configID == "" {
		configID = "config." + safeReportID(caseID) + ".execution"
	}
	config, exists := findCatalogTemplateConfig(catalog.TemplateConfigs, configID)
	config.ID = configID
	if !exists || !isCaseExecutionConfigScope(config.ScopeType) {
		config.ScopeType = "case"
	}
	config.ScopeID = caseID
	config.Status = "active"
	configJSON, err := mergeCaseExecutionConfigJSON(config.ConfigJSON, caseID, apiCase, catalog, caseConfigExecutionPatch{
		Method:              strings.TrimSpace(options.Method),
		Path:                strings.TrimSpace(options.Path),
		Body:                body,
		HasBody:             hasBody,
		NodeID:              strings.TrimSpace(options.NodeID),
		Headers:             headers,
		Auth:                auth,
		HasAuth:             hasAuth,
		Signed:              options.Signed,
		TraceEndpoint:       strings.TrimSpace(options.TraceEndpoint),
		ExpectedStatuses:    statuses,
		ResponseContains:    options.ResponseContains,
		ResponseNotContains: options.ResponseNotContains,
		Inputs:              inputs,
		HasInputs:           hasInputs,
		Exports:             exports,
		HasExports:          hasExports,
	})
	if err != nil {
		return caseConfigUpsertReport{}, err
	}
	config.ConfigJSON = configJSON
	catalog.TemplateConfigs = upsertCatalogTemplateConfig(catalog.TemplateConfigs, config)
	if hasDefaultOverrides {
		defaultOverrides = mergeCatalogDefaultOverrides(apiCase.DefaultOverridesJSON, defaultOverrides)
		apiCase.DefaultOverridesJSON = compactJSONObject(defaultOverrides)
		catalog.APICases = upsertCatalogAPICase(catalog.APICases, apiCase)
	}
	selectedID := selectedCaseExecutionTemplateConfigID(catalog, caseID)
	catalog.IndexedAt = time.Now().UTC()
	if err := runtime.ReplaceProfileCatalog(ctx, catalog); err != nil {
		return caseConfigUpsertReport{}, err
	}
	return caseConfigUpsertReport{
		OK:               true,
		CaseID:           caseID,
		Created:          !exists,
		Updated:          exists,
		Config:           caseConfigUpsertConfigRef{ID: configID},
		DefaultOverrides: defaultOverrides,
		SelectedByRunner: selectedID == configID,
	}, nil
}

type caseConfigExecutionPatch struct {
	Method              string
	Path                string
	Body                any
	HasBody             bool
	NodeID              string
	Headers             map[string]any
	Auth                any
	HasAuth             bool
	Signed              bool
	TraceEndpoint       string
	ExpectedStatuses    []int
	ResponseContains    []string
	ResponseNotContains []string
	Inputs              []map[string]any
	HasInputs           bool
	Exports             []map[string]any
	HasExports          bool
}

func mergeCaseExecutionConfigJSON(raw string, caseID string, apiCase store.CatalogAPICase, catalog store.ProfileCatalog, patch caseConfigExecutionPatch) (string, error) {
	doc := map[string]any{}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			return "", fmt.Errorf("decode existing template config JSON: %w", err)
		}
	}
	execution := mapFromReportAny(doc["caseExecution"])
	execution["method"] = firstNonEmpty(patch.Method, valueString(execution["method"]), apiCaseMethod(catalog, apiCase))
	execution["nodeId"] = firstNonEmpty(patch.NodeID, valueString(execution["nodeId"]), apiCase.NodeID)
	execution["path"] = firstNonEmpty(patch.Path, valueString(execution["path"]), apiCasePath(catalog, apiCase))
	if patch.HasBody {
		execution["body"] = patch.Body
	}
	if len(patch.ExpectedStatuses) > 0 {
		execution["expectedHttpCodes"] = patch.ExpectedStatuses
	}
	if len(patch.ResponseContains) > 0 {
		execution["expectedResponseContains"] = patch.ResponseContains
	}
	if len(patch.ResponseNotContains) > 0 {
		execution["expectedResponseNotContains"] = patch.ResponseNotContains
	}
	if len(patch.Headers) > 0 {
		existingHeaders := mapFromReportAny(execution["headers"])
		for key, value := range patch.Headers {
			existingHeaders[key] = value
		}
		execution["headers"] = existingHeaders
	}
	if patch.HasAuth {
		execution["auth"] = patch.Auth
	}
	if patch.Signed {
		execution["signed"] = true
	}
	if patch.TraceEndpoint != "" {
		execution["traceEndpoint"] = patch.TraceEndpoint
	}
	doc["caseId"] = caseID
	doc["caseExecution"] = execution
	if patch.HasInputs {
		doc["inputs"] = patch.Inputs
	}
	if patch.HasExports {
		doc["exports"] = patch.Exports
	}
	return compactJSON(doc)
}

func parseDefaultOverrideOptions(flagValues map[string]any, rawJSON string) (map[string]any, bool, error) {
	out := map[string]any{}
	hasValue := false
	if strings.TrimSpace(rawJSON) != "" {
		parsed, ok, err := parseOptionalJSONObject("default-overrides-json", rawJSON)
		if err != nil {
			return nil, false, err
		}
		if ok {
			hasValue = true
			for key, value := range parsed {
				out[key] = value
			}
		}
	}
	if len(flagValues) > 0 {
		hasValue = true
		for key, value := range flagValues {
			out[key] = value
		}
	}
	return out, hasValue, nil
}

func parseOptionalJSONArrayObjects(name string, raw string) ([]map[string]any, bool, error) {
	value, ok, err := parseOptionalJSONValue(name, raw)
	if err != nil || !ok {
		return nil, ok, err
	}
	items, valid := value.([]any)
	if !valid {
		return nil, false, fmt.Errorf("--%s must be a JSON array", name)
	}
	out := make([]map[string]any, 0, len(items))
	for index, item := range items {
		typed, valid := item.(map[string]any)
		if !valid || typed == nil {
			return nil, false, fmt.Errorf("--%s item %d must be a JSON object", name, index)
		}
		out = append(out, typed)
	}
	return out, true, nil
}

func compactJSONObject(value map[string]any) string {
	if len(value) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func mergeCatalogDefaultOverrides(existingJSON string, updates map[string]any) map[string]any {
	merged := rawJSONObject(existingJSON)
	for key, value := range updates {
		merged[key] = value
	}
	return merged
}

func parseOptionalJSONValue(name string, raw string) (any, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false, nil
	}
	var out any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, false, fmt.Errorf("decode --%s: %w", name, err)
	}
	return out, true, nil
}

func parseOptionalJSONObject(name string, raw string) (map[string]any, bool, error) {
	value, ok, err := parseOptionalJSONValue(name, raw)
	if err != nil || !ok {
		return nil, ok, err
	}
	out, valid := value.(map[string]any)
	if !valid {
		return nil, false, fmt.Errorf("--%s must be a JSON object", name)
	}
	return out, true, nil
}

func parseHeadersOptions(values []string, rawJSON string) (map[string]any, error) {
	out := map[string]any{}
	if strings.TrimSpace(rawJSON) != "" {
		parsed, ok, err := parseOptionalJSONObject("headers-json", rawJSON)
		if err != nil {
			return nil, err
		}
		if ok {
			for key, value := range parsed {
				out[key] = value
			}
		}
	}
	for _, value := range values {
		key, headerValue, ok := strings.Cut(value, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --header %q, expected Name=Value", value)
		}
		out[key] = strings.TrimSpace(headerValue)
	}
	return out, nil
}

func parseExpectedStatuses(values []string) ([]int, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]int, 0, len(values))
	for _, value := range values {
		status, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || status <= 0 {
			return nil, fmt.Errorf("invalid --expected-status %q", value)
		}
		out = append(out, status)
	}
	return out, nil
}

func findCatalogAPICase(items []store.CatalogAPICase, id string) (store.CatalogAPICase, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return store.CatalogAPICase{}, false
}

func findCatalogTemplateConfig(items []store.CatalogTemplateConfig, id string) (store.CatalogTemplateConfig, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return store.CatalogTemplateConfig{}, false
}

func selectedCaseExecutionTemplateConfigID(catalog store.ProfileCatalog, caseID string) string {
	for _, config := range catalog.TemplateConfigs {
		if config.Status != "" && config.Status != "active" {
			continue
		}
		if !isCaseExecutionConfigScope(config.ScopeType) {
			continue
		}
		if config.ScopeID != "" && config.ScopeID != caseID {
			continue
		}
		var parsed struct {
			CaseID        string         `json:"caseId"`
			CaseExecution map[string]any `json:"caseExecution"`
		}
		if err := json.Unmarshal([]byte(config.ConfigJSON), &parsed); err != nil {
			continue
		}
		if parsed.CaseID != caseID {
			continue
		}
		if valueString(parsed.CaseExecution["method"]) == "" && valueString(parsed.CaseExecution["path"]) == "" && valueString(parsed.CaseExecution["nodeId"]) == "" {
			continue
		}
		return config.ID
	}
	return ""
}

func isCaseExecutionConfigScope(scopeType string) bool {
	scopeType = strings.TrimSpace(scopeType)
	return scopeType == "" || scopeType == "case" || scopeType == "api-case"
}

func upsertCatalogTemplateConfig(items []store.CatalogTemplateConfig, next store.CatalogTemplateConfig) []store.CatalogTemplateConfig {
	for i, item := range items {
		if item.ID == next.ID {
			items[i] = next
			return items
		}
	}
	return append(items, next)
}

func upsertCatalogAPICase(items []store.CatalogAPICase, next store.CatalogAPICase) []store.CatalogAPICase {
	for i, item := range items {
		if item.ID == next.ID {
			items[i] = next
			return items
		}
	}
	return append(items, next)
}

func apiCaseMethod(catalog store.ProfileCatalog, item store.CatalogAPICase) string {
	for _, node := range catalog.InterfaceNodes {
		if node.ID == item.NodeID && strings.TrimSpace(node.Method) != "" {
			return strings.TrimSpace(node.Method)
		}
	}
	return "GET"
}

func apiCasePath(catalog store.ProfileCatalog, item store.CatalogAPICase) string {
	for _, node := range catalog.InterfaceNodes {
		if node.ID == item.NodeID && strings.TrimSpace(node.Path) != "" {
			return strings.TrimSpace(node.Path)
		}
	}
	return "/"
}
