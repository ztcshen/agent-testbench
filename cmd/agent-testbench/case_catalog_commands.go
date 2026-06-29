package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"agent-testbench/internal/domain/commandline"
	"agent-testbench/internal/store"
)

const caseCatalogCommandUpsert = "upsert"

type caseCatalogUpsertReport struct {
	OK          bool                        `json:"ok"`
	ProfileID   string                      `json:"profileId"`
	Created     bool                        `json:"created"`
	Updated     bool                        `json:"updated"`
	Case        caseCatalogUpsertCaseRef    `json:"case"`
	Counts      workflowCatalogUpsertCounts `json:"counts"`
	NextActions []string                    `json:"nextActions,omitempty"`
}

type caseCatalogUpsertCaseRef struct {
	ID                string         `json:"id"`
	DisplayName       string         `json:"displayName,omitempty"`
	NodeID            string         `json:"nodeId"`
	RequestTemplateID string         `json:"requestTemplateId,omitempty"`
	CaseType          string         `json:"caseType,omitempty"`
	RenderMode        string         `json:"renderMode,omitempty"`
	Status            string         `json:"status,omitempty"`
	DefaultOverrides  map[string]any `json:"defaultOverrides,omitempty"`
}

type caseCatalogUpsertOptions struct {
	ProfileID            string
	CaseID               string
	NodeID               string
	DisplayName          string
	Description          string
	RequestTemplateID    string
	CaseType             string
	Scenario             string
	Tags                 []string
	Priority             string
	Owner                string
	Status               string
	SortOrder            int
	CasePath             string
	SourceKind           string
	SourcePath           string
	ExecutorID           string
	BaseURL              string
	EvidenceDir          string
	TimeoutSeconds       int
	RenderMode           string
	PatchJSON            string
	ExpectedJSON         string
	DefaultOverrides     map[string]any
	DefaultOverridesJSON string
	PassedFlags          map[string]bool
}

func runCaseCatalog(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing case catalog command")
	}
	switch args[0] {
	case caseCatalogCommandUpsert:
		return runCaseCatalogUpsert(ctx, args[1:])
	default:
		return fmt.Errorf("unknown case catalog command: %s", args[0])
	}
}

func runCaseCatalogUpsert(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case catalog "+caseCatalogCommandUpsert, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	profileID := flags.String("profile", "", "Profile id to use when the Store has no profile catalog yet")
	caseID := flags.String("case", "", "API case id")
	nodeID := flags.String("node", "", "Interface node id")
	displayName := flags.String("display-name", "", "API case display name")
	description := flags.String("description", "", "API case description")
	requestTemplateID := flags.String("request-template", "", "Request template id")
	caseType := flags.String("case-type", "", "Case type metadata")
	scenario := flags.String("scenario", "", "Scenario metadata")
	priority := flags.String("priority", "", "Case priority metadata")
	owner := flags.String("owner", "", "Case owner metadata")
	status := flags.String("status", "", "Case status; new cases default to active")
	sortOrder := flags.Int("sort-order", 0, "Case sort order")
	casePath := flags.String("case-path", "", "Runnable case file path")
	sourceKind := flags.String("source-kind", "", "External source kind")
	sourcePath := flags.String("source-path", "", "External source path")
	executorID := flags.String("executor", "", "External executor id")
	baseURL := flags.String("base-url", "", "Default base URL for catalog case runs")
	evidenceDir := flags.String("evidence-dir", "", "Default Evidence directory")
	timeoutSeconds := flags.Int("timeout-seconds", 0, "Default request timeout in seconds")
	renderMode := flags.String("render-mode", "", "Case render mode, such as template_patch")
	patchJSON := flags.String("patch-json", "", "JSON Patch document for template patch cases")
	expectedJSON := flags.String("expected-json", "", "Expected result JSON metadata")
	defaultOverridesJSON := flags.String("default-overrides-json", "", "Default request overrides JSON object")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	tags := stringListFlag{}
	defaultOverrides := mapFlag{}
	flags.Var(&tags, "tag", "Case tag metadata; repeat for multiple tags")
	flags.Var(&defaultOverrides, "default-override", "Default override as key=value; repeat for multiple values")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected case catalog arguments: %s", strings.Join(flags.Args(), " "))
	}
	if strings.TrimSpace(*caseID) == "" {
		return errors.New("--case is required")
	}
	if strings.TrimSpace(*nodeID) == "" {
		return errors.New("--node is required")
	}
	if *sortOrder < 0 || *timeoutSeconds < 0 {
		return errors.New("--sort-order and --timeout-seconds must be non-negative")
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
	var report caseCatalogUpsertReport
	err = withProfileCatalogWriteLock(storeDSN, func() error {
		var upsertErr error
		report, upsertErr = upsertCaseCatalogCase(ctx, runtime, caseCatalogUpsertOptions{
			ProfileID:            *profileID,
			CaseID:               *caseID,
			NodeID:               *nodeID,
			DisplayName:          *displayName,
			Description:          *description,
			RequestTemplateID:    *requestTemplateID,
			CaseType:             *caseType,
			Scenario:             *scenario,
			Tags:                 tags.Values(),
			Priority:             *priority,
			Owner:                *owner,
			Status:               *status,
			SortOrder:            *sortOrder,
			CasePath:             *casePath,
			SourceKind:           *sourceKind,
			SourcePath:           *sourcePath,
			ExecutorID:           *executorID,
			BaseURL:              *baseURL,
			EvidenceDir:          *evidenceDir,
			TimeoutSeconds:       *timeoutSeconds,
			RenderMode:           *renderMode,
			PatchJSON:            *patchJSON,
			ExpectedJSON:         *expectedJSON,
			DefaultOverrides:     defaultOverrides.Values(),
			DefaultOverridesJSON: *defaultOverridesJSON,
			PassedFlags:          parsedFlagNames(flags),
		})
		return upsertErr
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printCaseCatalogUpsertReport(report)
	return nil
}

func upsertCaseCatalogCase(ctx context.Context, runtime store.Store, options caseCatalogUpsertOptions) (caseCatalogUpsertReport, error) {
	catalog, err := loadMutableProfileCatalog(ctx, runtime, options.ProfileID)
	if err != nil {
		return caseCatalogUpsertReport{}, err
	}
	beforeCounts := profileImportCountsFromCatalog(catalog)
	apiCase, exists := findCatalogAPICase(catalog.APICases, options.CaseID)
	apiCase.ID = strings.TrimSpace(options.CaseID)
	apiCase.NodeID = strings.TrimSpace(options.NodeID)
	applyCaseCatalogMetadataFields(&apiCase, catalog.APICases, options, exists)
	if err := applyCaseCatalogJSONFields(&apiCase, options); err != nil {
		return caseCatalogUpsertReport{}, err
	}
	catalog.APICases = upsertCatalogAPICase(catalog.APICases, apiCase)
	catalog.IndexedAt = time.Now().UTC()
	if err := runtime.ReplaceProfileCatalog(ctx, catalog); err != nil {
		return caseCatalogUpsertReport{}, err
	}
	return caseCatalogUpsertReport{
		OK:        true,
		ProfileID: catalog.ProfileID,
		Created:   !exists,
		Updated:   exists,
		Case:      caseCatalogUpsertCaseRefFromCatalog(apiCase),
		Counts: workflowCatalogUpsertCounts{
			Before: beforeCounts,
			After:  profileImportCountsFromCatalog(catalog),
		},
		NextActions: []string{
			"agent-testbench case config " + caseCatalogCommandUpsert + " --case " + commandline.ShellQuote(apiCase.ID) + " --method METHOD --path PATH --json",
			"agent-testbench map import-workflows --json",
		},
	}, nil
}

func applyCaseCatalogMetadataFields(apiCase *store.CatalogAPICase, existingCases []store.CatalogAPICase, options caseCatalogUpsertOptions, exists bool) {
	if !exists || options.PassedFlags["display-name"] {
		apiCase.DisplayName = firstNonEmpty(strings.TrimSpace(options.DisplayName), apiCase.ID)
	}
	if options.PassedFlags["description"] {
		apiCase.Description = strings.TrimSpace(options.Description)
	}
	if options.PassedFlags["request-template"] {
		apiCase.RequestTemplateID = strings.TrimSpace(options.RequestTemplateID)
	}
	if options.PassedFlags["case-type"] {
		apiCase.CaseType = strings.TrimSpace(options.CaseType)
	}
	if options.PassedFlags["scenario"] {
		apiCase.Scenario = strings.TrimSpace(options.Scenario)
	}
	if options.PassedFlags["tag"] {
		apiCase.Tags = options.Tags
	}
	if options.PassedFlags["priority"] {
		apiCase.Priority = strings.TrimSpace(options.Priority)
	}
	if options.PassedFlags["owner"] {
		apiCase.Owner = strings.TrimSpace(options.Owner)
	}
	if options.PassedFlags["status"] {
		apiCase.Status = strings.TrimSpace(options.Status)
	} else if !exists {
		apiCase.Status = "active"
	}
	if options.PassedFlags["sort-order"] {
		apiCase.SortOrder = options.SortOrder
	} else if !exists {
		apiCase.SortOrder = nextCatalogAPICaseSortOrder(existingCases)
	}
	applyOptionalCaseCatalogRuntimeFields(apiCase, options)
}

func applyCaseCatalogJSONFields(apiCase *store.CatalogAPICase, options caseCatalogUpsertOptions) error {
	if options.PassedFlags["patch-json"] {
		patchJSON, err := compactOptionalJSONString("patch-json", options.PatchJSON)
		if err != nil {
			return err
		}
		apiCase.PatchJSON = patchJSON
	}
	if options.PassedFlags["expected-json"] {
		expectedJSON, err := compactOptionalJSONString("expected-json", options.ExpectedJSON)
		if err != nil {
			return err
		}
		apiCase.ExpectedJSON = expectedJSON
	}
	defaults, hasDefaults, err := parseDefaultOverrideOptions(options.DefaultOverrides, options.DefaultOverridesJSON)
	if err != nil {
		return err
	}
	if hasDefaults {
		defaults = mergeCatalogDefaultOverrides(apiCase.DefaultOverridesJSON, defaults)
		apiCase.DefaultOverridesJSON = compactJSONObject(defaults)
	}
	return nil
}

func applyOptionalCaseCatalogRuntimeFields(apiCase *store.CatalogAPICase, options caseCatalogUpsertOptions) {
	if options.PassedFlags["case-path"] {
		apiCase.CasePath = strings.TrimSpace(options.CasePath)
	}
	if options.PassedFlags["source-kind"] {
		apiCase.SourceKind = strings.TrimSpace(options.SourceKind)
	}
	if options.PassedFlags["source-path"] {
		apiCase.SourcePath = strings.TrimSpace(options.SourcePath)
	}
	if options.PassedFlags["executor"] {
		apiCase.ExecutorID = strings.TrimSpace(options.ExecutorID)
	}
	if options.PassedFlags["base-url"] {
		apiCase.BaseURL = strings.TrimSpace(options.BaseURL)
	}
	if options.PassedFlags["evidence-dir"] {
		apiCase.EvidenceDir = strings.TrimSpace(options.EvidenceDir)
	}
	if options.PassedFlags["timeout-seconds"] {
		apiCase.TimeoutSeconds = options.TimeoutSeconds
	}
	if options.PassedFlags["render-mode"] {
		apiCase.RenderMode = strings.TrimSpace(options.RenderMode)
	}
}

func compactOptionalJSONString(name string, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return "", fmt.Errorf("decode --%s: %w", name, err)
	}
	return mustCompactJSON(value), nil
}

func nextCatalogAPICaseSortOrder(items []store.CatalogAPICase) int {
	maxOrder := 0
	for _, item := range items {
		if item.SortOrder > maxOrder {
			maxOrder = item.SortOrder
		}
	}
	return maxOrder + 1
}

func caseCatalogUpsertCaseRefFromCatalog(item store.CatalogAPICase) caseCatalogUpsertCaseRef {
	return caseCatalogUpsertCaseRef{
		ID:                item.ID,
		DisplayName:       item.DisplayName,
		NodeID:            item.NodeID,
		RequestTemplateID: item.RequestTemplateID,
		CaseType:          item.CaseType,
		RenderMode:        item.RenderMode,
		Status:            item.Status,
		DefaultOverrides:  jsonObjectString(item.DefaultOverridesJSON),
	}
}

func printCaseCatalogUpsertReport(report caseCatalogUpsertReport) {
	fmt.Printf("Case Catalog Upsert: %s\n", report.Case.ID)
	fmt.Printf("Profile: %s\n", report.ProfileID)
	fmt.Printf("Node: %s\n", report.Case.NodeID)
	fmt.Printf("Created: %t\n", report.Created)
	fmt.Printf("Updated: %t\n", report.Updated)
	fmt.Printf("API Cases: %d -> %d\n", report.Counts.Before.APICases, report.Counts.After.APICases)
	for _, action := range report.NextActions {
		fmt.Printf("Next: %s\n", action)
	}
}
