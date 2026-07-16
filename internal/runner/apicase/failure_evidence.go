package apicase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	FailurePhaseMaterialization    = "materialization"
	FailureCategoryMaterialization = "materialization-error"
	FailurePhasePersistence        = "persistence"
	FailureCategoryEvidenceWrite   = "evidence-persistence-error"
)

// FailureEvidenceOptions describes a case failure that happened before the
// normal HTTP runner could produce a RunResult. The caller supplies the
// smallest request model it knows so the failure remains diagnosable.
type FailureEvidenceOptions struct {
	Context             context.Context
	RunID               string
	EvidenceDir         string
	Case                Case
	Phase               string
	Category            string
	Message             string
	StartedAt           time.Time
	BeforeEvidenceWrite func(context.Context) error
}

// WriteFailureEvidence creates the same failed RunResult and core Evidence
// contract used by Run for a failure that occurs outside the HTTP runner.
func WriteFailureEvidence(options FailureEvidenceOptions) (RunResult, error) {
	if err := validateExplicitRunID(options.RunID); err != nil {
		return RunResult{}, err
	}
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	started := options.StartedAt.UTC()
	if started.IsZero() {
		started = time.Now().UTC()
	}
	runID := plannedCaseRunID(options.RunID)
	result := RunResult{
		OK:              false,
		RunID:           runID,
		CaseID:          strings.TrimSpace(options.Case.ID),
		Status:          "failed",
		FailurePhase:    firstFailureValue(options.Phase, "execution"),
		FailureCategory: firstFailureValue(options.Category, "case-failure"),
		Error:           firstFailureValue(options.Message, "case run failed"),
		EvidencePath:    caseRunEvidencePath(options.EvidenceDir, runID),
		StartedAt:       started.Format(time.RFC3339Nano),
		CreatedAt:       started.Format(time.RFC3339Nano),
	}
	if result.CaseID == "" {
		return result, errors.New("case id is required for failure Evidence")
	}
	if err := beforeEvidenceWrite(ctx, options.BeforeEvidenceWrite); err != nil {
		return evidencePersistenceFailure(started, result, err)
	}
	if err := os.MkdirAll(result.EvidencePath, 0o755); err != nil {
		return evidencePersistenceFailure(started, result, fmt.Errorf("create failure evidence directory: %w", err))
	}

	files := []struct {
		name  string
		value any
	}{
		{name: "case.json", value: options.Case},
		{name: "request.json", value: options.Case.Request},
	}
	for _, file := range files {
		if err := beforeEvidenceWrite(ctx, options.BeforeEvidenceWrite); err != nil {
			return evidencePersistenceFailure(started, result, err)
		}
		if err := writeJSON(filepath.Join(result.EvidencePath, file.name), file.value); err != nil {
			return evidencePersistenceFailure(started, result, err)
		}
	}
	return writeFailureOutcome(ctx, started, result, options.BeforeEvidenceWrite)
}

func writeFailureOutcome(ctx context.Context, started time.Time, result RunResult, beforeWrite func(context.Context) error) (RunResult, error) {
	assertions := AssertionEvidence{Status: "failed", Errors: []string{result.Error}}
	if err := beforeEvidenceWrite(ctx, beforeWrite); err != nil {
		return evidencePersistenceFailure(started, result, err)
	}
	if err := writeJSON(filepath.Join(result.EvidencePath, "assertions.json"), assertions); err != nil {
		return evidencePersistenceFailure(started, result, err)
	}
	if err := beforeEvidenceWrite(ctx, beforeWrite); err != nil {
		return evidencePersistenceFailure(started, result, err)
	}
	if err := writeJSON(filepath.Join(result.EvidencePath, "error.json"), ErrorEvidence{
		Status:   result.Status,
		Phase:    result.FailurePhase,
		Category: result.FailureCategory,
		Message:  result.Error,
	}); err != nil {
		return evidencePersistenceFailure(started, result, err)
	}
	return finishRun(ctx, started, result, beforeWrite)
}

func evidencePersistenceFailure(started time.Time, result RunResult, cause error) (RunResult, error) {
	result.OK = false
	result.Status = "failed"
	result.FailurePhase = FailurePhasePersistence
	result.FailureCategory = FailureCategoryEvidenceWrite
	result.Error = cause.Error()
	if result.StartedAt == "" {
		result.StartedAt = started.UTC().Format(time.RFC3339Nano)
	}
	finished := time.Now().UTC()
	result.FinishedAt = finished.Format(time.RFC3339Nano)
	result.ElapsedMs = finished.Sub(started).Milliseconds()
	return result, cause
}

func beforeEvidenceWrite(ctx context.Context, beforeWrite func(context.Context) error) error {
	if beforeWrite == nil {
		return nil
	}
	return beforeWrite(ctx)
}

func firstFailureValue(value string, defaultValue string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return defaultValue
}
