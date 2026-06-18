package storecontract

import (
	"context"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

func requireMapPlannerContract(t *testing.T, ctx context.Context, s store.Store, started time.Time) {
	t.Helper()

	record := store.TestMapPlanRecord{
		Instance: store.TestMapPlanInstance{
			ID:                 "plan.contract",
			MapID:              "map.contract",
			ProfileID:          contractProfileID,
			EnvironmentID:      "env.contract",
			Scope:              "all",
			TargetKind:         "map",
			TargetID:           "map.contract",
			Mode:               "explain",
			Status:             store.StatusPassed,
			PlannerVersion:     "map-planner/v1",
			PlannerOptionsJSON: `{"scope":"all"}`,
			LogicalPlanJSON:    `[{"id":"logical.scan","op":"scan_map"}]`,
			OptimizedPlanJSON:  `[{"id":"logical.scan","op":"scan_map"}]`,
			PhysicalPlanJSON:   `[{"id":"task.workflow","kind":"run_path"}]`,
			RuleTraceJSON:      `[{"rule":"select_candidate_paths","status":"applied"}]`,
			CandidatePlanJSON:  `[{"id":"candidate.workflow","selected":true}]`,
			CostJSON:           `{"estimatedTasks":1}`,
			PropertyJSON:       `{"required":{},"provided":{}}`,
			SummaryJSON:        `{"workflowTasks":1}`,
			CreatedAt:          started,
			StartedAt:          started,
			FinishedAt:         started,
		},
		Tasks: []store.TestMapPlanTask{{
			PlanID:               "plan.contract",
			ID:                   "task.workflow",
			Index:                1,
			Kind:                 "run_path",
			Operation:            "run_path",
			PathID:               "workflow.contract",
			WorkflowID:           "workflow.contract",
			RequiredPropertyJSON: `{}`,
			ProvidedPropertyJSON: `{}`,
			CostJSON:             `{"steps":2}`,
			Status:               "planned",
			Reason:               "run mapped workflow path",
			SummaryJSON:          `{}`,
			CreatedAt:            started,
		}},
		TaskEdges: []store.TestMapPlanTaskEdge{{
			PlanID:       "plan.contract",
			FromTaskID:   "task.workflow",
			ToTaskID:     "task.case",
			Kind:         "control",
			Required:     true,
			MappingsJSON: `[]`,
			SummaryJSON:  `{}`,
			SortOrder:    1,
		}},
	}

	if err := s.SaveTestMapPlan(ctx, record); err != nil {
		t.Fatalf("save test map plan: %v", err)
	}
	loaded, err := s.GetTestMapPlan(ctx, "plan.contract")
	if err != nil {
		t.Fatalf("get test map plan: %v", err)
	}
	if loaded.Instance.ID != "plan.contract" || loaded.Instance.MapID != "map.contract" || loaded.Instance.LogicalPlanJSON == "" {
		t.Fatalf("loaded instance = %#v", loaded.Instance)
	}
	if len(loaded.Tasks) != 1 || loaded.Tasks[0].ID != "task.workflow" || loaded.Tasks[0].PathID != "workflow.contract" {
		t.Fatalf("loaded tasks = %#v", loaded.Tasks)
	}
	if len(loaded.TaskEdges) != 1 || loaded.TaskEdges[0].FromTaskID != "task.workflow" || !loaded.TaskEdges[0].Required {
		t.Fatalf("loaded task edges = %#v", loaded.TaskEdges)
	}
	plans, err := s.ListTestMapPlans(ctx, "map.contract", 10)
	if err != nil {
		t.Fatalf("list test map plans: %v", err)
	}
	if len(plans) != 1 || plans[0].ID != "plan.contract" || plans[0].MapID != "map.contract" || plans[0].EnvironmentID != "env.contract" {
		t.Fatalf("listed plans = %#v", plans)
	}
}
