package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/runner/apicase"
	"agent-testbench/internal/store"
)

func apiCaseBatchFailureMessage(result apicase.RunResult) string {
	if result.Status != store.StatusFailed {
		return ""
	}
	if strings.TrimSpace(result.EvidencePath) == "" {
		return "case run failed"
	}
	raw, err := os.ReadFile(filepath.Join(result.EvidencePath, "assertions.json"))
	if err != nil {
		return "case run failed"
	}
	var assertions apicase.AssertionEvidence
	if err := json.Unmarshal(raw, &assertions); err != nil {
		return "case run failed"
	}
	if len(assertions.Errors) == 0 {
		return "case run failed"
	}
	return strings.Join(assertions.Errors, "; ")
}

func apiCaseBatchFailureCategory(result apicase.RunResult) string {
	if result.Status != store.StatusFailed {
		return ""
	}
	if strings.TrimSpace(result.EvidencePath) == "" {
		return "case-failure"
	}
	raw, err := os.ReadFile(filepath.Join(result.EvidencePath, "assertions.json"))
	if err != nil {
		return "case-failure"
	}
	var assertions apicase.AssertionEvidence
	if err := json.Unmarshal(raw, &assertions); err != nil {
		return "case-failure"
	}
	if len(assertions.Errors) == 0 {
		return "case-failure"
	}
	return "assertion-mismatch"
}

func apiCaseBatchFailureCategoryFromError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "context deadline exceeded"), strings.Contains(message, "timeout"):
		return "timeout"
	case strings.Contains(message, "base url"), strings.Contains(message, "send request"), strings.Contains(message, "create request"), strings.Contains(message, "parse"):
		return "transport-error"
	default:
		return "case-failure"
	}
}

func apiCaseBatchApplyFailureCategoryRules(rules []profile.FailureCategoryRule, status string, defaultCategory string, message string) string {
	if strings.TrimSpace(defaultCategory) == "" {
		return ""
	}
	for _, rule := range rules {
		if apiCaseBatchFailureCategoryRuleMatches(rule, status, defaultCategory, message) {
			return firstNonEmpty(rule.Category, rule.Name, defaultCategory)
		}
	}
	return defaultCategory
}

func apiCaseBatchFailureCategoryRuleMatches(rule profile.FailureCategoryRule, status string, defaultCategory string, message string) bool {
	matcher := rule.Matchers
	if len(matcher.Statuses) > 0 && !containsFold(matcher.Statuses, status) {
		return false
	}
	if len(matcher.FailureCategories) > 0 && !containsFold(matcher.FailureCategories, defaultCategory) {
		return false
	}
	if len(matcher.MessageContains) > 0 && !containsMessageFragment(matcher.MessageContains, message) {
		return false
	}
	return len(matcher.Statuses) > 0 || len(matcher.FailureCategories) > 0 || len(matcher.MessageContains) > 0
}

func containsFold(values []string, want string) bool {
	want = strings.TrimSpace(strings.ToLower(want))
	if want == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(strings.ToLower(value)) == want {
			return true
		}
	}
	return false
}

func containsMessageFragment(fragments []string, message string) bool {
	message = strings.ToLower(message)
	for _, fragment := range fragments {
		fragment = strings.TrimSpace(strings.ToLower(fragment))
		if fragment != "" && strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}
