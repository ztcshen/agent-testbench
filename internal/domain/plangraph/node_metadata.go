package plangraph

import (
	"encoding/json"
	"strings"
)

func NodeDisplayName(node Node) string {
	summary := nodeSummary(node.SummaryJSON)
	if value, ok := summary["displayName"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if strings.TrimSpace(node.CaseID) != "" {
		return strings.TrimSpace(node.CaseID)
	}
	return strings.TrimSpace(node.ID)
}

func ValidationFamilyForNode(node Node) string {
	text := strings.ToLower(strings.Join([]string{node.ID, node.CaseID, NodeDisplayName(node), node.PatchJSON, node.ExpectedJSON}, " "))
	switch {
	case strings.Contains(text, "length"), strings.Contains(text, "too-long"), strings.Contains(text, "long"), strings.Contains(text, "max"):
		return "length"
	case strings.Contains(text, "blank"), strings.Contains(text, "empty"), strings.Contains(text, "null"), strings.Contains(text, "required"), strings.Contains(text, "missing"):
		return "empty/null"
	case strings.Contains(text, "type"), strings.Contains(text, "numeric"), strings.Contains(text, "number"):
		return "type"
	case strings.Contains(text, "enum"):
		return "enum"
	case strings.Contains(text, "boundary"), strings.Contains(text, "min"):
		return "boundary"
	case strings.Contains(text, "state"), strings.Contains(text, "status"):
		return "state"
	default:
		return "contract"
	}
}

func nodeSummary(raw string) map[string]any {
	var out map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}
