// Package mapplanner turns Store-backed test maps into explainable execution plans.
package mapplanner

import "time"

const (
	PlannerVersion = "map-planner/v1"

	ModeExplain = "explain"
	ModeRun     = "run"

	ScopeAll       = "all"
	ScopeWorkflows = "workflows"
	ScopeCases     = "cases"
	ScopeCase      = "case"

	TargetMap      = "map"
	TargetPath     = "path"
	TargetWorkflow = "workflow"
	TargetNode     = "node"
	TargetCase     = "case"

	TaskRunPath             = "run_path"
	TaskRunPathPrefix       = "run_path_prefix"
	TaskRunCase             = "run_case"
	TaskReuseMaterialized   = "reuse_materialization"
	TaskSkip                = "skip"
	TaskGateEvidence        = "gate_evidence"
	TaskStatusPlanned       = "planned"
	TaskStatusRunning       = "running"
	TaskStatusPassed        = "passed"
	TaskStatusFailed        = "failed"
	TaskStatusSkipped       = "skipped"
	TaskStatusBlocked       = "blocked"
	RuleStatusApplied       = "applied"
	RuleStatusNotApplicable = "not_applicable"
)

type Query struct {
	MapID         string `json:"mapId,omitempty"`
	EnvironmentID string `json:"environmentId,omitempty"`
	Scope         string `json:"scope,omitempty"`
	TargetKind    string `json:"targetKind,omitempty"`
	TargetID      string `json:"targetId,omitempty"`
	CaseID        string `json:"caseId,omitempty"`
	NodeID        string `json:"nodeId,omitempty"`
	PathID        string `json:"pathId,omitempty"`
	WorkflowID    string `json:"workflowId,omitempty"`
	PlannerMode   string `json:"mode,omitempty"`
}

type Plan struct {
	ID                 string              `json:"planId,omitempty"`
	MapID              string              `json:"mapId"`
	ProfileID          string              `json:"profileId,omitempty"`
	EnvironmentID      string              `json:"environmentId,omitempty"`
	Scope              string              `json:"scope"`
	TargetKind         string              `json:"targetKind"`
	TargetID           string              `json:"targetId,omitempty"`
	TargetNodeID       string              `json:"targetNodeId,omitempty"`
	TargetCaseID       string              `json:"targetCaseId,omitempty"`
	Mode               string              `json:"mode"`
	Status             string              `json:"status"`
	PlannerVersion     string              `json:"plannerVersion"`
	PlannerOptions     map[string]any      `json:"plannerOptions"`
	LogicalPlan        []LogicalOp         `json:"logicalPlan"`
	OptimizedPlan      []LogicalOp         `json:"optimizedPlan"`
	RulesApplied       []RuleTrace         `json:"rulesApplied"`
	CandidatePlans     []CandidatePlan     `json:"candidatePlans"`
	RejectedPlans      []RejectedPlan      `json:"rejectedPlans,omitempty"`
	PhysicalTasks      []PhysicalTask      `json:"physicalTasks"`
	TaskEdges          []TaskEdge          `json:"taskEdges"`
	Operations         []PhysicalOperation `json:"operations,omitempty"`
	Cost               PlanCost            `json:"cost"`
	RequiredProperties map[string]any      `json:"requiredProperties"`
	ProvidedProperties map[string]any      `json:"providedProperties"`
	Summary            PlanSummary         `json:"summary"`
	CreatedAt          time.Time           `json:"createdAt,omitempty"`
}

type LogicalOp struct {
	ID         string         `json:"id"`
	Op         string         `json:"op"`
	TargetKind string         `json:"targetKind,omitempty"`
	TargetID   string         `json:"targetId,omitempty"`
	Children   []string       `json:"children,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

type RuleTrace struct {
	Rule    string         `json:"rule"`
	Status  string         `json:"status"`
	Before  string         `json:"before,omitempty"`
	After   string         `json:"after,omitempty"`
	Reason  string         `json:"reason,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

type CandidatePlan struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	PathID     string         `json:"pathId,omitempty"`
	WorkflowID string         `json:"workflowId,omitempty"`
	NodeID     string         `json:"nodeId,omitempty"`
	CaseID     string         `json:"caseId,omitempty"`
	Selected   bool           `json:"selected"`
	Cost       PlanCost       `json:"cost"`
	Reason     string         `json:"reason,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

type RejectedPlan struct {
	ID     string `json:"id"`
	Kind   string `json:"kind,omitempty"`
	Reason string `json:"reason"`
}

type PhysicalTask struct {
	ID                 string         `json:"id"`
	Index              int            `json:"index"`
	Kind               string         `json:"kind"`
	Operation          string         `json:"operation"`
	PathID             string         `json:"pathId,omitempty"`
	WorkflowID         string         `json:"workflowId,omitempty"`
	UntilNodeID        string         `json:"untilNodeId,omitempty"`
	NodeID             string         `json:"nodeId,omitempty"`
	CaseID             string         `json:"caseId,omitempty"`
	MaterializationID  string         `json:"materializationId,omitempty"`
	Status             string         `json:"status"`
	Reason             string         `json:"reason,omitempty"`
	RequiredProperties map[string]any `json:"requiredProperties,omitempty"`
	ProvidedProperties map[string]any `json:"providedProperties,omitempty"`
	Cost               PlanCost       `json:"cost"`
	Summary            map[string]any `json:"summary,omitempty"`
}

type TaskEdge struct {
	FromTaskID string           `json:"fromTaskId"`
	ToTaskID   string           `json:"toTaskId"`
	Kind       string           `json:"kind"`
	Required   bool             `json:"required"`
	Mappings   []map[string]any `json:"mappings,omitempty"`
	Summary    map[string]any   `json:"summary,omitempty"`
	SortOrder  int              `json:"sortOrder,omitempty"`
}

type PhysicalOperation struct {
	Kind                 string `json:"kind"`
	PathID               string `json:"pathId,omitempty"`
	WorkflowID           string `json:"workflowId,omitempty"`
	UntilNodeID          string `json:"untilNodeId,omitempty"`
	NodeID               string `json:"nodeId,omitempty"`
	CaseID               string `json:"caseId,omitempty"`
	MaterializationID    string `json:"materializationId,omitempty"`
	Reason               string `json:"reason,omitempty"`
	PatchJSON            string `json:"patchJson,omitempty"`
	RequiredPropertyJSON string `json:"requiredPropertyJson,omitempty"`
	ProvidedPropertyJSON string `json:"providedPropertyJson,omitempty"`
}

type PlanCost struct {
	EstimatedTasks int `json:"estimatedTasks,omitempty"`
	WorkflowTasks  int `json:"workflowTasks,omitempty"`
	ReplayTasks    int `json:"replayTasks,omitempty"`
	CaseTasks      int `json:"caseTasks,omitempty"`
	SkippedTasks   int `json:"skippedTasks,omitempty"`
	TotalSteps     int `json:"totalSteps,omitempty"`
}

type PlanSummary struct {
	WorkflowTasks int `json:"workflowTasks"`
	ReplayTasks   int `json:"replayTasks"`
	CaseTasks     int `json:"caseTasks"`
	SkippedTasks  int `json:"skippedTasks"`
	TotalTasks    int `json:"totalTasks"`
	TotalSteps    int `json:"totalSteps"`
}
