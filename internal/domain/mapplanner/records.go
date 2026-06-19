package mapplanner

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type PlanRecord struct {
	Instance  PlanInstance     `json:"instance"`
	Tasks     []TaskRecord     `json:"tasks"`
	TaskEdges []TaskEdgeRecord `json:"taskEdges"`
}

type PlanInstance struct {
	ID                 string
	MapID              string
	ProfileID          string
	EnvironmentID      string
	Scope              string
	TargetKind         string
	TargetID           string
	Mode               string
	Status             string
	PlannerVersion     string
	PlannerOptionsJSON string
	LogicalPlanJSON    string
	OptimizedPlanJSON  string
	PhysicalPlanJSON   string
	RuleTraceJSON      string
	CandidatePlanJSON  string
	CostJSON           string
	PropertyJSON       string
	SummaryJSON        string
	CreatedAt          time.Time
	StartedAt          time.Time
	FinishedAt         time.Time
}

type TaskRecord struct {
	PlanID               string
	ID                   string
	Index                int
	Kind                 string
	Operation            string
	PathID               string
	WorkflowID           string
	NodeID               string
	CaseID               string
	MaterializationID    string
	RequiredPropertyJSON string
	ProvidedPropertyJSON string
	CostJSON             string
	Status               string
	Reason               string
	WorkflowRunID        string
	APICaseRunID         string
	EvidenceRoot         string
	SummaryJSON          string
	StartedAt            time.Time
	FinishedAt           time.Time
	CreatedAt            time.Time
}

type TaskEdgeRecord struct {
	PlanID       string
	FromTaskID   string
	ToTaskID     string
	Kind         string
	Required     bool
	MappingsJSON string
	SummaryJSON  string
	SortOrder    int
}

func RecordFromPlan(plan Plan, now time.Time) (PlanRecord, error) {
	if strings.TrimSpace(plan.ID) == "" {
		return PlanRecord{}, errors.New("plan id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	instance := PlanInstance{
		ID:                 plan.ID,
		MapID:              plan.MapID,
		ProfileID:          plan.ProfileID,
		EnvironmentID:      plan.EnvironmentID,
		Scope:              plan.Scope,
		TargetKind:         plan.TargetKind,
		TargetID:           plan.TargetID,
		Mode:               plan.Mode,
		Status:             plan.Status,
		PlannerVersion:     plan.PlannerVersion,
		PlannerOptionsJSON: mustJSONText(plan.PlannerOptions, "{}"),
		LogicalPlanJSON:    mustJSONText(plan.LogicalPlan, "[]"),
		OptimizedPlanJSON:  mustJSONText(plan.OptimizedPlan, "[]"),
		PhysicalPlanJSON:   mustJSONText(plan.PhysicalTasks, "[]"),
		RuleTraceJSON:      mustJSONText(plan.RulesApplied, "[]"),
		CandidatePlanJSON:  mustJSONText(plan.CandidatePlans, "[]"),
		CostJSON:           mustJSONText(plan.Cost, "{}"),
		PropertyJSON: mustJSONText(map[string]any{
			"required": plan.RequiredProperties,
			"provided": plan.ProvidedProperties,
		}, "{}"),
		SummaryJSON: mustJSONText(plan.Summary, "{}"),
		CreatedAt:   now,
		StartedAt:   now,
		FinishedAt:  now,
	}
	tasks := make([]TaskRecord, 0, len(plan.PhysicalTasks))
	for _, task := range plan.PhysicalTasks {
		tasks = append(tasks, TaskRecord{
			PlanID:               plan.ID,
			ID:                   task.ID,
			Index:                task.Index,
			Kind:                 task.Kind,
			Operation:            task.Operation,
			PathID:               task.PathID,
			WorkflowID:           task.WorkflowID,
			NodeID:               task.NodeID,
			CaseID:               task.CaseID,
			MaterializationID:    task.MaterializationID,
			RequiredPropertyJSON: mustJSONText(task.RequiredProperties, "{}"),
			ProvidedPropertyJSON: mustJSONText(task.ProvidedProperties, "{}"),
			CostJSON:             mustJSONText(task.Cost, "{}"),
			Status:               stringDefault(task.Status, TaskStatusPlanned),
			Reason:               task.Reason,
			SummaryJSON:          mustJSONText(taskRecordSummary(task), "{}"),
			CreatedAt:            now,
		})
	}
	edges := make([]TaskEdgeRecord, 0, len(plan.TaskEdges))
	for _, edge := range plan.TaskEdges {
		edges = append(edges, TaskEdgeRecord{
			PlanID:       plan.ID,
			FromTaskID:   edge.FromTaskID,
			ToTaskID:     edge.ToTaskID,
			Kind:         stringDefault(edge.Kind, "control"),
			Required:     edge.Required,
			MappingsJSON: mustJSONText(edge.Mappings, "[]"),
			SummaryJSON:  mustJSONText(edge.Summary, "{}"),
			SortOrder:    edge.SortOrder,
		})
	}
	return PlanRecord{Instance: instance, Tasks: tasks, TaskEdges: edges}, nil
}

func taskRecordSummary(task PhysicalTask) map[string]any {
	summary := map[string]any{}
	for key, value := range task.Summary {
		summary[key] = value
	}
	if strings.TrimSpace(task.ReplayGroupID) != "" {
		summary["replayGroupId"] = task.ReplayGroupID
	}
	if strings.TrimSpace(task.InterfaceNodeID) != "" {
		summary["interfaceNodeId"] = task.InterfaceNodeID
	}
	if strings.TrimSpace(task.AnchorNodeID) != "" {
		summary["anchorNodeId"] = task.AnchorNodeID
	}
	if strings.TrimSpace(task.ValidationFamily) != "" {
		summary["validationFamily"] = task.ValidationFamily
	}
	if strings.TrimSpace(task.UntilNodeID) != "" {
		summary["untilNodeId"] = task.UntilNodeID
	}
	if strings.TrimSpace(task.Reason) != "" {
		summary["reason"] = task.Reason
	}
	return summary
}

func mustJSONText(value any, defaultText string) string {
	raw, err := json.Marshal(value)
	if err != nil || string(raw) == "null" {
		return defaultText
	}
	return string(raw)
}

func stringDefault(value string, defaultText string) string {
	if strings.TrimSpace(value) == "" {
		return defaultText
	}
	return value
}
