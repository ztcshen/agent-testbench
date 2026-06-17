package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

func (s *Store) ReplaceTestPlanGraph(ctx context.Context, graph store.TestPlanGraph) (err error) {
	graph = prepareTestPlanGraphForReplace(graph, utcNow())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackTxOnError(tx, &err)
	if err := s.deleteTestPlanGraph(ctx, tx, graph.Map.ID); err != nil {
		return err
	}
	if err := s.insertTestPlanMap(ctx, tx, graph.Map); err != nil {
		return err
	}
	for _, item := range graph.Nodes {
		if err := s.insertTestPlanNode(ctx, tx, item); err != nil {
			return err
		}
	}
	for _, item := range graph.Paths {
		if err := s.insertTestPlanPath(ctx, tx, item); err != nil {
			return err
		}
	}
	for _, item := range graph.PathSteps {
		if err := s.insertTestPlanPathStep(ctx, tx, item); err != nil {
			return err
		}
	}
	for _, item := range graph.Materializations {
		if err := s.insertTestPlanMaterialization(ctx, tx, item); err != nil {
			return err
		}
	}
	for _, item := range graph.Edges {
		if err := s.insertTestPlanEdge(ctx, tx, item); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *Store) GetTestPlanGraph(ctx context.Context, mapID string) (store.TestPlanGraph, error) {
	mapID = strings.TrimSpace(mapID)
	if mapID == "" {
		return store.TestPlanGraph{}, store.ErrNotFound
	}
	planMap, err := s.getTestPlanMap(ctx, mapID)
	if err != nil {
		return store.TestPlanGraph{}, err
	}
	mapIDBind := s.dialect.BindVar(1)
	nodes, err := queryStoreRows(ctx, s.db, fmt.Sprintf(`
select map_id, node_id, case_id, interface_node_id, request_template_id, base_case_id, anchor_node_id,
  role, state_effect, render_mode, patch_json, expected_json, required_property_json, provided_property_json,
  summary_json, sort_order
from test_map_nodes
where map_id = %s
order by sort_order, node_id;`, mapIDBind), scanTestPlanNode, mapID)
	if err != nil {
		return store.TestPlanGraph{}, err
	}
	edges, err := queryStoreRows(ctx, s.db, fmt.Sprintf(`
select map_id, edge_id, from_node_id, to_node_id, kind, path_id, materialization_id, required,
  mappings_json, summary_json, sort_order
from test_map_edges
where map_id = %s
order by sort_order, edge_id;`, mapIDBind), scanTestPlanEdge, mapID)
	if err != nil {
		return store.TestPlanGraph{}, err
	}
	paths, err := queryStoreRows(ctx, s.db, fmt.Sprintf(`
select map_id, path_id, workflow_id, display_name, status, required_property_json, provided_property_json,
  summary_json, sort_order
from test_map_paths
where map_id = %s
order by sort_order, path_id;`, mapIDBind), scanTestPlanPath, mapID)
	if err != nil {
		return store.TestPlanGraph{}, err
	}
	pathSteps, err := queryStoreRows(ctx, s.db, fmt.Sprintf(`
select map_id, path_id, step_index, step_id, node_id, case_id, required, materialize_after, summary_json
from test_map_path_steps
where map_id = %s
order by path_id, step_index;`, mapIDBind), scanTestPlanPathStep, mapID)
	if err != nil {
		return store.TestPlanGraph{}, err
	}
	materializations, err := queryStoreRows(ctx, s.db, fmt.Sprintf(`
select map_id, materialization_id, fixture_id, source_path_id, source_workflow_id, source_until_step,
  source_until_node_id, snapshot_kind, ttl_seconds, status, summary_json, sort_order
from test_plan_materializations
where map_id = %s
order by sort_order, materialization_id;`, mapIDBind), scanTestPlanMaterialization, mapID)
	if err != nil {
		return store.TestPlanGraph{}, err
	}
	return store.TestPlanGraph{
		Map:              planMap,
		Nodes:            nodes,
		Edges:            edges,
		Paths:            paths,
		PathSteps:        pathSteps,
		Materializations: materializations,
	}, nil
}

func prepareTestPlanGraphForReplace(graph store.TestPlanGraph, now time.Time) store.TestPlanGraph {
	graph.Map.ID = strings.TrimSpace(graph.Map.ID)
	if graph.Map.Status == "" {
		graph.Map.Status = "active"
	}
	if strings.TrimSpace(graph.Map.SummaryJSON) == "" {
		graph.Map.SummaryJSON = "{}"
	}
	applyAuditTimeDefaults(&graph.Map.CreatedAt, &graph.Map.UpdatedAt, now)
	for i := range graph.Nodes {
		graph.Nodes[i] = prepareTestPlanNodeForInsert(graph.Map.ID, graph.Nodes[i])
	}
	for i := range graph.Edges {
		graph.Edges[i] = prepareTestPlanEdgeForInsert(graph.Map.ID, graph.Edges[i])
	}
	for i := range graph.Paths {
		graph.Paths[i] = prepareTestPlanPathForInsert(graph.Map.ID, graph.Paths[i])
	}
	for i := range graph.PathSteps {
		graph.PathSteps[i] = prepareTestPlanPathStepForInsert(graph.Map.ID, graph.PathSteps[i])
	}
	for i := range graph.Materializations {
		graph.Materializations[i] = prepareTestPlanMaterializationForInsert(graph.Map.ID, graph.Materializations[i])
	}
	return graph
}

func prepareTestPlanNodeForInsert(mapID string, item store.TestPlanNode) store.TestPlanNode {
	item.MapID = stringDefault(item.MapID, mapID)
	item.ID = strings.TrimSpace(item.ID)
	item.CaseID = stringDefault(item.CaseID, item.ID)
	item.Role = stringDefault(item.Role, "primary")
	item.StateEffect = stringDefault(item.StateEffect, "advance")
	item.PatchJSON = jsonForDB(item.PatchJSON, "{}")
	item.ExpectedJSON = jsonForDB(item.ExpectedJSON, "{}")
	item.RequiredPropertyJSON = jsonForDB(item.RequiredPropertyJSON, "{}")
	item.ProvidedPropertyJSON = jsonForDB(item.ProvidedPropertyJSON, "{}")
	item.SummaryJSON = jsonForDB(item.SummaryJSON, "{}")
	return item
}

func prepareTestPlanEdgeForInsert(mapID string, item store.TestPlanEdge) store.TestPlanEdge {
	item.MapID = stringDefault(item.MapID, mapID)
	item.ID = strings.TrimSpace(item.ID)
	item.Kind = stringDefault(item.Kind, "control")
	item.MappingsJSON = jsonForDB(item.MappingsJSON, "[]")
	item.SummaryJSON = jsonForDB(item.SummaryJSON, "{}")
	return item
}

func prepareTestPlanPathForInsert(mapID string, item store.TestPlanPath) store.TestPlanPath {
	item.MapID = stringDefault(item.MapID, mapID)
	item.ID = strings.TrimSpace(item.ID)
	item.WorkflowID = stringDefault(item.WorkflowID, item.ID)
	item.Status = stringDefault(item.Status, "active")
	item.RequiredPropertyJSON = jsonForDB(item.RequiredPropertyJSON, "{}")
	item.ProvidedPropertyJSON = jsonForDB(item.ProvidedPropertyJSON, "{}")
	item.SummaryJSON = jsonForDB(item.SummaryJSON, "{}")
	return item
}

func prepareTestPlanPathStepForInsert(mapID string, item store.TestPlanPathStep) store.TestPlanPathStep {
	item.MapID = stringDefault(item.MapID, mapID)
	item.SummaryJSON = jsonForDB(item.SummaryJSON, "{}")
	return item
}

func prepareTestPlanMaterializationForInsert(mapID string, item store.TestPlanMaterialization) store.TestPlanMaterialization {
	item.MapID = stringDefault(item.MapID, mapID)
	item.ID = strings.TrimSpace(item.ID)
	item.FixtureID = stringDefault(item.FixtureID, item.ID)
	item.SnapshotKind = stringDefault(item.SnapshotKind, "workflow_prefix")
	item.Status = stringDefault(item.Status, "active")
	item.SummaryJSON = jsonForDB(item.SummaryJSON, "{}")
	return item
}

func (s *Store) deleteTestPlanGraph(ctx context.Context, tx *sql.Tx, mapID string) error {
	for _, tableName := range []string{
		"test_map_edges",
		"test_plan_materializations",
		"test_map_path_steps",
		"test_map_paths",
		"test_map_nodes",
		"test_maps",
	} {
		query := fmt.Sprintf("delete from %s where map_id = %s;", s.dialect.QuoteIdent(tableName), s.dialect.BindVar(1))
		if _, err := tx.ExecContext(ctx, query, mapID); err != nil {
			return fmt.Errorf("delete %s for test plan map %q: %w", tableName, mapID, err)
		}
	}
	return nil
}

func (s *Store) insertTestPlanMap(ctx context.Context, tx *sql.Tx, item store.TestPlanMap) error {
	query := fmt.Sprintf(`
insert into test_maps (
  map_id, profile_id, display_name, description, status, summary_json, created_at, updated_at
)
values (%s);`, s.bindVars(8))
	_, err := tx.ExecContext(ctx, query,
		item.ID, item.ProfileID, item.DisplayName, item.Description, item.Status, jsonForDB(item.SummaryJSON, "{}"),
		dbTimeArg(s.dialect, item.CreatedAt), dbTimeArg(s.dialect, item.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert test plan map %q: %w", item.ID, err)
	}
	return nil
}

func (s *Store) insertTestPlanNode(ctx context.Context, tx *sql.Tx, item store.TestPlanNode) error {
	query := fmt.Sprintf(`
insert into test_map_nodes (
  map_id, node_id, case_id, interface_node_id, request_template_id, base_case_id, anchor_node_id,
  role, state_effect, render_mode, patch_json, expected_json, required_property_json, provided_property_json,
  summary_json, sort_order
)
values (%s);`, s.bindVars(16))
	_, err := tx.ExecContext(ctx, query,
		item.MapID, item.ID, item.CaseID, item.InterfaceNodeID, item.RequestTemplateID, item.BaseCaseID, item.AnchorNodeID,
		item.Role, item.StateEffect, item.RenderMode, item.PatchJSON, item.ExpectedJSON, item.RequiredPropertyJSON, item.ProvidedPropertyJSON,
		item.SummaryJSON, item.SortOrder,
	)
	if err != nil {
		return fmt.Errorf("insert test plan node %q: %w", item.ID, err)
	}
	return nil
}

func (s *Store) insertTestPlanEdge(ctx context.Context, tx *sql.Tx, item store.TestPlanEdge) error {
	query := fmt.Sprintf(`
insert into test_map_edges (
  map_id, edge_id, from_node_id, to_node_id, kind, path_id, materialization_id, required,
  mappings_json, summary_json, sort_order
)
values (%s);`, s.bindVars(11))
	_, err := tx.ExecContext(ctx, query,
		item.MapID, item.ID, item.FromNodeID, item.ToNodeID, item.Kind, item.PathID, item.MaterializationID, item.Required,
		item.MappingsJSON, item.SummaryJSON, item.SortOrder,
	)
	if err != nil {
		return fmt.Errorf("insert test plan edge %q: %w", item.ID, err)
	}
	return nil
}

func (s *Store) insertTestPlanPath(ctx context.Context, tx *sql.Tx, item store.TestPlanPath) error {
	query := fmt.Sprintf(`
insert into test_map_paths (
  map_id, path_id, workflow_id, display_name, status, required_property_json, provided_property_json,
  summary_json, sort_order
)
values (%s);`, s.bindVars(9))
	_, err := tx.ExecContext(ctx, query,
		item.MapID, item.ID, item.WorkflowID, item.DisplayName, item.Status, item.RequiredPropertyJSON, item.ProvidedPropertyJSON,
		item.SummaryJSON, item.SortOrder,
	)
	if err != nil {
		return fmt.Errorf("insert test plan path %q: %w", item.ID, err)
	}
	return nil
}

func (s *Store) insertTestPlanPathStep(ctx context.Context, tx *sql.Tx, item store.TestPlanPathStep) error {
	query := fmt.Sprintf(`
insert into test_map_path_steps (
  map_id, path_id, step_index, step_id, node_id, case_id, required, materialize_after, summary_json
)
values (%s);`, s.bindVars(9))
	_, err := tx.ExecContext(ctx, query,
		item.MapID, item.PathID, item.StepIndex, item.StepID, item.NodeID, item.CaseID, item.Required, item.MaterializeAfter, item.SummaryJSON,
	)
	if err != nil {
		return fmt.Errorf("insert test plan path step %q/%d: %w", item.PathID, item.StepIndex, err)
	}
	return nil
}

func (s *Store) insertTestPlanMaterialization(ctx context.Context, tx *sql.Tx, item store.TestPlanMaterialization) error {
	query := fmt.Sprintf(`
insert into test_plan_materializations (
  map_id, materialization_id, fixture_id, source_path_id, source_workflow_id, source_until_step,
  source_until_node_id, snapshot_kind, ttl_seconds, status, summary_json, sort_order
)
values (%s);`, s.bindVars(12))
	_, err := tx.ExecContext(ctx, query,
		item.MapID, item.ID, item.FixtureID, item.SourcePathID, item.SourceWorkflowID, item.SourceUntilStep,
		item.SourceUntilNodeID, item.SnapshotKind, item.TTLSeconds, item.Status, item.SummaryJSON, item.SortOrder,
	)
	if err != nil {
		return fmt.Errorf("insert test plan materialization %q: %w", item.ID, err)
	}
	return nil
}

func (s *Store) getTestPlanMap(ctx context.Context, mapID string) (store.TestPlanMap, error) {
	query := fmt.Sprintf(`
select map_id, profile_id, display_name, description, status, summary_json, created_at, updated_at
from test_maps
where map_id = %s;`, s.dialect.BindVar(1))
	row := s.db.QueryRowContext(ctx, query, mapID)
	var item store.TestPlanMap
	var createdAt, updatedAt any
	if err := row.Scan(&item.ID, &item.ProfileID, &item.DisplayName, &item.Description, &item.Status, &item.SummaryJSON, &createdAt, &updatedAt); err != nil {
		return store.TestPlanMap{}, scanStoreRowError(err)
	}
	item.SummaryJSON = normalizeJSONText(item.SummaryJSON)
	applyDecodedAuditTimes(createdAt, updatedAt, &item.CreatedAt, &item.UpdatedAt)
	return item, nil
}

func scanTestPlanNode(row scanner) (store.TestPlanNode, error) {
	var item store.TestPlanNode
	err := row.Scan(
		&item.MapID, &item.ID, &item.CaseID, &item.InterfaceNodeID, &item.RequestTemplateID, &item.BaseCaseID, &item.AnchorNodeID,
		&item.Role, &item.StateEffect, &item.RenderMode, &item.PatchJSON, &item.ExpectedJSON, &item.RequiredPropertyJSON, &item.ProvidedPropertyJSON,
		&item.SummaryJSON, &item.SortOrder,
	)
	normalizeTestPlanNodeJSON(&item)
	return item, err
}

func scanTestPlanEdge(row scanner) (store.TestPlanEdge, error) {
	var item store.TestPlanEdge
	err := row.Scan(
		&item.MapID, &item.ID, &item.FromNodeID, &item.ToNodeID, &item.Kind, &item.PathID, &item.MaterializationID, &item.Required,
		&item.MappingsJSON, &item.SummaryJSON, &item.SortOrder,
	)
	item.MappingsJSON = normalizeJSONText(item.MappingsJSON)
	item.SummaryJSON = normalizeJSONText(item.SummaryJSON)
	return item, err
}

func scanTestPlanPath(row scanner) (store.TestPlanPath, error) {
	var item store.TestPlanPath
	err := row.Scan(&item.MapID, &item.ID, &item.WorkflowID, &item.DisplayName, &item.Status, &item.RequiredPropertyJSON, &item.ProvidedPropertyJSON, &item.SummaryJSON, &item.SortOrder)
	item.RequiredPropertyJSON = normalizeJSONText(item.RequiredPropertyJSON)
	item.ProvidedPropertyJSON = normalizeJSONText(item.ProvidedPropertyJSON)
	item.SummaryJSON = normalizeJSONText(item.SummaryJSON)
	return item, err
}

func scanTestPlanPathStep(row scanner) (store.TestPlanPathStep, error) {
	var item store.TestPlanPathStep
	err := row.Scan(&item.MapID, &item.PathID, &item.StepIndex, &item.StepID, &item.NodeID, &item.CaseID, &item.Required, &item.MaterializeAfter, &item.SummaryJSON)
	item.SummaryJSON = normalizeJSONText(item.SummaryJSON)
	return item, err
}

func scanTestPlanMaterialization(row scanner) (store.TestPlanMaterialization, error) {
	var item store.TestPlanMaterialization
	err := row.Scan(
		&item.MapID, &item.ID, &item.FixtureID, &item.SourcePathID, &item.SourceWorkflowID, &item.SourceUntilStep,
		&item.SourceUntilNodeID, &item.SnapshotKind, &item.TTLSeconds, &item.Status, &item.SummaryJSON, &item.SortOrder,
	)
	item.SummaryJSON = normalizeJSONText(item.SummaryJSON)
	return item, err
}

func normalizeTestPlanNodeJSON(item *store.TestPlanNode) {
	item.PatchJSON = normalizeJSONText(item.PatchJSON)
	item.ExpectedJSON = normalizeJSONText(item.ExpectedJSON)
	item.RequiredPropertyJSON = normalizeJSONText(item.RequiredPropertyJSON)
	item.ProvidedPropertyJSON = normalizeJSONText(item.ProvidedPropertyJSON)
	item.SummaryJSON = normalizeJSONText(item.SummaryJSON)
}

func jsonForDB(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return normalizeJSONText(value)
}
