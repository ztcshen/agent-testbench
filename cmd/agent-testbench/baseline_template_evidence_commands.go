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

	"agent-testbench/internal/runner/evidence"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

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
