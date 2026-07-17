package storecontract

import (
	"context"
	"errors"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

func requireMapPlannerContract(t *testing.T, ctx context.Context, s store.Store, started time.Time) {
	t.Helper()
	record := mapPlannerContractRecord(started)
	loaded := requireStoredMapPlannerRecord(t, ctx, s, record)
	checkpointed := requireMapPlannerCheckpoint(t, ctx, s, loaded, started.Add(time.Second))
	requireListedMapPlannerRecord(t, ctx, s)
	requireMapPlannerLeaseContract(t, ctx, s, checkpointed, started.Add(2*time.Second))
}

func mapPlannerContractRecord(started time.Time) store.TestMapPlanRecord {
	return store.TestMapPlanRecord{
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
}

func requireStoredMapPlannerRecord(t *testing.T, ctx context.Context, s store.Store, record store.TestMapPlanRecord) store.TestMapPlanRecord {
	t.Helper()
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
	return loaded
}

func requireMapPlannerCheckpoint(t *testing.T, ctx context.Context, s store.Store, loaded store.TestMapPlanRecord, finishedAt time.Time) store.TestMapPlanRecord {
	t.Helper()
	checkpoint := s.(store.MapPlannerCheckpointStore)
	loaded.Tasks[0].Status = store.StatusPassed
	loaded.Tasks[0].SummaryJSON = `{"checkpoint":true}`
	loaded.Tasks[0].FinishedAt = finishedAt
	if err := checkpoint.UpdateTestMapPlanTask(ctx, loaded.Tasks[0]); err != nil {
		t.Fatalf("checkpoint test map task: %v", err)
	}
	loaded.Instance.Status = store.StatusPassed
	loaded.Instance.SummaryJSON = `{"completed":1}`
	loaded.Instance.FinishedAt = finishedAt
	if err := checkpoint.UpdateTestMapPlanInstance(ctx, loaded.Instance); err != nil {
		t.Fatalf("checkpoint test map plan instance: %v", err)
	}
	checkpointed, err := s.GetTestMapPlan(ctx, "plan.contract")
	if err != nil {
		t.Fatalf("get checkpointed test map plan: %v", err)
	}
	if checkpointed.Instance.Status != store.StatusPassed || checkpointed.Tasks[0].Status != store.StatusPassed || checkpointed.Tasks[0].SummaryJSON != `{"checkpoint":true}` {
		t.Fatalf("checkpointed test map plan = %#v", checkpointed)
	}
	return checkpointed
}

func requireListedMapPlannerRecord(t *testing.T, ctx context.Context, s store.Store) {
	t.Helper()
	plans, err := s.ListTestMapPlans(ctx, "map.contract", 10)
	if err != nil {
		t.Fatalf("list test map plans: %v", err)
	}
	if len(plans) != 1 || plans[0].ID != "plan.contract" || plans[0].MapID != "map.contract" || plans[0].EnvironmentID != "env.contract" {
		t.Fatalf("listed plans = %#v", plans)
	}
}

func requireMapPlannerLeaseContract(t *testing.T, ctx context.Context, s store.Store, record store.TestMapPlanRecord, now time.Time) {
	t.Helper()
	leases, ok := s.(store.MapPlannerLeaseStore)
	if !ok {
		t.Fatalf("Store does not implement MapPlannerLeaseStore")
	}
	first, err := leases.ClaimTestMapPlanLease(ctx, store.TestMapPlanLease{
		PlanID: record.Instance.ID, OwnerID: "contract.owner.one", Token: "contract-token-one", ExpiresAt: now.Add(time.Second),
	}, now, false)
	if err != nil {
		t.Fatalf("claim test map plan lease: %v", err)
	}
	_, err = leases.ClaimTestMapPlanLease(ctx, store.TestMapPlanLease{
		PlanID: record.Instance.ID, OwnerID: "contract.owner.two", Token: "contract-token-two", ExpiresAt: now.Add(time.Minute),
	}, now, false)
	if !errors.Is(err, store.ErrTestMapPlanLeaseConflict) {
		t.Fatalf("concurrent test map plan claim error = %v", err)
	}

	record.Tasks[0].Status = store.StatusRunning
	record.Tasks[0].StartedAt = now
	first, err = leases.UpdateTestMapPlanTaskWithLease(ctx, first, record.Tasks[0], now, now.Add(time.Second))
	if err != nil {
		t.Fatalf("checkpoint claimed test map task: %v", err)
	}
	stale := first
	stale.Token = "stale-token"
	if _, err := leases.UpdateTestMapPlanTaskWithLease(ctx, stale, record.Tasks[0], now, now.Add(time.Second)); !errors.Is(err, store.ErrTestMapPlanLeaseLost) {
		t.Fatalf("stale test map plan checkpoint error = %v", err)
	}

	secondNow := now.Add(2 * time.Second)
	second, err := leases.ClaimTestMapPlanLease(ctx, store.TestMapPlanLease{
		PlanID: record.Instance.ID, OwnerID: "contract.owner.two", Token: "contract-token-two", ExpiresAt: secondNow.Add(time.Minute),
	}, secondNow, false)
	if err != nil {
		t.Fatalf("recover expired test map plan lease: %v", err)
	}
	staleFinal := record.Instance
	staleFinal.Status = store.StatusFailed
	staleFinal.FinishedAt = secondNow
	if err := leases.ReleaseTestMapPlanLease(ctx, first, staleFinal, secondNow); !errors.Is(err, store.ErrTestMapPlanLeaseLost) {
		t.Fatalf("stale owner finish error = %v", err)
	}
	record.Tasks[0].Status = store.StatusFailed
	record.Tasks[0].Reason = "interrupted-unknown-outcome"
	record.Tasks[0].FinishedAt = secondNow
	record.Instance.Status = store.StatusRunning
	record.Instance.StartedAt = secondNow
	record.Instance.FinishedAt = time.Time{}
	second, err = leases.ResetTestMapPlanWithLease(ctx, second, record, secondNow, secondNow.Add(time.Minute))
	if err != nil {
		t.Fatalf("reset recovered test map plan with lease: %v", err)
	}
	reset, err := s.GetTestMapPlan(ctx, record.Instance.ID)
	if err != nil {
		t.Fatalf("get reset test map plan: %v", err)
	}
	if reset.Instance.Status != store.StatusRunning || reset.Tasks[0].Reason != "interrupted-unknown-outcome" {
		t.Fatalf("owner-guarded reset map plan = %#v", reset)
	}
	record.Instance.Status = store.StatusPassed
	record.Instance.StartedAt = secondNow
	record.Instance.FinishedAt = secondNow.Add(time.Second)
	if err := leases.ReleaseTestMapPlanLease(ctx, second, record.Instance, secondNow); err != nil {
		t.Fatalf("finish and release test map plan lease: %v", err)
	}
}
