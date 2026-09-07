package direct

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"agent-testbench/internal/runner/nativeprocess"
	"agent-testbench/internal/store"
)

func Run(ctx context.Context, runtime store.Store, request RunRequest) (Report, error) {
	if request.RunID == "" {
		request.RunID = newRunID()
	}
	if !identityPattern.MatchString(request.RunID) {
		return Report{}, fmt.Errorf("%w: runId must be a safe identifier", ErrInvalid)
	}
	digest, err := requestDigest(request)
	if err != nil {
		return Report{}, err
	}
	if report, found, err := existingRun(ctx, runtime, request.RunID, digest); found || err != nil {
		return report, err
	}
	registration, item, err := lookup(ctx, runtime, request.BindingID, request.CaseID)
	if err != nil {
		return Report{}, err
	}
	if _, ok := runtime.(store.RunUpdater); !ok {
		return Report{}, fmt.Errorf("direct mode requires checkpoint-capable Store")
	}
	report := initialReport(request, registration, item, digest)
	summary, err := json.Marshal(report)
	if err != nil {
		return Report{}, err
	}
	run := store.Run{
		ID: report.RunID, ProfileID: registration.ProfileID, Status: store.StatusRunning,
		EvidenceRoot: report.EvidenceDir, SummaryJSON: string(summary),
		StartedAt: report.StartedAt, CreatedAt: report.StartedAt, UpdatedAt: report.StartedAt,
	}
	if _, err := runtime.CreateRun(ctx, run); err != nil {
		if prior, found, readErr := existingRun(ctx, runtime, request.RunID, digest); found || readErr != nil {
			return prior, readErr
		}
		return Report{}, err
	}
	return executeRun(ctx, runtime, registration.Registration, request, run, report)
}

func initialReport(request RunRequest, registration RegistrationResult, item Case, digest string) Report {
	plan := executionPlan(registration, item)
	plan.DryRun = false
	plan.Overrides = publicFields(request.Overrides)
	return Report{
		Mode: Mode, RunID: request.RunID, BindingID: request.BindingID, CaseID: request.CaseID,
		CaseKey: "direct." + digestBytes([]byte(request.BindingID+"\x00"+request.CaseID)),
		Status:  store.StatusRunning, ExitCode: -1, RequestDigest: digest,
		Snapshot:    plan,
		EvidenceDir: filepath.Join(registration.Registration.Binding.EvidenceDir, request.RunID), StartedAt: time.Now().UTC(),
	}
}

func executeRun(ctx context.Context, runtime store.Store, registration Registration, request RunRequest, run store.Run, report Report) (Report, error) {
	created := false
	output := nativeprocess.Result{ExitCode: -1}
	err := os.MkdirAll(registration.Binding.EvidenceDir, 0o700)
	if err == nil {
		err = os.Mkdir(report.EvidenceDir, 0o700)
	}
	if err != nil {
		failReport(&report, "evidence", "cannot create a fresh execution directory: "+err.Error())
	} else {
		created = true
		if err := validateSource(registration); err != nil {
			failReport(&report, "source-drift", err.Error())
		} else {
			output = invoke(ctx, registration.Binding, AdapterRequest{
				ProtocolVersion: ProtocolVersion, Operation: "run", Mode: Mode, RunID: report.RunID,
				CaseID: report.CaseID, CaseKind: report.Snapshot.Case.Kind,
				TargetURL: registration.Binding.TargetURL, StateDir: report.EvidenceDir, Overrides: request.Overrides,
			}, time.Duration(registration.Binding.TimeoutSeconds)*time.Second)
			applyOutcome(&report, output)
			if err := validateSource(registration); err != nil {
				failReport(&report, "source-drift", err.Error())
			}
		}
	}
	report.FinishedAt = time.Now().UTC()
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := finalizeRun(finishCtx, runtime, &run, &report, output, created); err != nil {
		return report, err
	}
	return report, nil
}

func Inspect(ctx context.Context, runtime store.Store, id string) (Report, error) {
	run, err := runtime.GetRun(ctx, id)
	if err != nil {
		return Report{}, err
	}
	var report Report
	if err := json.Unmarshal([]byte(run.SummaryJSON), &report); err != nil {
		return Report{}, err
	}
	if report.Mode != Mode || report.RunID != id {
		return Report{}, fmt.Errorf("%w: runId belongs to another execution", ErrConflict)
	}
	return report, nil
}

func existingRun(ctx context.Context, runtime store.Store, id, digest string) (Report, bool, error) {
	report, err := Inspect(ctx, runtime, id)
	if errors.Is(err, store.ErrNotFound) {
		return Report{}, false, nil
	}
	if err != nil {
		return Report{}, false, err
	}
	if report.RequestDigest != digest {
		return report, true, fmt.Errorf("%w: runId was already used with different inputs", ErrConflict)
	}
	return report, true, nil
}

func requestDigest(request RunRequest) (string, error) {
	overrides := request.Overrides
	if len(overrides) == 0 {
		overrides = nil
	}
	raw, err := json.Marshal(map[string]any{"bindingId": request.BindingID, "caseId": request.CaseID, "overrides": overrides})
	if err != nil {
		return "", fmt.Errorf("%w: execution input is not JSON", ErrInvalid)
	}
	if len(raw) > 128<<10 {
		return "", fmt.Errorf("%w: execution input exceeds 128 KiB", ErrInvalid)
	}
	return digestBytes(raw), nil
}

func newRunID() string {
	var value [16]byte
	_, _ = rand.Read(value[:])
	return "direct." + hex.EncodeToString(value[:])
}
