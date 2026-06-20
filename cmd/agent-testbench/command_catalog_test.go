package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTopLevelHelpShowsStoreFlagNotLegacyStoreURL(t *testing.T) {
	out := runCLI(t)
	if !strings.Contains(out, "--store NAME_OR_DSN") {
		t.Fatalf("top-level help should show Store-first flag, got %q", out)
	}
	if !strings.Contains(out, "Recommended workflows") || !strings.Contains(out, "agent-testbench commands --all") {
		t.Fatalf("top-level help should be a task-oriented start page with a path to the full catalog:\n%s", out)
	}
	if strings.Contains(out, "agent-testbench research ") {
		t.Fatalf("top-level help should not expose feature radar as an AgentTestBench product capability:\n%s", out)
	}
	catalogOut := runCLI(t, "commands", "--filter", "research")
	if strings.Contains(catalogOut, "research ") {
		t.Fatalf("command catalog should not expose feature radar as an AgentTestBench command:\n%s", catalogOut)
	}
	exampleOut := runCLI(t, "commands", "--filter", "store config set local", "--json")
	if strings.Contains(exampleOut, `"command": "store config set local"`) {
		t.Fatalf("command catalog should not index copyable examples as command definitions:\n%s", exampleOut)
	}
	if strings.Contains(out, "case run --case PATH") {
		t.Fatalf("top-level help should not promote file-based case run compatibility:\n%s", out)
	}
	if !strings.Contains(out, "agent-testbench task suggest --goal \"maintain map\" --json") || !strings.Contains(out, "agent-testbench task plan map-maintain --map MAP_ID --json") {
		t.Fatalf("top-level help should expose task-intent discovery:\n%s", out)
	}
	if !strings.Contains(out, "agent-testbench case diagnose") {
		t.Fatalf("top-level help should expose case diagnosis:\n%s", out)
	}
	if !strings.Contains(out, "agent-testbench case gate") {
		t.Fatalf("top-level help should expose CI-ready case gates:\n%s", out)
	}
	if !strings.Contains(out, "agent-testbench workflow gate") {
		t.Fatalf("top-level help should expose workflow orchestration gates:\n%s", out)
	}
	if !strings.Contains(out, "agent-testbench workflow task run") || !strings.Contains(out, "--step STEP=TASK_NAME_OR_ID") {
		t.Fatalf("top-level help should expose workflow task trigger/postcondition steps:\n%s", out)
	}
	if !strings.Contains(out, "agent-testbench update") || !strings.Contains(out, "--check") || !strings.Contains(out, "--output PATH") {
		t.Fatalf("top-level help should expose self-update command:\n%s", out)
	}
	if !strings.Contains(out, "agent-testbench status [--deep] [--json]") || !strings.Contains(out, "agent-testbench doctor [--fix] [--deep]") {
		t.Fatalf("top-level help should expose Hermes-style status and doctor commands:\n%s", out)
	}
	if !strings.Contains(out, "Examples:") || !strings.Contains(out, "agent-testbench commands --filter \"case gate\"") {
		t.Fatalf("top-level help should include copyable common CLI examples:\n%s", out)
	}
	for _, noisy := range []string{
		"agent-testbench workflow register",
		"agent-testbench workflow binding register",
		"agent-testbench interface-node case draft",
		"agent-testbench profile import-plan http-capture",
	} {
		if strings.Contains(out, noisy) {
			t.Fatalf("top-level help should not list advanced command %q:\n%s", noisy, out)
		}
	}
	if !strings.Contains(out, "agent-testbench store config set NAME --url postgres://...") || !strings.Contains(out, "agent-testbench store config set NAME --url mysql://...") {
		t.Fatalf("top-level help should show copyable PostgreSQL and MySQL Store setup commands:\n%s", out)
	}
	if strings.Contains(out, "--store-url PATH") {
		t.Fatalf("top-level help should not promote deprecated store-url path flag:\n%s", out)
	}
}

func TestGroupedHelpShowsAreaAndExactCommandUsage(t *testing.T) {
	mapHelp := runCLI(t, "map", "--help")
	if !strings.Contains(mapHelp, "Commands: map") || !strings.Contains(mapHelp, "agent-testbench map explain") || !strings.Contains(mapHelp, "agent-testbench map run") {
		t.Fatalf("map grouped help should show map commands:\n%s", mapHelp)
	}
	for _, heading := range []string{"Inspect:", "Maintain:", "Plan:", "Execute:", "Review:"} {
		if !strings.Contains(mapHelp, heading) {
			t.Fatalf("map grouped help should include lifecycle heading %q:\n%s", heading, mapHelp)
		}
	}
	if strings.Contains(mapHelp, "agent-testbench case run") || strings.Contains(mapHelp, "unknown command") {
		t.Fatalf("map grouped help should not fall back to noisy global help:\n%s", mapHelp)
	}

	mapRunHelp := runCLI(t, "map", "run", "--help")
	if !strings.Contains(mapRunHelp, "Command: map run") || !strings.Contains(mapRunHelp, "agent-testbench map run [--map ID | --plan PLAN_ID]") {
		t.Fatalf("map run exact help should show the run usage:\n%s", mapRunHelp)
	}
	if strings.Contains(mapRunHelp, "map run explain") || strings.Contains(mapRunHelp, "agent-testbench case run") || strings.Contains(mapRunHelp, "unknown command") {
		t.Fatalf("map run exact help should stay focused:\n%s", mapRunHelp)
	}

	mapRunComposingHelp := runCLI(t, "map", "run", "--map", "map.checkout", "--help")
	if !strings.Contains(mapRunComposingHelp, "Command: map run") || !strings.Contains(mapRunComposingHelp, "agent-testbench map run [--map ID | --plan PLAN_ID]") {
		t.Fatalf("map run help should ignore already typed flags:\n%s", mapRunComposingHelp)
	}
	if strings.Contains(mapRunComposingHelp, "unknown help target") || strings.Contains(mapRunComposingHelp, "map run --map") {
		t.Fatalf("map run composing help should not treat flags as command tokens:\n%s", mapRunComposingHelp)
	}

	caseHelp := runCLI(t, "case", "--help")
	if !strings.Contains(caseHelp, "Commands: case") || !strings.Contains(caseHelp, "agent-testbench case diagnose") || strings.Contains(caseHelp, "agent-testbench workflow run") {
		t.Fatalf("case grouped help should show only case commands:\n%s", caseHelp)
	}
}

func TestTemplatePackageHelpUsesCanonicalCatalog(t *testing.T) {
	initHelp := runCLI(t, "template-package", "init", "--help")
	if !strings.Contains(initHelp, "Command: template-package init") || !strings.Contains(initHelp, "agent-testbench template-package init --output PATH") {
		t.Fatalf("template-package init help should be catalog-backed:\n%s", initHelp)
	}
	if strings.Contains(initHelp, "unknown help target") {
		t.Fatalf("template-package init help should not fail before dispatcher fallback:\n%s", initHelp)
	}

	verifyHelp := runCLI(t, "template-package", "verify", "--help")
	if !strings.Contains(verifyHelp, "Command: template-package verify") || !strings.Contains(verifyHelp, "agent-testbench template-package verify --template-package PATH_OR_ID") {
		t.Fatalf("template-package verify help should be catalog-backed:\n%s", verifyHelp)
	}
}

func TestCommandsFilterCanSearchLiteralHelp(t *testing.T) {
	out := runCLI(t, "commands", "--filter", "help", "--json")
	if !strings.Contains(out, `"filter": "help"`) || !strings.Contains(out, `"ok": true`) {
		t.Fatalf("commands should treat help as a literal filter value:\n%s", out)
	}
	if strings.Contains(out, "Command: commands") || strings.Contains(out, "unknown help target") {
		t.Fatalf("commands literal help filter should not be intercepted as focused help:\n%s", out)
	}
}

func TestCommandsCommandEmitsSearchableCommandCatalog(t *testing.T) {
	out := runCLI(t, "commands", "--filter", "gate", "--json")

	var report struct {
		OK       bool   `json:"ok"`
		Filter   string `json:"filter"`
		Count    int    `json:"count"`
		Commands []struct {
			Command    string   `json:"command"`
			Area       string   `json:"area"`
			Path       []string `json:"path"`
			Usage      string   `json:"usage"`
			StoreAware bool     `json:"storeAware"`
			Tags       []string `json:"tags"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode commands json: %v\n%s", err, out)
	}
	if !report.OK || report.Filter != "gate" || report.Count < 2 {
		t.Fatalf("command catalog report = %#v", report)
	}
	foundCaseGate := false
	foundWorkflowGate := false
	foundWorkflowRegister := false
	for _, item := range report.Commands {
		switch item.Command {
		case "case gate":
			foundCaseGate = true
			if item.Area != "case" || len(item.Path) != 2 || item.Path[0] != "case" || item.Path[1] != "gate" || !item.StoreAware || !strings.Contains(item.Usage, "--require-no-failures") {
				t.Fatalf("case gate catalog item = %#v", item)
			}
		case "workflow gate":
			foundWorkflowGate = true
			if item.Area != "workflow" || len(item.Path) != 2 || item.Path[0] != "workflow" || item.Path[1] != "gate" || !item.StoreAware || !strings.Contains(item.Usage, "--require-passed") {
				t.Fatalf("workflow gate catalog item = %#v", item)
			}
		case "workflow register":
			foundWorkflowRegister = true
			if item.Area != "workflow" || len(item.Path) != 2 || !item.StoreAware || !strings.Contains(item.Usage, "--audit") {
				t.Fatalf("workflow register catalog item = %#v", item)
			}
		}
	}
	if !foundCaseGate || !foundWorkflowGate {
		t.Fatalf("command catalog missing gates: %#v", report.Commands)
	}
	if !foundWorkflowRegister {
		registerOut := runCLI(t, "commands", "--all", "--filter", "workflow register", "--json")
		if !strings.Contains(registerOut, `"command": "workflow register"`) {
			t.Fatalf("command catalog missing workflow register: %s", registerOut)
		}
	}

	textOut := runCLI(t, "commands", "--filter", "workflow gate")
	if !strings.Contains(textOut, "workflow gate") || !strings.Contains(textOut, "--require-evidence") {
		t.Fatalf("commands text output = %q", textOut)
	}

	taskOut := runCLI(t, "commands", "--all", "--filter", "workflow task run", "--json")
	if !strings.Contains(taskOut, `"command": "workflow task run"`) || !strings.Contains(taskOut, `STEP=TASK_NAME_OR_ID`) {
		t.Fatalf("command catalog missing workflow task run: %s", taskOut)
	}
}

func TestCommandsAllOmitsDuplicateCompatibilityEntrypoints(t *testing.T) {
	out := runCLI(t, "commands", "--all", "--json")

	var report struct {
		OK       bool `json:"ok"`
		Commands []struct {
			Command string `json:"command"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode all commands json: %v\n%s", err, out)
	}
	if !report.OK {
		t.Fatalf("all command catalog should be ok: %#v", report)
	}
	commands := map[string]bool{}
	for _, item := range report.Commands {
		commands[item.Command] = true
	}
	for _, canonical := range []string{
		"case suite report",
		"environment acceptance start",
		"environment acceptance report",
		"gate baseline get",
		"gate baseline set",
		"map plan inspect",
		"task watch",
		"template-package verify",
	} {
		if !commands[canonical] {
			t.Fatalf("canonical command %q should remain in catalog; commands=%#v", canonical, commands)
		}
	}
	for _, duplicate := range []string{
		"case suite coverage",
		"case suite stability",
		"case suite priority",
		"case suite brief",
		"case suite quality",
		"case suite quality-plan",
		"case suite quality-report",
		"case suite inspect",
		"case suite plan",
		"case suite impact",
		"case suite impact-report",
		"workflow acceptance start",
		"workflow acceptance report",
		"baseline get",
		"baseline set",
		"map run explain",
		"watch",
		"template-packages verify",
		"profile init",
		"profile install",
		"profile pack",
		"profile list",
		"profile inspect",
		"profile export",
		"profile audit",
		"profile audit-plan",
		"profile doctor",
		"profile repair",
		"profile generation-plan openapi",
		"profile import-plan openapi",
		"profile import-plan http-capture",
		"profile catalog list",
		"profile catalog restore",
		"profile verify",
		"profile import",
		"sandbox service register",
		"sandbox interface register",
		"case batch start",
		"case batch report",
		"workflow report",
	} {
		if commands[duplicate] {
			t.Fatalf("historical compatibility entrypoint %q should not remain in catalog", duplicate)
		}
	}
}

func TestHistoricalEntrypointsAreRemoved(t *testing.T) {
	for _, args := range [][]string{
		{"baseline", "get"},
		{"watch", "catalog-smoke", "--command", "commands --json"},
		{"template-packages", "verify", "--template-package", "sample"},
		{"profile", "list"},
		{"profile", "audit", "--profile", "sample", "--offline-template-package"},
		{"case", "batch", "start", "--server-url", "http://127.0.0.1"},
		{"case", "batch", "report", "--server-url", "http://127.0.0.1", "--run", "run.demo"},
		{"sandbox", "service", "register", "--id", "svc.demo"},
		{"sandbox", "interface", "register", "--id", "api.demo", "--service-id", "svc.demo", "--path", "/demo"},
		{"workflow", "report", "--workflow", "workflow.demo"},
	} {
		err := runRootCommand(args)
		if err == nil {
			t.Fatalf("historical entrypoint %q unexpectedly succeeded", strings.Join(args, " "))
		}
		if !strings.Contains(err.Error(), "unknown") {
			t.Fatalf("historical entrypoint %q should be removed from command dispatch, got %T %v", strings.Join(args, " "), err, err)
		}
	}
}

func TestDuplicateMapRunExplainSubcommandIsRejected(t *testing.T) {
	out := runCLIFails(t, "map", "run", "explain", "--plan", "plan.demo")
	if !strings.Contains(out, "map run does not accept positional arguments: explain") {
		t.Fatalf("map run explain should be rejected as a removed duplicate subcommand:\n%s", out)
	}
}

func TestCommandsCatalogIncludesEnvironmentLifecycleCommands(t *testing.T) {
	out := runCLI(t, "commands", "--area", "environment", "--filter", "environment", "--json")

	var report struct {
		OK       bool `json:"ok"`
		Commands []struct {
			Command    string   `json:"command"`
			Usage      string   `json:"usage"`
			StoreAware bool     `json:"storeAware"`
			Tags       []string `json:"tags"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode environment command catalog: %v\n%s", err, out)
	}
	commands := map[string]struct {
		Usage      string
		StoreAware bool
		Tags       []string
	}{}
	for _, item := range report.Commands {
		commands[item.Command] = struct {
			Usage      string
			StoreAware bool
			Tags       []string
		}{Usage: item.Usage, StoreAware: item.StoreAware, Tags: item.Tags}
	}
	for _, command := range []string{"environment status", "environment stop", "environment service restart"} {
		item, ok := commands[command]
		if !report.OK || !ok || !item.StoreAware || !strings.Contains(item.Usage, "--workspace PATH") || !stringSliceContains(item.Tags, "store-first") {
			t.Fatalf("environment lifecycle command %q catalog item = %#v report ok=%t", command, item, report.OK)
		}
	}
}

func TestCommandsCanFilterByArea(t *testing.T) {
	out := runCLI(t, "commands", "--area", "workflow", "--filter", "gate", "--json")

	var report struct {
		OK       bool   `json:"ok"`
		Area     string `json:"area"`
		Commands []struct {
			Command string `json:"command"`
			Area    string `json:"area"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode commands area report: %v\n%s", err, out)
	}
	if !report.OK || report.Area != "workflow" || len(report.Commands) == 0 {
		t.Fatalf("commands area report = %#v", report)
	}
	for _, item := range report.Commands {
		if item.Area != "workflow" {
			t.Fatalf("area filter returned non-workflow command: %#v", item)
		}
	}
}

func TestCommandsSupportTaskOrientedFilters(t *testing.T) {
	tests := []struct {
		filter string
		want   string
	}{
		{filter: "maintain map", want: "map doctor"},
		{filter: "execute map", want: "map run"},
		{filter: "restore environment", want: "environment restore"},
		{filter: "diagnose evidence", want: "case diagnose"},
	}
	for _, tt := range tests {
		out := runCLI(t, "commands", "--all", "--filter", tt.filter, "--json")
		if !strings.Contains(out, `"command": "`+tt.want+`"`) {
			t.Fatalf("commands --filter %q should find %q:\n%s", tt.filter, tt.want, out)
		}
	}

	out := runCLI(t, "commands", "--all", "--filter", "maintain map", "--json")
	var report struct {
		Commands []struct {
			Command string `json:"command"`
			Rank    int    `json:"rank"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode maintain map command catalog: %v\n%s", err, out)
	}
	wantOrder := []string{"map doctor", "map coverage", "map diff", "map validation list", "map validation attach"}
	if len(report.Commands) < len(wantOrder) {
		t.Fatalf("maintain map command catalog too short: %#v", report.Commands)
	}
	for i, want := range wantOrder {
		if report.Commands[i].Command != want {
			t.Fatalf("maintain map command %d = %q, want %q; full report: %#v", i, report.Commands[i].Command, want, report.Commands)
		}
		if report.Commands[i].Rank == 0 {
			t.Fatalf("maintain map command %q should expose a stable rank: %#v", want, report.Commands[i])
		}
	}
}

func TestMapCommandsExposeLifecycleMetadata(t *testing.T) {
	out := runCLI(t, "commands", "--area", "map", "--all", "--json")

	var report struct {
		OK       bool `json:"ok"`
		Commands []struct {
			Command   string   `json:"command"`
			Lifecycle string   `json:"lifecycle"`
			Tags      []string `json:"tags"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode map command catalog: %v\n%s", err, out)
	}
	if !report.OK || len(report.Commands) == 0 {
		t.Fatalf("map command catalog should be populated: %#v", report)
	}
	wantLifecycle := map[string]string{
		"map list":              "inspect",
		"map workflows":         "inspect",
		"map coverage":          "inspect",
		"map plans":             "inspect",
		"map doctor":            "maintain",
		"map diff":              "maintain",
		"map validation list":   "maintain",
		"map validation attach": "maintain",
		"map update":            "maintain",
		"map snapshot":          "maintain",
		"map publish":           "maintain",
		"map explain":           "plan",
		"map plan inspect":      "plan",
		"map run":               "execute",
		"map gate":              "execute",
		"map atlas":             "review",
	}
	seen := map[string]string{}
	for _, item := range report.Commands {
		seen[item.Command] = item.Lifecycle
		if item.Lifecycle == "" {
			t.Fatalf("map command %q missing lifecycle metadata: %#v", item.Command, item)
		}
		if !stringSliceContains(item.Tags, item.Lifecycle) {
			t.Fatalf("map command %q should include lifecycle tag %q: %#v", item.Command, item.Lifecycle, item.Tags)
		}
	}
	for command, lifecycle := range wantLifecycle {
		if seen[command] != lifecycle {
			t.Fatalf("map command %q lifecycle = %q, want %q; seen=%#v", command, seen[command], lifecycle, seen)
		}
	}
}

func TestCommandsDefaultSurfaceShowsDailyCommandsOnly(t *testing.T) {
	out := runCLI(t, "commands", "--json")

	var report struct {
		OK       bool   `json:"ok"`
		Tier     string `json:"tier"`
		All      bool   `json:"all"`
		Count    int    `json:"count"`
		Commands []struct {
			Command     string `json:"command"`
			Area        string `json:"area"`
			Tier        string `json:"tier"`
			Audience    string `json:"audience"`
			Stability   string `json:"stability"`
			Replacement string `json:"replacement,omitempty"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode default commands json: %v\n%s", err, out)
	}
	if !report.OK || report.All || report.Tier != "daily" || report.Count == 0 || report.Count > 40 {
		t.Fatalf("default command catalog should be a focused daily surface: %#v", report)
	}
	commands := map[string]struct {
		Tier        string
		Audience    string
		Stability   string
		Replacement string
	}{}
	for _, item := range report.Commands {
		commands[item.Command] = struct {
			Tier        string
			Audience    string
			Stability   string
			Replacement string
		}{Tier: item.Tier, Audience: item.Audience, Stability: item.Stability, Replacement: item.Replacement}
		if item.Tier != "daily" || item.Audience == "" || item.Stability == "" {
			t.Fatalf("default catalog returned non-daily or unclassified command: %#v", item)
		}
	}
	if report.Count > 30 {
		t.Fatalf("default command catalog should stay at or below the first target of 30 commands, got %d", report.Count)
	}
	for _, want := range []string{"status", "doctor", "store current", "environment restore", "task suggest", "task plan", "map explain", "map run", "case run", "case suite report"} {
		if _, ok := commands[want]; !ok {
			t.Fatalf("default catalog missing daily command %q in %#v", want, commands)
		}
	}
	for _, hidden := range []string{"profile import", "template-package import", "runtime mysql endpoints", "executor plan", "case suite coverage", "workflow acceptance start", "baseline get", "workflow report", "case suite plan", "map plan inspect"} {
		if _, ok := commands[hidden]; ok {
			t.Fatalf("default catalog should hide %q: %#v", hidden, commands[hidden])
		}
	}
}

func TestCommandsDailySurfaceExplainsAdmission(t *testing.T) {
	out := runCLI(t, "commands", "--json")

	var report struct {
		Commands []struct {
			Command     string `json:"command"`
			DailyReason string `json:"dailyReason"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode daily command catalog: %v\n%s", err, out)
	}
	reasons := map[string]string{}
	for _, item := range report.Commands {
		reasons[item.Command] = item.DailyReason
		if item.DailyReason == "" {
			t.Fatalf("daily command %q missing dailyReason: %#v", item.Command, item)
		}
	}
	for _, command := range []string{"map run", "case run", "task plan", "workflow gate"} {
		if reasons[command] == "" {
			t.Fatalf("daily command %q should explain admission: %#v", command, reasons)
		}
	}
}

func TestNonDailyWorkflowCommandsHaveReplacementHints(t *testing.T) {
	out := runCLI(t, "commands", "--area", "workflow", "--all", "--json")

	var report struct {
		Commands []struct {
			Command     string `json:"command"`
			Tier        string `json:"tier"`
			Replacement string `json:"replacement"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode workflow command catalog: %v\n%s", err, out)
	}
	for _, item := range report.Commands {
		if item.Tier == "daily" {
			continue
		}
		if item.Replacement == "" {
			t.Fatalf("non-daily workflow command should have a replacement hint: %#v", item)
		}
	}
}

func TestCommandsAllExposesAdvancedReplacementMetadata(t *testing.T) {
	out := runCLI(t, "commands", "--all", "--filter", "executor plan", "--json")

	var report struct {
		OK       bool `json:"ok"`
		All      bool `json:"all"`
		Commands []struct {
			Command     string `json:"command"`
			Tier        string `json:"tier"`
			Audience    string `json:"audience"`
			Stability   string `json:"stability"`
			Replacement string `json:"replacement,omitempty"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode all commands json: %v\n%s", err, out)
	}
	if !report.OK || !report.All || len(report.Commands) == 0 {
		t.Fatalf("all command catalog = %#v", report)
	}
	foundExecutorPlan := false
	for _, item := range report.Commands {
		if item.Command == "executor plan" {
			foundExecutorPlan = true
			if item.Tier != "advanced" || item.Audience != "developer" || item.Stability != "stable" || !strings.Contains(item.Replacement, "map explain") {
				t.Fatalf("executor plan metadata = %#v", item)
			}
		}
	}
	if !foundExecutorPlan {
		t.Fatalf("executor plan missing from filtered catalog: %#v", report.Commands)
	}

	textOut := runCLI(t, "commands", "--all", "--filter", "executor plan")
	if !strings.Contains(textOut, "Tier: advanced") || !strings.Contains(textOut, "Replacement: agent-testbench map explain") {
		t.Fatalf("text command catalog should show tier and replacement:\n%s", textOut)
	}
}

func TestCommandsCanFilterByTierAndAudience(t *testing.T) {
	out := runCLI(t, "commands", "--tier", "advanced", "--audience", "developer", "--filter", "executor plan", "--json")

	var report struct {
		OK       bool   `json:"ok"`
		Tier     string `json:"tier"`
		Audience string `json:"audience"`
		Commands []struct {
			Command     string `json:"command"`
			Tier        string `json:"tier"`
			Audience    string `json:"audience"`
			Replacement string `json:"replacement,omitempty"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode tier/audience commands json: %v\n%s", err, out)
	}
	if !report.OK || report.Tier != "advanced" || report.Audience != "developer" || len(report.Commands) == 0 {
		t.Fatalf("tier/audience command catalog = %#v", report)
	}
	for _, item := range report.Commands {
		if item.Tier != "advanced" || item.Audience != "developer" || item.Command != "executor plan" {
			t.Fatalf("executor plan tier/audience metadata = %#v", item)
		}
	}
}
