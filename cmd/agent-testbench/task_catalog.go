package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"agent-testbench/internal/store"
)

const (
	builtInTaskMapMaintain    = "map-maintain"
	builtInTaskMapExecute     = "map-execute"
	builtInTaskJSONCount      = "count"
	builtInTaskFlagMap        = "--map"
	builtInTaskFlagWorkspace  = "--workspace"
	builtInTaskInputWorkspace = "workspace"
	builtInTaskStepEvidence   = "evidence"
)

type builtInTaskDescriptor struct {
	ID             string                   `json:"id"`
	Name           string                   `json:"name"`
	Goal           string                   `json:"goal"`
	Summary        string                   `json:"summary"`
	Tags           []string                 `json:"tags"`
	RequiredInputs []builtInTaskInput       `json:"requiredInputs,omitempty"`
	Steps          []builtInTaskStepPattern `json:"steps"`
}

type builtInTaskInput struct {
	Name        string `json:"name"`
	Flag        string `json:"flag"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type builtInTaskStepPattern struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Command  string `json:"command"`
	ReadOnly bool   `json:"readOnly"`
}

type builtInTaskInputs struct {
	Map         string
	Environment string
	Workspace   string
	CaseRun     string
	Run         string
	Store       string
}

type builtInTaskPlanReport struct {
	OK      bool                  `json:"ok"`
	DryRun  bool                  `json:"dryRun"`
	Task    builtInTaskDescriptor `json:"task"`
	Inputs  map[string]string     `json:"inputs,omitempty"`
	Missing []string              `json:"missing,omitempty"`
	Steps   []builtInTaskPlanStep `json:"steps"`
	Runs    []taskRunView         `json:"runs,omitempty"`
	Error   string                `json:"error,omitempty"`
}

type builtInTaskPlanStep struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Command  string `json:"command"`
	ReadOnly bool   `json:"readOnly"`
	Execute  bool   `json:"execute"`
}

type builtInTaskSuggestion struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Goal   string `json:"goal"`
	Score  int    `json:"score"`
	Reason string `json:"reason"`
}

func runTaskCatalog(args []string) error {
	flags := flag.NewFlagSet("task catalog", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	filter := flags.String("filter", "", "Filter built-in tasks by id, goal, tag, or command")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable task catalog")
	if err := flags.Parse(args); err != nil {
		return err
	}
	tasks := filterBuiltInTasks(*filter)
	report := map[string]any{"ok": true, "filter": strings.TrimSpace(*filter), builtInTaskJSONCount: len(tasks), "tasks": tasks}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	fmt.Println("Task Catalog")
	for _, task := range tasks {
		fmt.Printf("- %s: %s\n", task.ID, task.Goal)
	}
	return nil
}

func runTaskSuggest(args []string) error {
	flags := flag.NewFlagSet("task suggest", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	goal := flags.String("goal", "", "User goal text, such as maintain map or execute map")
	jsonOutput := flags.Bool("json", false, "Emit machine-readable suggestions")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*goal) == "" {
		return errors.New("--goal is required")
	}
	suggestions := suggestBuiltInTasks(*goal)
	report := map[string]any{"ok": true, "goal": strings.TrimSpace(*goal), builtInTaskJSONCount: len(suggestions), "suggestions": suggestions}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	fmt.Println("Task Suggestions")
	for _, suggestion := range suggestions {
		fmt.Printf("- %s: %s\n", suggestion.ID, suggestion.Reason)
	}
	return nil
}

func runTaskPlan(args []string) error {
	flags := flag.NewFlagSet("task plan", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	inputs, jsonOutput := registerBuiltInTaskFlags(flags)
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("task id is required")
	}
	report, err := planBuiltInTask(flags.Arg(0), *inputs, false)
	if *jsonOutput {
		if writeErr := writeIndentedJSON(report); writeErr != nil {
			return writeErr
		}
	}
	if err != nil {
		return err
	}
	if !*jsonOutput {
		printBuiltInTaskPlan(report)
	}
	return nil
}

func runBuiltInTask(ctx context.Context, id string, inputs builtInTaskInputs, dryRun bool, jsonOutput bool) error {
	report, err := planBuiltInTask(id, inputs, !dryRun)
	report.DryRun = dryRun
	if err != nil {
		if jsonOutput {
			if writeErr := writeIndentedJSON(report); writeErr != nil {
				return writeErr
			}
		}
		return err
	}
	if dryRun {
		if jsonOutput {
			return writeIndentedJSON(report)
		}
		printBuiltInTaskPlan(report)
		return nil
	}
	if report.Task.ID != builtInTaskMapMaintain {
		report.OK = false
		report.Error = "built-in task execution is only enabled for read-only map-maintain; use --dry-run to inspect this task"
		if jsonOutput {
			if writeErr := writeIndentedJSON(report); writeErr != nil {
				return writeErr
			}
		}
		return errors.New(report.Error)
	}
	runtime, cleanup, err := openTaskStore(ctx, inputs.Store)
	if err != nil {
		return err
	}
	defer cleanup()
	task, err := upsertTask(ctx, runtime, report.Task.ID, builtInTaskCommandSummary(report.Steps), "", "active", "cli", taskNotificationOptions{})
	if err != nil {
		return err
	}
	report.Runs, err = executeBuiltInTaskSteps(ctx, runtime, task, report.Steps)
	report.OK = err == nil
	if err != nil {
		report.Error = err.Error()
	}
	if jsonOutput {
		if writeErr := writeIndentedJSON(report); writeErr != nil {
			return writeErr
		}
	}
	if err != nil {
		return err
	}
	if !jsonOutput {
		printBuiltInTaskPlan(report)
	}
	return nil
}

func registerBuiltInTaskFlags(flags *flag.FlagSet) (*builtInTaskInputs, *bool) {
	inputs := &builtInTaskInputs{}
	flags.StringVar(&inputs.Map, "map", "", "Test map ID")
	flags.StringVar(&inputs.Environment, "environment", "", "Environment ID")
	flags.StringVar(&inputs.Workspace, "workspace", "", "Workspace path")
	flags.StringVar(&inputs.CaseRun, "case-run", "", "Case run ID")
	flags.StringVar(&inputs.Run, "run", "", "Run ID")
	flags.StringVar(&inputs.Store, "store", "", "Named Store config or Store DSN")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable task plan")
	return inputs, jsonOutput
}

func builtInTaskDescriptors() []builtInTaskDescriptor {
	return []builtInTaskDescriptor{
		{
			ID:      builtInTaskMapMaintain,
			Name:    "Maintain test scenario map",
			Goal:    "Inspect and maintain a test scenario map before execution.",
			Summary: "Runs read-only map health, coverage, diff, and validation-list checks.",
			Tags:    []string{"maintain map", "map maintenance", cliCommandDoctor, "coverage", mapCommandValidation},
			RequiredInputs: []builtInTaskInput{
				{Name: "map", Flag: builtInTaskFlagMap, Description: "Test map ID", Required: true},
			},
			Steps: []builtInTaskStepPattern{
				{ID: "doctor", Title: "Check map health", Command: "map doctor --map {{map}}", ReadOnly: true},
				{ID: "coverage", Title: "Inspect map coverage", Command: "map coverage --map {{map}}", ReadOnly: true},
				{ID: "diff", Title: "Compare working map to published version", Command: "map diff --map {{map}} --from published", ReadOnly: true},
				{ID: "validation-list", Title: "List validation cases attached to the map", Command: "map validation list --map {{map}}", ReadOnly: true},
			},
		},
		{
			ID:      builtInTaskMapExecute,
			Name:    "Execute test scenario map",
			Goal:    "Plan, run, gate, and review a test scenario map.",
			Summary: "Shows the map execution lifecycle without hiding explain/gate/review steps.",
			Tags:    []string{"execute map", "map execution", "planner", "gate", "atlas"},
			RequiredInputs: []builtInTaskInput{
				{Name: "map", Flag: builtInTaskFlagMap, Description: "Test map ID", Required: true},
			},
			Steps: []builtInTaskStepPattern{
				{ID: "explain", Title: "Create a run plan", Command: "map explain --map {{map}} --scope all --save"},
				{ID: "run", Title: "Run the planned map scope", Command: "map run --map {{map}} --scope all"},
				{ID: "gate", Title: "Gate the saved run plan", Command: "map gate --plan PLAN_ID_FROM_EXPLAIN --require-passed --require-tasks --require-evidence"},
				{ID: "atlas", Title: "Open the review atlas", Command: "map atlas --map {{map}} --plan PLAN_ID_FROM_EXPLAIN"},
			},
		},
		{
			ID:      "environment-restore",
			Name:    "Restore environment",
			Goal:    "Inspect, restore, and check a registered environment.",
			Summary: "Keeps environment restore discoverable as a task-oriented flow.",
			Tags:    []string{"restore environment", "environment operations", "health"},
			RequiredInputs: []builtInTaskInput{
				{Name: "environment", Flag: "--environment", Description: "Environment ID", Required: true},
				{Name: builtInTaskInputWorkspace, Flag: builtInTaskFlagWorkspace, Description: "Workspace path", Required: true},
			},
			Steps: []builtInTaskStepPattern{
				{ID: "inspect", Title: "Inspect environment catalog", Command: "environment inspect {{environment}}", ReadOnly: true},
				{ID: "restore", Title: "Restore environment", Command: "environment restore {{environment}} --workspace {{workspace}} --execute"},
				{ID: "status", Title: "Check environment status", Command: "environment status {{environment}} --workspace {{workspace}}", ReadOnly: true},
			},
		},
		{
			ID:      "case-diagnose",
			Name:    "Diagnose case evidence",
			Goal:    "Inspect case evidence and gate a case run.",
			Summary: "Groups case evidence diagnosis commands for failed or suspicious case runs.",
			Tags:    []string{"diagnose evidence", "case diagnosis", commandCatalogCaseGate},
			RequiredInputs: []builtInTaskInput{
				{Name: "caseRun", Flag: "--case-run", Description: "Case run ID", Required: true},
			},
			Steps: []builtInTaskStepPattern{
				{ID: "diagnose", Title: "Diagnose case run", Command: "case diagnose --case-run {{caseRun}}", ReadOnly: true},
				{ID: builtInTaskStepEvidence, Title: "Inspect case evidence", Command: "case inspect --view evidence --case-run {{caseRun}}", ReadOnly: true},
			},
		},
	}
}

func filterBuiltInTasks(filter string) []builtInTaskDescriptor {
	filter = strings.TrimSpace(filter)
	tasks := builtInTaskDescriptors()
	if filter == "" {
		return tasks
	}
	needle := normalizedDiscoveryText(filter)
	out := []builtInTaskDescriptor{}
	for _, task := range tasks {
		if strings.Contains(normalizedDiscoveryText(builtInTaskSearchText(task)), needle) {
			out = append(out, task)
		}
	}
	return out
}

func suggestBuiltInTasks(goal string) []builtInTaskSuggestion {
	needle := normalizedDiscoveryText(goal)
	suggestions := []builtInTaskSuggestion{}
	for _, task := range builtInTaskDescriptors() {
		score, reason := scoreBuiltInTaskSuggestion(task, needle)
		if score == 0 {
			continue
		}
		suggestions = append(suggestions, builtInTaskSuggestion{ID: task.ID, Name: task.Name, Goal: task.Goal, Score: score, Reason: reason})
	}
	sort.SliceStable(suggestions, func(i, j int) bool {
		if suggestions[i].Score != suggestions[j].Score {
			return suggestions[i].Score > suggestions[j].Score
		}
		return suggestions[i].ID < suggestions[j].ID
	})
	return suggestions
}

func scoreBuiltInTaskSuggestion(task builtInTaskDescriptor, needle string) (int, string) {
	if needle == "" {
		return 0, ""
	}
	if strings.Contains(normalizedDiscoveryText(task.ID), needle) || strings.Contains(normalizedDiscoveryText(task.Name), needle) {
		return 100, "goal matches task id or name"
	}
	for _, tag := range task.Tags {
		if strings.Contains(normalizedDiscoveryText(tag), needle) {
			return 90, "goal matches task tag " + tag
		}
	}
	if strings.Contains(normalizedDiscoveryText(task.Goal), needle) {
		return 70, "goal matches task goal"
	}
	for _, step := range task.Steps {
		if strings.Contains(normalizedDiscoveryText(step.Command), needle) {
			return 50, "goal matches planned command " + step.ID
		}
	}
	return 0, ""
}

func planBuiltInTask(id string, inputs builtInTaskInputs, execute bool) (builtInTaskPlanReport, error) {
	task, ok := builtInTaskDescriptorByID(id)
	if !ok {
		return builtInTaskPlanReport{OK: false, Error: "unknown built-in task: " + id}, fmt.Errorf("unknown built-in task: %s", id)
	}
	report := builtInTaskPlanReport{OK: true, Task: task, Inputs: inputs.asMap(), Steps: renderBuiltInTaskSteps(task, inputs, execute)}
	report.Missing = missingBuiltInTaskInputs(task, inputs)
	if len(report.Missing) > 0 {
		report.OK = false
		report.Error = "missing required task inputs: " + strings.Join(report.Missing, ", ")
		return report, errors.New(report.Error)
	}
	return report, nil
}

func builtInTaskDescriptorByID(id string) (builtInTaskDescriptor, bool) {
	id = strings.TrimSpace(id)
	for _, task := range builtInTaskDescriptors() {
		if task.ID == id {
			return task, true
		}
	}
	return builtInTaskDescriptor{}, false
}

func missingBuiltInTaskInputs(task builtInTaskDescriptor, inputs builtInTaskInputs) []string {
	missing := []string{}
	for _, input := range task.RequiredInputs {
		if !input.Required {
			continue
		}
		if strings.TrimSpace(inputs.value(input.Name)) == "" {
			missing = append(missing, input.Flag)
		}
	}
	return missing
}

func renderBuiltInTaskSteps(task builtInTaskDescriptor, inputs builtInTaskInputs, execute bool) []builtInTaskPlanStep {
	steps := make([]builtInTaskPlanStep, 0, len(task.Steps))
	for _, pattern := range task.Steps {
		command := renderBuiltInTaskCommand(pattern.Command, inputs)
		command = appendBuiltInTaskStoreFlag(command, inputs.Store)
		steps = append(steps, builtInTaskPlanStep{
			ID:       pattern.ID,
			Title:    pattern.Title,
			Command:  command,
			ReadOnly: pattern.ReadOnly,
			Execute:  execute,
		})
	}
	return steps
}

func renderBuiltInTaskCommand(command string, inputs builtInTaskInputs) string {
	replacer := strings.NewReplacer(
		"{{map}}", strings.TrimSpace(inputs.Map),
		"{{environment}}", strings.TrimSpace(inputs.Environment),
		"{{workspace}}", strings.TrimSpace(inputs.Workspace),
		"{{caseRun}}", strings.TrimSpace(inputs.CaseRun),
		"{{run}}", strings.TrimSpace(inputs.Run),
	)
	return strings.TrimSpace(replacer.Replace(command))
}

func appendBuiltInTaskStoreFlag(command string, storeRef string) string {
	storeRef = strings.TrimSpace(storeRef)
	if storeRef == "" || strings.Contains(command, " --store ") {
		return command
	}
	return command + " --store " + storeRef
}

func executeBuiltInTaskSteps(ctx context.Context, runtime store.Store, task store.AgentTask, steps []builtInTaskPlanStep) ([]taskRunView, error) {
	runs := make([]taskRunView, 0, len(steps))
	for _, step := range steps {
		if !step.Execute {
			continue
		}
		run, err := executeAndRecordTaskRun(ctx, runtime, task, step.Command)
		runs = append(runs, taskRunViewFromStore(run))
		if err != nil {
			return runs, err
		}
	}
	return runs, nil
}

func builtInTaskCommandSummary(steps []builtInTaskPlanStep) string {
	commands := make([]string, 0, len(steps))
	for _, step := range steps {
		commands = append(commands, step.Command)
	}
	return strings.Join(commands, " && ")
}

func builtInTaskSearchText(task builtInTaskDescriptor) string {
	parts := make([]string, 0, 4+len(task.Tags)+3*len(task.RequiredInputs)+3*len(task.Steps))
	parts = append(parts, task.ID, task.Name, task.Goal, task.Summary)
	parts = append(parts, task.Tags...)
	for _, input := range task.RequiredInputs {
		parts = append(parts, input.Name, input.Flag, input.Description)
	}
	for _, step := range task.Steps {
		parts = append(parts, step.ID, step.Title, step.Command)
	}
	return strings.Join(parts, " ")
}

func (inputs builtInTaskInputs) value(name string) string {
	switch name {
	case "map":
		return inputs.Map
	case "environment":
		return inputs.Environment
	case builtInTaskInputWorkspace:
		return inputs.Workspace
	case "caseRun":
		return inputs.CaseRun
	case "run":
		return inputs.Run
	default:
		return ""
	}
}

func (inputs builtInTaskInputs) asMap() map[string]string {
	values := map[string]string{}
	for key, value := range map[string]string{
		"map":                     inputs.Map,
		"environment":             inputs.Environment,
		builtInTaskInputWorkspace: inputs.Workspace,
		"caseRun":                 inputs.CaseRun,
		"run":                     inputs.Run,
		"store":                   inputs.Store,
	} {
		value = strings.TrimSpace(value)
		if value != "" {
			values[key] = value
		}
	}
	return values
}

func printBuiltInTaskPlan(report builtInTaskPlanReport) {
	fmt.Printf("Task: %s\n", report.Task.ID)
	if report.DryRun {
		fmt.Println("Mode: dry-run")
	}
	if len(report.Missing) > 0 {
		fmt.Printf("Missing: %s\n", strings.Join(report.Missing, ", "))
	}
	for _, step := range report.Steps {
		fmt.Printf("- %s: %s\n", step.ID, step.Command)
	}
}
