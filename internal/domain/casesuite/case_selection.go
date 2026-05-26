package casesuite

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"agent-testbench/internal/domain/execution"
	"agent-testbench/internal/domain/profile"
)

func CaseMatches(item profile.APICase, filter Filter) bool {
	filter = NormalizeFilter(filter)
	if filter.NodeID != "" && item.NodeID != filter.NodeID {
		return false
	}
	if filter.Status != "" && !strings.EqualFold(CaseStatus(item), filter.Status) {
		return false
	}
	if filter.Owner != "" && !strings.EqualFold(strings.TrimSpace(item.Owner), filter.Owner) {
		return false
	}
	if filter.Priority != "" && !strings.EqualFold(strings.TrimSpace(item.Priority), filter.Priority) {
		return false
	}
	if len(filter.Tags) > 0 && !HasAllTags(item.Tags, filter.Tags) {
		return false
	}
	return MatchesText(filter.Filter, item.ID, item.DisplayName, item.Description, item.Scenario, item.Owner, item.Priority, strings.Join(item.Tags, " "), item.NodeID)
}

func NormalizeFilter(filter Filter) Filter {
	filter.Filter = strings.TrimSpace(filter.Filter)
	filter.NodeID = strings.TrimSpace(filter.NodeID)
	filter.Tags = NormalizeStringList(filter.Tags)
	filter.Status = strings.TrimSpace(filter.Status)
	filter.Owner = strings.TrimSpace(filter.Owner)
	filter.Priority = strings.TrimSpace(filter.Priority)
	return filter
}

func CaseStatus(item profile.APICase) string {
	return NormalizeCaseLifecycle(item.Status)
}

func NormalizeCaseLifecycle(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return CaseLifecycleActive
	}
	if IsKnownCaseLifecycle(status) {
		return status
	}
	return CaseLifecycleInvalid
}

func IsKnownCaseLifecycle(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case CaseLifecycleDraft, CaseLifecycleReview, CaseLifecycleActive, CaseLifecycleQuarantined, CaseLifecycleDeprecated:
		return true
	default:
		return false
	}
}

func IsExecutableCaseLifecycle(status string) bool {
	return NormalizeCaseLifecycle(status) == CaseLifecycleActive
}

func HasAllTags(actual []string, required []string) bool {
	actualSet := map[string]bool{}
	for _, tag := range actual {
		if normalized := SearchText(tag); normalized != "" {
			actualSet[normalized] = true
		}
	}
	for _, tag := range required {
		if normalized := SearchText(tag); normalized != "" && !actualSet[normalized] {
			return false
		}
	}
	return true
}

func MatchesText(filter string, values ...string) bool {
	needle := SearchText(filter)
	if needle == "" {
		return true
	}
	for _, value := range values {
		haystack := SearchText(value)
		if haystack != "" && (strings.Contains(haystack, needle) || strings.Contains(needle, haystack)) {
			return true
		}
	}
	return false
}

func SearchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "", ".", "", "/", "")
	return replacer.Replace(value)
}

func NormalizeStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			key := strings.ToLower(part)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, part)
		}
	}
	return out
}

func CaseIDs(cases []profile.APICase) []string {
	out := make([]string, 0, len(cases))
	for _, item := range cases {
		if strings.TrimSpace(item.ID) != "" {
			out = append(out, item.ID)
		}
	}
	return out
}

func DetailURL(caseRunID string) string {
	if strings.TrimSpace(caseRunID) == "" {
		return ""
	}
	return "/api/case-run/evidence?caseRunId=" + url.QueryEscape(caseRunID)
}

func AssertionSummaryReason(summaryJSON string) string {
	var payload struct {
		FailureReason string `json:"failureReason"`
		ErrorCount    int    `json:"errorCount"`
	}
	if json.Unmarshal([]byte(summaryJSON), &payload) != nil {
		return ""
	}
	if strings.TrimSpace(payload.FailureReason) != "" {
		return payload.FailureReason
	}
	if payload.ErrorCount > 0 {
		return fmt.Sprintf("assertion errors: %d", payload.ErrorCount)
	}
	return ""
}

func ElapsedMs(started time.Time, finished time.Time) int64 {
	if started.IsZero() || finished.IsZero() || finished.Before(started) {
		return 0
	}
	return finished.Sub(started).Milliseconds()
}

func NormalizeRunState(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "notrun", "not-run", "missing", "never-run":
		return "not-run"
	case "pass", "passed", "success", "ok":
		return execution.StatusPassed
	case "fail", "failed", "error":
		return execution.StatusFailed
	default:
		return value
	}
}

func RunStateSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		if normalized := NormalizeRunState(value); normalized != "" {
			out[normalized] = true
		}
	}
	return out
}

func isPassedStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pass", "passed", "success", "ok":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
