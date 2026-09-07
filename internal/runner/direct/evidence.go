package direct

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"agent-testbench/internal/domain/redaction"
	"agent-testbench/internal/runner/nativeprocess"
	"agent-testbench/internal/store"
)

func recordCase(ctx context.Context, runtime store.Store, run store.Run, report Report) error {
	request, err := json.Marshal(redaction.Value(map[string]any{"mode": Mode, "bindingId": report.BindingID, "caseId": report.CaseID, "targetUrl": report.Snapshot.TargetURL}))
	if err != nil {
		return err
	}
	assertions, err := json.Marshal(report.Result)
	if err != nil {
		return err
	}
	_, err = runtime.RecordAPICaseRun(ctx, store.APICaseRun{
		ID: run.ID + ".case", RunID: run.ID, CaseID: report.CaseKey, Status: report.Status,
		RequestSummaryJSON: string(request), AssertionSummaryJSON: string(assertions),
		StartedAt: report.StartedAt, FinishedAt: report.FinishedAt, CreatedAt: report.StartedAt,
	})
	return err
}

func finalizeRun(ctx context.Context, runtime store.Store, run *store.Run, report *Report, output nativeprocess.Result, directoryCreated bool) error {
	var failures []error
	if directoryCreated {
		if err := recordArtifacts(ctx, runtime, *report, output); err != nil {
			failures = append(failures, err)
			failReport(report, "evidence", "direct execution evidence could not be persisted")
		}
	}
	if err := recordCase(ctx, runtime, *run, *report); err != nil {
		failures = append(failures, err)
		failReport(report, "store", "direct case outcome could not be persisted")
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return errors.Join(append(failures, err)...)
	}
	run.Status, run.FinishedAt, run.UpdatedAt, run.SummaryJSON = report.Status, report.FinishedAt, report.FinishedAt, string(raw)
	updater, ok := runtime.(store.RunUpdater)
	if !ok {
		return errors.New("direct mode requires checkpoint-capable Store")
	}
	_, err = updater.UpdateRun(ctx, *run)
	return errors.Join(append(failures, err)...)
}

func recordArtifacts(ctx context.Context, runtime store.Store, report Report, output nativeprocess.Result) error {
	snapshot, err := json.MarshalIndent(report.Snapshot, "", "  ")
	if err != nil {
		return err
	}
	result, err := json.Marshal(report)
	if err != nil {
		return err
	}
	artifacts := []struct {
		name, visibility string
		data             []byte
	}{
		{"stdout.txt", "private", output.Stdout},
		{"stderr.txt", "private", output.Stderr},
		{"input.json", "private", output.Input},
		{"snapshot.json", "public", snapshot},
		{"result.json", "public", result},
	}
	var failures []error
	for _, artifact := range artifacts {
		path := filepath.Join(report.EvidenceDir, artifact.name)
		if err := os.WriteFile(path, artifact.data, 0o600); err != nil {
			failures = append(failures, err)
			continue
		}
		_, err := runtime.RecordEvidence(ctx, store.EvidenceRecord{
			ID: report.RunID + "." + artifact.name, RunID: report.RunID, CaseRunID: report.RunID + ".case",
			Kind: "direct-" + artifact.name, URI: path, MediaType: artifactMediaType(artifact.name),
			SHA256: digestBytes(artifact.data), SizeBytes: int64(len(artifact.data)),
			Visibility: artifact.visibility, Category: "direct", Summary: "Local direct execution artifact", CreatedAt: report.FinishedAt,
		})
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func artifactMediaType(name string) string {
	if filepath.Ext(name) == ".json" {
		return "application/json"
	}
	return "text/plain"
}
