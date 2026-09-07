package direct

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"agent-testbench/internal/domain/redaction"
	"agent-testbench/internal/runner/nativeprocess"
	"agent-testbench/internal/store"
)

func decodeProtocol(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: adapter JSON: %v", ErrInvalid, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("%w: adapter must emit exactly one JSON object", ErrInvalid)
	}
	return nil
}

func applyOutcome(report *Report, output nativeprocess.Result) {
	report.ExitCode = output.ExitCode
	if output.Category != "" && output.Category != "process-exit" {
		failReport(report, output.Category, output.Error)
		return
	}
	var result AdapterResult
	if err := decodeProtocol(output.Stdout, &result); err != nil {
		category := "protocol"
		if output.Category == "process-exit" {
			category = output.Category
		}
		failReport(report, category, err.Error())
		return
	}
	if err := validateResult(report.CaseID, result); err != nil {
		failReport(report, "protocol", err.Error())
		return
	}
	if result.Data != nil {
		result.Data = publicFields(result.Data)
		data, err := json.Marshal(result.Data)
		if err != nil {
			failReport(report, "protocol", "adapter data could not be encoded")
			return
		}
		if len(data) > 128<<10 {
			result.Data = map[string]any{"artifact": "stdout.txt", "summaryTruncated": true, "originalBytes": len(data)}
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > 256<<10 {
		failReport(report, "output-limit", "normalized adapter result exceeds the Store summary budget; raw output is retained")
		return
	}
	report.Result = &result
	if output.ExitCode != 0 && result.Status != store.StatusFailed {
		failReport(report, "process-exit", "executor exit code conflicts with its reported result")
		return
	}
	report.Status, report.OK = result.Status, result.Status == store.StatusPassed
	if result.Status == store.StatusFailed {
		report.FailureCategory = "assertion"
	}
}

func validateResult(caseID string, result AdapterResult) error {
	if result.ProtocolVersion != ProtocolVersion || result.CaseID != caseID {
		return fmt.Errorf("adapter result version or caseId does not match the request")
	}
	if result.DryRun {
		return fmt.Errorf("a dry-run cannot satisfy a direct execution")
	}
	if result.Status == store.StatusSkipped {
		return nil
	}
	if !result.Executed || len(result.Checks) == 0 {
		return fmt.Errorf("adapter must report an executed test and explicit checks")
	}
	if result.Status != store.StatusPassed && result.Status != store.StatusFailed {
		return fmt.Errorf("adapter status must be passed, failed, or skipped")
	}
	failed := false
	for _, check := range result.Checks {
		if check.Name == "" {
			return fmt.Errorf("adapter checks require a name")
		}
		failed = failed || !check.Passed
	}
	if (result.Status == store.StatusPassed) == failed {
		return fmt.Errorf("adapter status contradicts its checks")
	}
	return nil
}

func failReport(report *Report, category, message string) {
	report.OK, report.Status, report.FailureCategory, report.Error = false, store.StatusFailed, category, message
}

func publicFields(value map[string]any) map[string]any {
	fields, ok := redaction.Value(value).(map[string]any)
	if !ok {
		return nil
	}
	return fields
}
