package controlplane

import (
	"context"
	"fmt"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

func materializeAPICaseBatchWorkflowParent(ctx context.Context, runtime store.Store, report apiCaseBatchRunReport) error {
	if runtime == nil || strings.TrimSpace(report.WorkflowID) == "" || strings.TrimSpace(report.BatchRunID) == "" {
		return nil
	}
	finishedAt := parseAPICaseBatchReportTime(report.FinishedAt, time.Now().UTC())
	payload := apiCaseBatchRunStoreSummary(report)
	if err := recordWorkflowRunStepCases(ctx, runtime, report.BatchRunID, payload, finishedAt); err != nil {
		return fmt.Errorf("record workflow batch parent case runs: %w", err)
	}
	if err := copyWorkflowRunStepEvidence(ctx, runtime, report.BatchRunID, payload, finishedAt); err != nil {
		return fmt.Errorf("copy workflow batch parent Evidence: %w", err)
	}
	if err := copyWorkflowRunStepPostProcessTasks(ctx, runtime, report.BatchRunID, report.WorkflowID, payload, finishedAt); err != nil {
		return fmt.Errorf("copy workflow batch parent post-process tasks: %w", err)
	}
	return nil
}
