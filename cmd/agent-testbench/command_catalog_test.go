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
		registerOut := runCLI(t, "commands", "--filter", "workflow register", "--json")
		if !strings.Contains(registerOut, `"command": "workflow register"`) {
			t.Fatalf("command catalog missing workflow register: %s", registerOut)
		}
	}

	textOut := runCLI(t, "commands", "--filter", "workflow gate")
	if !strings.Contains(textOut, "workflow gate") || !strings.Contains(textOut, "--require-evidence") {
		t.Fatalf("commands text output = %q", textOut)
	}

	taskOut := runCLI(t, "commands", "--filter", "workflow task run", "--json")
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
