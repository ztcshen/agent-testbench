package controlplane

import (
	"context"
	"strings"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/runner/apicase"
	"agent-testbench/internal/store"
)

func (r *apiCaseBatchRunner) run(ctx context.Context, batchRunID string, bundle profile.Bundle, environmentID string, workflowID string, plans []apiCaseBatchCasePlan, runtime store.Store, rules []profile.FailureCategoryRule, collector traceCollector) {
	workflowOverrides := map[string]any{}
	for index, plan := range plans {
		caseCtx := ctx
		var cancel context.CancelFunc
		if plan.TimeoutSeconds > 0 {
			caseCtx, cancel = context.WithTimeout(ctx, time.Duration(plan.TimeoutSeconds)*time.Second)
		}
		casePath := plan.CasePath
		baseURL := plan.BaseURL
		overrides := mergeStringAnyMaps(workflowOverrides, plan.Overrides)
		if plan.Execution != nil {
			plan.Overrides = overrides
			materializedPath, materializedBaseURL, err := materializeAPICaseBatchExecution(caseCtx, bundle, runtime, batchRunID, workflowID, plan)
			if err != nil {
				if cancel != nil {
					cancel()
				}
				item := apiCaseBatchCaseReport{
					CaseID:          plan.ID,
					DisplayName:     plan.DisplayName,
					Scenario:        plan.Scenario,
					NodeID:          plan.NodeID,
					NodeDisplayName: plan.NodeDisplayName,
					Operation:       plan.Operation,
					Method:          plan.Method,
					Path:            plan.Path,
					StepID:          plan.StepID,
					Status:          store.StatusFailed,
					Error:           err.Error(),
				}
				item.FailureCategory = apiCaseBatchApplyFailureCategoryRules(rules, item.Status, apiCaseBatchFailureCategoryFromError(err), item.Error)
				r.updateCase(batchRunID, index, item)
				continue
			}
			casePath = materializedPath
			baseURL = materializedBaseURL
			overrides = nil
		}
		result, err := apicase.Run(caseCtx, apicase.RunOptions{
			CasePath:    casePath,
			BaseURL:     baseURL,
			EvidenceDir: plan.EvidenceDir,
			RunID:       apiCaseBatchCaseRunID(batchRunID, plan.StepID, plan.ID),
			Overrides:   overrides,
		})
		if cancel != nil {
			cancel()
		}
		item := apiCaseBatchCaseReport{
			CaseID:          plan.ID,
			DisplayName:     plan.DisplayName,
			Scenario:        plan.Scenario,
			NodeID:          plan.NodeID,
			NodeDisplayName: plan.NodeDisplayName,
			Operation:       plan.Operation,
			Method:          plan.Method,
			Path:            plan.Path,
			StepID:          plan.StepID,
			Status:          store.StatusFailed,
		}
		if err != nil {
			item.Error = err.Error()
			item.FailureCategory = apiCaseBatchApplyFailureCategoryRules(rules, item.Status, apiCaseBatchFailureCategoryFromError(err), item.Error)
		} else {
			item.RunID = result.RunID
			item.CaseRunID = apiCaseRunRecordID(result.RunID)
			item.Status = result.Status
			item.ViewerURL = apiCaseViewerURL(result)
			item.DetailURL = apiCaseEvidenceDetailURL(item.CaseRunID)
			item.EvidencePath = result.EvidencePath
			item.ElapsedMs = result.ElapsedMs
			item.StartedAt = result.StartedAt
			item.FinishedAt = result.FinishedAt
			item.Error = apiCaseBatchFailureMessage(result)
			item.FailureCategory = apiCaseBatchApplyFailureCategoryRules(rules, item.Status, apiCaseBatchFailureCategory(result), item.Error)
			if runtime != nil {
				if err := recordAPICaseRunWithContext(ctx, runtime, recordAPICaseRunContext{
					ProfileID:     bundle.ID,
					EnvironmentID: environmentID,
					WorkflowID:    workflowID,
					StepID:        plan.StepID,
				}, result); err != nil {
					item.Status = store.StatusFailed
					item.Error = err.Error()
					item.FailureCategory = apiCaseBatchApplyFailureCategoryRules(rules, item.Status, apiCaseBatchFailureCategoryFromError(err), item.Error)
				}
				if item.Status == store.StatusPassed && strings.TrimSpace(workflowID) != "" && strings.TrimSpace(plan.StepID) != "" {
					collectAPICaseBatchTraceTopology(ctx, runtime, collector, workflowID, plan, result)
				}
			}
			if item.Status == store.StatusPassed {
				workflowOverrides = mergeStringAnyMaps(workflowOverrides, apiCaseBatchEvidenceOverridesForPlan(plan, result.EvidencePath))
			}
		}
		r.updateCase(batchRunID, index, item)
	}
	r.finish(ctx, batchRunID, bundle.ID, workflowID, runtime)
}
