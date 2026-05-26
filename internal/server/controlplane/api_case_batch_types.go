package controlplane

import (
	_ "embed"
	"html/template"
	"sync"

	"agent-testbench/internal/domain/profile"
)

type apiCaseBatchRunRequest struct {
	RequestID      string                    `json:"requestId"`
	EnvironmentID  string                    `json:"environmentId,omitempty"`
	CaseIDs        []string                  `json:"caseIds"`
	NodeIDs        []string                  `json:"nodeIds"`
	WorkflowID     string                    `json:"workflowId"`
	Suite          apiCaseBatchSuiteSelector `json:"suite,omitempty"`
	BaseURL        string                    `json:"baseUrl"`
	EvidenceDir    string                    `json:"evidenceDir"`
	TimeoutSeconds int                       `json:"timeoutSeconds"`
	Overrides      map[string]any            `json:"overrides"`
}

type apiCaseBatchSuiteSelector struct {
	Filter    string   `json:"filter,omitempty"`
	NodeID    string   `json:"nodeId,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Status    string   `json:"status,omitempty"`
	Owner     string   `json:"owner,omitempty"`
	Priority  string   `json:"priority,omitempty"`
	RunStates []string `json:"runStates,omitempty"`
}

type apiCaseBatchCasePlan struct {
	ID              string
	DisplayName     string
	Scenario        string
	NodeID          string
	NodeDisplayName string
	Operation       string
	Method          string
	Path            string
	StepID          string
	CasePath        string
	BaseURL         string
	EvidenceDir     string
	TimeoutSeconds  int
	Overrides       map[string]any
	Execution       *caseExecutionConfig
	Exports         []map[string]any
	Case            profile.APICase
}

type apiCaseBatchCaseReport struct {
	CaseID          string `json:"caseId"`
	DisplayName     string `json:"displayName,omitempty"`
	Scenario        string `json:"scenario,omitempty"`
	NodeID          string `json:"nodeId"`
	NodeDisplayName string `json:"nodeDisplayName,omitempty"`
	Operation       string `json:"operation,omitempty"`
	Method          string `json:"method,omitempty"`
	Path            string `json:"path,omitempty"`
	StepID          string `json:"stepId,omitempty"`
	RunID           string `json:"runId,omitempty"`
	CaseRunID       string `json:"caseRunId,omitempty"`
	Status          string `json:"status"`
	ViewerURL       string `json:"viewerUrl,omitempty"`
	DetailURL       string `json:"detailUrl,omitempty"`
	EvidencePath    string `json:"evidencePath,omitempty"`
	ElapsedMs       int64  `json:"elapsedMs"`
	Error           string `json:"error,omitempty"`
	FailureCategory string `json:"failureCategory,omitempty"`
	StartedAt       string `json:"startedAt,omitempty"`
	FinishedAt      string `json:"finishedAt,omitempty"`
}

type apiCaseBatchNodeReport struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	Operation   string `json:"operation,omitempty"`
	Method      string `json:"method,omitempty"`
	Path        string `json:"path,omitempty"`
}

type apiCaseBatchRunReport struct {
	OK                   bool                       `json:"ok"`
	BatchRunID           string                     `json:"batchRunId"`
	RequestID            string                     `json:"requestId"`
	EnvironmentID        string                     `json:"environmentId,omitempty"`
	ProfileID            string                     `json:"profileId"`
	CaseIDs              []string                   `json:"caseIds,omitempty"`
	NodeIDs              []string                   `json:"nodeIds"`
	WorkflowID           string                     `json:"workflowId,omitempty"`
	Suite                *apiCaseBatchSuiteSelector `json:"suite,omitempty"`
	Status               string                     `json:"status"`
	Total                int                        `json:"total"`
	Completed            int                        `json:"completed"`
	Passed               int                        `json:"passed"`
	Failed               int                        `json:"failed"`
	Skipped              int                        `json:"skipped"`
	ReportURL            string                     `json:"reportUrl,omitempty"`
	StartedAt            string                     `json:"startedAt"`
	FinishedAt           string                     `json:"finishedAt,omitempty"`
	Nodes                []apiCaseBatchNodeReport   `json:"nodes,omitempty"`
	Cases                []apiCaseBatchCaseReport   `json:"cases"`
	Acceptance           workflowAcceptanceReport   `json:"acceptance,omitempty"`
	Error                string                     `json:"error,omitempty"`
	HTMLReportPath       string                     `json:"htmlReportPath,omitempty"`
	HTMLReportURL        string                     `json:"htmlReportUrl,omitempty"`
	JUnitReportPath      string                     `json:"junitReportPath,omitempty"`
	JUnitReportURL       string                     `json:"junitReportUrl,omitempty"`
	ArtifactManifestPath string                     `json:"artifactManifestPath,omitempty"`
	ArtifactManifestURL  string                     `json:"artifactManifestUrl,omitempty"`
	FailureSummaryPath   string                     `json:"failureSummaryPath,omitempty"`
	FailureSummaryURL    string                     `json:"failureSummaryUrl,omitempty"`
}

type apiCaseBatchArtifactManifest struct {
	OK          bool                   `json:"ok"`
	BatchRunID  string                 `json:"batchRunId"`
	RequestID   string                 `json:"requestId"`
	ProfileID   string                 `json:"profileId"`
	Status      string                 `json:"status"`
	GeneratedAt string                 `json:"generatedAt"`
	Artifacts   []apiCaseBatchArtifact `json:"artifacts"`
}

type apiCaseBatchArtifact struct {
	Kind      string `json:"kind"`
	CaseID    string `json:"caseId,omitempty"`
	CaseRunID string `json:"caseRunId,omitempty"`
	URL       string `json:"url,omitempty"`
	Path      string `json:"path,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
}

type apiCaseBatchFailureSummary struct {
	OK          bool                     `json:"ok"`
	BatchRunID  string                   `json:"batchRunId"`
	RequestID   string                   `json:"requestId"`
	ProfileID   string                   `json:"profileId"`
	Status      string                   `json:"status"`
	Failed      int                      `json:"failed"`
	GeneratedAt string                   `json:"generatedAt"`
	Failures    []apiCaseBatchCaseReport `json:"failures"`
}

//go:embed templates/api_case_batch_report.html
var apiCaseBatchReportTemplateSource string

var apiCaseBatchReportTemplate = template.Must(template.New("api-case-batch-report").Parse(apiCaseBatchReportTemplateSource))

type apiCaseBatchRunner struct {
	mu   sync.RWMutex
	runs map[string]apiCaseBatchRunReport
}

func newAPICaseBatchRunner() *apiCaseBatchRunner {
	return &apiCaseBatchRunner{runs: map[string]apiCaseBatchRunReport{}}
}
