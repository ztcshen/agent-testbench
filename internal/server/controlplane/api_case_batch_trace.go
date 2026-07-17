package controlplane

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"agent-testbench/internal/runner/apicase"
	"agent-testbench/internal/store"
)

func collectAPICaseBatchTraceTopology(ctx context.Context, runtime store.Store, collector traceCollector, workflowID string, plan apiCaseBatchCasePlan, result apicase.RunResult) {
	if runtime == nil || result.Status != store.StatusPassed {
		return
	}
	request, _ := jsonFileObject(filepath.Join(result.EvidencePath, apiCaseEvidenceFileRequest))
	response, _ := jsonFileObject(filepath.Join(result.EvidencePath, apiCaseEvidenceFileResponse))
	payload := map[string]any{
		"workflowId": workflowID,
		"stepId":     plan.StepID,
	}
	if plan.Execution != nil && strings.TrimSpace(plan.Execution.TraceEndpoint) != "" {
		payload["traceEndpoint"] = plan.Execution.TraceEndpoint
	}
	resultPayload := map[string]any{
		"ok":         true,
		"caseId":     result.CaseID,
		"stepId":     plan.StepID,
		"startedAt":  result.StartedAt,
		"finishedAt": result.FinishedAt,
		"result": map[string]any{
			"request":  request,
			"response": response,
		},
	}
	collectAndRecordTestKitTraceTopology(ctx, runtime, collector, result.RunID, payload, resultPayload)
}

func copyAPICaseBatchTraceTopologies(ctx context.Context, runtime store.Store, report apiCaseBatchRunReport) error {
	if runtime == nil || strings.TrimSpace(report.BatchRunID) == "" {
		return nil
	}
	var failures []error
	for _, item := range report.Cases {
		sourceRunID := strings.TrimSpace(item.RunID)
		if sourceRunID == "" {
			continue
		}
		rows, err := runtime.ListTraceTopologies(ctx, sourceRunID)
		if err != nil {
			failures = append(failures, fmt.Errorf("list trace topologies for case %s: %w", item.CaseID, err))
			continue
		}
		for _, row := range rows {
			if !isSkyWalkingTraceTopology(row) {
				continue
			}
			copied := row
			copied.ID = report.BatchRunID + "." + safeRuntimeLogPathSegment(firstNonEmpty(item.StepID, row.StepID, item.CaseID, row.CaseID)) + ".topology.skywalking"
			copied.WorkflowRunID = report.BatchRunID
			copied.WorkflowID = firstNonEmpty(report.WorkflowID, row.WorkflowID)
			copied.StepID = firstNonEmpty(item.StepID, row.StepID)
			copied.CaseID = firstNonEmpty(item.CaseID, row.CaseID)
			copied.CreatedAt = time.Now().UTC()
			if _, err := runtime.SaveTraceTopology(ctx, copied); err != nil {
				failures = append(failures, fmt.Errorf("save trace topology for case %s: %w", item.CaseID, err))
				continue
			}
		}
	}
	return errors.Join(failures...)
}
