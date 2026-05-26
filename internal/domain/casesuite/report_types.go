// Package casesuite inspects, ranks, and reports API case suite readiness.
package casesuite

import (
	"context"

	"agent-testbench/internal/domain/execution"
)

type Filter struct {
	Filter   string   `json:"filter,omitempty"`
	NodeID   string   `json:"nodeId,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Status   string   `json:"status,omitempty"`
	Owner    string   `json:"owner,omitempty"`
	Priority string   `json:"priority,omitempty"`
}

type Counts struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
	NotRun int `json:"notRun"`
}

type Item struct {
	CaseID       string   `json:"caseId"`
	Title        string   `json:"title"`
	Description  string   `json:"description,omitempty"`
	NodeID       string   `json:"nodeId,omitempty"`
	NodeName     string   `json:"nodeName,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Priority     string   `json:"priority,omitempty"`
	Owner        string   `json:"owner,omitempty"`
	LatestStatus string   `json:"latestStatus"`
	LatestRunID  string   `json:"latestRunId,omitempty"`
	CaseRunID    string   `json:"caseRunId,omitempty"`
	DetailURL    string   `json:"detailUrl,omitempty"`
	ElapsedMs    int64    `json:"elapsedMs,omitempty"`
	HasPassed    bool     `json:"hasPassed"`
	Reason       string   `json:"reason,omitempty"`
}

type Report struct {
	OK          bool     `json:"ok"`
	ProfileID   string   `json:"profileId"`
	GeneratedAt string   `json:"generatedAt"`
	Filters     Filter   `json:"filters"`
	Counts      Counts   `json:"counts"`
	Items       []Item   `json:"items"`
	Warnings    []string `json:"warnings,omitempty"`
}

type InspectionCounts struct {
	Total            int `json:"total"`
	Ready            int `json:"ready"`
	Blocked          int `json:"blocked"`
	Passed           int `json:"passed"`
	Failed           int `json:"failed"`
	NotRun           int `json:"notRun"`
	MissingRunnable  int `json:"missingRunnable"`
	MissingExecution int `json:"missingExecution"`
	Inactive         int `json:"inactive"`
}

type InspectionItem struct {
	CaseID             string   `json:"caseId"`
	Title              string   `json:"title"`
	Description        string   `json:"description,omitempty"`
	NodeID             string   `json:"nodeId,omitempty"`
	NodeName           string   `json:"nodeName,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	Priority           string   `json:"priority,omitempty"`
	Owner              string   `json:"owner,omitempty"`
	Status             string   `json:"status"`
	Ready              bool     `json:"ready"`
	HasRunnableFile    bool     `json:"hasRunnableFile"`
	HasExecutionConfig bool     `json:"hasExecutionConfig"`
	LatestStatus       string   `json:"latestStatus"`
	LatestRunID        string   `json:"latestRunId,omitempty"`
	CaseRunID          string   `json:"caseRunId,omitempty"`
	DetailURL          string   `json:"detailUrl,omitempty"`
	ElapsedMs          int64    `json:"elapsedMs,omitempty"`
	HasPassed          bool     `json:"hasPassed"`
	Issues             []string `json:"issues,omitempty"`
	SuggestedAction    string   `json:"suggestedAction,omitempty"`
}

type InspectionReport struct {
	OK          bool             `json:"ok"`
	ProfileID   string           `json:"profileId"`
	GeneratedAt string           `json:"generatedAt"`
	Filters     Filter           `json:"filters"`
	Counts      InspectionCounts `json:"counts"`
	Items       []InspectionItem `json:"items"`
	Warnings    []string         `json:"warnings,omitempty"`
}

type PlanOptions struct {
	RequestID      string   `json:"requestId,omitempty"`
	Actions        []string `json:"actions,omitempty"`
	BaseURL        string   `json:"baseUrl,omitempty"`
	EvidenceDir    string   `json:"evidenceDir,omitempty"`
	TimeoutSeconds int      `json:"timeoutSeconds,omitempty"`
}

type PlanCounts struct {
	Total    int `json:"total"`
	Ready    int `json:"ready"`
	Blocked  int `json:"blocked"`
	Selected int `json:"selected"`
	Skipped  int `json:"skipped"`
}

type BatchRequest struct {
	RequestID      string         `json:"requestId,omitempty"`
	CaseIDs        []string       `json:"caseIds"`
	BaseURL        string         `json:"baseUrl,omitempty"`
	EvidenceDir    string         `json:"evidenceDir,omitempty"`
	TimeoutSeconds int            `json:"timeoutSeconds,omitempty"`
	Overrides      map[string]any `json:"overrides,omitempty"`
}

type PlanReport struct {
	OK           bool             `json:"ok"`
	ProfileID    string           `json:"profileId"`
	GeneratedAt  string           `json:"generatedAt"`
	Filters      Filter           `json:"filters"`
	Options      PlanOptions      `json:"options"`
	Counts       PlanCounts       `json:"counts"`
	CaseIDs      []string         `json:"caseIds"`
	Selected     []InspectionItem `json:"selected"`
	Blocked      []InspectionItem `json:"blocked"`
	Skipped      []InspectionItem `json:"skipped"`
	BatchRequest BatchRequest     `json:"batchRequest"`
	Warnings     []string         `json:"warnings,omitempty"`
}

const (
	CaseLifecycleDraft       = "draft"
	CaseLifecycleReview      = "review"
	CaseLifecycleActive      = "active"
	CaseLifecycleQuarantined = "quarantined"
	CaseLifecycleDeprecated  = "deprecated"
	CaseLifecycleInvalid     = "invalid"
)

type RecordStore interface {
	ListRuns(context.Context) ([]execution.Run, error)
	ListAPICaseRuns(context.Context, string) ([]execution.APICaseRun, error)
}
