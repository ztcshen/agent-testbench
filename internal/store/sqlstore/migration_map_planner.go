package sqlstore

import "fmt"

func coreMapPlannerSchemaSQL(d Dialect, types coreSchemaTypes) []string {
	idType := types.runIDText
	taskIDType := types.keyText
	return []string{
		fmt.Sprintf(`
create table if not exists test_map_plan_instances (
  plan_id %s primary key,
  map_id %s not null,
  profile_id %s not null,
  environment_id %s not null,
  scope %s not null,
  target_kind %s not null,
  target_id %s not null,
  mode %s not null,
  status %s not null,
  planner_version %s not null,
  planner_options_json %s not null,
  logical_plan_json %s not null,
  optimized_plan_json %s not null,
  physical_plan_json %s not null,
  rule_trace_json %s not null,
  candidate_plan_json %s not null,
  cost_json %s not null,
  property_json %s not null,
  summary_json %s not null,
  created_at %s not null,
  started_at %s,
  finished_at %s
);`, idType, types.keyText, types.profileIDText, types.keyText, types.keyText, types.keyText, types.keyText, types.keyText, types.keyText, types.keyText, types.jsonType, types.jsonType, types.jsonType, types.jsonType, types.jsonType, types.jsonType, types.jsonType, types.jsonType, types.jsonType, types.timeType, types.timeType, types.timeType),
		d.CreateIndexSQL("idx_test_map_plan_instances_map", "test_map_plan_instances", []string{"map_id", "scope", "created_at", "plan_id"}),
		d.CreateIndexSQL("idx_test_map_plan_instances_target", "test_map_plan_instances", []string{"map_id", "target_kind", "target_id", "created_at", "plan_id"}),
		fmt.Sprintf(`
create table if not exists test_map_plan_tasks (
  plan_id %s not null,
  task_id %s not null,
  task_index %s not null,
  task_kind %s not null,
  operation %s not null,
  path_id %s not null,
  workflow_id %s not null,
  node_id %s not null,
  case_id %s not null,
  materialization_id %s not null,
  required_property_json %s not null,
  provided_property_json %s not null,
  cost_json %s not null,
  status %s not null,
  reason %s not null,
  workflow_run_id %s not null,
  api_case_run_id %s not null,
  evidence_root %s not null,
  summary_json %s not null,
  started_at %s,
  finished_at %s,
  created_at %s not null,
  primary key (plan_id, task_id),
  foreign key (plan_id) references test_map_plan_instances(plan_id) on delete cascade
);`, idType, taskIDType, types.intType, types.keyText, types.keyText, types.keyText, types.keyText, types.keyText, types.keyText, types.keyText, types.jsonType, types.jsonType, types.jsonType, types.keyText, types.text, idType, idType, types.text, types.jsonType, types.timeType, types.timeType, types.timeType),
		d.CreateIndexSQL("idx_test_map_plan_tasks_status", "test_map_plan_tasks", []string{"plan_id", "status", "task_index", "task_id"}),
		d.CreateIndexSQL("idx_test_map_plan_tasks_case", "test_map_plan_tasks", []string{"plan_id", "case_id", "task_kind", "task_id"}),
		fmt.Sprintf(`
create table if not exists test_map_plan_task_edges (
  plan_id %s not null,
  from_task_id %s not null,
  to_task_id %s not null,
  edge_kind %s not null,
  required %s not null,
  mappings_json %s not null,
  summary_json %s not null,
  sort_order %s not null,
  primary key (plan_id, from_task_id, to_task_id, edge_kind),
  foreign key (plan_id) references test_map_plan_instances(plan_id) on delete cascade
);`, idType, taskIDType, taskIDType, types.keyText, types.boolType, types.jsonType, types.jsonType, types.intType),
		d.CreateIndexSQL("idx_test_map_plan_task_edges_to", "test_map_plan_task_edges", []string{"plan_id", "to_task_id", "edge_kind"}),
	}
}
