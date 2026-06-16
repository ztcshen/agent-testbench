// Package plangraph models workflow maps as planner-style graphs.
package plangraph

import "time"

const (
	NodeRolePrimary    = "primary"
	NodeRoleValidation = "validation"

	StateEffectAdvance   = "advance"
	StateEffectUnchanged = "unchanged"

	EdgeKindControl = "control"
	EdgeKindFixture = "fixture"

	OperationRunPathPrefix = "run_path_prefix"
	OperationRunCase       = "run_case"
)

type Graph struct {
	Map              Map               `json:"map"`
	Nodes            []Node            `json:"nodes"`
	Edges            []Edge            `json:"edges"`
	Paths            []Path            `json:"paths"`
	PathSteps        []PathStep        `json:"pathSteps"`
	Materializations []Materialization `json:"materializations"`
}

type Map struct {
	ID          string    `json:"id"`
	ProfileID   string    `json:"profileId"`
	DisplayName string    `json:"displayName,omitempty"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status,omitempty"`
	SummaryJSON string    `json:"summaryJson,omitempty"`
	CreatedAt   time.Time `json:"createdAt,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty"`
}

type Node struct {
	MapID                string `json:"mapId"`
	ID                   string `json:"id"`
	CaseID               string `json:"caseId,omitempty"`
	InterfaceNodeID      string `json:"interfaceNodeId,omitempty"`
	RequestTemplateID    string `json:"requestTemplateId,omitempty"`
	BaseCaseID           string `json:"baseCaseId,omitempty"`
	AnchorNodeID         string `json:"anchorNodeId,omitempty"`
	Role                 string `json:"role,omitempty"`
	StateEffect          string `json:"stateEffect,omitempty"`
	RenderMode           string `json:"renderMode,omitempty"`
	PatchJSON            string `json:"patchJson,omitempty"`
	ExpectedJSON         string `json:"expectedJson,omitempty"`
	RequiredPropertyJSON string `json:"requiredPropertyJson,omitempty"`
	ProvidedPropertyJSON string `json:"providedPropertyJson,omitempty"`
	SummaryJSON          string `json:"summaryJson,omitempty"`
	SortOrder            int    `json:"sortOrder,omitempty"`
}

type Edge struct {
	MapID             string `json:"mapId"`
	ID                string `json:"id"`
	FromNodeID        string `json:"fromNodeId,omitempty"`
	ToNodeID          string `json:"toNodeId,omitempty"`
	Kind              string `json:"kind,omitempty"`
	PathID            string `json:"pathId,omitempty"`
	MaterializationID string `json:"materializationId,omitempty"`
	Required          bool   `json:"required,omitempty"`
	MappingsJSON      string `json:"mappingsJson,omitempty"`
	SummaryJSON       string `json:"summaryJson,omitempty"`
	SortOrder         int    `json:"sortOrder,omitempty"`
}

type Path struct {
	MapID                string `json:"mapId"`
	ID                   string `json:"id"`
	WorkflowID           string `json:"workflowId,omitempty"`
	DisplayName          string `json:"displayName,omitempty"`
	Status               string `json:"status,omitempty"`
	RequiredPropertyJSON string `json:"requiredPropertyJson,omitempty"`
	ProvidedPropertyJSON string `json:"providedPropertyJson,omitempty"`
	SummaryJSON          string `json:"summaryJson,omitempty"`
	SortOrder            int    `json:"sortOrder,omitempty"`
}

type PathStep struct {
	MapID            string `json:"mapId"`
	PathID           string `json:"pathId"`
	StepIndex        int    `json:"stepIndex"`
	StepID           string `json:"stepId,omitempty"`
	NodeID           string `json:"nodeId,omitempty"`
	CaseID           string `json:"caseId,omitempty"`
	Required         bool   `json:"required,omitempty"`
	MaterializeAfter bool   `json:"materializeAfter,omitempty"`
	SummaryJSON      string `json:"summaryJson,omitempty"`
}

type Materialization struct {
	MapID             string `json:"mapId"`
	ID                string `json:"id"`
	FixtureID         string `json:"fixtureId,omitempty"`
	SourcePathID      string `json:"sourcePathId,omitempty"`
	SourceWorkflowID  string `json:"sourceWorkflowId,omitempty"`
	SourceUntilStep   string `json:"sourceUntilStep,omitempty"`
	SourceUntilNodeID string `json:"sourceUntilNodeId,omitempty"`
	SnapshotKind      string `json:"snapshotKind,omitempty"`
	TTLSeconds        int    `json:"ttlSeconds,omitempty"`
	Status            string `json:"status,omitempty"`
	SummaryJSON       string `json:"summaryJson,omitempty"`
	SortOrder         int    `json:"sortOrder,omitempty"`
}

type ExplainOptions struct {
	CaseID string
	NodeID string
}

type Explanation struct {
	MapID                string              `json:"mapId"`
	TargetNodeID         string              `json:"targetNodeId"`
	TargetCaseID         string              `json:"targetCaseId,omitempty"`
	RequiredPropertyJSON string              `json:"requiredPropertyJson,omitempty"`
	ProvidedPropertyJSON string              `json:"providedPropertyJson,omitempty"`
	PathID               string              `json:"pathId,omitempty"`
	LogicalPath          []PathStep          `json:"logicalPath,omitempty"`
	PathSteps            []PathStep          `json:"pathSteps,omitempty"`
	CandidatePaths       []CandidatePath     `json:"candidatePaths,omitempty"`
	RejectedReasons      []RejectedReason    `json:"rejectedReasons,omitempty"`
	Operations           []PhysicalOperation `json:"operations"`
}

type CandidatePath struct {
	PathID     string `json:"pathId"`
	WorkflowID string `json:"workflowId,omitempty"`
	Selected   bool   `json:"selected,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type RejectedReason struct {
	PathID string `json:"pathId"`
	Reason string `json:"reason"`
}

type PhysicalOperation struct {
	Kind                 string `json:"kind"`
	PathID               string `json:"pathId,omitempty"`
	UntilNodeID          string `json:"untilNodeId,omitempty"`
	NodeID               string `json:"nodeId,omitempty"`
	CaseID               string `json:"caseId,omitempty"`
	Reason               string `json:"reason,omitempty"`
	PatchJSON            string `json:"patchJson,omitempty"`
	RequiredPropertyJSON string `json:"requiredPropertyJson,omitempty"`
	ProvidedPropertyJSON string `json:"providedPropertyJson,omitempty"`
}
