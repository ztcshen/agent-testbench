package controlplane

import (
	"testing"
	"time"

	"agent-testbench/internal/store"
)

func TestFindCaseExecutionConfigUsesCaseScopeWhenPayloadOmitsCaseID(t *testing.T) {
	config := findCaseExecutionTemplateConfigFromEntries([]caseExecutionConfigEntry{{
		Status:     "active",
		ScopeType:  "case",
		ScopeID:    "case.scoped",
		ConfigJSON: `{"caseExecution":{"method":"GET","path":"/scoped"}}`,
	}}, "case.scoped", map[string]any{})

	if config == nil || config.CaseExecution.Path != "/scoped" {
		t.Fatalf("case-scoped execution config = %#v", config)
	}
}

func TestFindCaseExecutionConfigRejectsMatchingStepForAnotherCase(t *testing.T) {
	config := findCaseExecutionTemplateConfigFromEntries([]caseExecutionConfigEntry{{
		Status:     "active",
		WorkflowID: "workflow.alpha",
		ScopeType:  "step",
		ScopeID:    "step.alpha",
		ConfigJSON: `{"caseId":"case.other","caseExecution":{"method":"GET","path":"/wrong"}}`,
	}}, "case.requested", map[string]any{"workflowId": "workflow.alpha", "stepId": "step.alpha"})

	if config != nil {
		t.Fatalf("wrong-case step config must not match: %#v", config)
	}
}

func TestNextTestKitRunIDIsUniqueAtTheSameInstant(t *testing.T) {
	now := time.Date(2026, time.July, 16, 8, 0, 0, 123, time.UTC)
	first := nextTestKitRunID(now)
	second := nextTestKitRunID(now)
	if first == second {
		t.Fatalf("test-kit run ids must be unique: %q", first)
	}
}

func TestSiblingExecutionConfigDoesNotTreatMissingNodesAsEqual(t *testing.T) {
	execution := deriveCaseExecutionConfigFromSiblingConfig(store.ProfileCatalog{
		TemplateConfigs: []store.CatalogTemplateConfig{{
			Status:     "active",
			ConfigJSON: `{"caseId":"case.other","caseExecution":{"method":"GET","path":"/wrong"}}`,
		}},
	}, store.CatalogAPICase{ID: "case.requested"})
	if execution != nil {
		t.Fatalf("empty node ids must not authorize sibling reuse: %#v", execution)
	}
}
