package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-testbench/internal/runner/junit"
	"agent-testbench/internal/store"
)

func recordAPICaseBatchReportArtifacts(ctx context.Context, runtime store.Store, profileID string, workflowID string, report apiCaseBatchRunReport) {
	if runtime == nil || strings.TrimSpace(report.BatchRunID) == "" {
		return
	}
	startedAt := parseAPICaseBatchReportTime(report.StartedAt, time.Now().UTC())
	finishedAt := parseAPICaseBatchReportTime(report.FinishedAt, time.Now().UTC())
	if finishedAt.Before(startedAt) {
		finishedAt = startedAt
	}
	evidenceRoot := strings.TrimSpace(filepath.Dir(report.HTMLReportPath))
	if evidenceRoot == "." {
		evidenceRoot = strings.TrimSpace(filepath.Dir(report.ArtifactManifestPath))
	}
	if evidenceRoot == "." {
		evidenceRoot = ""
	}
	if _, err := runtime.CreateRun(ctx, store.Run{
		ID:            report.BatchRunID,
		ProfileID:     strings.TrimSpace(profileID),
		EnvironmentID: strings.TrimSpace(report.EnvironmentID),
		WorkflowID:    strings.TrimSpace(workflowID),
		Status:        report.Status,
		EvidenceRoot:  evidenceRoot,
		SummaryJSON:   compactJSON(apiCaseBatchRunStoreSummary(report)),
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
		CreatedAt:     startedAt,
		UpdatedAt:     finishedAt,
	}); err != nil {
		return
	}
	for _, artifact := range apiCaseBatchReportEvidenceArtifacts(report) {
		info, err := os.Stat(artifact.Path)
		if err != nil {
			continue
		}
		_, _ = runtime.RecordEvidence(ctx, store.EvidenceRecord{
			ID:         report.BatchRunID + ".report." + artifact.Kind,
			RunID:      report.BatchRunID,
			Kind:       artifact.Kind,
			URI:        artifact.Path,
			MediaType:  artifact.MediaType,
			SizeBytes:  info.Size(),
			Summary:    artifact.Summary,
			Category:   "report",
			Visibility: "public",
			LabelsJSON: compactJSON(map[string]any{
				"batchRunId": report.BatchRunID,
				"requestId":  report.RequestID,
				"kind":       artifact.Kind,
			}),
			CreatedAt: finishedAt,
		})
	}
}

func apiCaseBatchRunStoreSummary(report apiCaseBatchRunReport) map[string]any {
	out := jsonObject(compactJSON(report))
	steps := make([]map[string]any, 0, len(report.Cases))
	for _, item := range report.Cases {
		step := map[string]any{
			"stepId":    item.StepID,
			"caseId":    item.CaseID,
			"nodeId":    item.NodeID,
			"status":    item.Status,
			"elapsedMs": item.ElapsedMs,
		}
		if item.RunID != "" {
			step["runId"] = item.RunID
		}
		if item.CaseRunID != "" {
			step["caseRunId"] = item.CaseRunID
		}
		if item.Error != "" {
			step["error"] = item.Error
		}
		if item.FailureCategory != "" {
			step["failureCategory"] = item.FailureCategory
		}
		steps = append(steps, step)
	}
	out["steps"] = steps
	out["summary"] = map[string]any{
		"expectedStepCount": report.Total,
		"stepCount":         report.Completed,
		"passed":            report.Passed,
		"failed":            report.Failed,
		"skipped":           report.Skipped,
	}
	return out
}

type apiCaseBatchReportEvidenceArtifact struct {
	Kind      string
	Path      string
	MediaType string
	Summary   string
}

func apiCaseBatchReportEvidenceArtifacts(report apiCaseBatchRunReport) []apiCaseBatchReportEvidenceArtifact {
	return []apiCaseBatchReportEvidenceArtifact{
		{Kind: "html", Path: report.HTMLReportPath, MediaType: "text/html", Summary: "API case batch HTML report"},
		{Kind: "junit", Path: report.JUnitReportPath, MediaType: "application/xml", Summary: "API case batch JUnit report"},
		{Kind: "artifact-manifest", Path: report.ArtifactManifestPath, MediaType: "application/json", Summary: "API case batch artifact manifest"},
		{Kind: "failure-summary", Path: report.FailureSummaryPath, MediaType: "application/json", Summary: "API case batch failure summary"},
	}
}

func parseAPICaseBatchReportTime(value string, defaultValue time.Time) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return defaultValue
	}
	return parsed.UTC()
}

func writeAPICaseBatchHTMLReport(report apiCaseBatchRunReport) error {
	if strings.TrimSpace(report.HTMLReportPath) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(report.HTMLReportPath), 0o755); err != nil {
		return err
	}
	var rendered bytes.Buffer
	if err := apiCaseBatchReportTemplate.Execute(&rendered, report); err != nil {
		return err
	}
	return os.WriteFile(report.HTMLReportPath, rendered.Bytes(), 0o644)
}

func writeAPICaseBatchJUnitReport(report apiCaseBatchRunReport) error {
	if strings.TrimSpace(report.JUnitReportPath) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(report.JUnitReportPath), 0o755); err != nil {
		return err
	}
	raw, err := renderAPICaseBatchJUnit(report)
	if err != nil {
		return err
	}
	return os.WriteFile(report.JUnitReportPath, raw, 0o644)
}

func renderAPICaseBatchJUnit(report apiCaseBatchRunReport) ([]byte, error) {
	cases := make([]junit.Case, 0, len(report.Cases))
	for _, item := range report.Cases {
		cases = append(cases, junit.Case{
			Name:           item.CaseID,
			ClassName:      item.NodeID,
			Status:         item.Status,
			TimeSeconds:    float64(item.ElapsedMs) / 1000,
			FailureMessage: item.Error,
			Output:         item.Error,
		})
	}
	return junit.Render(junit.Suite{Name: "API Case Batch " + report.RequestID, Cases: cases})
}

func writeAPICaseBatchArtifactManifest(report apiCaseBatchRunReport) error {
	if strings.TrimSpace(report.ArtifactManifestPath) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(report.ArtifactManifestPath), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(apiCaseBatchArtifacts(report), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(report.ArtifactManifestPath, append(raw, '\n'), 0o644)
}

func apiCaseBatchArtifacts(report apiCaseBatchRunReport) apiCaseBatchArtifactManifest {
	manifest := apiCaseBatchArtifactManifest{
		OK:          report.OK,
		BatchRunID:  report.BatchRunID,
		RequestID:   report.RequestID,
		ProfileID:   report.ProfileID,
		Status:      report.Status,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Artifacts:   []apiCaseBatchArtifact{},
	}
	manifest.Artifacts = append(manifest.Artifacts,
		apiCaseBatchArtifact{Kind: "json", URL: report.ReportURL, MediaType: "application/json"},
		apiCaseBatchArtifact{Kind: "html", URL: report.HTMLReportURL, Path: report.HTMLReportPath, MediaType: "text/html"},
		apiCaseBatchArtifact{Kind: "junit", URL: report.JUnitReportURL, Path: report.JUnitReportPath, MediaType: "application/xml"},
		apiCaseBatchArtifact{Kind: "artifact-manifest", URL: report.ArtifactManifestURL, Path: report.ArtifactManifestPath, MediaType: "application/json"},
		apiCaseBatchArtifact{Kind: "failure-summary", URL: report.FailureSummaryURL, Path: report.FailureSummaryPath, MediaType: "application/json"},
	)
	for _, item := range report.Cases {
		if strings.TrimSpace(item.DetailURL) != "" {
			manifest.Artifacts = append(manifest.Artifacts, apiCaseBatchArtifact{
				Kind:      "case-detail",
				CaseID:    item.CaseID,
				CaseRunID: item.CaseRunID,
				URL:       item.DetailURL,
				MediaType: "application/json",
			})
		}
		if strings.TrimSpace(item.EvidencePath) != "" {
			manifest.Artifacts = append(manifest.Artifacts, apiCaseBatchArtifact{
				Kind:      "case-evidence",
				CaseID:    item.CaseID,
				CaseRunID: item.CaseRunID,
				Path:      item.EvidencePath,
			})
		}
	}
	return manifest
}

func writeAPICaseBatchFailureSummary(report apiCaseBatchRunReport) error {
	if strings.TrimSpace(report.FailureSummaryPath) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(report.FailureSummaryPath), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(apiCaseBatchFailures(report), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(report.FailureSummaryPath, append(raw, '\n'), 0o644)
}

func apiCaseBatchFailures(report apiCaseBatchRunReport) apiCaseBatchFailureSummary {
	summary := apiCaseBatchFailureSummary{
		OK:          report.Failed == 0,
		BatchRunID:  report.BatchRunID,
		RequestID:   report.RequestID,
		ProfileID:   report.ProfileID,
		Status:      report.Status,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Failures:    []apiCaseBatchCaseReport{},
	}
	for _, item := range report.Cases {
		if item.Status == store.StatusFailed {
			summary.Failures = append(summary.Failures, item)
		}
	}
	summary.Failed = len(summary.Failures)
	summary.OK = summary.Failed == 0
	return summary
}
