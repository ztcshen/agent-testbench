package workflowaudit

import (
	"context"
	"testing"

	"agent-testbench/internal/domain/profile"
)

func TestAuditDetectsRequiredStepInputWithoutEarlierExport(t *testing.T) {
	bundle := profile.Bundle{
		ID:          "profile.workflow-context",
		DisplayName: "Workflow Context Profile",
		Workflows: []profile.Workflow{{
			ID:          "workflow.context",
			DisplayName: "Context Workflow",
		}},
		InterfaceNodes: []profile.InterfaceNode{
			{ID: "node.first", DisplayName: "First"},
			{ID: "node.second", DisplayName: "Second"},
		},
		APICases: []profile.APICase{
			{ID: "case.first", NodeID: "node.first"},
			{ID: "case.second", NodeID: "node.second"},
		},
		WorkflowBindings: []profile.WorkflowBinding{
			{WorkflowID: "workflow.context", StepID: "step.first", NodeID: "node.first", CaseID: "case.first", Required: true, SortOrder: 1},
			{WorkflowID: "workflow.context", StepID: "step.second", NodeID: "node.second", CaseID: "case.second", Required: true, SortOrder: 2},
		},
		TemplateConfigs: []profile.TemplateConfig{
			{
				ID:         "config.step.first",
				WorkflowID: "workflow.context",
				ScopeType:  "step",
				ScopeID:    "step.first",
				Status:     "active",
				ConfigJSON: `{"caseId":"case.first","caseExecution":{"method":"GET","path":"/first"}}`,
			},
			{
				ID:         "config.step.second",
				WorkflowID: "workflow.context",
				ScopeType:  "step",
				ScopeID:    "step.second",
				Status:     "active",
				ConfigJSON: `{"caseId":"case.second","caseExecution":{"method":"GET","path":"/second"},"inputs":[{"name":"item_id","source":"previous"}]}`,
			},
		},
	}

	report, err := Audit(context.Background(), Options{Bundle: bundle, WorkflowID: "workflow.context"})
	if err != nil {
		t.Fatalf("audit workflow context: %v", err)
	}
	if report.OK {
		t.Fatalf("workflow audit should fail when a required input has no earlier export: %#v", report)
	}
	for _, issue := range report.Issues {
		if issue.Code == "workflow-step-input-unbound" && issue.SubjectID == "workflow.context/step.second" && issue.Field == "inputs.item_id" {
			return
		}
	}
	t.Fatalf("expected workflow-step-input-unbound issue, got %#v", report.Issues)
}
