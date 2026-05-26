// Package auditrefs defines shared reference-audit issue primitives.
package auditrefs

import "strings"

type Issue struct {
	Severity    string `json:"severity"`
	Code        string `json:"code"`
	SubjectType string `json:"subjectType"`
	SubjectID   string `json:"subjectId"`
	Field       string `json:"field"`
	Message     string `json:"message"`
}

func NewIssue(code string, subjectType string, subjectID string, field string, message string) Issue {
	return Issue{
		Severity:    "error",
		Code:        code,
		SubjectType: subjectType,
		SubjectID:   subjectID,
		Field:       field,
		Message:     message,
	}
}

func SubjectID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "(missing)"
	}
	return value
}

func BindingSubject(workflowID string, stepID string) string {
	return SubjectID(workflowID) + "/" + SubjectID(stepID)
}

func IDSetFrom[T any](items []T, id func(T) string) map[string]bool {
	out := map[string]bool{}
	for _, item := range items {
		value := strings.TrimSpace(id(item))
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func ItemMapFrom[T any](items []T, id func(T) string) map[string]T {
	out := map[string]T{}
	for _, item := range items {
		value := strings.TrimSpace(id(item))
		if value != "" {
			out[value] = item
		}
	}
	return out
}
