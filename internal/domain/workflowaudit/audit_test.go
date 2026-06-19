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
				ScopeType:  templateConfigScopeStep,
				ScopeID:    "step.first",
				Status:     "active",
				ConfigJSON: `{"caseId":"case.first","caseExecution":{"method":"GET","path":"/first"}}`,
			},
			{
				ID:         "config.step.second",
				WorkflowID: "workflow.context",
				ScopeType:  templateConfigScopeStep,
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

func TestAuditAcceptsRequiredInputFromEarlierCaseScopedExport(t *testing.T) {
	bundle := profile.Bundle{
		ID:          "profile.workflow-case-context",
		DisplayName: "Workflow Case Context Profile",
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
				ID:         "config.case.first",
				ScopeType:  templateConfigScopeAPICase,
				ScopeID:    "case.first",
				Status:     "active",
				ConfigJSON: `{"caseId":"case.first","caseExecution":{"method":"GET","path":"/first"},"exports":[{"name":"item_id","from":"responseBody","path":"item_id"}]}`,
			},
			{
				ID:         "config.step.second",
				WorkflowID: "workflow.context",
				ScopeType:  templateConfigScopeStep,
				ScopeID:    "step.second",
				Status:     "active",
				ConfigJSON: `{"caseId":"case.second","caseExecution":{"method":"GET","path":"/second"},"inputs":[{"name":"item_id","source":"previous"}]}`,
			},
		},
	}

	report, err := Audit(context.Background(), Options{Bundle: bundle, WorkflowID: "workflow.context"})
	if err != nil {
		t.Fatalf("audit workflow case scoped context: %v", err)
	}
	for _, issue := range report.Issues {
		if issue.Code == "workflow-step-input-unbound" && issue.SubjectID == "workflow.context/step.second" {
			t.Fatalf("case scoped export should satisfy later step input: %#v", report.Issues)
		}
	}
}

func TestAuditUsesCaseExecutionConfigWhenStepConfigIsPresentationOnly(t *testing.T) {
	bundle := profile.Bundle{
		ID:          "profile.workflow-merged-context",
		DisplayName: "Workflow Merged Context Profile",
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
				ScopeType:  templateConfigScopeStep,
				ScopeID:    "step.first",
				Status:     "active",
				ConfigJSON: `{"displayName":"First step presentation","caseId":"case.first"}`,
			},
			{
				ID:         "config.case.first",
				ScopeType:  templateConfigScopeAPICase,
				ScopeID:    "case.first",
				Status:     "active",
				ConfigJSON: `{"caseId":"case.first","caseExecution":{"method":"GET","path":"/first"},"exports":[{"name":"item_id","from":"responseBody","path":"item_id"}]}`,
			},
			{
				ID:         "config.step.second",
				WorkflowID: "workflow.context",
				ScopeType:  templateConfigScopeStep,
				ScopeID:    "step.second",
				Status:     "active",
				ConfigJSON: `{"caseId":"case.second","caseExecution":{"method":"GET","path":"/second"},"inputs":[{"name":"item_id","source":"previous"}]}`,
			},
		},
	}

	report, err := Audit(context.Background(), Options{Bundle: bundle, WorkflowID: "workflow.context"})
	if err != nil {
		t.Fatalf("audit presentation-only workflow case context: %v", err)
	}
	for _, issue := range report.Issues {
		if issue.Code == "workflow-step-input-unbound" && issue.SubjectID == "workflow.context/step.second" {
			t.Fatalf("case scoped export should not be masked by step config: %#v", report.Issues)
		}
	}
}

func TestAuditHonorsStepExecutionConfigPrecedenceOverCaseConfig(t *testing.T) {
	bundle := profile.Bundle{
		ID:          "profile.workflow-step-precedence",
		DisplayName: "Workflow Step Precedence Profile",
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
				ScopeType:  templateConfigScopeStep,
				ScopeID:    "step.first",
				Status:     "active",
				ConfigJSON: `{"caseId":"case.first","caseExecution":{"method":"GET","path":"/first"}}`,
			},
			{
				ID:         "config.case.first",
				ScopeType:  templateConfigScopeAPICase,
				ScopeID:    "case.first",
				Status:     "active",
				ConfigJSON: `{"caseId":"case.first","caseExecution":{"method":"GET","path":"/first"},"exports":[{"name":"item_id","from":"responseBody","path":"item_id"}]}`,
			},
			{
				ID:         "config.step.second",
				WorkflowID: "workflow.context",
				ScopeType:  templateConfigScopeStep,
				ScopeID:    "step.second",
				Status:     "active",
				ConfigJSON: `{"caseId":"case.second","caseExecution":{"method":"GET","path":"/second"},"inputs":[{"name":"item_id","source":"previous"}]}`,
			},
		},
	}

	report, err := Audit(context.Background(), Options{Bundle: bundle, WorkflowID: "workflow.context"})
	if err != nil {
		t.Fatalf("audit workflow step precedence: %v", err)
	}
	for _, issue := range report.Issues {
		if issue.Code == "workflow-step-input-unbound" && issue.SubjectID == "workflow.context/step.second" && issue.Field == "inputs.item_id" {
			return
		}
	}
	t.Fatalf("expected step execution config to mask case export, got %#v", report.Issues)
}

func TestAuditAcceptsLegacyCaseExecutionConfigByCaseID(t *testing.T) {
	bundle := profile.Bundle{
		ID:          "profile.workflow-legacy-case-context",
		DisplayName: "Workflow Legacy Case Context Profile",
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
				ID:         "config.legacy.case.first",
				Status:     "active",
				ConfigJSON: `{"caseId":"case.first","caseExecution":{"method":"GET","path":"/first"},"exports":[{"name":"item_id","from":"responseBody","path":"item_id"}]}`,
			},
			{
				ID:         "config.step.second",
				WorkflowID: "workflow.context",
				ScopeType:  templateConfigScopeStep,
				ScopeID:    "step.second",
				Status:     "active",
				ConfigJSON: `{"caseId":"case.second","caseExecution":{"method":"GET","path":"/second"},"inputs":[{"name":"item_id","source":"previous"}]}`,
			},
		},
	}

	report, err := Audit(context.Background(), Options{Bundle: bundle, WorkflowID: "workflow.context"})
	if err != nil {
		t.Fatalf("audit legacy case scoped context: %v", err)
	}
	for _, issue := range report.Issues {
		if issue.Code == "workflow-step-input-unbound" && issue.SubjectID == "workflow.context/step.second" {
			t.Fatalf("legacy case config export should satisfy later step input: %#v", report.Issues)
		}
	}
}
