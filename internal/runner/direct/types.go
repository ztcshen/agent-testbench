// Package direct provides Store-backed lightweight execution of local test adapters.
package direct

import (
	"errors"
	"time"
)

const Mode = "direct"
const ProtocolVersion = 1
const configScope = "direct-executor"

var ErrConflict = errors.New("direct execution conflict")
var ErrInvalid = errors.New("invalid direct execution request")

type Binding struct {
	ID               string   `json:"id"`
	DisplayName      string   `json:"displayName,omitempty"`
	ProjectDir       string   `json:"projectDir"`
	Executable       string   `json:"executable"`
	Args             []string `json:"args,omitempty"`
	EnvironmentNames []string `json:"environmentNames,omitempty"`
	TargetURL        string   `json:"targetUrl"`
	TargetEnvNames   []string `json:"targetEnvNames,omitempty"`
	EvidenceDir      string   `json:"evidenceDir"`
	TimeoutSeconds   int      `json:"timeoutSeconds,omitempty"`
}

type Case struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Kind       string   `json:"kind,omitempty"`
	SourcePath string   `json:"sourcePath"`
	Tags       []string `json:"tags,omitempty"`
}

type Manifest struct {
	ProtocolVersion int      `json:"protocolVersion"`
	Cases           []Case   `json:"cases"`
	SourceFiles     []string `json:"sourceFiles"`
}

type Registration struct {
	Mode         string            `json:"mode"`
	Binding      Binding           `json:"binding"`
	Manifest     Manifest          `json:"manifest"`
	SourceHashes map[string]string `json:"sourceHashes"`
	SourceDigest string            `json:"sourceDigest"`
	Host         string            `json:"host"`
}

type RegisterRequest struct {
	Binding          Binding `json:"binding"`
	ExpectedRevision *int64  `json:"expectedRevision,omitempty"`
}

type RegistrationResult struct {
	OK           bool         `json:"ok"`
	Mode         string       `json:"mode"`
	ProfileID    string       `json:"profileId"`
	Revision     int64        `json:"revision"`
	Registration Registration `json:"registration"`
}

type RunRequest struct {
	BindingID string         `json:"bindingId"`
	CaseID    string         `json:"caseId"`
	RunID     string         `json:"runId,omitempty"`
	Overrides map[string]any `json:"overrides,omitempty"`
}

type AdapterRequest struct {
	ProtocolVersion int            `json:"protocolVersion"`
	Operation       string         `json:"operation"`
	Mode            string         `json:"mode"`
	RunID           string         `json:"runId,omitempty"`
	CaseID          string         `json:"caseId,omitempty"`
	CaseKind        string         `json:"caseKind,omitempty"`
	TargetURL       string         `json:"targetUrl,omitempty"`
	StateDir        string         `json:"stateDir,omitempty"`
	Overrides       map[string]any `json:"overrides,omitempty"`
}

type Check struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

type AdapterResult struct {
	ProtocolVersion int            `json:"protocolVersion"`
	CaseID          string         `json:"caseId"`
	Status          string         `json:"status"`
	Executed        bool           `json:"executed"`
	DryRun          bool           `json:"dryRun,omitempty"`
	Checks          []Check        `json:"checks"`
	Data            map[string]any `json:"data,omitempty"`
}

type Plan struct {
	OK              bool              `json:"ok"`
	DryRun          bool              `json:"dryRun"`
	Mode            string            `json:"mode"`
	BindingID       string            `json:"bindingId"`
	CatalogRevision int64             `json:"catalogRevision"`
	ProfileID       string            `json:"profileId"`
	Case            Case              `json:"case"`
	SourceDigest    string            `json:"sourceDigest"`
	SourceHashes    map[string]string `json:"sourceHashes"`
	TargetURL       string            `json:"targetUrl"`
	Runtime         string            `json:"runtime"`
	Host            string            `json:"host"`
	ManagesTarget   bool              `json:"managesTarget"`
	Executable      string            `json:"executable"`
	WorkingDir      string            `json:"workingDir"`
	Overrides       map[string]any    `json:"overrides,omitempty"`
}

type Report struct {
	OK              bool           `json:"ok"`
	Mode            string         `json:"mode"`
	RunID           string         `json:"runId"`
	BindingID       string         `json:"bindingId"`
	CaseID          string         `json:"caseId"`
	CaseKey         string         `json:"caseKey"`
	Status          string         `json:"status"`
	FailureCategory string         `json:"failureCategory,omitempty"`
	Error           string         `json:"error,omitempty"`
	RequestDigest   string         `json:"requestDigest"`
	Snapshot        Plan           `json:"snapshot"`
	Result          *AdapterResult `json:"result,omitempty"`
	ExitCode        int            `json:"exitCode"`
	EvidenceDir     string         `json:"evidenceDir"`
	StartedAt       time.Time      `json:"startedAt"`
	FinishedAt      time.Time      `json:"finishedAt,omitempty"`
}
