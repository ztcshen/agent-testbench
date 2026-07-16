package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

const (
	apiCaseBatchPersistenceFailureCategory = "batch-persistence-error"
	apiCaseBatchInterruptedFailureCategory = "interrupted-unknown-outcome"
)

func createAPICaseBatchRunParent(ctx context.Context, runtime store.Store, report apiCaseBatchRunReport) error {
	if runtime == nil {
		return nil
	}
	if _, ok := runtime.(store.RunCompareAndSwapStore); !ok {
		return fmt.Errorf("persist running batch %q: Store does not support compare-and-swap run checkpoints", report.BatchRunID)
	}
	if _, err := runtime.CreateRun(ctx, newAPICaseBatchStoreRun(report)); err != nil {
		return fmt.Errorf("persist running batch %q: %w", report.BatchRunID, err)
	}
	return nil
}

func checkpointAPICaseBatchRun(ctx context.Context, runtime store.Store, report apiCaseBatchRunReport) error {
	if runtime == nil {
		return nil
	}
	updater, ok := runtime.(store.RunCompareAndSwapStore)
	if !ok {
		return fmt.Errorf("checkpoint batch %q: Store does not support compare-and-swap run updates", report.BatchRunID)
	}
	run, err := runtime.GetRun(ctx, report.BatchRunID)
	if err != nil {
		return fmt.Errorf("load batch %q for checkpoint: %w", report.BatchRunID, err)
	}
	currentLease, err := decodeAPICaseBatchLease(run.SummaryJSON)
	if err != nil {
		return fmt.Errorf("decode batch %q lease for checkpoint: %w", report.BatchRunID, err)
	}
	if run.Status != store.StatusRunning || !report.lease.sameOwner(currentLease) {
		return fmt.Errorf("%w: batch run %q is now status %q and held by %q", errAPICaseBatchLeaseLost, report.BatchRunID, run.Status, currentLease.HolderIdentity)
	}
	report.lease = currentLease
	report.lease.RenewTime = time.Now().UTC().Format(time.RFC3339Nano)
	expectedUpdatedAt := run.UpdatedAt
	applyAPICaseBatchReportToStoreRun(&run, report)
	run.UpdatedAt = nextAPICaseBatchRunRevision(expectedUpdatedAt, time.Now().UTC())
	if _, err := updater.CompareAndSwapRun(ctx, expectedUpdatedAt, store.StatusRunning, run); err != nil {
		if errors.Is(err, store.ErrRunRevisionConflict) {
			return fmt.Errorf("%w: batch run %q changed during checkpoint", errAPICaseBatchLeaseLost, report.BatchRunID)
		}
		return fmt.Errorf("checkpoint batch %q: %w", report.BatchRunID, err)
	}
	return nil
}

func applyAPICaseBatchReportToStoreRun(run *store.Run, report apiCaseBatchRunReport) {
	run.ProfileID = strings.TrimSpace(report.ProfileID)
	run.EnvironmentID = strings.TrimSpace(report.EnvironmentID)
	run.WorkflowID = strings.TrimSpace(report.WorkflowID)
	run.Status = report.Status
	run.EvidenceRoot = apiCaseBatchEvidenceRoot(report)
	run.SummaryJSON = compactJSON(apiCaseBatchPersistedSummary(report))
	run.StartedAt = parseAPICaseBatchReportTime(report.StartedAt, run.StartedAt)
	if strings.TrimSpace(report.FinishedAt) == "" {
		run.FinishedAt = time.Time{}
	} else {
		run.FinishedAt = parseAPICaseBatchReportTime(report.FinishedAt, time.Now().UTC())
	}
}

func newAPICaseBatchStoreRun(report apiCaseBatchRunReport) store.Run {
	startedAt := parseAPICaseBatchReportTime(report.StartedAt, time.Now().UTC())
	return store.Run{
		ID:            report.BatchRunID,
		ProfileID:     strings.TrimSpace(report.ProfileID),
		EnvironmentID: strings.TrimSpace(report.EnvironmentID),
		WorkflowID:    strings.TrimSpace(report.WorkflowID),
		Status:        report.Status,
		EvidenceRoot:  apiCaseBatchEvidenceRoot(report),
		SummaryJSON:   compactJSON(apiCaseBatchPersistedSummary(report)),
		StartedAt:     startedAt,
		CreatedAt:     startedAt,
		UpdatedAt:     startedAt,
	}
}

func apiCaseBatchEvidenceRoot(report apiCaseBatchRunReport) string {
	for _, path := range []string{report.HTMLReportPath, report.ArtifactManifestPath, report.JUnitReportPath, report.FailureSummaryPath} {
		if dir := strings.TrimSpace(filepath.Dir(strings.TrimSpace(path))); dir != "" && dir != "." {
			return dir
		}
	}
	return ""
}

func storedAPICaseBatchRunReport(ctx context.Context, runtime store.Store, batchRunID string) (apiCaseBatchRunReport, bool, error) {
	batchRunID = strings.TrimSpace(batchRunID)
	if runtime == nil || batchRunID == "" || !strings.HasPrefix(batchRunID, "batch.") {
		return apiCaseBatchRunReport{}, false, nil
	}
	for attempt := 0; attempt < 3; attempt++ {
		run, err := runtime.GetRun(ctx, batchRunID)
		if errors.Is(err, store.ErrNotFound) {
			return apiCaseBatchRunReport{}, false, nil
		}
		if err != nil {
			return apiCaseBatchRunReport{}, false, fmt.Errorf("load batch run %q: %w", batchRunID, err)
		}
		report, err := decodeStoredAPICaseBatchRunReport(run)
		if err != nil {
			return apiCaseBatchRunReport{}, false, err
		}
		if strings.TrimSpace(report.BatchRunID) == "" {
			return apiCaseBatchRunReport{}, false, nil
		}
		if report.Status != store.StatusRunning || report.lease.activeAt(time.Now().UTC()) {
			return report, true, nil
		}
		updater, ok := runtime.(store.RunCompareAndSwapStore)
		if !ok {
			return apiCaseBatchRunReport{}, false, fmt.Errorf("persist interrupted batch run %q: Store does not support compare-and-swap run updates", batchRunID)
		}
		markAPICaseBatchRunInterrupted(&report)
		expectedUpdatedAt := run.UpdatedAt
		applyAPICaseBatchReportToStoreRun(&run, report)
		run.UpdatedAt = nextAPICaseBatchRunRevision(expectedUpdatedAt, time.Now().UTC())
		if _, err := updater.CompareAndSwapRun(ctx, expectedUpdatedAt, store.StatusRunning, run); err != nil {
			if errors.Is(err, store.ErrRunRevisionConflict) {
				continue
			}
			return apiCaseBatchRunReport{}, false, fmt.Errorf("persist interrupted batch run %q: %w", batchRunID, err)
		}
		return report, true, nil
	}
	return apiCaseBatchRunReport{}, false, fmt.Errorf("load batch run %q: owner changed repeatedly during recovery", batchRunID)
}

func decodeStoredAPICaseBatchRunReport(run store.Run) (apiCaseBatchRunReport, error) {
	var report apiCaseBatchRunReport
	if err := json.Unmarshal([]byte(strings.TrimSpace(run.SummaryJSON)), &report); err != nil {
		return apiCaseBatchRunReport{}, fmt.Errorf("decode batch run %q summary: %w", run.ID, err)
	}
	if strings.TrimSpace(report.BatchRunID) == "" {
		return report, nil
	}
	if report.BatchRunID != run.ID {
		return apiCaseBatchRunReport{}, fmt.Errorf("decode batch run %q summary: batchRunId is %q", run.ID, report.BatchRunID)
	}
	lease, err := decodeAPICaseBatchLease(run.SummaryJSON)
	if err != nil {
		return apiCaseBatchRunReport{}, fmt.Errorf("decode batch run %q lease: %w", run.ID, err)
	}
	report.lease = lease
	hydrateStoredAPICaseBatchRunReport(&report, run)
	return report, nil
}

func hydrateStoredAPICaseBatchRunReport(report *apiCaseBatchRunReport, run store.Run) {
	report.BatchRunID = run.ID
	if report.ProfileID == "" {
		report.ProfileID = run.ProfileID
	}
	if report.EnvironmentID == "" {
		report.EnvironmentID = run.EnvironmentID
	}
	if report.WorkflowID == "" {
		report.WorkflowID = run.WorkflowID
	}
	if strings.TrimSpace(run.Status) != "" {
		report.Status = run.Status
	}
	if report.StartedAt == "" && !run.StartedAt.IsZero() {
		report.StartedAt = run.StartedAt.UTC().Format(time.RFC3339Nano)
	}
	if report.FinishedAt == "" && !run.FinishedAt.IsZero() {
		report.FinishedAt = run.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	escapedID := url.PathEscape(run.ID)
	if report.ReportURL == "" {
		report.ReportURL = "/api/cases/batch-runs/" + escapedID
	}
	if report.HTMLReportURL == "" {
		report.HTMLReportURL = report.ReportURL + "/report.html"
	}
	if report.JUnitReportURL == "" {
		report.JUnitReportURL = report.ReportURL + "/report.junit.xml"
	}
	if report.ArtifactManifestURL == "" {
		report.ArtifactManifestURL = report.ReportURL + "/artifacts.json"
	}
	if report.FailureSummaryURL == "" {
		report.FailureSummaryURL = report.ReportURL + "/failures.json"
	}
	root := strings.TrimSpace(run.EvidenceRoot)
	if root == "" {
		return
	}
	if report.HTMLReportPath == "" {
		report.HTMLReportPath = filepath.Join(root, "report.html")
	}
	if report.JUnitReportPath == "" {
		report.JUnitReportPath = filepath.Join(root, "report.junit.xml")
	}
	if report.ArtifactManifestPath == "" {
		report.ArtifactManifestPath = filepath.Join(root, "artifacts.json")
	}
	if report.FailureSummaryPath == "" {
		report.FailureSummaryPath = filepath.Join(root, "failures.json")
	}
}

func markAPICaseBatchRunInterrupted(report *apiCaseBatchRunReport) {
	message := "batch run owner lease expired; outcome is unknown and requests were not replayed"
	report.OK = false
	report.Status = store.StatusFailed
	report.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	report.FailureCategory = apiCaseBatchInterruptedFailureCategory
	appendAPICaseBatchReportError(report, message)
	for index := range report.Cases {
		if report.Cases[index].Status != store.StatusRunning {
			continue
		}
		report.Cases[index].Status = store.StatusFailed
		report.Cases[index].FailureCategory = apiCaseBatchInterruptedFailureCategory
		if report.Cases[index].Error == "" {
			report.Cases[index].Error = message
		}
	}
	refreshAPICaseBatchCounts(report)
}

func appendAPICaseBatchReportError(report *apiCaseBatchRunReport, message string) {
	message = strings.TrimSpace(message)
	if message == "" || strings.Contains(report.Error, message) {
		return
	}
	if strings.TrimSpace(report.Error) == "" {
		report.Error = message
		return
	}
	report.Error += "; " + message
}
