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
	OK      bool                      `json:"ok"`
	CaseID  string                    `json:"caseId"`
	Created bool                      `json:"created"`
	Updated bool                      `json:"updated"`
	Config  caseConfigUpsertConfigRef `json:"config"`
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
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	expectedStatuses := stringListFlag{}
	responseContains := stringListFlag{}
	responseNotContains := stringListFlag{}
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
			CaseID:              *caseID,
			Method:              *method,
			Path:                *path,
			BodyJSON:            *bodyJSON,
			ExpectedStatuses:    expectedStatuses.Values(),
			ResponseContains:    responseContains.Values(),
			ResponseNotContains: responseNotContains.Values(),
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
	CaseID              string
	Method              string
	Path                string
	BodyJSON            string
	ExpectedStatuses    []string
	ResponseContains    []string
	ResponseNotContains []string
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
	body, err := parseOptionalJSONObject(options.BodyJSON)
	if err != nil {
		return caseConfigUpsertReport{}, err
	}
	statuses, err := parseExpectedStatuses(options.ExpectedStatuses)
	if err != nil {
		return caseConfigUpsertReport{}, err
	}
	configID := "config." + safeReportID(caseID) + ".execution"
	config, exists := findCatalogTemplateConfig(catalog.TemplateConfigs, configID)
	config.ID = configID
	config.ScopeType = "case"
	config.ScopeID = caseID
	config.Status = "active"
	configJSON, err := compactJSON(map[string]any{
		"caseId": caseID,
		"caseExecution": map[string]any{
			"method":                      firstNonEmpty(strings.TrimSpace(options.Method), apiCaseMethod(catalog, apiCase)),
			"nodeId":                      apiCase.NodeID,
			"path":                        firstNonEmpty(strings.TrimSpace(options.Path), apiCasePath(catalog, apiCase)),
			"body":                        body,
			"expectedHttpCodes":           statuses,
			"expectedResponseContains":    options.ResponseContains,
			"expectedResponseNotContains": options.ResponseNotContains,
		},
	})
	if err != nil {
		return caseConfigUpsertReport{}, err
	}
	config.ConfigJSON = configJSON
	catalog.TemplateConfigs = upsertCatalogTemplateConfig(catalog.TemplateConfigs, config)
	catalog.IndexedAt = time.Now().UTC()
	if err := runtime.ReplaceProfileCatalog(ctx, catalog); err != nil {
		return caseConfigUpsertReport{}, err
	}
	return caseConfigUpsertReport{
		OK:      true,
		CaseID:  caseID,
		Created: !exists,
		Updated: exists,
		Config:  caseConfigUpsertConfigRef{ID: configID},
	}, nil
}

func parseOptionalJSONObject(raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var out any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("decode --body-json: %w", err)
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

func upsertCatalogTemplateConfig(items []store.CatalogTemplateConfig, next store.CatalogTemplateConfig) []store.CatalogTemplateConfig {
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
