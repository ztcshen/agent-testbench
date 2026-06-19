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
	if !strings.Contains(out, "case run --case PATH") || !strings.Contains(out, "--dry-run") {
		t.Fatalf("top-level help should expose case run dry-run preflight:\n%s", out)
	}
	if !strings.Contains(out, "agent-testbench case diagnose") {
		t.Fatalf("top-level help should expose case diagnosis:\n%s", out)
	}
	if !strings.Contains(out, "agent-testbench case gate") {
		t.Fatalf("top-level help should expose CI-ready case gates:\n%s", out)
	}
	if !strings.Contains(out, "agent-testbench case config upsert") || !strings.Contains(out, "--response-not-contains") {
		t.Fatalf("top-level help should expose Store-backed case config upsert:\n%s", out)
	}
	if !strings.Contains(out, "agent-testbench workflow gate") {
		t.Fatalf("top-level help should expose workflow orchestration gates:\n%s", out)
	}
	if !strings.Contains(out, "agent-testbench workflow task run") || !strings.Contains(out, "--step STEP=TASK_NAME_OR_ID") {
		t.Fatalf("top-level help should expose workflow task trigger/postcondition steps:\n%s", out)
	}
	if !strings.Contains(out, "agent-testbench workflow register") || !strings.Contains(out, "agent-testbench workflow binding register") {
		t.Fatalf("top-level help should expose Store-first workflow upsert commands:\n%s", out)
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
	if !strings.Contains(out, "agent-testbench store config set NAME --url postgres://...") || !strings.Contains(out, "agent-testbench store config set NAME --url mysql://...") {
		t.Fatalf("top-level help should show copyable PostgreSQL and MySQL Store setup commands:\n%s", out)
	}
	for _, want := range []string{"--clean-docker-state", "--clean-docker-images", "--allow-destructive-docker-cleanup"} {
		if !strings.Contains(out, want) {
			t.Fatalf("top-level help missing restore cleanup flag %q:\n%s", want, out)
		}
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

func TestTemplatePackageAliasHelpUsesCatalog(t *testing.T) {
	initHelp := runCLI(t, "template-package", "init", "--help")
	if !strings.Contains(initHelp, "Command: template-package init") || !strings.Contains(initHelp, "agent-testbench template-package init --output PATH") {
		t.Fatalf("template-package init help should be catalog-backed:\n%s", initHelp)
	}
	if strings.Contains(initHelp, "unknown help target") {
		t.Fatalf("template-package init help should not fail before dispatcher fallback:\n%s", initHelp)
	}

	verifyHelp := runCLI(t, "template-packages", "verify", "--help")
	if !strings.Contains(verifyHelp, "Command: template-packages verify") || !strings.Contains(verifyHelp, "agent-testbench template-packages verify --template-package PATH_OR_ID") {
		t.Fatalf("template-packages verify alias help should be catalog-backed:\n%s", verifyHelp)
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
	for _, want := range []string{"status", "doctor", "store current", "environment restore", "map explain", "map run", "case run", "case suite report"} {
		if _, ok := commands[want]; !ok {
			t.Fatalf("default catalog missing daily command %q in %#v", want, commands)
		}
	}
	for _, hidden := range []string{"profile import", "template-package import", "runtime mysql endpoints", "executor plan", "case suite coverage", "workflow acceptance start", "baseline get"} {
		if _, ok := commands[hidden]; ok {
			t.Fatalf("default catalog should hide %q: %#v", hidden, commands[hidden])
		}
	}
}

func TestCommandsAllExposesAdvancedAndCompatibilityMetadata(t *testing.T) {
	out := runCLI(t, "commands", "--all", "--filter", "case suite coverage", "--json")

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
	item := report.Commands[0]
	if item.Command != "case suite coverage" || item.Tier != "compat" || item.Audience != "agent" || item.Stability != "legacy" || !strings.Contains(item.Replacement, "case suite report --view coverage") {
		t.Fatalf("case suite coverage metadata = %#v", item)
	}

	textOut := runCLI(t, "commands", "--all", "--filter", "executor plan")
	if !strings.Contains(textOut, "Tier: advanced") || !strings.Contains(textOut, "Replacement: agent-testbench map explain") {
		t.Fatalf("text command catalog should show tier and replacement:\n%s", textOut)
	}
}

func TestCommandsCanFilterByTierAndAudience(t *testing.T) {
	out := runCLI(t, "commands", "--tier", "compat", "--audience", "agent", "--filter", "workflow acceptance", "--json")

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
	if !report.OK || report.Tier != "compat" || report.Audience != "agent" || len(report.Commands) == 0 {
		t.Fatalf("tier/audience command catalog = %#v", report)
	}
	for _, item := range report.Commands {
		if item.Tier != "compat" || item.Audience != "agent" || !strings.Contains(item.Replacement, "environment acceptance") {
			t.Fatalf("workflow acceptance metadata = %#v", item)
		}
	}
}
