package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"open-test-sandbox/internal/store"
)

const environmentWorkflowSkyWalkingTemplateID = "environment.workflow.skywalking.v1"

type workflowAcceptanceReport struct {
	OK               bool                            `json:"ok"`
	TemplateID       string                          `json:"templateId"`
	WorkflowID       string                          `json:"workflowId"`
	ExpectedSteps    int                             `json:"expectedSteps"`
	CompletedSteps   int                             `json:"completedSteps"`
	PassedSteps      int                             `json:"passedSteps"`
	FailedSteps      int                             `json:"failedSteps"`
	TopologyProvider string                          `json:"topologyProvider"`
	Requirements     []workflowAcceptanceRequirement `json:"requirements,omitempty"`
	Steps            []workflowAcceptanceStep        `json:"steps"`
}

type workflowAcceptanceRequirement struct {
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

type workflowAcceptanceStep struct {
	StepID           string `json:"stepId"`
	CaseID           string `json:"caseId"`
	Status           string `json:"status"`
	ElapsedMs        int64  `json:"elapsedMs"`
	EvidenceComplete bool   `json:"evidenceComplete"`
	TopologyComplete bool   `json:"topologyComplete"`
}

func buildWorkflowAcceptanceReport(ctx context.Context, runtime store.Store, report apiCaseBatchRunReport) workflowAcceptanceReport {
	workflowID := strings.TrimSpace(report.WorkflowID)
	acceptance := workflowAcceptanceReport{
		OK:               true,
		TemplateID:       environmentWorkflowSkyWalkingTemplateID,
		WorkflowID:       workflowID,
		ExpectedSteps:    report.Total,
		CompletedSteps:   report.Completed,
		PassedSteps:      report.Passed,
		FailedSteps:      report.Failed,
		TopologyProvider: "skywalking",
		Steps:            make([]workflowAcceptanceStep, 0, len(report.Cases)),
	}
	if workflowID == "" {
		acceptance.OK = false
		return acceptance
	}
	evidenceOK := true
	topologyOK := true
	for _, item := range report.Cases {
		step := workflowAcceptanceStep{
			StepID:    strings.TrimSpace(item.StepID),
			CaseID:    strings.TrimSpace(item.CaseID),
			Status:    strings.TrimSpace(item.Status),
			ElapsedMs: item.ElapsedMs,
		}
		step.EvidenceComplete = workflowAcceptanceCaseEvidenceComplete(ctx, runtime, item.RunID)
		step.TopologyComplete = workflowAcceptanceStepTopologyComplete(ctx, runtime, report.BatchRunID, item.RunID, step.StepID, step.CaseID)
		if !step.EvidenceComplete {
			evidenceOK = false
		}
		if !step.TopologyComplete {
			topologyOK = false
		}
		acceptance.Steps = append(acceptance.Steps, step)
	}
	stepsOK := acceptance.ExpectedSteps > 0 && acceptance.CompletedSteps == acceptance.ExpectedSteps && len(acceptance.Steps) == acceptance.ExpectedSteps
	passedOK := stepsOK && acceptance.PassedSteps == acceptance.ExpectedSteps && acceptance.FailedSteps == 0 && report.Status == store.StatusPassed
	acceptance.Requirements = []workflowAcceptanceRequirement{
		{ID: "workflow-steps", OK: stepsOK, Message: fmt.Sprintf("%d/%d workflow steps completed", acceptance.CompletedSteps, acceptance.ExpectedSteps)},
		{ID: "passed-steps", OK: passedOK, Message: fmt.Sprintf("%d/%d workflow steps passed", acceptance.PassedSteps, acceptance.ExpectedSteps)},
		{ID: "evidence", OK: evidenceOK, Message: "each workflow interface step must have indexed Evidence"},
		{ID: "skywalking-topology", OK: topologyOK, Message: "each workflow step must have complete real SkyWalking topology"},
	}
	for _, requirement := range acceptance.Requirements {
		if !requirement.OK {
			acceptance.OK = false
			break
		}
	}
	return acceptance
}

func workflowAcceptanceCaseEvidenceComplete(ctx context.Context, runtime store.Store, runID string) bool {
	if runtime == nil || strings.TrimSpace(runID) == "" {
		return false
	}
	records, err := runtime.ListEvidence(ctx, runID)
	if err != nil {
		return false
	}
	kinds := map[string]bool{}
	for _, record := range records {
		kinds[strings.TrimSpace(record.Kind)] = true
	}
	for _, required := range []string{"case", "request", "response", "assertions", "summary"} {
		if !kinds[required] {
			return false
		}
	}
	return true
}

func workflowAcceptanceStepTopologyComplete(ctx context.Context, runtime store.Store, batchRunID string, caseRunID string, stepID string, caseID string) bool {
	if runtime == nil {
		return false
	}
	for _, runID := range compactUniqueStringListPreserveOrder([]string{caseRunID, batchRunID}) {
		rows, err := runtime.ListTraceTopologies(ctx, runID)
		if err != nil {
			return false
		}
		for _, row := range rows {
			if workflowAcceptanceTopologyMatches(row, stepID, caseID) && completeSkyWalkingTopologyRow(row) {
				return true
			}
		}
	}
	return false
}

func workflowAcceptanceTopologyMatches(row store.TraceTopology, stepID string, caseID string) bool {
	stepID = strings.TrimSpace(stepID)
	caseID = strings.TrimSpace(caseID)
	rowStepID := strings.TrimSpace(row.StepID)
	rowCaseID := strings.TrimSpace(row.CaseID)
	return (stepID == "" || rowStepID == "" || rowStepID == stepID) && (caseID == "" || rowCaseID == "" || rowCaseID == caseID)
}

func workflowAcceptancePassed(summaryJSON string, workflowID string) error {
	var payload struct {
		Acceptance workflowAcceptanceReport `json:"acceptance"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(summaryJSON)), &payload); err != nil {
		return fmt.Errorf("acceptance report could not be read: %w", err)
	}
	acceptance := payload.Acceptance
	if strings.TrimSpace(acceptance.TemplateID) == "" {
		return fmt.Errorf("acceptance report is required")
	}
	if acceptance.TemplateID != environmentWorkflowSkyWalkingTemplateID {
		return fmt.Errorf("acceptance report template must be %s", environmentWorkflowSkyWalkingTemplateID)
	}
	if workflowID != "" && acceptance.WorkflowID != workflowID {
		return fmt.Errorf("acceptance report workflow %s does not match environment workflow %s", acceptance.WorkflowID, workflowID)
	}
	if !acceptance.OK {
		return fmt.Errorf("acceptance report did not pass")
	}
	if acceptance.TopologyProvider != "skywalking" {
		return fmt.Errorf("acceptance report must use SkyWalking topology")
	}
	if acceptance.ExpectedSteps <= 0 || len(acceptance.Steps) != acceptance.ExpectedSteps {
		return fmt.Errorf("acceptance report must cover every workflow step")
	}
	for _, step := range acceptance.Steps {
		if step.Status != store.StatusPassed || !step.EvidenceComplete || !step.TopologyComplete {
			return fmt.Errorf("acceptance report step %s is incomplete", step.StepID)
		}
	}
	return nil
}
