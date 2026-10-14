package controlplane

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/runner/apicase"
	"agent-testbench/internal/store"
)

const apiCaseBatchPersistenceFailurePhase = "persistence"

func recordAPICaseBatchMaterializationFailure(ctx context.Context, runtime store.Store, runContext recordAPICaseRunContext, plan apiCaseBatchCasePlan, runID string, startedAt time.Time, cause error, beforeEvidenceWrite func(context.Context) error) (apicase.RunResult, error) {
	result, err := apicase.WriteFailureEvidence(apicase.FailureEvidenceOptions{
		Context:             ctx,
		RunID:               runID,
		EvidenceDir:         plan.EvidenceDir,
		Case:                apiCaseBatchMaterializationFailureCase(plan),
		Phase:               apicase.FailurePhaseMaterialization,
		Category:            apicase.FailureCategoryMaterialization,
		Message:             cause.Error(),
		StartedAt:           startedAt,
		BeforeEvidenceWrite: beforeEvidenceWrite,
	})
	if err != nil {
		return result, fmt.Errorf("write materialization failure Evidence for case %s: %w", plan.ID, err)
	}
	if runtime == nil {
		return result, nil
	}
	if err := recordAPICaseRunWithContext(ctx, runtime, runContext, result); err != nil {
		return result, fmt.Errorf("persist materialization failure for case %s: %w", plan.ID, err)
	}
	return result, nil
}

func apiCaseBatchMaterializationFailureCase(plan apiCaseBatchCasePlan) apicase.Case {
	method := strings.TrimSpace(plan.Method)
	path := strings.TrimSpace(plan.Path)
	assertions := apicase.Assertions{}
	if plan.Execution != nil {
		method = firstNonEmpty(method, plan.Execution.Method)
		path = firstNonEmpty(path, plan.Execution.Path)
		assertions.ExpectedStatusCodes = append([]int(nil), plan.Execution.ExpectedHTTPCodes...)
		assertions.ResponseContains = append([]string(nil), plan.Execution.ExpectedResponse...)
		assertions.ResponseNotContains = append([]string(nil), plan.Execution.ExpectedResponseAbsent...)
	}
	return apicase.Case{
		ID:    plan.ID,
		Title: firstNonEmpty(plan.DisplayName, plan.ID),
		Request: apicase.Request{
			Method: firstNonEmpty(method, http.MethodGet),
			Path:   firstNonEmpty(path, "/"),
		},
		Assertions: assertions,
	}
}

func newAPICaseBatchCaseReport(plan apiCaseBatchCasePlan) apiCaseBatchCaseReport {
	return apiCaseBatchCaseReport{
		CaseID:          plan.ID,
		DisplayName:     plan.DisplayName,
		Scenario:        plan.Scenario,
		NodeID:          plan.NodeID,
		NodeDisplayName: plan.NodeDisplayName,
		Operation:       plan.Operation,
		Method:          plan.Method,
		Path:            plan.Path,
		StepID:          plan.StepID,
		TimeoutSeconds:  plan.TimeoutSeconds,
		Status:          store.StatusFailed,
	}
}

func applyAPICaseBatchRunResult(item *apiCaseBatchCaseReport, result apicase.RunResult, rules []profile.FailureCategoryRule) {
	item.RunID = result.RunID
	item.CaseRunID = apiCaseRunRecordID(result.RunID)
	item.Status = result.Status
	item.ViewerURL = apiCaseViewerURL(result)
	item.DetailURL = apiCaseEvidenceDetailURL(item.CaseRunID)
	item.EvidencePath = result.EvidencePath
	item.ElapsedMs = result.ElapsedMs
	item.StartedAt = result.StartedAt
	item.FinishedAt = result.FinishedAt
	item.FailurePhase = result.FailurePhase
	item.Error = apiCaseBatchFailureMessage(result)
	item.FailureCategory = apiCaseBatchApplyFailureCategoryRules(rules, item.Status, apiCaseBatchFailureCategory(result), item.Error)
}

func (r *apiCaseBatchRunner) failCasePersistence(ctx context.Context, runtime store.Store, batchRunID string, index int, item apiCaseBatchCaseReport, cause error) {
	message := fmt.Sprintf("batch persistence failed while recording case %s: %v", item.CaseID, cause)
	item.Status = store.StatusFailed
	item.FailurePhase = apiCaseBatchPersistenceFailurePhase
	item.FailureCategory = apiCaseBatchPersistenceFailureCategory
	item.Error = message
	if err := r.updateCase(ctx, runtime, batchRunID, index, item); err != nil {
		return
	}
	if err := r.requireOwnerFence(ctx, runtime, batchRunID); err != nil {
		return
	}
	report, ok := r.persistenceFailureReport(batchRunID, cause)
	if !ok {
		return
	}
	if err := r.writeReports(ctx, runtime, batchRunID, &report); err != nil && errors.Is(err, errAPICaseBatchLeaseLost) {
		r.remove(batchRunID)
		return
	}
	if err := r.checkpoint(ctx, runtime, report); err != nil {
		if errors.Is(err, errAPICaseBatchLeaseLost) {
			r.remove(batchRunID)
			return
		}
		appendAPICaseBatchReportError(&report, err.Error())
		r.save(report)
		return
	}
	r.save(report)
}
