package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"agent-testbench/internal/domain/commandline"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func runWorkflow(args []string) error {
	if len(args) == 0 {
		return errors.New("missing workflow command")
	}
	switch args[0] {
	case "discover":
		return runWorkflowDiscover(context.Background(), args[1:])
	case "register", "upsert":
		return runWorkflowRegister(context.Background(), args[1:])
	case "binding":
		return runWorkflowBinding(context.Background(), args[1:])
	case "plan":
		return runWorkflowPlan(args[1:])
	case "audit":
		return runWorkflowAudit(context.Background(), args[1:])
	case "runs":
		return runWorkflowRuns(context.Background(), args[1:])
	case "run":
		return runWorkflowRun(context.Background(), args[1:])
	case "step":
		return runWorkflowStep(context.Background(), args[1:])
	case "latest-step":
		return runWorkflowLatestStep(context.Background(), args[1:])
	case cliCommandTask:
		return runWorkflowTask(context.Background(), args[1:])
	case "gate":
		return runWorkflowGate(context.Background(), args[1:])
	case "report":
		return runWorkflowReport(context.Background(), args[1:])
	case "acceptance":
		return runWorkflowAcceptance(context.Background(), args[1:])
	default:
		return fmt.Errorf("unknown workflow command: %s", args[0])
	}
}

func runWorkflowAcceptance(ctx context.Context, args []string) error {
	return runWorkflowAcceptanceCommand(ctx, args)
}

func runWorkflowStep(ctx context.Context, args []string) error {
	return runWorkflowStepLookup(ctx, args, workflowStepLookupOptions{
		Command:       "workflow step",
		ScopeFlag:     "run",
		ScopeHelp:     "Workflow run id",
		RequiredError: "--run and --step are required",
		Lookup:        controlplane.WorkflowStepRunPayload,
	})
}

func runWorkflowLatestStep(ctx context.Context, args []string) error {
	return runWorkflowStepLookup(ctx, args, workflowStepLookupOptions{
		Command:       "workflow latest-step",
		ScopeFlag:     "workflow",
		ScopeHelp:     "Workflow id",
		RequiredError: "--workflow and --step are required",
		Lookup:        controlplane.LatestWorkflowStepRunPayload,
	})
}

type workflowStepLookupOptions struct {
	Command       string
	ScopeFlag     string
	ScopeHelp     string
	RequiredError string
	Lookup        func(context.Context, store.Store, string, string) (map[string]any, bool, error)
}

func runWorkflowStepLookup(ctx context.Context, args []string, options workflowStepLookupOptions) error {
	flags := flag.NewFlagSet(options.Command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	scopeID := flags.String(options.ScopeFlag, "", options.ScopeHelp)
	stepID := flags.String("step", "", "Workflow step id")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*scopeID) == "" || strings.TrimSpace(*stepID) == "" {
		return errors.New(options.RequiredError)
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	payload, ok, err := options.Lookup(ctx, runtime, *scopeID, *stepID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("workflow run step not found: %s %s", strings.TrimSpace(*scopeID), strings.TrimSpace(*stepID))
	}
	if *jsonOutput {
		return writeIndentedJSON(payload)
	}
	printWorkflowStep(payload)
	return nil
}

func printWorkflowStep(payload map[string]any) {
	run := mapFromReportAny(payload["run"])
	summary := mapFromReportAny(payload["summary"])
	fmt.Println("Workflow Step")
	fmt.Printf("Run: %s\n", valueString(run["id"]))
	fmt.Printf("Workflow: %s\n", valueString(run["workflowId"]))
	steps := listFromReportAny(summary["steps"])
	if len(steps) > 0 {
		step := mapFromReportAny(steps[0])
		fmt.Printf("Step: %s\n", valueString(step["stepId"]))
		fmt.Printf("Case: %s\n", valueString(step["caseId"]))
		fmt.Printf("Status: %s\n", valueString(step["status"]))
	}
}

func runWorkflowRuns(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("workflow runs", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	payload, err := controlplane.WorkflowRunsPayload(ctx, runtime)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(payload)
	}
	printWorkflowRuns(payload)
	return nil
}

func runWorkflowRun(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("workflow run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	runID := flags.String("run", "", "Workflow run id")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*runID) == "" {
		return errors.New("--run is required")
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	payload, ok, err := controlplane.WorkflowRunPayload(ctx, runtime, *runID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("workflow run not found: %s", strings.TrimSpace(*runID))
	}
	if *jsonOutput {
		return writeIndentedJSON(payload)
	}
	printWorkflowRun(payload)
	return nil
}

func printWorkflowRuns(payload map[string]any) {
	rawItems := listFromReportAny(payload["workflowRuns"])
	items := make([]map[string]any, 0, len(rawItems))
	for _, raw := range rawItems {
		if item := mapFromReportAny(raw); len(item) > 0 {
			items = append(items, item)
		}
	}
	fmt.Println("Workflow Runs")
	fmt.Printf("Total: %d\n", len(items))
	for _, item := range items {
		fmt.Printf("- %s [%s] %s steps=%s\n", valueString(item["id"]), valueString(item["status"]), valueString(item["workflowId"]), valueString(item["stepCount"]))
	}
}

func printWorkflowRun(payload map[string]any) {
	run := mapFromReportAny(payload["run"])
	summary := mapFromReportAny(payload["summary"])
	fmt.Println("Workflow Run")
	fmt.Printf("Run: %s\n", valueString(run["id"]))
	fmt.Printf("Workflow: %s\n", valueString(run["workflowId"]))
	fmt.Printf("Status: %s\n", valueString(run["status"]))
	if count := valueString(run["stepCount"]); count != "" {
		fmt.Printf("Steps: %s\n", count)
	} else if steps := listFromReportAny(summary["steps"]); len(steps) > 0 {
		fmt.Printf("Steps: %d\n", len(steps))
	}
}

type workflowGateReport struct {
	OK              bool               `json:"ok"`
	RunID           string             `json:"runId"`
	WorkflowID      string             `json:"workflowId,omitempty"`
	Status          string             `json:"status"`
	Counts          workflowGateCounts `json:"counts"`
	Gates           workflowGateGates  `json:"gates"`
	FailedSteps     []workflowGateStep `json:"failedSteps"`
	MissingEvidence []workflowGateStep `json:"missingEvidence"`
	NextActions     []string           `json:"nextActions"`
	Warnings        []string           `json:"warnings"`
}

type workflowGateCounts struct {
	Steps            int `json:"steps"`
	PassedSteps      int `json:"passedSteps"`
	FailedSteps      int `json:"failedSteps"`
	OtherSteps       int `json:"otherSteps"`
	CaseRuns         int `json:"caseRuns"`
	EvidenceComplete int `json:"evidenceComplete"`
}

type workflowGateGates struct {
	RunPassed        bool `json:"runPassed"`
	StepsPresent     bool `json:"stepsPresent"`
	StepsPassed      bool `json:"stepsPassed"`
	EvidenceComplete bool `json:"evidenceComplete"`
}

type workflowGateStep struct {
	StepID        string `json:"stepId,omitempty"`
	Kind          string `json:"kind,omitempty"`
	CaseID        string `json:"caseId,omitempty"`
	CaseRunID     string `json:"caseRunId,omitempty"`
	TaskRunID     string `json:"taskRunId,omitempty"`
	Status        string `json:"status,omitempty"`
	EvidenceCount int    `json:"evidenceCount"`
}

type workflowGateOptions struct {
	RunID           string
	RequirePassed   bool
	RequireSteps    bool
	RequireEvidence bool
}

func runWorkflowGate(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("workflow gate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	runID := flags.String("run", "", "Workflow run id")
	requirePassed := flags.Bool("require-passed", false, "Fail unless the workflow run status is passed")
	requireSteps := flags.Bool("require-steps", false, "Fail unless workflow steps exist and every step passed")
	requireEvidence := flags.Bool("require-evidence", false, "Fail unless every step case run has indexed Evidence")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*runID) == "" {
		return errors.New("--run is required")
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	report, err := buildWorkflowGateReport(ctx, runtime, workflowGateOptions{
		RunID:           *runID,
		RequirePassed:   *requirePassed,
		RequireSteps:    *requireSteps,
		RequireEvidence: *requireEvidence,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		if err := writeIndentedJSON(report); err != nil {
			return err
		}
	} else {
		printWorkflowGate(report)
	}
	if !report.OK {
		return errors.New("workflow gate failed")
	}
	return nil
}

func buildWorkflowGateReport(ctx context.Context, runtime store.Store, options workflowGateOptions) (workflowGateReport, error) {
	run, err := runtime.GetRun(ctx, strings.TrimSpace(options.RunID))
	if err != nil {
		return workflowGateReport{}, err
	}
	caseRuns, err := runtime.ListAPICaseRuns(ctx, run.ID)
	if err != nil {
		return workflowGateReport{}, err
	}
	steps := workflowGateSteps(run.SummaryJSON)
	evidence, err := workflowGateEvidenceRecords(ctx, runtime, run.ID, caseRuns, workflowGateSummaryCaseRunIDs(steps))
	if err != nil {
		return workflowGateReport{}, err
	}
	caseRunIndex := indexWorkflowGateCaseRuns(caseRuns)
	evidenceCountByCaseRun := indexWorkflowGateEvidence(evidence)
	evidenceCountByStep := indexWorkflowGateEvidenceByStep(evidence)

	report := workflowGateReport{
		RunID:           run.ID,
		WorkflowID:      run.WorkflowID,
		Status:          run.Status,
		FailedSteps:     []workflowGateStep{},
		MissingEvidence: []workflowGateStep{},
		NextActions:     []string{},
		Warnings:        []string{},
	}
	report.Counts.Steps = len(steps)
	report.Counts.CaseRuns = len(caseRuns)
	for _, rawStep := range steps {
		step := workflowGateStepFrom(rawStep, caseRunIndex.byID, caseRunIndex.byStep, caseRunIndex.byCase, evidenceCountByCaseRun, evidenceCountByStep)
		addWorkflowGateStep(&report, step)
	}
	report.Gates = workflowGateGates{
		RunPassed:        strings.EqualFold(run.Status, store.StatusPassed),
		StepsPresent:     report.Counts.Steps > 0,
		StepsPassed:      report.Counts.Steps > 0 && report.Counts.FailedSteps == 0 && report.Counts.OtherSteps == 0,
		EvidenceComplete: report.Counts.Steps > 0 && len(report.MissingEvidence) == 0,
	}
	report.OK = (!options.RequirePassed || report.Gates.RunPassed) &&
		(!options.RequireSteps || (report.Gates.StepsPresent && report.Gates.StepsPassed)) &&
		(!options.RequireEvidence || report.Gates.EvidenceComplete)
	report.NextActions = workflowGateNextActions(report, options)
	return report, nil
}

func workflowGateEvidenceRecords(ctx context.Context, runtime store.Store, runID string, caseRuns []store.APICaseRun, summaryCaseRunIDs []string) ([]store.EvidenceRecord, error) {
	out, err := runtime.ListEvidence(ctx, runID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, row := range out {
		seen[row.ID] = true
	}
	caseRunIDs := make([]string, 0, len(caseRuns)+len(summaryCaseRunIDs))
	for _, caseRun := range caseRuns {
		caseRunIDs = append(caseRunIDs, caseRun.ID)
	}
	caseRunIDs = append(caseRunIDs, summaryCaseRunIDs...)
	for _, caseRunID := range compactUniqueStringListPreserveOrder(caseRunIDs) {
		if strings.TrimSpace(caseRunID) == "" || strings.TrimSpace(caseRunID) == runID {
			continue
		}
		rows, err := runtime.ListEvidence(ctx, caseRunID)
		if err != nil {
			return nil, fmt.Errorf("list case-run evidence %s: %w", caseRunID, err)
		}
		for _, row := range rows {
			if seen[row.ID] {
				continue
			}
			seen[row.ID] = true
			out = append(out, row)
		}
	}
	return out, nil
}

type workflowGateCaseRunIndex struct {
	byID   map[string]store.APICaseRun
	byCase map[string][]store.APICaseRun
	byStep map[string][]store.APICaseRun
}

func indexWorkflowGateCaseRuns(caseRuns []store.APICaseRun) workflowGateCaseRunIndex {
	index := workflowGateCaseRunIndex{
		byID:   map[string]store.APICaseRun{},
		byCase: map[string][]store.APICaseRun{},
		byStep: map[string][]store.APICaseRun{},
	}
	for _, item := range caseRuns {
		index.byID[item.ID] = item
		index.byCase[item.CaseID] = append(index.byCase[item.CaseID], item)
		if stepID := apiCaseRunStepID(item); stepID != "" {
			index.byStep[stepID] = append(index.byStep[stepID], item)
		}
	}
	return index
}

func indexWorkflowGateEvidence(evidence []store.EvidenceRecord) map[string]int {
	out := map[string]int{}
	for _, record := range evidence {
		if strings.TrimSpace(record.CaseRunID) != "" {
			out[record.CaseRunID]++
		}
	}
	return out
}

func indexWorkflowGateEvidenceByStep(evidence []store.EvidenceRecord) map[string]int {
	out := map[string]int{}
	for _, record := range evidence {
		if strings.TrimSpace(record.StepID) != "" {
			out[record.StepID]++
		}
	}
	return out
}

func addWorkflowGateStep(report *workflowGateReport, step workflowGateStep) {
	switch {
	case strings.EqualFold(step.Status, store.StatusPassed):
		report.Counts.PassedSteps++
	case strings.EqualFold(step.Status, store.StatusFailed):
		report.Counts.FailedSteps++
		report.FailedSteps = append(report.FailedSteps, step)
	default:
		report.Counts.OtherSteps++
		report.FailedSteps = append(report.FailedSteps, step)
	}
	if step.EvidenceCount > 0 {
		report.Counts.EvidenceComplete++
		return
	}
	report.MissingEvidence = append(report.MissingEvidence, step)
}

func workflowGateSteps(summaryJSON string) []map[string]any {
	summary := rawJSONObject(summaryJSON)
	steps := listFromReportAny(summary["steps"])
	out := make([]map[string]any, 0, len(steps))
	for _, raw := range steps {
		step := mapFromReportAny(raw)
		if len(step) > 0 {
			out = append(out, step)
		}
	}
	return out
}

func workflowGateSummaryCaseRunIDs(steps []map[string]any) []string {
	out := make([]string, 0, len(steps))
	for _, step := range steps {
		out = append(out, valueString(step["caseRunId"]))
	}
	return compactUniqueStringListPreserveOrder(out)
}

func workflowGateStepFrom(step map[string]any, caseRunByID map[string]store.APICaseRun, caseRunsByStep map[string][]store.APICaseRun, caseRunsByCase map[string][]store.APICaseRun, evidenceCountByCaseRun map[string]int, evidenceCountByStep map[string]int) workflowGateStep {
	out := workflowGateStep{
		StepID:    firstNonEmpty(valueString(step["stepId"]), valueString(step["id"])),
		Kind:      valueString(step["kind"]),
		CaseID:    valueString(step["caseId"]),
		CaseRunID: valueString(step["caseRunId"]),
		TaskRunID: valueString(step["taskRunId"]),
		Status:    valueString(step["status"]),
	}
	if out.CaseRunID != "" {
		if item, ok := caseRunByID[out.CaseRunID]; ok {
			out.CaseID = firstNonEmpty(out.CaseID, item.CaseID)
			out.Status = firstNonEmpty(out.Status, item.Status)
		}
	}
	if out.CaseRunID == "" && out.StepID != "" {
		if items := caseRunsByStep[out.StepID]; len(items) == 1 {
			item := items[0]
			out.CaseID = firstNonEmpty(out.CaseID, item.CaseID)
			out.CaseRunID = item.ID
			out.Status = firstNonEmpty(out.Status, item.Status)
		}
	}
	if out.CaseRunID == "" && out.CaseID != "" {
		if items := caseRunsByCase[out.CaseID]; len(items) == 1 {
			item := items[0]
			out.CaseRunID = item.ID
			out.Status = firstNonEmpty(out.Status, item.Status)
		}
	}
	if out.Status == "" {
		out.Status = "unknown"
	}
	out.EvidenceCount = evidenceCountByCaseRun[out.CaseRunID]
	if out.EvidenceCount == 0 && workflowGateStepAllowsStepEvidence(out) {
		out.EvidenceCount = evidenceCountByStep[out.StepID]
	}
	return out
}

func workflowGateStepAllowsStepEvidence(step workflowGateStep) bool {
	return step.StepID != "" && (step.Kind == cliCommandTask || step.TaskRunID != "")
}

func apiCaseRunStepID(item store.APICaseRun) string {
	return strings.TrimSpace(valueString(jsonObjectString(item.RequestSummaryJSON)["stepId"]))
}

func workflowGateNextActions(report workflowGateReport, options workflowGateOptions) []string {
	actions := []string{}
	if !report.Gates.StepsPresent {
		return []string{"agent-testbench workflow run --run " + commandline.ShellQuote(report.RunID) + " --json"}
	}
	for index, item := range report.FailedSteps {
		if index >= 3 {
			break
		}
		if item.StepID != "" {
			actions = append(actions, "agent-testbench workflow step --run "+commandline.ShellQuote(report.RunID)+" --step "+commandline.ShellQuote(item.StepID)+" --json")
		}
		if item.CaseRunID != "" {
			actions = append(actions, "agent-testbench case diagnose --case-run "+commandline.ShellQuote(item.CaseRunID)+" --json")
		}
	}
	if options.RequireEvidence {
		for index, item := range report.MissingEvidence {
			if index >= 3 {
				break
			}
			if item.CaseRunID != "" {
				actions = append(actions, "agent-testbench case evidence --case-run "+commandline.ShellQuote(item.CaseRunID)+" --json")
				continue
			}
			if item.StepID != "" {
				actions = append(actions, "agent-testbench workflow step --run "+commandline.ShellQuote(report.RunID)+" --step "+commandline.ShellQuote(item.StepID)+" --json")
			}
		}
	}
	if len(actions) == 0 {
		actions = append(actions, "Workflow gate passed; no action needed")
	}
	return actions
}

func printWorkflowGate(report workflowGateReport) {
	fmt.Println("Workflow Gate")
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Run: %s\n", report.RunID)
	fmt.Printf("Workflow: %s\n", report.WorkflowID)
	fmt.Printf("Status: %s\n", report.Status)
	fmt.Printf("Steps: %d Passed: %d Failed: %d Other: %d CaseRuns: %d EvidenceComplete: %d\n", report.Counts.Steps, report.Counts.PassedSteps, report.Counts.FailedSteps, report.Counts.OtherSteps, report.Counts.CaseRuns, report.Counts.EvidenceComplete)
	fmt.Printf("Gates: runPassed=%t stepsPresent=%t stepsPassed=%t evidenceComplete=%t\n", report.Gates.RunPassed, report.Gates.StepsPresent, report.Gates.StepsPassed, report.Gates.EvidenceComplete)
	for _, item := range report.FailedSteps {
		fmt.Printf("Failed Step: %s %s %s %s\n", item.StepID, item.CaseID, item.CaseRunID, item.Status)
	}
	for _, item := range report.MissingEvidence {
		fmt.Printf("Missing Evidence: %s %s %s\n", item.StepID, item.CaseID, item.CaseRunID)
	}
	for _, action := range report.NextActions {
		fmt.Printf("Next: %s\n", action)
	}
}
