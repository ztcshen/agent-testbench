package controlplane

import (
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agent-testbench/internal/domain/casesuite"
)

func stringListValue(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if raw := strings.TrimSpace(valueString(value)); raw != "" {
			return casesuite.NormalizeStringList([]string{raw})
		}
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if value := strings.TrimSpace(valueString(item)); value != "" {
			out = append(out, value)
		}
	}
	return casesuite.NormalizeStringList(out)
}

func apiCaseBatchSuiteSelectorValue(value any) apiCaseBatchSuiteSelector {
	raw := mapValue(value)
	if len(raw) == 0 {
		return apiCaseBatchSuiteSelector{}
	}
	return normalizeAPICaseBatchSuiteSelector(apiCaseBatchSuiteSelector{
		Filter:    strings.TrimSpace(valueString(raw["filter"])),
		NodeID:    firstNonEmpty(valueString(raw["nodeId"]), valueString(raw["node"])),
		Tags:      firstNonNilStringList(raw["tags"], raw["tag"]),
		Status:    strings.TrimSpace(valueString(raw["status"])),
		Owner:     strings.TrimSpace(valueString(raw["owner"])),
		Priority:  strings.TrimSpace(valueString(raw["priority"])),
		RunStates: firstNonNilStringList(raw["runStates"], raw["runState"]),
	})
}

func normalizeAPICaseBatchSuiteSelector(selector apiCaseBatchSuiteSelector) apiCaseBatchSuiteSelector {
	selector.Filter = strings.TrimSpace(selector.Filter)
	selector.NodeID = strings.TrimSpace(selector.NodeID)
	selector.Tags = casesuite.NormalizeStringList(selector.Tags)
	selector.Status = strings.TrimSpace(selector.Status)
	if selector.Status == "" && selector.configuredWithoutStatus() {
		selector.Status = "active"
	}
	selector.Owner = strings.TrimSpace(selector.Owner)
	selector.Priority = strings.TrimSpace(selector.Priority)
	selector.RunStates = casesuite.NormalizeStringList(selector.RunStates)
	for index, value := range selector.RunStates {
		selector.RunStates[index] = casesuite.NormalizeRunState(value)
	}
	return selector
}

func (s apiCaseBatchSuiteSelector) configured() bool {
	return s.configuredWithoutStatus() || strings.TrimSpace(s.Status) != ""
}

func (s apiCaseBatchSuiteSelector) configuredWithoutStatus() bool {
	return strings.TrimSpace(s.Filter) != "" || strings.TrimSpace(s.NodeID) != "" || len(s.Tags) > 0 || strings.TrimSpace(s.Owner) != "" || strings.TrimSpace(s.Priority) != "" || len(s.RunStates) > 0
}

func firstNonNilStringList(values ...any) []string {
	for _, value := range values {
		if out := stringListValue(value); len(out) > 0 {
			return out
		}
	}
	return nil
}

func compactUniqueStringList(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func compactUniqueStringListPreserveOrder(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func resolveBundleFilePath(baseDir string, value string) string {
	if filepath.IsAbs(value) || strings.TrimSpace(baseDir) == "" {
		return value
	}
	return filepath.Join(baseDir, value)
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func mergeStringAnyMaps(base map[string]any, overlay map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range base {
		out[key] = value
	}
	for key, value := range overlay {
		out[key] = value
	}
	return out
}

func newAPICaseBatchRunID(requestID string) string {
	return "batch." + safeRunIDPart(requestID) + "." + time.Now().UTC().Format("20060102T150405.000000000Z")
}

func apiCaseBatchCaseRunID(batchRunID string, stepID string, caseID string) string {
	if strings.TrimSpace(stepID) != "" {
		return batchRunID + "." + safeRunIDPart(stepID) + "." + safeRunIDPart(caseID)
	}
	return batchRunID + "." + safeRunIDPart(caseID)
}

func apiCaseEvidenceDetailURL(caseRunID string) string {
	if strings.TrimSpace(caseRunID) == "" {
		return ""
	}
	return "/api/case-run/evidence?caseRunId=" + url.QueryEscape(caseRunID)
}

func safeRunIDPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "item"
	}
	var builder strings.Builder
	builder.Grow(len(value))
	for _, ch := range value {
		switch {
		case ch >= 'a' && ch <= 'z':
			builder.WriteRune(ch)
		case ch >= 'A' && ch <= 'Z':
			builder.WriteRune(ch)
		case ch >= '0' && ch <= '9':
			builder.WriteRune(ch)
		case ch == '.', ch == '-', ch == '_':
			builder.WriteRune(ch)
		default:
			builder.WriteByte('-')
		}
	}
	out := strings.Trim(builder.String(), "-._")
	if out == "" {
		return "item"
	}
	return out
}
