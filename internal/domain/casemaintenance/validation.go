// Package casemaintenance owns validation rules for maintained API cases.
package casemaintenance

import (
	"encoding/json"
	"fmt"
	"strings"

	"agent-testbench/internal/domain/casesuite"
	"agent-testbench/internal/domain/catalog"
)

const (
	StatusDraft       = casesuite.CaseLifecycleDraft
	StatusReview      = casesuite.CaseLifecycleReview
	StatusActive      = casesuite.CaseLifecycleActive
	StatusQuarantined = casesuite.CaseLifecycleQuarantined
	StatusDeprecated  = casesuite.CaseLifecycleDeprecated
)

const (
	IssueInvalidStatus               = "invalid-status"
	IssueCaseIDRequired              = "case-id-required"
	IssueNodeRequired                = "node-required"
	IssueNodeNotFound                = "node-not-found"
	IssueRequestTemplateNotFound     = "request-template-not-found"
	IssueRequestTemplateNodeMismatch = "request-template-node-mismatch"
	IssueRequestTemplateInactive     = "request-template-inactive"
	IssueExternalSourceIncomplete    = "external-source-incomplete"
	IssueExternalSourcePlanningOnly  = "external-source-planning-only"
	IssueActiveCaseNotRunnable       = "active-case-not-runnable"
)

// Issue is one stable, machine-readable case maintenance validation failure.
type Issue struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// ValidationError reports every case maintenance issue found in one pass.
type ValidationError struct {
	Issues []Issue `json:"issues"`
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "case maintenance validation failed"
	}
	messages := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		messages = append(messages, issue.Message)
	}
	return "case maintenance validation failed: " + strings.Join(messages, "; ")
}

// NormalizeCaseStatus applies the lifecycle default used by maintenance
// writes. New cases start in draft; legacy existing cases with an empty status
// retain the historical active meaning.
func NormalizeCaseStatus(status string, isNew bool) (string, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		if isNew {
			return StatusDraft, nil
		}
		return StatusActive, nil
	}
	if casesuite.IsKnownCaseLifecycle(status) {
		return status, nil
	}
	return "", &ValidationError{Issues: []Issue{{
		Code:    IssueInvalidStatus,
		Field:   "status",
		Message: fmt.Sprintf("unsupported case status %q; use draft, review, active, quarantined, or deprecated", status),
	}}}
}

// ValidateCase enforces catalog references for every maintained case and the
// stronger runnable-source invariant for active cases.
func ValidateCase(catalogValue catalog.ProfileCatalog, candidate catalog.APICase) error {
	issues := validateCaseReferences(catalogValue, candidate)
	status, statusErr := NormalizeCaseStatus(candidate.Status, false)
	if statusErr != nil {
		var validationErr *ValidationError
		if ok := asValidationError(statusErr, &validationErr); ok {
			issues = append(issues, validationErr.Issues...)
		}
	}
	if status == StatusActive {
		issues = append(issues, validateActiveCase(catalogValue, candidate)...)
	}
	if len(issues) == 0 {
		return nil
	}
	return &ValidationError{Issues: issues}
}

// ExecutionReady reports whether a case can be executed by the current local
// workbench runtime. External executor descriptors are planning metadata until
// the product has an explicit executor-run contract.
func ExecutionReady(catalogValue catalog.ProfileCatalog, candidate catalog.APICase) bool {
	status, err := NormalizeCaseStatus(candidate.Status, false)
	return err == nil && status == StatusActive && ValidateCase(catalogValue, candidate) == nil
}

func asValidationError(err error, target **ValidationError) bool {
	value, ok := err.(*ValidationError)
	if ok {
		*target = value
	}
	return ok
}

func validateCaseReferences(catalogValue catalog.ProfileCatalog, candidate catalog.APICase) []Issue {
	issues := []Issue{}
	caseID := strings.TrimSpace(candidate.ID)
	if caseID == "" {
		issues = append(issues, Issue{Code: IssueCaseIDRequired, Field: "id", Message: "case id is required"})
	}
	nodeID := strings.TrimSpace(candidate.NodeID)
	if nodeID == "" {
		issues = append(issues, Issue{Code: IssueNodeRequired, Field: "nodeId", Message: "case node id is required"})
	} else if !catalogHasNode(catalogValue, nodeID) {
		issues = append(issues, Issue{Code: IssueNodeNotFound, Field: "nodeId", Message: fmt.Sprintf("case node %q was not found", nodeID)})
	}
	templateID := strings.TrimSpace(candidate.RequestTemplateID)
	if templateID == "" {
		return issues
	}
	template, ok := catalogRequestTemplate(catalogValue, templateID)
	if !ok {
		return append(issues, Issue{Code: IssueRequestTemplateNotFound, Field: "requestTemplateId", Message: fmt.Sprintf("request template %q was not found", templateID)})
	}
	if templateNodeID := strings.TrimSpace(template.NodeID); templateNodeID != "" && nodeID != "" && templateNodeID != nodeID {
		issues = append(issues, Issue{
			Code: IssueRequestTemplateNodeMismatch, Field: "requestTemplateId",
			Message: fmt.Sprintf("request template %q belongs to node %q, not %q", templateID, templateNodeID, nodeID),
		})
	}
	return issues
}

func validateActiveCase(catalogValue catalog.ProfileCatalog, candidate catalog.APICase) []Issue {
	issues := []Issue{}
	templateRunnable := false
	if templateID := strings.TrimSpace(candidate.RequestTemplateID); templateID != "" {
		if template, ok := catalogRequestTemplate(catalogValue, templateID); ok {
			templateRunnable = activeStatus(template.Status)
			if !templateRunnable {
				issues = append(issues, Issue{
					Code: IssueRequestTemplateInactive, Field: "requestTemplateId",
					Message: fmt.Sprintf("request template %q is not active", templateID),
				})
			}
		}
	}
	externalFields := []string{strings.TrimSpace(candidate.SourceKind), strings.TrimSpace(candidate.SourcePath), strings.TrimSpace(candidate.ExecutorID)}
	externalCount := 0
	for _, value := range externalFields {
		if value != "" {
			externalCount++
		}
	}
	externalComplete := externalCount == len(externalFields)
	if externalCount > 0 && !externalComplete {
		issues = append(issues, Issue{
			Code: IssueExternalSourceIncomplete, Field: "source",
			Message: "active external cases require sourceKind, sourcePath, and executorId",
		})
	}
	runnable := strings.TrimSpace(candidate.CasePath) != "" ||
		templateRunnable ||
		hasActiveCaseExecutionConfig(catalogValue, strings.TrimSpace(candidate.ID))
	if externalComplete && !runnable {
		issues = append(issues, Issue{
			Code: IssueExternalSourcePlanningOnly, Field: "source",
			Message: "external executor cases are planning-only; add a case file, active request template, or active execution config for local execution",
		})
	}
	if !runnable {
		issues = append(issues, Issue{
			Code: IssueActiveCaseNotRunnable, Field: "status",
			Message: "active cases require a case file, active request template, or active execution config",
		})
	}
	return issues
}

func catalogHasNode(catalogValue catalog.ProfileCatalog, id string) bool {
	for _, node := range catalogValue.InterfaceNodes {
		if strings.TrimSpace(node.ID) == id {
			return true
		}
	}
	return false
}

func catalogRequestTemplate(catalogValue catalog.ProfileCatalog, id string) (catalog.RequestTemplate, bool) {
	for _, template := range catalogValue.RequestTemplates {
		if strings.TrimSpace(template.ID) == id {
			return template, true
		}
	}
	return catalog.RequestTemplate{}, false
}

func hasActiveCaseExecutionConfig(catalogValue catalog.ProfileCatalog, caseID string) bool {
	if caseID == "" {
		return false
	}
	for _, config := range catalogValue.TemplateConfigs {
		if !activeStatus(config.Status) || !ExecutionConfigTargetsCase(config.ScopeType, config.ScopeID, config.ConfigJSON, caseID) {
			continue
		}
		var payload struct {
			CaseExecution map[string]any `json:"caseExecution"`
		}
		if json.Unmarshal([]byte(config.ConfigJSON), &payload) != nil {
			continue
		}
		if executionHasRequestTarget(payload.CaseExecution) {
			return true
		}
	}
	return false
}

// ExecutionConfigTargetsCase is the shared scope rule used by maintenance
// validation and runtime config selection.
func ExecutionConfigTargetsCase(scopeType string, scopeID string, configJSON string, caseID string) bool {
	var payload struct {
		CaseID string `json:"caseId"`
	}
	if json.Unmarshal([]byte(configJSON), &payload) != nil {
		return false
	}
	configuredCaseID := strings.TrimSpace(payload.CaseID)
	if configuredCaseID == "" && caseScopedConfig(scopeType) {
		configuredCaseID = strings.TrimSpace(scopeID)
	}
	return configuredCaseID != "" && configuredCaseID == strings.TrimSpace(caseID)
}

func executionHasRequestTarget(execution map[string]any) bool {
	return stringValue(execution["method"]) != "" ||
		stringValue(execution["path"]) != "" ||
		stringValue(execution["nodeId"]) != ""
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func activeStatus(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	return status == "" || status == StatusActive
}

func caseScopedConfig(scopeType string) bool {
	switch strings.ToLower(strings.TrimSpace(scopeType)) {
	case "", "case", "api-case":
		return true
	default:
		return false
	}
}
