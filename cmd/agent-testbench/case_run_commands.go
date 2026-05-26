package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agent-testbench/internal/runner/apicase"
	"agent-testbench/internal/store"
)

func runCaseRun(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	overrides := mapFlag{}
	casePath := flags.String("case", "", "API case file path")
	caseID := flags.String("case-id", "", "API case id from the active Store catalog")
	baseURL := flags.String("base-url", "", "Base URL for live request execution")
	evidenceDir := flags.String("evidence-dir", filepath.Join(".runtime", "cases"), "Evidence output directory")
	runID := flags.String("run-id", "", "Run id")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	profileID := flags.String("profile", "default", "Profile id for store records")
	timeoutSeconds := flags.Int("timeout-seconds", 0, "Request timeout in seconds for Store catalog case execution")
	dryRun := flags.Bool("dry-run", false, "Preview the file-backed case run without sending HTTP, writing Evidence, or indexing Store records")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	flags.Var(&overrides, "override", "Request body override as key=value; repeat for multiple values")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *dryRun {
		if strings.TrimSpace(*caseID) != "" {
			return errors.New("case run --dry-run currently supports --case PATH")
		}
		if strings.TrimSpace(*casePath) == "" {
			return errors.New("case run --dry-run requires --case PATH")
		}
		plan, err := apicase.Plan(apicase.RunOptions{
			CasePath:    *casePath,
			BaseURL:     *baseURL,
			EvidenceDir: *evidenceDir,
			RunID:       *runID,
			Overrides:   overrides.Values(),
		})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return writeIndentedJSON(plan)
		}
		printCaseRunDryRun(plan)
		return nil
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*caseID) != "" {
		result, err := runStoreCatalogCase(ctx, resolvedStoreURL, *profileID, *caseID, *baseURL, *evidenceDir, *runID, *timeoutSeconds, overrides.Values())
		if err != nil {
			return err
		}
		if *jsonOutput {
			return writeIndentedJSON(result)
		}
		printStoreCatalogCaseRun(result)
		return nil
	}
	if strings.TrimSpace(*casePath) == "" {
		return errors.New("case run requires --case PATH or --case-id ID")
	}
	result, err := apicase.Run(ctx, apicase.RunOptions{
		CasePath:    *casePath,
		BaseURL:     *baseURL,
		EvidenceDir: *evidenceDir,
		RunID:       *runID,
		Overrides:   overrides.Values(),
	})
	if err != nil {
		return err
	}
	if err := indexCaseRun(ctx, resolvedStoreURL, *profileID, result); err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(result)
	}
	fmt.Printf("Case Run: %s\n", result.RunID)
	fmt.Printf("Case: %s\n", result.CaseID)
	fmt.Printf("Status: %s\n", result.Status)
	fmt.Printf("Evidence: %s\n", result.EvidencePath)
	return nil
}

func printCaseRunDryRun(plan apicase.DryRunPlan) {
	fmt.Printf("Case Run Dry Run: %s\n", plan.RunID)
	fmt.Printf("Case: %s\n", plan.CaseID)
	fmt.Printf("Request: %s %s\n", plan.Request.Method, plan.Request.Path)
	if plan.Request.URL != "" {
		fmt.Printf("URL: %s\n", plan.Request.URL)
	}
	fmt.Printf("Headers: %d\n", len(plan.Request.HeaderKeys))
	fmt.Printf("Body: %t", plan.Request.HasBody)
	if len(plan.Request.BodyKeys) > 0 {
		fmt.Printf(" keys=%s", strings.Join(plan.Request.BodyKeys, ","))
	}
	fmt.Println()
	if len(plan.Assertions.ExpectedStatusCodes) > 0 {
		fmt.Printf("Expected Status: %s\n", intListString(plan.Assertions.ExpectedStatusCodes))
	}
	if plan.Assertions.ResponseContainsCount > 0 {
		fmt.Printf("Response Contains Checks: %d\n", plan.Assertions.ResponseContainsCount)
	}
	fmt.Printf("Will Send HTTP: %t\n", plan.Effects.HTTPRequest)
	fmt.Printf("Will Write Evidence: %t\n", plan.Effects.WritesEvidence)
	fmt.Printf("Will Write Store: %t\n", plan.Effects.WritesStore)
	fmt.Printf("Planned Evidence: %s\n", plan.Effects.PlannedEvidencePath)
	for _, warning := range plan.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}

func intListString(values []int) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, ",")
}

func indexCaseRun(ctx context.Context, storeURL string, profileID string, result apicase.RunResult) error {
	s, err := openStore(ctx, storeURL)
	if err != nil {
		return err
	}
	defer s.Close()

	now := time.Now().UTC()
	startedAt := runResultTime(result.StartedAt, now)
	finishedAt := runResultTime(result.FinishedAt, now)
	if finishedAt.Before(startedAt) {
		finishedAt = startedAt
	}
	requestSummary, assertionSummary, err := apiCaseRunSummaries(result.EvidencePath)
	if err != nil {
		return err
	}
	if _, err := s.CreateRun(ctx, store.Run{
		ID:           result.RunID,
		ProfileID:    profileID,
		WorkflowID:   "",
		Status:       result.Status,
		EvidenceRoot: result.EvidencePath,
		SummaryJSON:  caseRunSummaryJSON(result),
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
		CreatedAt:    startedAt,
		UpdatedAt:    finishedAt,
	}); err != nil {
		return err
	}
	if _, err := s.RecordAPICaseRun(ctx, store.APICaseRun{
		ID:                   result.RunID + ".case",
		RunID:                result.RunID,
		CaseID:               result.CaseID,
		Status:               result.Status,
		RequestSummaryJSON:   requestSummary,
		AssertionSummaryJSON: assertionSummary,
		StartedAt:            startedAt,
		FinishedAt:           finishedAt,
		CreatedAt:            startedAt,
	}); err != nil {
		return err
	}
	for _, name := range []string{"case.json", "request.json", "response.json", "assertions.json", "summary.json"} {
		path := filepath.Join(result.EvidencePath, name)
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		summary, err := evidenceSummary(path, strings.TrimSuffix(name, ".json"))
		if err != nil {
			return err
		}
		if _, err := s.RecordEvidence(ctx, store.EvidenceRecord{
			ID:        result.RunID + "." + name,
			RunID:     result.RunID,
			CaseRunID: result.RunID + ".case",
			Kind:      strings.TrimSuffix(name, ".json"),
			URI:       path,
			MediaType: "application/json",
			Summary:   summary,
			CreatedAt: now,
		}); err != nil {
			return err
		}
	}
	return nil
}

func caseRunSummaryJSON(result apicase.RunResult) string {
	path := filepath.Join(result.EvidencePath, "summary.json")
	if raw, err := os.ReadFile(path); err == nil && json.Valid(raw) {
		return strings.TrimSpace(string(raw))
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func runResultTime(value string, defaultValue time.Time) time.Time {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return defaultValue
	}
	return parsed.UTC()
}

type requestSummary struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	HeaderCount int    `json:"headerCount"`
	HasBody     bool   `json:"hasBody"`
}

type assertionSummary struct {
	Status     string `json:"status"`
	ErrorCount int    `json:"errorCount"`
}

type responseSummary struct {
	StatusCode  int `json:"statusCode"`
	HeaderCount int `json:"headerCount"`
	BodyBytes   int `json:"bodyBytes"`
}

func apiCaseRunSummaries(evidencePath string) (string, string, error) {
	request, err := requestSummaryJSON(filepath.Join(evidencePath, "request.json"))
	if err != nil {
		return "", "", err
	}
	assertions, err := assertionSummaryJSON(filepath.Join(evidencePath, "assertions.json"))
	if err != nil {
		return "", "", err
	}
	return request, assertions, nil
}

func requestSummaryJSON(path string) (string, error) {
	var request apicase.Request
	if err := readJSONFile(path, &request); err != nil {
		return "", err
	}
	return compactJSON(requestSummary{
		Method:      strings.ToUpper(request.Method),
		Path:        request.Path,
		HeaderCount: len(request.Headers),
		HasBody:     request.Body != nil,
	})
}

func responseSummaryJSON(path string) (string, error) {
	var response apicase.ResponseEvidence
	if err := readJSONFile(path, &response); err != nil {
		return "", err
	}
	return compactJSON(responseSummary{
		StatusCode:  response.StatusCode,
		HeaderCount: len(response.Headers),
		BodyBytes:   len([]byte(response.Body)),
	})
}

func assertionSummaryJSON(path string) (string, error) {
	var assertions apicase.AssertionEvidence
	if err := readJSONFile(path, &assertions); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return compactJSON(assertionSummary{Status: "not-run"})
		}
		return "", err
	}
	return compactJSON(assertionSummary{
		Status:     assertions.Status,
		ErrorCount: len(assertions.Errors),
	})
}
