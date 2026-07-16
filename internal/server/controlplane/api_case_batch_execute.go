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
		if err := r.requireOwnerFence(ctx, runtime, batchRunID); err != nil {
			return
		}
		caseCtx, cancel := context.WithTimeout(ctx, time.Duration(safeAPICaseBatchExecutionTimeoutSeconds(plan.TimeoutSeconds))*time.Second)
		casePath := plan.CasePath
		baseURL := plan.BaseURL
		overrides := mergeStringAnyMaps(workflowOverrides, plan.Overrides)
		if plan.Execution != nil {
			plan.Overrides = overrides
			materializationStarted := time.Now().UTC()
			materializedPath, materializedBaseURL, err := materializeAPICaseBatchExecution(caseCtx, bundle, runtime, batchRunID, workflowID, plan)
			if err != nil {
				cancel()
				if !r.recordMaterializationFailure(ctx, runtime, batchRunID, bundle.ID, environmentID, workflowID, index, plan, materializationStarted, err, rules) {
					return
				}
				continue
			}
			casePath = materializedPath
			baseURL = materializedBaseURL
			overrides = nil
		}
		if err := r.requireOwnerFence(ctx, runtime, batchRunID); err != nil {
			cancel()
			return
		}
		caseRunStarted := time.Now().UTC()
		result, err := apicase.Run(caseCtx, apicase.RunOptions{
			CasePath:    casePath,
			BaseURL:     baseURL,
			EvidenceDir: plan.EvidenceDir,
			RunID:       apiCaseBatchCaseRunID(batchRunID, plan.StepID, plan.ID),
			Overrides:   overrides,
			BeforeEvidenceWrite: func(context.Context) error {
				return r.requireOwnerFence(ctx, runtime, batchRunID)
			},
		})
		cancel()
		item := newAPICaseBatchCaseReport(plan)
		if err != nil {
			if strings.TrimSpace(result.RunID) == "" {
				if !r.recordMaterializationFailure(ctx, runtime, batchRunID, bundle.ID, environmentID, workflowID, index, plan, caseRunStarted, err, rules) {
					return
				}
				continue
			}
			applyAPICaseBatchRunResult(&item, result, rules)
			if result.FailureCategory == apicase.FailureCategoryEvidenceWrite {
				r.failCasePersistence(ctx, runtime, batchRunID, index, item, err)
				return
			}
		} else {
			applyAPICaseBatchRunResult(&item, result, rules)
			if runtime != nil {
				caseRuntime := r.fencedStore(runtime, batchRunID)
				if err := recordAPICaseRunWithContext(ctx, caseRuntime, recordAPICaseRunContext{
					ProfileID:     bundle.ID,
					EnvironmentID: environmentID,
					WorkflowID:    workflowID,
					StepID:        plan.StepID,
				}, result); err != nil {
					r.failCasePersistence(ctx, runtime, batchRunID, index, item, err)
					return
				}
				if item.Status == store.StatusPassed && strings.TrimSpace(workflowID) != "" && strings.TrimSpace(plan.StepID) != "" {
					traceRuntime := r.fencedStore(runtime, batchRunID)
					collectAPICaseBatchTraceTopology(ctx, traceRuntime, collector, workflowID, plan, result)
					if err := traceRuntime.FenceError(); err != nil {
						return
					}
				}
			}
			if item.Status == store.StatusPassed {
				workflowOverrides = mergeStringAnyMaps(workflowOverrides, apiCaseBatchEvidenceOverridesForPlan(plan, result.EvidencePath))
			}
		}
		if err := r.updateCase(ctx, runtime, batchRunID, index, item); err != nil {
			return
		}
	}
	r.finish(ctx, batchRunID, runtime)
}

func (r *apiCaseBatchRunner) recordMaterializationFailure(ctx context.Context, runtime store.Store, batchRunID string, profileID string, environmentID string, workflowID string, index int, plan apiCaseBatchCasePlan, startedAt time.Time, cause error, rules []profile.FailureCategoryRule) bool {
	if err := r.requireOwnerFence(ctx, runtime, batchRunID); err != nil {
		return false
	}
	fencedRuntime := runtime
	if runtime != nil {
		fencedRuntime = r.fencedStore(runtime, batchRunID)
	}
	result, persistenceErr := recordAPICaseBatchMaterializationFailure(ctx, fencedRuntime, recordAPICaseRunContext{
		ProfileID:     profileID,
		EnvironmentID: environmentID,
		WorkflowID:    workflowID,
		StepID:        plan.StepID,
	}, plan, apiCaseBatchCaseRunID(batchRunID, plan.StepID, plan.ID), startedAt, cause, func(context.Context) error {
		return r.requireOwnerFence(ctx, runtime, batchRunID)
	})
	item := newAPICaseBatchCaseReport(plan)
	applyAPICaseBatchRunResult(&item, result, rules)
	if persistenceErr != nil {
		r.failCasePersistence(ctx, runtime, batchRunID, index, item, persistenceErr)
		return false
	}
	return r.updateCase(ctx, runtime, batchRunID, index, item) == nil
}
