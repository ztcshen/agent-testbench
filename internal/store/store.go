package store

import (
	"context"
	"strings"
	"time"

	"agent-testbench/internal/domain/catalog"
	"agent-testbench/internal/domain/execution"
	"agent-testbench/internal/domain/plangraph"
)

var ErrNotFound = execution.ErrNotFound

const (
	StatusRunning = execution.StatusRunning
	StatusPassed  = execution.StatusPassed
	StatusFailed  = execution.StatusFailed
	StatusSkipped = execution.StatusSkipped
)

type Store interface {
	Closer
	RunStore
	APICaseRunStore
	EvidenceStore
	BaselineGateStore
	ProfileCatalogStore
	PlanGraphStore
	EnvironmentStore
	AgentTaskStore
}

type Closer interface {
	Close() error
}

type RunStore interface {
	CreateRun(context.Context, Run) (Run, error)
	GetRun(context.Context, string) (Run, error)
	ListRuns(context.Context) ([]Run, error)
}

type APICaseRunStore interface {
	RecordAPICaseRun(context.Context, APICaseRun) (APICaseRun, error)
	ListAPICaseRuns(context.Context, string) ([]APICaseRun, error)
}

type EvidenceStore interface {
	RecordEvidence(context.Context, EvidenceRecord) (EvidenceRecord, error)
	ListEvidence(context.Context, string) ([]EvidenceRecord, error)
	SaveTraceTopology(context.Context, TraceTopology) (TraceTopology, error)
	ListTraceTopologies(context.Context, string) ([]TraceTopology, error)
	RecordPostProcessTask(context.Context, PostProcessTask) (PostProcessTask, error)
	ListPostProcessTasks(context.Context, string) ([]PostProcessTask, error)
}

type BaselineGateStore interface {
	UpsertBaselineGate(context.Context, BaselineGate) (BaselineGate, error)
	GetBaselineGate(context.Context, string, string) (BaselineGate, error)
}

type ProfileCatalogStore interface {
	UpsertProfileIndex(context.Context, ProfileIndex) (ProfileIndex, error)
	GetProfileIndex(context.Context, string) (ProfileIndex, error)
	UpsertConfigVersion(context.Context, ConfigVersion) (ConfigVersion, error)
	GetActiveConfigVersion(context.Context) (ConfigVersion, error)
	UpsertReadModel(context.Context, ReadModel) (ReadModel, error)
	GetReadModel(context.Context, string, string) (ReadModel, error)
	ReplaceProfileCatalog(context.Context, ProfileCatalog) error
	GetProfileCatalog(context.Context) (ProfileCatalog, error)
	GetProfileCatalogIndex(context.Context) (ProfileCatalogIndex, error)
}

type PlanGraphStore interface {
	ReplaceTestPlanGraph(context.Context, TestPlanGraph) error
	GetTestPlanGraph(context.Context, string) (TestPlanGraph, error)
}

type EnvironmentStore interface {
	UpsertEnvironment(context.Context, Environment) (Environment, error)
	GetEnvironment(context.Context, string) (Environment, error)
	ListEnvironments(context.Context) ([]Environment, error)
	ReplaceEnvironmentFiles(context.Context, string, []EnvironmentFile) error
	ListEnvironmentFiles(context.Context, string) ([]EnvironmentFile, error)
	ReplaceEnvironmentServices(context.Context, string, []EnvironmentService) error
	ListEnvironmentServices(context.Context, string) ([]EnvironmentService, error)
	ReplaceEnvironmentHealthChecks(context.Context, string, []EnvironmentHealthCheck) error
	ListEnvironmentHealthChecks(context.Context, string) ([]EnvironmentHealthCheck, error)
	ReplaceEnvironmentComponentGraph(context.Context, string, EnvironmentComponentGraph) error
	GetEnvironmentComponentGraph(context.Context, string) (EnvironmentComponentGraph, error)
}

type AgentTaskStore interface {
	UpsertAgentTask(context.Context, AgentTask) (AgentTask, error)
	GetAgentTask(context.Context, string) (AgentTask, error)
	ListAgentTasks(context.Context) ([]AgentTask, error)
	RecordAgentTaskRun(context.Context, AgentTaskRun) (AgentTaskRun, error)
	ListAgentTaskRuns(context.Context, string, int) ([]AgentTaskRun, error)
}

type EnvironmentStructuredStateLister interface {
	ListEnvironmentFiles(context.Context, string) ([]EnvironmentFile, error)
	ListEnvironmentServices(context.Context, string) ([]EnvironmentService, error)
	ListEnvironmentHealthChecks(context.Context, string) ([]EnvironmentHealthCheck, error)
}

type Run = execution.Run
type APICaseRun = execution.APICaseRun
type APICaseRunRecord = execution.APICaseRunRecord

type EvidenceRecord struct {
	ID         string
	RunID      string
	CaseRunID  string
	StepID     string
	Kind       string
	URI        string
	MediaType  string
	SHA256     string
	SizeBytes  int64
	Summary    string
	Category   string
	Visibility string
	LabelsJSON string
	CreatedAt  time.Time
}

type TraceTopology struct {
	ID            string
	WorkflowRunID string
	WorkflowID    string
	StepID        string
	CaseID        string
	RequestID     string
	TraceID       string
	Status        string
	TopologyJSON  string
	TextTopology  string
	CreatedAt     time.Time
}

type PostProcessTask struct {
	ID          string
	RunID       string
	WorkflowID  string
	StepID      string
	CaseID      string
	Kind        string
	Status      string
	StartedAt   time.Time
	FinishedAt  time.Time
	DurationMs  int64
	Error       string
	SummaryJSON string
	CreatedAt   time.Time
}

type AgentTask struct {
	ID           string
	Name         string
	Kind         string
	Command      string
	Schedule     string
	Status       string
	NotifyJSON   string
	SummaryJSON  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LatestStatus string
	LatestRunID  string
	LastRunAt    time.Time
	RunCount     int
}

type AgentTaskRun struct {
	ID          string
	TaskID      string
	Status      string
	Command     string
	StartedAt   time.Time
	FinishedAt  time.Time
	DurationMs  int64
	ExitCode    int
	Output      string
	Error       string
	SummaryJSON string
	CreatedAt   time.Time
}

func PrepareAgentTaskForUpsert(t AgentTask, now time.Time) AgentTask {
	if t.Kind == "" {
		t.Kind = "cli"
	}
	if t.Status == "" {
		t.Status = "active"
	}
	if strings.TrimSpace(t.NotifyJSON) == "" {
		t.NotifyJSON = "{}"
	}
	if strings.TrimSpace(t.SummaryJSON) == "" {
		t.SummaryJSON = "{}"
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = now
	}
	return t
}

func PrepareAgentTaskRunForRecord(r AgentTaskRun, now time.Time) AgentTaskRun {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	if r.StartedAt.IsZero() {
		r.StartedAt = r.CreatedAt
	}
	if r.FinishedAt.IsZero() && r.Status != StatusRunning {
		r.FinishedAt = r.StartedAt
	}
	if r.DurationMs == 0 && !r.StartedAt.IsZero() && !r.FinishedAt.IsZero() {
		r.DurationMs = r.FinishedAt.Sub(r.StartedAt).Milliseconds()
		if r.DurationMs < 0 {
			r.DurationMs = 0
		}
	}
	if r.Status == "" {
		r.Status = StatusPassed
	}
	if strings.TrimSpace(r.SummaryJSON) == "" {
		r.SummaryJSON = "{}"
	}
	return r
}

type BaselineGate struct {
	ProfileID   string
	SubjectID   string
	Status      string
	Required    bool
	SummaryJSON string
	CheckedAt   time.Time
	UpdatedAt   time.Time
}

type ProfileIndex = catalog.ProfileIndex
type ConfigVersion = catalog.ConfigVersion
type ReadModel = catalog.ReadModel

type ProfileCatalog = catalog.ProfileCatalog
type ProfileCatalogIndex = catalog.ProfileCatalogIndex
type ProfileCatalogCounts = catalog.ProfileCatalogCounts
type CatalogService = catalog.Service
type CatalogWorkflow = catalog.Workflow
type CatalogInterfaceNode = catalog.InterfaceNode
type CatalogInterfaceNodeField = catalog.InterfaceNodeField
type CatalogAPICase = catalog.APICase
type CatalogRequestTemplate = catalog.RequestTemplate
type CatalogWorkflowBinding = catalog.WorkflowBinding
type CatalogCaseDependency = catalog.CaseDependency
type CatalogFixture = catalog.Fixture
type CatalogTemplateConfig = catalog.TemplateConfig

type TestPlanGraph = plangraph.Graph
type TestPlanMap = plangraph.Map
type TestPlanNode = plangraph.Node
type TestPlanEdge = plangraph.Edge
type TestPlanPath = plangraph.Path
type TestPlanPathStep = plangraph.PathStep
type TestPlanMaterialization = plangraph.Materialization
