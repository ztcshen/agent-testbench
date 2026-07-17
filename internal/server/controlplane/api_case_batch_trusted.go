package controlplane

import (
	"context"
	"net/http"
	"strings"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/store"
)

// TrustedAPICaseBatchRunRequest is the typed execution context for an
// in-process caller. Callers must build it from trusted local command state,
// never by decoding an external HTTP request into it.
type TrustedAPICaseBatchRunRequest struct {
	RequestID      string
	EnvironmentID  string
	CaseIDs        []string
	NodeIDs        []string
	WorkflowID     string
	BaseURL        string
	EvidenceDir    string
	TimeoutSeconds int
	Overrides      map[string]any
}

// TrustedAPICaseBatchRunResult identifies an asynchronously started trusted
// batch. The caller can inspect its durable state through runtime using
// BatchRunID.
type TrustedAPICaseBatchRunResult struct {
	BatchRunID string
	Status     string
	Total      int
}

// StartTrustedAPICaseBatchRun starts one typed in-process batch. Unlike the
// public HTTP endpoint, this entry may carry explicit target and Evidence
// overrides because the values never cross an untrusted request boundary.
func StartTrustedAPICaseBatchRun(ctx context.Context, bundle profile.Bundle, runtime store.Store, request TrustedAPICaseBatchRunRequest) (TrustedAPICaseBatchRunResult, int, error) {
	if err := validateAPICaseBatchTimeoutSeconds(request.TimeoutSeconds, true); err != nil {
		return TrustedAPICaseBatchRunResult{}, http.StatusBadRequest, err
	}
	current, err := currentProfileBundle(ctx, runtime, bundle)
	if err != nil {
		return TrustedAPICaseBatchRunResult{}, http.StatusInternalServerError, err
	}
	report, status, err := startAPICaseBatchRun(ctx, current, runtime, newAPICaseBatchRunner(0), apiCaseBatchRunRequest{
		RequestID:      strings.TrimSpace(request.RequestID),
		EnvironmentID:  strings.TrimSpace(request.EnvironmentID),
		CaseIDs:        append([]string(nil), request.CaseIDs...),
		NodeIDs:        append([]string(nil), request.NodeIDs...),
		WorkflowID:     strings.TrimSpace(request.WorkflowID),
		BaseURL:        strings.TrimSpace(request.BaseURL),
		EvidenceDir:    strings.TrimSpace(request.EvidenceDir),
		TimeoutSeconds: request.TimeoutSeconds,
		Overrides:      mergeStringAnyMaps(nil, request.Overrides),
	}, traceCollector{})
	if err != nil {
		return TrustedAPICaseBatchRunResult{}, status, err
	}
	return TrustedAPICaseBatchRunResult{
		BatchRunID: report.BatchRunID,
		Status:     report.Status,
		Total:      report.Total,
	}, status, nil
}
