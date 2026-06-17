package main

import (
	"agent-testbench/internal/domain/plangraph"
	"agent-testbench/internal/store"
)

type mapReviewCountsReport struct {
	Nodes            int `json:"nodes"`
	Edges            int `json:"edges"`
	Paths            int `json:"paths"`
	PathSteps        int `json:"pathSteps"`
	Materializations int `json:"materializations"`
	Warnings         int `json:"warnings"`
}

type mapReviewDocument struct {
	Version          string                          `json:"version"`
	Map              store.TestPlanMap               `json:"map"`
	Counts           mapReviewCountsReport           `json:"counts"`
	Nodes            []mapReviewNode                 `json:"nodes"`
	Edges            []mapReviewEdge                 `json:"edges"`
	Paths            []mapReviewPath                 `json:"paths"`
	Materializations []store.TestPlanMaterialization `json:"materializations"`
	Warnings         []string                        `json:"warnings"`
	GeneratedBy      string                          `json:"generatedBy"`
}

type mapReviewNode struct {
	ID                   string                    `json:"id"`
	CaseID               string                    `json:"caseId,omitempty"`
	DisplayName          string                    `json:"displayName"`
	Description          string                    `json:"description,omitempty"`
	InterfaceNodeID      string                    `json:"interfaceNodeId,omitempty"`
	RequestTemplateID    string                    `json:"requestTemplateId,omitempty"`
	BaseCaseID           string                    `json:"baseCaseId,omitempty"`
	AnchorNodeID         string                    `json:"anchorNodeId,omitempty"`
	Role                 string                    `json:"role,omitempty"`
	StateEffect          string                    `json:"stateEffect,omitempty"`
	RenderMode           string                    `json:"renderMode,omitempty"`
	CaseType             string                    `json:"caseType,omitempty"`
	Scenario             string                    `json:"scenario,omitempty"`
	Tags                 []string                  `json:"tags,omitempty"`
	Priority             string                    `json:"priority,omitempty"`
	Owner                string                    `json:"owner,omitempty"`
	PatchJSON            string                    `json:"patchJson,omitempty"`
	ExpectedJSON         string                    `json:"expectedJson,omitempty"`
	RequiredPropertyJSON string                    `json:"requiredPropertyJson,omitempty"`
	ProvidedPropertyJSON string                    `json:"providedPropertyJson,omitempty"`
	SummaryJSON          string                    `json:"summaryJson,omitempty"`
	RequestTemplate      *mapReviewRequestTemplate `json:"requestTemplate,omitempty"`
	Paths                []mapReviewNodePath       `json:"paths,omitempty"`
	Explanation          *plangraph.Explanation    `json:"explanation,omitempty"`
	Layout               mapReviewNodeLayout       `json:"layout"`
	SharedCount          int                       `json:"sharedCount"`
}

type mapReviewRequestTemplate struct {
	ID           string `json:"id"`
	DisplayName  string `json:"displayName,omitempty"`
	NodeID       string `json:"nodeId,omitempty"`
	Method       string `json:"method,omitempty"`
	Path         string `json:"path,omitempty"`
	TemplateJSON string `json:"templateJson,omitempty"`
	Version      string `json:"version,omitempty"`
	Status       string `json:"status,omitempty"`
}

type mapReviewNodePath struct {
	PathID      string `json:"pathId"`
	WorkflowID  string `json:"workflowId,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	StepIndex   int    `json:"stepIndex"`
	StepID      string `json:"stepId,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

type mapReviewNodeLayout struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type mapReviewEdge struct {
	ID                string `json:"id"`
	FromNodeID        string `json:"fromNodeId"`
	ToNodeID          string `json:"toNodeId"`
	Kind              string `json:"kind"`
	PathID            string `json:"pathId,omitempty"`
	MaterializationID string `json:"materializationId,omitempty"`
	Required          bool   `json:"required,omitempty"`
	MappingsJSON      string `json:"mappingsJson,omitempty"`
	SummaryJSON       string `json:"summaryJson,omitempty"`
	Generated         bool   `json:"generated,omitempty"`
	SortOrder         int    `json:"sortOrder,omitempty"`
}

type mapReviewPath struct {
	ID                   string                   `json:"id"`
	WorkflowID           string                   `json:"workflowId,omitempty"`
	DisplayName          string                   `json:"displayName,omitempty"`
	Status               string                   `json:"status,omitempty"`
	Color                string                   `json:"color"`
	RequiredPropertyJSON string                   `json:"requiredPropertyJson,omitempty"`
	ProvidedPropertyJSON string                   `json:"providedPropertyJson,omitempty"`
	SummaryJSON          string                   `json:"summaryJson,omitempty"`
	Steps                []store.TestPlanPathStep `json:"steps"`
}

type mapReviewNodeContext struct {
	cases       map[string]store.CatalogAPICase
	templates   map[string]mapReviewRequestTemplate
	usageByNode map[string][]mapReviewNodePath
	layout      map[string]mapReviewNodeLayout
}

type mapReviewCaseDetails struct {
	displayName string
	description string
	caseType    string
	scenario    string
	tags        []string
	priority    string
	owner       string
}

type mapReviewLayoutSlot struct {
	level int
	row   float64
	count int
}
