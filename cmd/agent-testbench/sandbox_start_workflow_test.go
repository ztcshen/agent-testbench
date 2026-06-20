package main

import (
	"context"
	"strings"
	"testing"

	"agent-testbench/internal/store"
)

func TestSandboxStartSelectedServiceFailsWhenStartupCommandMissing(t *testing.T) {
	fixture := writeSandboxStartStoreFixture(t)

	out := runCLIFails(t, "sandbox", "start", "--store", "sqlite://"+fixture.storePath, "--service", "documented-service", "--json")
	for _, want := range []string{
		`"ok": false`,
		`"failed": 1`,
		`"id": "documented-service"`,
		sandboxStartupCommandEmpty,
		"publish an updated template package for service documented-service",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("selected service missing startup command output should contain %q:\n%s", want, out)
		}
	}
	requireSandboxNoStartupSideEffects(t, fixture)
}

func TestSandboxStartDryRunDoesNotRunStartupCommands(t *testing.T) {
	fixture := writeSandboxStartStoreFixture(t)

	report := runSandboxStartJSON(t, "sqlite://"+fixture.storePath, "sandbox start dry-run", "--dry-run")
	if !report.OK || !report.DryRun || report.Counts.Planned != 2 {
		t.Fatalf("sandbox start dry-run report = %#v", report)
	}
	planned := map[string]bool{}
	skipped := map[string]bool{}
	for _, service := range report.Services {
		planned[service.ID] = service.Planned
		skipped[service.ID] = service.Skipped
	}
	if !planned["entry-service"] || !planned["platform-service"] || !skipped["documented-service"] {
		t.Fatalf("sandbox start dry-run services = %#v", report.Services)
	}
	requireSandboxNoStartupSideEffects(t, fixture)
}

func TestSandboxStartWorkflowBlocksMissingStartupCommand(t *testing.T) {
	fixture := writeSandboxStartStoreFixture(t)
	addSandboxStartWorkflow(t, fixture.storePath, true)

	out := runCLIFails(t, "sandbox", "start", "--store", "sqlite://"+fixture.storePath, "--workflow", "workflow.smoke", "--dry-run", "--json")
	for _, want := range []string{
		`"ok": false`,
		`"workflowId": "workflow.smoke"`,
		`"planned": 1`,
		`"failed": 1`,
		`"id": "entry-service"`,
		`"id": "documented-service"`,
		sandboxStartupCommandEmpty,
		"required by workflow workflow.smoke",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("workflow start missing startup command output should contain %q:\n%s", want, out)
		}
	}
	requireSandboxNoStartupSideEffects(t, fixture)
}

func TestSandboxStartWorkflowIgnoresOptionalBindings(t *testing.T) {
	fixture := writeSandboxStartStoreFixture(t)
	addSandboxStartWorkflow(t, fixture.storePath, false)

	out := runCLI(t, "sandbox", "start", "--store", "sqlite://"+fixture.storePath, "--workflow", "workflow.smoke", "--dry-run", "--json")
	for _, want := range []string{
		`"ok": true`,
		`"workflowId": "workflow.smoke"`,
		`"planned": 1`,
		`"id": "entry-service"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("workflow start with optional binding output should contain %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{
		`"id": "documented-service"`,
		sandboxStartupCommandEmpty,
	} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("workflow start should ignore optional binding %q:\n%s", unwanted, out)
		}
	}
	requireSandboxNoStartupSideEffects(t, fixture)
}

func addSandboxStartWorkflow(t *testing.T, storePath string, documentedRequired bool) {
	t.Helper()

	ctx := context.Background()
	s, err := openStore(ctx, "sqlite://"+storePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer closeCLIStore(s)
	catalog, err := s.GetProfileCatalog(ctx)
	if err != nil {
		t.Fatalf("get catalog: %v", err)
	}
	catalog.Workflows = append(catalog.Workflows, store.CatalogWorkflow{ID: "workflow.smoke", DisplayName: "Smoke Workflow"})
	catalog.InterfaceNodes = append(catalog.InterfaceNodes,
		store.CatalogInterfaceNode{ID: "node.entry", ServiceID: "entry-service"},
		store.CatalogInterfaceNode{ID: "node.documented", ServiceID: "documented-service"},
	)
	catalog.WorkflowBindings = append(catalog.WorkflowBindings,
		store.CatalogWorkflowBinding{WorkflowID: "workflow.smoke", StepID: "step.entry", NodeID: "node.entry", CaseID: "case.entry", Required: true, SortOrder: 1},
		store.CatalogWorkflowBinding{WorkflowID: "workflow.smoke", StepID: "step.documented", NodeID: "node.documented", CaseID: "case.documented", Required: documentedRequired, SortOrder: 2},
	)
	if err := s.ReplaceProfileCatalog(ctx, catalog); err != nil {
		t.Fatalf("replace catalog with workflow: %v", err)
	}
}
