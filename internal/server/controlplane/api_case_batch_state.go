package controlplane

import (
	"context"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

func (r *apiCaseBatchRunner) save(report apiCaseBatchRunReport) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[report.BatchRunID] = cloneAPICaseBatchReport(report)
}

func (r *apiCaseBatchRunner) get(id string) (apiCaseBatchRunReport, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	report, ok := r.runs[id]
	return cloneAPICaseBatchReport(report), ok
}

func (r *apiCaseBatchRunner) updateCase(batchRunID string, index int, item apiCaseBatchCaseReport) {
	r.mu.Lock()
	defer r.mu.Unlock()
	report := r.runs[batchRunID]
	if index >= 0 && index < len(report.Cases) {
		report.Cases[index] = item
	}
	refreshAPICaseBatchCounts(&report)
	_ = writeAPICaseBatchHTMLReport(report)
	_ = writeAPICaseBatchJUnitReport(report)
	_ = writeAPICaseBatchArtifactManifest(report)
	_ = writeAPICaseBatchFailureSummary(report)
	r.runs[batchRunID] = report
}

func (r *apiCaseBatchRunner) finish(ctx context.Context, batchRunID string, profileID string, workflowID string, runtime store.Store) {
	r.mu.Lock()
	report := r.runs[batchRunID]
	refreshAPICaseBatchCounts(&report)
	if report.Failed > 0 {
		report.Status = store.StatusFailed
		report.OK = false
	} else {
		report.Status = store.StatusPassed
		report.OK = true
	}
	report.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if strings.TrimSpace(report.WorkflowID) != "" {
		report.Acceptance = buildWorkflowAcceptanceReport(ctx, runtime, report)
	}
	_ = writeAPICaseBatchHTMLReport(report)
	_ = writeAPICaseBatchJUnitReport(report)
	_ = writeAPICaseBatchArtifactManifest(report)
	_ = writeAPICaseBatchFailureSummary(report)
	recordAPICaseBatchReportArtifacts(ctx, runtime, profileID, workflowID, report)
	if strings.TrimSpace(report.WorkflowID) != "" {
		copyAPICaseBatchTraceTopologies(ctx, runtime, report)
	}
	finalizeEnvironmentAcceptanceRun(ctx, runtime, report)
	r.runs[batchRunID] = report
	r.mu.Unlock()
}

func refreshAPICaseBatchCounts(report *apiCaseBatchRunReport) {
	report.Completed = 0
	report.Passed = 0
	report.Failed = 0
	report.Skipped = 0
	for _, item := range report.Cases {
		switch item.Status {
		case store.StatusPassed:
			report.Completed++
			report.Passed++
		case store.StatusFailed:
			report.Completed++
			report.Failed++
		case store.StatusSkipped:
			report.Completed++
			report.Skipped++
		}
	}
}

func cloneAPICaseBatchReport(report apiCaseBatchRunReport) apiCaseBatchRunReport {
	report.NodeIDs = append([]string(nil), report.NodeIDs...)
	if report.Suite != nil {
		suite := *report.Suite
		suite.Tags = append([]string(nil), report.Suite.Tags...)
		suite.RunStates = append([]string(nil), report.Suite.RunStates...)
		report.Suite = &suite
	}
	report.Nodes = append([]apiCaseBatchNodeReport(nil), report.Nodes...)
	report.Cases = append([]apiCaseBatchCaseReport(nil), report.Cases...)
	report.Acceptance.Steps = append([]workflowAcceptanceStep(nil), report.Acceptance.Steps...)
	report.Acceptance.Requirements = append([]workflowAcceptanceRequirement(nil), report.Acceptance.Requirements...)
	return report
}
