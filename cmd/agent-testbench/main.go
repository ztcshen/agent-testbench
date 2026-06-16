package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/mysql"
	"agent-testbench/internal/store/postgres"
)

const version = "0.1.0"
const interfaceNodeCommand = "interface-node"
const cliCommandTask = "task"

var buildRevision = ""

type versionCommandReport struct {
	Version       string `json:"version"`
	BuildRevision string `json:"buildRevision,omitempty"`
}

type rootCommand func([]string) error

type unknownRootCommandError string

func (e unknownRootCommandError) Error() string {
	return "unknown command: " + string(e)
}

var rootCommands = map[string]rootCommand{
	"commands":           runCommands,
	"setup":              func(args []string) error { return runSetup(context.Background(), args) },
	"onboard":            func(args []string) error { return runOnboard(context.Background(), args) },
	"status":             func(args []string) error { return runStatus(context.Background(), args) },
	"doctor":             func(args []string) error { return runDoctor(context.Background(), args) },
	"update":             func(args []string) error { return runUpdate(context.Background(), args) },
	"completion":         runCompletion,
	"logs":               func(args []string) error { return runLogs(context.Background(), args) },
	cliCommandTask:       func(args []string) error { return runTask(context.Background(), args) },
	"watch":              func(args []string) error { return runWatch(context.Background(), args) },
	"notify":             func(args []string) error { return runNotify(context.Background(), args) },
	"store":              func(args []string) error { return runStore(context.Background(), args) },
	"sandbox":            func(args []string) error { return runSandbox(context.Background(), args) },
	"environment":        func(args []string) error { return runEnvironment(context.Background(), args) },
	"runtime":            func(args []string) error { return runRuntime(context.Background(), args) },
	"profile":            runProfile,
	"template-package":   runTemplatePackage,
	"template-packages":  runTemplatePackage,
	"config":             func(args []string) error { return runConfig(context.Background(), args) },
	"evidence":           func(args []string) error { return runEvidence(context.Background(), args) },
	"trace":              func(args []string) error { return runTrace(context.Background(), args) },
	"replay":             runReplay,
	"executor":           func(args []string) error { return runExecutor(context.Background(), args) },
	"workflow":           runWorkflow,
	"baseline":           func(args []string) error { return runBaseline(context.Background(), args) },
	"template":           runTemplate,
	"case":               func(args []string) error { return runCase(context.Background(), args) },
	interfaceNodeCommand: runInterfaceNode,
	"serve":              runServe,
}

func main() {
	if err := runRootCommand(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var unknown unknownRootCommandError
		if errors.As(err, &unknown) {
			printHelp()
		}
		os.Exit(2)
	}
}

func runRootCommand(args []string) error {
	if len(args) < 1 {
		printHelp()
		return nil
	}
	switch args[0] {
	case "version", "--version", "-v":
		return runVersion(args[1:])
	case "help", "--help", "-h":
		printHelp()
		return nil
	}
	command, ok := rootCommands[args[0]]
	if !ok {
		return unknownRootCommandError(args[0])
	}
	return command(args[1:])
}

func runVersion(args []string) error {
	if len(args) == 1 && args[0] == "--json" {
		return writeIndentedJSON(versionCommandReport{Version: version, BuildRevision: strings.TrimSpace(buildRevision)})
	}
	if len(args) != 0 {
		return fmt.Errorf("unexpected version arguments: %s", strings.Join(args, " "))
	}
	fmt.Printf("AgentTestBench %s\n", version)
	return nil
}

func printHelp() {
	fmt.Println(helpText())
}

func helpText() string {
	return helpTextContent
}

func applyEnvironmentServiceRepoUpdate(item map[string]any, update map[string]string) {
	keyMap := map[string]string{
		"url":      "repo",
		"branch":   "branch",
		"ref":      "ref",
		"checkout": "checkout",
	}
	for repoKey, serviceKey := range keyMap {
		value, ok := update[repoKey]
		if !ok {
			continue
		}
		if strings.TrimSpace(value) == "" {
			delete(item, serviceKey)
			continue
		}
		item[serviceKey] = value
	}
}

func printPostgresStoreStatus(status postgres.SchemaStatusResult) {
	pending := status.TargetVersion - status.CurrentVersion
	if pending < 0 {
		pending = 0
	}
	fmt.Println("Store: postgres")
	fmt.Printf("URL: %s\n", maskStoreURL(status.URL))
	fmt.Printf("Version: %d\n", status.CurrentVersion)
	fmt.Printf("Target: %d\n", status.TargetVersion)
	fmt.Printf("Pending: %d\n", pending)
}

func printMySQLStoreStatus(status mysql.SchemaStatusResult) {
	pending := status.TargetVersion - status.CurrentVersion
	if pending < 0 {
		pending = 0
	}
	fmt.Println("Store: mysql")
	fmt.Printf("URL: %s\n", maskStoreURL(status.URL))
	fmt.Printf("Version: %d\n", status.CurrentVersion)
	fmt.Printf("Target: %d\n", status.TargetVersion)
	fmt.Printf("Pending: %d\n", pending)
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
