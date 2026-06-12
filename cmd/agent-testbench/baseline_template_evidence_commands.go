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

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/runner/evidence"
	"agent-testbench/internal/runner/requesttemplate"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func runBaseline(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing baseline command")
	}
	switch args[0] {
	case "get":
		return runBaselineGet(ctx, args[1:])
	case "set":
		return runBaselineSet(ctx, args[1:])
	default:
		return fmt.Errorf("unknown baseline command: %s", args[0])
	}
}

func runBaselineGet(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("baseline get", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	profileID := flags.String("profile", "", "Profile id")
	subjectID := flags.String("subject", "", "Subject id")
	if err := flags.Parse(args); err != nil {
		return err
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	s, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return err
	}
	defer closeCLIStore(s)

	gate, err := s.GetBaselineGate(ctx, *profileID, *subjectID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("baseline gate not found: %s %s", *profileID, *subjectID)
		}
		return err
	}
	printBaselineGate(gate)
	return nil
}

func runBaselineSet(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("baseline set", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	profileID := flags.String("profile", "", "Profile id")
	subjectID := flags.String("subject", "", "Subject id")
	status := flags.String("status", "", "Gate status")
	required := flags.Bool("required", false, "Mark the gate as required")
	summaryJSON := flags.String("summary-json", "{}", "Gate summary JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	s, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return err
	}
	defer closeCLIStore(s)

	now := time.Now().UTC()
	gate, err := s.UpsertBaselineGate(ctx, store.BaselineGate{
		ProfileID:   *profileID,
		SubjectID:   *subjectID,
		Status:      *status,
		Required:    *required,
		SummaryJSON: *summaryJSON,
		CheckedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return err
	}
	printBaselineGate(gate)
	return nil
}

func printBaselineGate(gate store.BaselineGate) {
	fmt.Printf("Baseline Gate: %s %s\n", gate.ProfileID, gate.SubjectID)
	fmt.Printf("Status: %s\n", gate.Status)
	fmt.Printf("Required: %t\n", gate.Required)
}

func runTemplate(args []string) error {
	if len(args) == 0 {
		return errors.New("missing template command")
	}
	switch args[0] {
	case "render":
		return runTemplateRender(args[1:])
	default:
		return fmt.Errorf("unknown template command: %s", args[0])
	}
}

func runTemplateRender(args []string) error {
	flags := flag.NewFlagSet("template render", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	templateID := flags.String("template", "", "Request template id")
	fixtureID := flags.String("fixture", "", "Fixture id")
	if err := flags.Parse(args); err != nil {
		return err
	}
	bundle, cleanup, err := loadTemplateRenderBundle(context.Background(), *profilePath, *profileHome, *storeRef, *storeURL, *templateID)
	if err != nil {
		return err
	}
	defer cleanup()
	rendered, err := requesttemplate.Render(bundle, requesttemplate.Options{
		TemplateID: *templateID,
		FixtureID:  *fixtureID,
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(rendered)
}

func loadTemplateRenderBundle(ctx context.Context, profileRef string, profileHomeRef string, storeRef string, legacyStoreURL string, templateID string) (profile.Bundle, func(), error) {
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(storeRef, legacyStoreURL)
	if err != nil {
		return profile.Bundle{}, func() {}, err
	}
	if strings.TrimSpace(profileRef) != "" {
		resolvedProfile, err := resolveProfileReference(profileRef, profileHomeRef)
		if err != nil {
			return profile.Bundle{}, func() {}, err
		}
		bundle, err := profile.Load(resolvedProfile)
		return bundle, func() {}, err
	}
	runtime, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return profile.Bundle{}, func() {}, err
	}
	bundle, err := serveBundle(ctx, runtime)
	if err != nil {
		closeCLIStore(runtime)
		return profile.Bundle{}, func() {}, err
	}
	if templateNeedsPublishedProfile(bundle, templateID) {
		if catalogIndex, err := runtime.GetProfileCatalogIndex(ctx); err == nil && strings.TrimSpace(catalogIndex.ProfileID) != "" {
			if profileIndex, err := runtime.GetProfileIndex(ctx, catalogIndex.ProfileID); err == nil && strings.TrimSpace(profileIndex.BundlePath) != "" {
				if pathBundle, err := profile.Load(profileIndex.BundlePath); err == nil {
					bundle = pathBundle
				}
			}
		}
	}
	return bundle, cleanupCLIStore(runtime), nil
}

func templateNeedsPublishedProfile(bundle profile.Bundle, templateID string) bool {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return false
	}
	for _, item := range bundle.RequestTemplates {
		if item.ID != templateID {
			continue
		}
		return strings.TrimSpace(item.Method) == "" || strings.TrimSpace(item.Path) == ""
	}
	return false
}

func runEvidence(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing evidence command")
	}
	switch args[0] {
	case "import":
		return runEvidenceImport(ctx, args[1:])
	case "list":
		return runEvidenceList(ctx, args[1:])
	case "tasks":
		return runEvidenceTasks(ctx, args[1:])
	default:
		return fmt.Errorf("unknown evidence command: %s", args[0])
	}
}

func runEvidenceList(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("evidence list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	runID := flags.String("run", "", "Run id")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	s, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()

	report, err := controlplane.EvidenceList(ctx, s, *runID)
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	printEvidenceList(report)
	return nil
}

func printEvidenceList(report controlplane.EvidenceListReport) {
	for _, run := range report.Runs {
		fmt.Printf("Run: %s\n", run.ID)
		fmt.Printf("Profile: %s\n", run.ProfileID)
		fmt.Printf("Status: %s\n", run.Status)
		for _, caseRun := range run.APICaseRuns {
			fmt.Printf("Case Run: %s\n", caseRun.ID)
			fmt.Printf("Case: %s\n", caseRun.CaseID)
			fmt.Printf("Case Status: %s\n", caseRun.Status)
		}
		for _, record := range run.EvidenceRecords {
			fmt.Printf("Evidence: %s %s\n", record.Kind, record.URI)
			if record.StepID != "" {
				fmt.Printf("  Step: %s\n", record.StepID)
			}
		}
	}
}

type evidenceTaskReport struct {
	OK     bool               `json:"ok"`
	RunID  string             `json:"runId"`
	StepID string             `json:"stepId,omitempty"`
	CaseID string             `json:"caseId,omitempty"`
	Kind   string             `json:"kind,omitempty"`
	Status string             `json:"status,omitempty"`
	Counts evidenceTaskCounts `json:"counts"`
	Tasks  []evidenceTaskItem `json:"tasks"`
}

type evidenceTaskCounts struct {
	Total      int   `json:"total"`
	Passed     int   `json:"passed"`
	Failed     int   `json:"failed"`
	Running    int   `json:"running"`
	Skipped    int   `json:"skipped"`
	DurationMs int64 `json:"durationMs"`
}

type evidenceTaskItem struct {
	ID            string    `json:"id"`
	RunID         string    `json:"runId"`
	WorkflowID    string    `json:"workflowId,omitempty"`
	StepID        string    `json:"stepId,omitempty"`
	CaseID        string    `json:"caseId,omitempty"`
	Kind          string    `json:"kind"`
	Status        string    `json:"status"`
	Outcome       string    `json:"outcome"`
	Reason        string    `json:"reason"`
	DisplayStatus string    `json:"displayStatus"`
	StartedAt     time.Time `json:"startedAt"`
	FinishedAt    time.Time `json:"finishedAt"`
	DurationMs    int64     `json:"durationMs"`
	Error         string    `json:"error,omitempty"`
	SummaryJSON   string    `json:"summaryJson,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}

type evidenceTaskFilter struct {
	RunID  string
	StepID string
	CaseID string
	Kind   string
	Status string
}

func runEvidenceTasks(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("evidence tasks", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	runID := flags.String("run", "", "Run id")
	stepID := flags.String("step", "", "Workflow step id")
	caseID := flags.String("case", "", "API case id")
	kind := flags.String("kind", "", "Post-process task kind")
	status := flags.String("status", "", "Post-process task status")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*runID) == "" {
		return errors.New("--run is required")
	}
	s, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	report, err := evidenceTasks(ctx, s, evidenceTaskFilter{
		RunID:  *runID,
		StepID: *stepID,
		CaseID: *caseID,
		Kind:   *kind,
		Status: *status,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printEvidenceTasks(report)
	return nil
}

func evidenceTasks(ctx context.Context, s store.Store, filter evidenceTaskFilter) (evidenceTaskReport, error) {
	filter.RunID = strings.TrimSpace(filter.RunID)
	if filter.RunID == "" {
		return evidenceTaskReport{}, errors.New("run id is required")
	}
	if _, err := s.GetRun(ctx, filter.RunID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return evidenceTaskReport{}, fmt.Errorf("run not found: %s", filter.RunID)
		}
		return evidenceTaskReport{}, err
	}
	rows, err := s.ListPostProcessTasks(ctx, filter.RunID)
	if err != nil {
		return evidenceTaskReport{}, err
	}
	report := evidenceTaskReport{
		OK:     true,
		RunID:  filter.RunID,
		StepID: strings.TrimSpace(filter.StepID),
		CaseID: strings.TrimSpace(filter.CaseID),
		Kind:   strings.TrimSpace(filter.Kind),
		Status: strings.TrimSpace(filter.Status),
		Tasks:  []evidenceTaskItem{},
	}
	for _, row := range rows {
		if !postProcessTaskMatches(row, filter) {
			continue
		}
		readable := controlplane.PostProcessTaskReadableStatus(row)
		report.Tasks = append(report.Tasks, evidenceTaskItem{
			ID:            row.ID,
			RunID:         row.RunID,
			WorkflowID:    row.WorkflowID,
			StepID:        row.StepID,
			CaseID:        row.CaseID,
			Kind:          row.Kind,
			Status:        row.Status,
			Outcome:       readable.Outcome,
			Reason:        readable.Reason,
			DisplayStatus: readable.DisplayStatus,
			StartedAt:     row.StartedAt,
			FinishedAt:    row.FinishedAt,
			DurationMs:    row.DurationMs,
			Error:         row.Error,
			SummaryJSON:   row.SummaryJSON,
			CreatedAt:     row.CreatedAt,
		})
		report.Counts.Total++
		report.Counts.DurationMs += row.DurationMs
		switch row.Status {
		case store.StatusPassed:
			report.Counts.Passed++
		case store.StatusFailed:
			report.Counts.Failed++
		case store.StatusRunning:
			report.Counts.Running++
		case store.StatusSkipped:
			report.Counts.Skipped++
		}
	}
	return report, nil
}

func postProcessTaskMatches(row store.PostProcessTask, filter evidenceTaskFilter) bool {
	if filter.StepID != "" && row.StepID != filter.StepID {
		return false
	}
	if filter.CaseID != "" && row.CaseID != filter.CaseID {
		return false
	}
	if filter.Kind != "" && row.Kind != filter.Kind {
		return false
	}
	if filter.Status != "" && row.Status != filter.Status {
		return false
	}
	return true
}

func printEvidenceTasks(report evidenceTaskReport) {
	fmt.Printf("Post Process Tasks: %s\n", report.RunID)
	fmt.Printf("Total: %d Passed: %d Failed: %d Running: %d Skipped: %d Duration: %d ms\n", report.Counts.Total, report.Counts.Passed, report.Counts.Failed, report.Counts.Running, report.Counts.Skipped, report.Counts.DurationMs)
	for _, task := range report.Tasks {
		fmt.Printf("- %s %s [%s] %d ms\n", task.ID, task.Kind, task.DisplayStatus, task.DurationMs)
		if task.StepID != "" {
			fmt.Printf("  Step: %s\n", task.StepID)
		}
		if task.CaseID != "" {
			fmt.Printf("  Case: %s\n", task.CaseID)
		}
		if task.Reason != "" {
			fmt.Printf("  Reason: %s\n", task.Reason)
		}
		if task.Error != "" {
			fmt.Printf("  Error: %s\n", task.Error)
		}
	}
}

func runEvidenceImport(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("evidence import", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	from := flags.String("from", "", "Source runtime SQLite path")
	profileID := flags.String("profile", "", "Profile id")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	resolvedStoreURL, err := resolveRequiredStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	s, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return err
	}
	defer closeCLIStore(s)
	result, err := evidence.ImportLegacyRuntime(ctx, evidence.ImportOptions{
		SourcePath: *from,
		ProfileID:  *profileID,
		Store:      s,
	})
	if err != nil {
		return err
	}
	report := evidenceImportReport{
		SourcePath:      *from,
		StorePath:       resolvedStoreURL,
		ProfileID:       *profileID,
		RunCount:        result.RunCount,
		APICaseRunCount: result.APICaseRunCount,
		EvidenceCount:   result.EvidenceCount,
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	fmt.Println("Imported evidence index")
	fmt.Printf("Runs: %d\n", result.RunCount)
	fmt.Printf("API Case Runs: %d\n", result.APICaseRunCount)
	fmt.Printf("Evidence Records: %d\n", result.EvidenceCount)
	return nil
}

type evidenceImportReport struct {
	SourcePath      string `json:"sourcePath"`
	StorePath       string `json:"storePath"`
	ProfileID       string `json:"profileId"`
	RunCount        int    `json:"runCount"`
	APICaseRunCount int    `json:"apiCaseRunCount"`
	EvidenceCount   int    `json:"evidenceCount"`
}

func evidenceSummary(path string, kind string) (string, error) {
	switch kind {
	case "request":
		return requestSummaryJSON(path)
	case "response":
		return responseSummaryJSON(path)
	case "assertions":
		return assertionSummaryJSON(path)
	default:
		return "", nil
	}
}
