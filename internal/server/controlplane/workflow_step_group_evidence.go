package controlplane

import (
	"encoding/json"
	"os"
	"path/filepath"

	"agent-testbench/internal/domain/redaction"
	"agent-testbench/internal/runner/apicase"
)

func stepGroupRequestSummary(summary string, result apicase.RunResult) string {
	raw, err := os.ReadFile(filepath.Join(result.EvidencePath, "case.json"))
	if err != nil {
		return summary
	}
	var definition apicase.Case
	if json.Unmarshal(raw, &definition) != nil {
		return summary
	}
	group := mapFromAny(definition.Metadata["stepGroup"])
	if valueString(group["id"]) == "" {
		return summary
	}
	return compactJSONObjectWithFields(summary, map[string]any{"stepGroup": redaction.Value(group)})
}
