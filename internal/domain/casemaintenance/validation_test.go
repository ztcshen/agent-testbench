package casemaintenance

import (
	"errors"
	"testing"

	"agent-testbench/internal/domain/catalog"
)

func TestNormalizeCaseStatusDefaultsNewCasesToDraft(t *testing.T) {
	status, err := NormalizeCaseStatus("", true)
	if err != nil {
		t.Fatalf("normalize new case status: %v", err)
	}
	if status != StatusDraft {
		t.Fatalf("new case status = %q, want %q", status, StatusDraft)
	}

	status, err = NormalizeCaseStatus("", false)
	if err != nil {
		t.Fatalf("normalize existing legacy case status: %v", err)
	}
	if status != StatusActive {
		t.Fatalf("existing legacy case status = %q, want %q", status, StatusActive)
	}
}

func TestNormalizeCaseStatusAcceptsOnlySupportedLifecycleValues(t *testing.T) {
	for _, status := range []string{StatusDraft, StatusReview, StatusActive, StatusQuarantined, StatusDeprecated} {
		t.Run(status, func(t *testing.T) {
			got, err := NormalizeCaseStatus("  "+status+"  ", false)
			if err != nil {
				t.Fatalf("normalize supported case status: %v", err)
			}
			if got != status {
				t.Fatalf("normalized status = %q, want %q", got, status)
			}
		})
	}

	_, err := NormalizeCaseStatus("enabled", false)
	requireValidationIssue(t, err, IssueInvalidStatus)
}

func TestValidateCaseRejectsDanglingCatalogReferences(t *testing.T) {
	tests := []struct {
		name      string
		catalog   catalog.ProfileCatalog
		candidate catalog.APICase
		wantIssue string
	}{
		{
			name:      "node is required",
			candidate: catalog.APICase{ID: "case.alpha", Status: StatusDraft},
			wantIssue: IssueNodeRequired,
		},
		{
			name:      "node must exist",
			catalog:   catalog.ProfileCatalog{InterfaceNodes: []catalog.InterfaceNode{{ID: "node.other"}}},
			candidate: catalog.APICase{ID: "case.alpha", NodeID: "node.missing", Status: StatusDraft},
			wantIssue: IssueNodeNotFound,
		},
		{
			name: "request template must exist",
			catalog: catalog.ProfileCatalog{
				InterfaceNodes: []catalog.InterfaceNode{{ID: "node.alpha"}},
			},
			candidate: catalog.APICase{ID: "case.alpha", NodeID: "node.alpha", RequestTemplateID: "template.missing", Status: StatusDraft},
			wantIssue: IssueRequestTemplateNotFound,
		},
		{
			name: "request template must belong to case node",
			catalog: catalog.ProfileCatalog{
				InterfaceNodes:   []catalog.InterfaceNode{{ID: "node.alpha"}, {ID: "node.other"}},
				RequestTemplates: []catalog.RequestTemplate{{ID: "template.alpha", NodeID: "node.other", Status: StatusActive}},
			},
			candidate: catalog.APICase{ID: "case.alpha", NodeID: "node.alpha", RequestTemplateID: "template.alpha", Status: StatusDraft},
			wantIssue: IssueRequestTemplateNodeMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCase(tt.catalog, tt.candidate)
			requireValidationIssue(t, err, tt.wantIssue)
		})
	}
}

func TestValidateActiveCaseRequiresExecutableConfiguration(t *testing.T) {
	base := catalog.ProfileCatalog{
		InterfaceNodes: []catalog.InterfaceNode{{ID: "node.alpha", Method: "GET", Path: "/alpha", Status: StatusActive}},
	}
	tests := []struct {
		name      string
		catalog   catalog.ProfileCatalog
		candidate catalog.APICase
		wantIssue string
	}{
		{
			name:      "missing runnable configuration",
			catalog:   base,
			candidate: catalog.APICase{ID: "case.alpha", NodeID: "node.alpha", Status: StatusActive},
			wantIssue: IssueActiveCaseNotRunnable,
		},
		{
			name:      "file backed case is runnable",
			catalog:   base,
			candidate: catalog.APICase{ID: "case.alpha", NodeID: "node.alpha", Status: StatusActive, CasePath: "cases/alpha.json"},
		},
		{
			name: "active request template is runnable",
			catalog: catalog.ProfileCatalog{
				InterfaceNodes:   base.InterfaceNodes,
				RequestTemplates: []catalog.RequestTemplate{{ID: "template.alpha", NodeID: "node.alpha", Method: "GET", Path: "/alpha", Status: StatusActive}},
			},
			candidate: catalog.APICase{ID: "case.alpha", NodeID: "node.alpha", Status: StatusActive, RequestTemplateID: "template.alpha"},
		},
		{
			name: "inactive request template is not runnable",
			catalog: catalog.ProfileCatalog{
				InterfaceNodes:   base.InterfaceNodes,
				RequestTemplates: []catalog.RequestTemplate{{ID: "template.alpha", NodeID: "node.alpha", Status: StatusDeprecated}},
			},
			candidate: catalog.APICase{ID: "case.alpha", NodeID: "node.alpha", Status: StatusActive, RequestTemplateID: "template.alpha"},
			wantIssue: IssueRequestTemplateInactive,
		},
		{
			name: "case execution config is runnable",
			catalog: catalog.ProfileCatalog{
				InterfaceNodes: base.InterfaceNodes,
				TemplateConfigs: []catalog.TemplateConfig{{
					ID: "config.case.alpha", ScopeType: "case", ScopeID: "case.alpha", Status: StatusActive,
					ConfigJSON: `{"caseExecution":{"method":"GET","path":"/alpha"}}`,
				}},
			},
			candidate: catalog.APICase{ID: "case.alpha", NodeID: "node.alpha", Status: StatusActive},
		},
		{
			name:      "complete external source remains planning only",
			catalog:   base,
			candidate: catalog.APICase{ID: "case.alpha", NodeID: "node.alpha", Status: StatusActive, SourceKind: "karate", SourcePath: "tests/alpha.feature", ExecutorID: "executor.karate"},
			wantIssue: IssueExternalSourcePlanningOnly,
		},
		{
			name:      "partial external source is not runnable",
			catalog:   base,
			candidate: catalog.APICase{ID: "case.alpha", NodeID: "node.alpha", Status: StatusActive, SourceKind: "karate", SourcePath: "tests/alpha.feature"},
			wantIssue: IssueExternalSourceIncomplete,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCase(tt.catalog, tt.candidate)
			if tt.wantIssue == "" {
				if err != nil {
					t.Fatalf("validate runnable active case: %v", err)
				}
				return
			}
			requireValidationIssue(t, err, tt.wantIssue)
		})
	}
}

func TestValidateDraftCaseAllowsExecutionSetupToRemainIncomplete(t *testing.T) {
	catalogValue := catalog.ProfileCatalog{InterfaceNodes: []catalog.InterfaceNode{{ID: "node.alpha"}}}
	err := ValidateCase(catalogValue, catalog.APICase{
		ID: "case.alpha", NodeID: "node.alpha", Status: StatusDraft, SourceKind: "karate",
	})
	if err != nil {
		t.Fatalf("draft case should allow incomplete execution setup: %v", err)
	}
}

func requireValidationIssue(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected validation issue %q", code)
	}
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("validation error type = %T, want *ValidationError: %v", err, err)
	}
	for _, issue := range validationErr.Issues {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("validation issues = %#v, want code %q", validationErr.Issues, code)
}
