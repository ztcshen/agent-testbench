package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

func (s *Store) SaveTestMapPlan(ctx context.Context, record store.TestMapPlanRecord) (err error) {
	record = prepareTestMapPlanRecord(record, utcNow())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackTxOnError(tx, &err)
	if err := s.deleteTestMapPlan(ctx, tx, record.Instance.ID); err != nil {
		return err
	}
	if err := s.insertTestMapPlanInstance(ctx, tx, record.Instance); err != nil {
		return err
	}
	for _, task := range record.Tasks {
		if err := s.insertTestMapPlanTask(ctx, tx, task); err != nil {
			return err
		}
	}
	for _, edge := range record.TaskEdges {
		if err := s.insertTestMapPlanTaskEdge(ctx, tx, edge); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetTestMapPlan(ctx context.Context, planID string) (store.TestMapPlanRecord, error) {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return store.TestMapPlanRecord{}, store.ErrNotFound
	}
	instance, err := s.getTestMapPlanInstance(ctx, planID)
	if err != nil {
		return store.TestMapPlanRecord{}, err
	}
	tasks, err := queryStoreRows(ctx, s.db, fmt.Sprintf(`
select plan_id, task_id, task_index, task_kind, operation, path_id, workflow_id, node_id, case_id,
  materialization_id, required_property_json, provided_property_json, cost_json, status, reason,
  workflow_run_id, api_case_run_id, evidence_root, summary_json, started_at, finished_at, created_at
from test_map_plan_tasks
where plan_id = %s
order by task_index, task_id;`, s.dialect.BindVar(1)), scanTestMapPlanTask, planID)
	if err != nil {
		return store.TestMapPlanRecord{}, err
	}
	edges, err := queryStoreRows(ctx, s.db, fmt.Sprintf(`
select plan_id, from_task_id, to_task_id, edge_kind, required, mappings_json, summary_json, sort_order
from test_map_plan_task_edges
where plan_id = %s
order by sort_order, from_task_id, to_task_id, edge_kind;`, s.dialect.BindVar(1)), scanTestMapPlanTaskEdge, planID)
	if err != nil {
		return store.TestMapPlanRecord{}, err
	}
	return store.TestMapPlanRecord{Instance: instance, Tasks: tasks, TaskEdges: edges}, nil
}

func prepareTestMapPlanRecord(record store.TestMapPlanRecord, now time.Time) store.TestMapPlanRecord {
	instance := &record.Instance
	if instance.Status == "" {
		instance.Status = "planned"
	}
	if instance.Mode == "" {
		instance.Mode = "explain"
	}
	if instance.PlannerVersion == "" {
		instance.PlannerVersion = "map-planner/v1"
	}
	instance.PlannerOptionsJSON = jsonForDB(instance.PlannerOptionsJSON, "{}")
	instance.LogicalPlanJSON = jsonForDB(instance.LogicalPlanJSON, "[]")
	instance.OptimizedPlanJSON = jsonForDB(instance.OptimizedPlanJSON, "[]")
	instance.PhysicalPlanJSON = jsonForDB(instance.PhysicalPlanJSON, "[]")
	instance.RuleTraceJSON = jsonForDB(instance.RuleTraceJSON, "[]")
	instance.CandidatePlanJSON = jsonForDB(instance.CandidatePlanJSON, "[]")
	instance.CostJSON = jsonForDB(instance.CostJSON, "{}")
	instance.PropertyJSON = jsonForDB(instance.PropertyJSON, "{}")
	instance.SummaryJSON = jsonForDB(instance.SummaryJSON, "{}")
	if instance.CreatedAt.IsZero() {
		instance.CreatedAt = now
	}
	for i := range record.Tasks {
		task := &record.Tasks[i]
		task.PlanID = stringDefault(task.PlanID, instance.ID)
		if task.Index == 0 {
			task.Index = i + 1
		}
		task.RequiredPropertyJSON = jsonForDB(task.RequiredPropertyJSON, "{}")
		task.ProvidedPropertyJSON = jsonForDB(task.ProvidedPropertyJSON, "{}")
		task.CostJSON = jsonForDB(task.CostJSON, "{}")
		task.SummaryJSON = jsonForDB(task.SummaryJSON, "{}")
		if task.Status == "" {
			task.Status = "planned"
		}
		if task.CreatedAt.IsZero() {
			task.CreatedAt = now
		}
	}
	for i := range record.TaskEdges {
		edge := &record.TaskEdges[i]
		edge.PlanID = stringDefault(edge.PlanID, instance.ID)
		edge.Kind = stringDefault(edge.Kind, "control")
		edge.MappingsJSON = jsonForDB(edge.MappingsJSON, "[]")
		edge.SummaryJSON = jsonForDB(edge.SummaryJSON, "{}")
		if edge.SortOrder == 0 {
			edge.SortOrder = i + 1
		}
	}
	return record
}

func (s *Store) deleteTestMapPlan(ctx context.Context, tx *sql.Tx, planID string) error {
	for _, tableName := range []string{
		"test_map_plan_task_edges",
		"test_map_plan_tasks",
		"test_map_plan_instances",
	} {
		query := fmt.Sprintf("delete from %s where plan_id = %s;", s.dialect.QuoteIdent(tableName), s.dialect.BindVar(1))
		if _, err := tx.ExecContext(ctx, query, planID); err != nil {
			return fmt.Errorf("delete %s for test map plan %q: %w", tableName, planID, err)
		}
	}
	return nil
}

func (s *Store) insertTestMapPlanInstance(ctx context.Context, tx *sql.Tx, item store.TestMapPlanInstance) error {
	return s.insertTestMapPlannerRow(ctx, tx, "test_map_plan_instances", testMapPlanInstanceColumns, testMapPlanInstanceValues(s.dialect, item), fmt.Sprintf("test map plan instance %q", item.ID))
}

func (s *Store) insertTestMapPlanTask(ctx context.Context, tx *sql.Tx, item store.TestMapPlanTask) error {
	return s.insertTestMapPlannerRow(ctx, tx, "test_map_plan_tasks", testMapPlanTaskColumns, testMapPlanTaskValues(s.dialect, item), fmt.Sprintf("test map plan task %q", item.ID))
}

func (s *Store) insertTestMapPlanTaskEdge(ctx context.Context, tx *sql.Tx, item store.TestMapPlanTaskEdge) error {
	return s.insertTestMapPlannerRow(ctx, tx, "test_map_plan_task_edges", testMapPlanTaskEdgeColumns, []any{
		item.PlanID, item.FromTaskID, item.ToTaskID, item.Kind, item.Required, item.MappingsJSON, item.SummaryJSON, item.SortOrder,
	}, fmt.Sprintf("test map plan task edge %q -> %q", item.FromTaskID, item.ToTaskID))
}

func (s *Store) insertTestMapPlannerRow(ctx context.Context, tx *sql.Tx, tableName string, columns []string, values []any, label string) error {
	query := fmt.Sprintf("insert into %s (\n  %s\n)\nvalues (%s);",
		s.dialect.QuoteIdent(tableName), strings.Join(columns, ",\n  "), s.bindVars(len(values)))
	_, err := tx.ExecContext(ctx, query, values...)
	if err != nil {
		return fmt.Errorf("insert %s: %w", label, err)
	}
	return nil
}

var testMapPlanInstanceColumns = []string{
	"plan_id", "map_id", "profile_id", "environment_id", "scope", "target_kind", "target_id", "mode", "status",
	"planner_version", "planner_options_json", "logical_plan_json", "optimized_plan_json", "physical_plan_json",
	"rule_trace_json", "candidate_plan_json", "cost_json", "property_json", "summary_json", "created_at", "started_at", "finished_at",
}

func testMapPlanInstanceValues(dialect Dialect, item store.TestMapPlanInstance) []any {
	return []any{
		item.ID, item.MapID, item.ProfileID, item.EnvironmentID, item.Scope, item.TargetKind, item.TargetID, item.Mode, item.Status,
		item.PlannerVersion, item.PlannerOptionsJSON, item.LogicalPlanJSON, item.OptimizedPlanJSON, item.PhysicalPlanJSON,
		item.RuleTraceJSON, item.CandidatePlanJSON, item.CostJSON, item.PropertyJSON, item.SummaryJSON,
		dbTimeArg(dialect, item.CreatedAt), dbTimeArg(dialect, item.StartedAt), dbTimeArg(dialect, item.FinishedAt),
	}
}

var testMapPlanTaskColumns = []string{
	"plan_id", "task_id", "task_index", "task_kind", "operation", "path_id", "workflow_id", "node_id", "case_id",
	"materialization_id", "required_property_json", "provided_property_json", "cost_json", "status", "reason",
	"workflow_run_id", "api_case_run_id", "evidence_root", "summary_json", "started_at", "finished_at", "created_at",
}

func testMapPlanTaskValues(dialect Dialect, item store.TestMapPlanTask) []any {
	return []any{
		item.PlanID, item.ID, item.Index, item.Kind, item.Operation, item.PathID, item.WorkflowID, item.NodeID, item.CaseID,
		item.MaterializationID, item.RequiredPropertyJSON, item.ProvidedPropertyJSON, item.CostJSON, item.Status, item.Reason,
		item.WorkflowRunID, item.APICaseRunID, item.EvidenceRoot, item.SummaryJSON,
		dbTimeArg(dialect, item.StartedAt), dbTimeArg(dialect, item.FinishedAt), dbTimeArg(dialect, item.CreatedAt),
	}
}

var testMapPlanTaskEdgeColumns = []string{
	"plan_id", "from_task_id", "to_task_id", "edge_kind", "required", "mappings_json", "summary_json", "sort_order",
}

func (s *Store) getTestMapPlanInstance(ctx context.Context, planID string) (store.TestMapPlanInstance, error) {
	query := fmt.Sprintf(`
select plan_id, map_id, profile_id, environment_id, scope, target_kind, target_id, mode, status,
  planner_version, planner_options_json, logical_plan_json, optimized_plan_json, physical_plan_json,
  rule_trace_json, candidate_plan_json, cost_json, property_json, summary_json, created_at, started_at, finished_at
from test_map_plan_instances
where plan_id = %s;`, s.dialect.BindVar(1))
	item, err := scanTestMapPlanInstance(s.db.QueryRowContext(ctx, query, planID))
	if err != nil {
		return store.TestMapPlanInstance{}, err
	}
	return item, nil
}

func scanTestMapPlanInstance(row scanner) (store.TestMapPlanInstance, error) {
	var item store.TestMapPlanInstance
	var createdAt, startedAt, finishedAt any
	if err := row.Scan(
		&item.ID, &item.MapID, &item.ProfileID, &item.EnvironmentID, &item.Scope, &item.TargetKind, &item.TargetID, &item.Mode, &item.Status,
		&item.PlannerVersion, &item.PlannerOptionsJSON, &item.LogicalPlanJSON, &item.OptimizedPlanJSON, &item.PhysicalPlanJSON,
		&item.RuleTraceJSON, &item.CandidatePlanJSON, &item.CostJSON, &item.PropertyJSON, &item.SummaryJSON,
		&createdAt, &startedAt, &finishedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return store.TestMapPlanInstance{}, store.ErrNotFound
		}
		return store.TestMapPlanInstance{}, err
	}
	item.PlannerOptionsJSON = normalizeJSONText(item.PlannerOptionsJSON)
	item.LogicalPlanJSON = normalizeJSONText(item.LogicalPlanJSON)
	item.OptimizedPlanJSON = normalizeJSONText(item.OptimizedPlanJSON)
	item.PhysicalPlanJSON = normalizeJSONText(item.PhysicalPlanJSON)
	item.RuleTraceJSON = normalizeJSONText(item.RuleTraceJSON)
	item.CandidatePlanJSON = normalizeJSONText(item.CandidatePlanJSON)
	item.CostJSON = normalizeJSONText(item.CostJSON)
	item.PropertyJSON = normalizeJSONText(item.PropertyJSON)
	item.SummaryJSON = normalizeJSONText(item.SummaryJSON)
	item.CreatedAt = decodeDBTime(createdAt)
	item.StartedAt = decodeDBTime(startedAt)
	item.FinishedAt = decodeDBTime(finishedAt)
	return item, nil
}

func scanTestMapPlanTask(row scanner) (store.TestMapPlanTask, error) {
	var item store.TestMapPlanTask
	var startedAt, finishedAt, createdAt any
	if err := row.Scan(
		&item.PlanID, &item.ID, &item.Index, &item.Kind, &item.Operation, &item.PathID, &item.WorkflowID, &item.NodeID, &item.CaseID,
		&item.MaterializationID, &item.RequiredPropertyJSON, &item.ProvidedPropertyJSON, &item.CostJSON, &item.Status, &item.Reason,
		&item.WorkflowRunID, &item.APICaseRunID, &item.EvidenceRoot, &item.SummaryJSON, &startedAt, &finishedAt, &createdAt,
	); err != nil {
		return store.TestMapPlanTask{}, err
	}
	item.RequiredPropertyJSON = normalizeJSONText(item.RequiredPropertyJSON)
	item.ProvidedPropertyJSON = normalizeJSONText(item.ProvidedPropertyJSON)
	item.CostJSON = normalizeJSONText(item.CostJSON)
	item.SummaryJSON = normalizeJSONText(item.SummaryJSON)
	item.StartedAt = decodeDBTime(startedAt)
	item.FinishedAt = decodeDBTime(finishedAt)
	item.CreatedAt = decodeDBTime(createdAt)
	return item, nil
}

func scanTestMapPlanTaskEdge(row scanner) (store.TestMapPlanTaskEdge, error) {
	var item store.TestMapPlanTaskEdge
	if err := row.Scan(&item.PlanID, &item.FromTaskID, &item.ToTaskID, &item.Kind, &item.Required, &item.MappingsJSON, &item.SummaryJSON, &item.SortOrder); err != nil {
		return store.TestMapPlanTaskEdge{}, err
	}
	item.MappingsJSON = normalizeJSONText(item.MappingsJSON)
	item.SummaryJSON = normalizeJSONText(item.SummaryJSON)
	return item, nil
}
