package controlplane

import (
	"context"
	"errors"
	"fmt"
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

func (r *apiCaseBatchRunner) updateCase(ctx context.Context, runtime store.Store, batchRunID string, index int, item apiCaseBatchCaseReport) error {
	report, ok := r.get(batchRunID)
	if !ok {
		return fmt.Errorf("batch run %q is not loaded", batchRunID)
	}
	if index >= 0 && index < len(report.Cases) {
		report.Cases[index] = item
	}
	refreshAPICaseBatchCounts(&report)
	if err := r.requireOwnerFence(ctx, runtime, batchRunID); err != nil {
		return err
	}
	if err := r.writeReports(ctx, runtime, batchRunID, &report); err != nil {
		if errors.Is(err, errAPICaseBatchLeaseLost) {
			r.remove(batchRunID)
			return err
		}
		r.save(report)
		r.abortPersistence(ctx, runtime, batchRunID, err)
		return err
	}
	if err := r.checkpoint(ctx, runtime, report); err != nil {
		if errors.Is(err, errAPICaseBatchLeaseLost) {
			r.remove(batchRunID)
			return err
		}
		r.save(report)
		r.abortPersistence(ctx, runtime, batchRunID, err)
		return err
	}
	r.save(report)
	return nil
}

func (r *apiCaseBatchRunner) finish(ctx context.Context, batchRunID string, runtime store.Store) {
	report, ok := r.get(batchRunID)
	if !ok {
		return
	}
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
	if err := r.requireOwnerFence(ctx, runtime, batchRunID); err != nil {
		return
	}
	if err := r.writeReports(ctx, runtime, batchRunID, &report); err != nil {
		if errors.Is(err, errAPICaseBatchLeaseLost) {
			r.remove(batchRunID)
			return
		}
		r.save(report)
		r.abortPersistence(ctx, runtime, batchRunID, err)
		return
	}
	if err := r.requireOwnerFence(ctx, runtime, batchRunID); err != nil {
		return
	}
	reportRuntime := runtime
	if runtime != nil {
		reportRuntime = r.fencedStore(runtime, batchRunID)
	}
	if err := recordAPICaseBatchReportArtifacts(ctx, reportRuntime, report); err != nil {
		r.failFinalPersistence(ctx, runtime, report, err)
		return
	}
	if strings.TrimSpace(report.WorkflowID) != "" {
		if err := r.requireOwnerFence(ctx, runtime, batchRunID); err != nil {
			return
		}
		traceRuntime := runtime
		if runtime != nil {
			traceRuntime = r.fencedStore(runtime, batchRunID)
		}
		if err := copyAPICaseBatchTraceTopologies(ctx, traceRuntime, report); err != nil {
			r.failFinalPersistence(ctx, runtime, report, err)
			return
		}
	}
	if err := r.requireOwnerFence(ctx, runtime, batchRunID); err != nil {
		return
	}
	environmentRuntime := runtime
	if runtime != nil {
		environmentRuntime = r.fencedStore(runtime, batchRunID)
	}
	if err := finalizeEnvironmentAcceptanceRun(ctx, environmentRuntime, report); err != nil {
		r.failFinalPersistence(ctx, runtime, report, err)
		return
	}
	if err := r.checkpoint(ctx, runtime, report); err != nil {
		if errors.Is(err, errAPICaseBatchLeaseLost) {
			r.remove(batchRunID)
			return
		}
		r.abortPersistence(ctx, runtime, batchRunID, err)
		return
	}
	r.save(report)
}

func (r *apiCaseBatchRunner) failFinalPersistence(ctx context.Context, runtime store.Store, report apiCaseBatchRunReport, cause error) {
	report.OK = false
	report.Status = store.StatusFailed
	report.FailureCategory = apiCaseBatchPersistenceFailureCategory
	appendAPICaseBatchReportError(&report, cause.Error())
	if err := r.requireOwnerFence(ctx, runtime, report.BatchRunID); err != nil {
		return
	}
	if err := r.writeReports(ctx, runtime, report.BatchRunID, &report); err != nil && errors.Is(err, errAPICaseBatchLeaseLost) {
		r.remove(report.BatchRunID)
		return
	}
	if checkpointErr := r.checkpoint(ctx, runtime, report); checkpointErr != nil {
		if errors.Is(checkpointErr, errAPICaseBatchLeaseLost) {
			r.remove(report.BatchRunID)
			return
		}
		appendAPICaseBatchReportError(&report, checkpointErr.Error())
		r.save(report)
		return
	}
	r.save(report)
}

func (r *apiCaseBatchRunner) abortPersistence(ctx context.Context, runtime store.Store, batchRunID string, cause error) {
	report, ok := r.persistenceFailureReport(batchRunID, cause)
	if !ok {
		return
	}
	if err := r.writeReports(ctx, runtime, batchRunID, &report); err != nil && errors.Is(err, errAPICaseBatchLeaseLost) {
		r.remove(batchRunID)
		return
	}
	r.save(report)
}

func (r *apiCaseBatchRunner) markPersistenceFailure(batchRunID string, cause error) (apiCaseBatchRunReport, bool) {
	report, ok := r.persistenceFailureReport(batchRunID, cause)
	if !ok {
		return apiCaseBatchRunReport{}, false
	}
	r.save(report)
	return report, true
}

func (r *apiCaseBatchRunner) persistenceFailureReport(batchRunID string, cause error) (apiCaseBatchRunReport, bool) {
	report, ok := r.get(batchRunID)
	if !ok {
		return apiCaseBatchRunReport{}, false
	}
	message := fmt.Sprintf("batch persistence failed; remaining cases were not executed: %v", cause)
	report.OK = false
	report.Status = store.StatusFailed
	report.FailureCategory = apiCaseBatchPersistenceFailureCategory
	report.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	appendAPICaseBatchReportError(&report, message)
	for index := range report.Cases {
		if report.Cases[index].Status != store.StatusRunning {
			continue
		}
		report.Cases[index].Status = store.StatusSkipped
		report.Cases[index].FailureCategory = apiCaseBatchPersistenceFailureCategory
		report.Cases[index].Error = message
	}
	refreshAPICaseBatchCounts(&report)
	return report, true
}

func (r *apiCaseBatchRunner) writeReports(ctx context.Context, runtime store.Store, batchRunID string, report *apiCaseBatchRunReport) error {
	var failures []error
	for _, writeReport := range r.reportWriters {
		if err := r.requireOwnerFence(ctx, runtime, batchRunID); err != nil {
			appendAPICaseBatchReportError(report, err.Error())
			failures = append(failures, err)
			break
		}
		if err := writeReport(*report); err != nil {
			appendAPICaseBatchReportError(report, err.Error())
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func defaultAPICaseBatchReportWriters() []func(apiCaseBatchRunReport) error {
	return []func(apiCaseBatchRunReport) error{
		writeAPICaseBatchHTMLReport,
		writeAPICaseBatchJUnitReport,
		writeAPICaseBatchArtifactManifest,
		writeAPICaseBatchFailureSummary,
	}
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
