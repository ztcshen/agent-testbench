package sqlstore

import "fmt"

func corePlanGraphSchemaSQL(d Dialect, types coreSchemaTypes) []string {
	idType := types.keyText
	statements := corePlanGraphMapSchemaSQL(d, types, idType)
	statements = append(statements, corePlanGraphNodeSchemaSQL(d, types, idType)...)
	statements = append(statements, corePlanGraphEdgeSchemaSQL(d, types, idType)...)
	statements = append(statements, corePlanGraphPathSchemaSQL(d, types, idType)...)
	return append(statements, corePlanGraphMaterializationSchemaSQL(d, types, idType)...)
}

func corePlanGraphMapSchemaSQL(d Dialect, types coreSchemaTypes, idType string) []string {
	return []string{
		fmt.Sprintf(`
create table if not exists test_maps (
  map_id %s primary key,
  profile_id %s not null,
  display_name %s not null,
  description %s not null,
  status %s not null,
  summary_json %s not null,
  created_at %s not null,
  updated_at %s not null
);`, idType, types.profileIDText, types.text, types.text, types.keyText, types.jsonType, types.timeType, types.timeType),
		d.CreateIndexSQL("idx_test_maps_profile", "test_maps", []string{"profile_id", "status", "updated_at", "map_id"}),
	}
}

func corePlanGraphNodeSchemaSQL(d Dialect, types coreSchemaTypes, idType string) []string {
	return []string{
		fmt.Sprintf(`
create table if not exists test_map_nodes (
  map_id %s not null,
  node_id %s not null,
  case_id %s not null,
  interface_node_id %s not null,
  request_template_id %s not null,
  base_case_id %s not null,
  anchor_node_id %s not null,
  role %s not null,
  state_effect %s not null,
  render_mode %s not null,
  patch_json %s not null,
  expected_json %s not null,
  required_property_json %s not null,
  provided_property_json %s not null,
  summary_json %s not null,
  sort_order %s not null,
  primary key (map_id, node_id),
  foreign key (map_id) references test_maps(map_id) on delete cascade
);`, idType, idType, idType, idType, idType, idType, idType, types.keyText, types.keyText, types.keyText, types.jsonType, types.jsonType, types.jsonType, types.jsonType, types.jsonType, types.intType),
		d.CreateIndexSQL("idx_test_map_nodes_case", "test_map_nodes", []string{"map_id", "case_id", "node_id"}),
		d.CreateIndexSQL("idx_test_map_nodes_interface", "test_map_nodes", []string{"map_id", "interface_node_id", "role"}),
	}
}

func corePlanGraphEdgeSchemaSQL(d Dialect, types coreSchemaTypes, idType string) []string {
	return []string{
		fmt.Sprintf(`
create table if not exists test_map_edges (
  map_id %s not null,
  edge_id %s not null,
  from_node_id %s not null,
  to_node_id %s not null,
  kind %s not null,
  path_id %s not null,
  materialization_id %s not null,
  required %s not null,
  mappings_json %s not null,
  summary_json %s not null,
  sort_order %s not null,
  primary key (map_id, edge_id),
  foreign key (map_id) references test_maps(map_id) on delete cascade
);`, idType, idType, idType, idType, types.keyText, idType, idType, types.boolType, types.jsonType, types.jsonType, types.intType),
		d.CreateIndexSQL("idx_test_map_edges_to", "test_map_edges", []string{"map_id", "to_node_id", "kind"}),
		d.CreateIndexSQL("idx_test_map_edges_from", "test_map_edges", []string{"map_id", "from_node_id", "kind"}),
	}
}

func corePlanGraphPathSchemaSQL(d Dialect, types coreSchemaTypes, idType string) []string {
	return []string{
		fmt.Sprintf(`
create table if not exists test_map_paths (
  map_id %s not null,
  path_id %s not null,
  workflow_id %s not null,
  display_name %s not null,
  status %s not null,
  required_property_json %s not null,
  provided_property_json %s not null,
  summary_json %s not null,
  sort_order %s not null,
  primary key (map_id, path_id),
  foreign key (map_id) references test_maps(map_id) on delete cascade
);`, idType, idType, idType, types.text, types.keyText, types.jsonType, types.jsonType, types.jsonType, types.intType),
		d.CreateIndexSQL("idx_test_map_paths_workflow", "test_map_paths", []string{"map_id", "workflow_id", "path_id"}),
		fmt.Sprintf(`
create table if not exists test_map_path_steps (
  map_id %s not null,
  path_id %s not null,
  step_index %s not null,
  step_id %s not null,
  node_id %s not null,
  case_id %s not null,
  required %s not null,
  materialize_after %s not null,
  summary_json %s not null,
  primary key (map_id, path_id, step_index),
  foreign key (map_id) references test_maps(map_id) on delete cascade
);`, idType, idType, types.intType, idType, idType, idType, types.boolType, types.boolType, types.jsonType),
		d.CreateIndexSQL("idx_test_map_path_steps_node", "test_map_path_steps", []string{"map_id", "node_id", "path_id", "step_index"}),
	}
}

func corePlanGraphMaterializationSchemaSQL(d Dialect, types coreSchemaTypes, idType string) []string {
	return []string{
		fmt.Sprintf(`
create table if not exists test_plan_materializations (
  map_id %s not null,
  materialization_id %s not null,
  fixture_id %s not null,
  source_path_id %s not null,
  source_workflow_id %s not null,
  source_until_step %s not null,
  source_until_node_id %s not null,
  snapshot_kind %s not null,
  ttl_seconds %s not null,
  status %s not null,
  summary_json %s not null,
  sort_order %s not null,
  primary key (map_id, materialization_id),
  foreign key (map_id) references test_maps(map_id) on delete cascade
);`, idType, idType, idType, idType, idType, idType, idType, types.keyText, types.intType, types.keyText, types.jsonType, types.intType),
		d.CreateIndexSQL("idx_test_plan_materializations_source", "test_plan_materializations", []string{"map_id", "source_path_id", "source_until_node_id"}),
	}
}
