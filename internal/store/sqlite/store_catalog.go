package sqlite

import (
	"context"
	"fmt"
	"strings"

	"agent-testbench/internal/store"
)

func (s *Store) ReplaceProfileCatalog(ctx context.Context, catalog store.ProfileCatalog) error {
	indexedAt := encodeTime(catalog.IndexedAt)
	if indexedAt == "" {
		indexedAt = encodeTime(utcNow())
	}
	statements := []string{
		"delete from interface_node_case_dependency;",
		"delete from fixture_profile;",
		"delete from workflow_interface_node;",
		"delete from interface_node_case;",
		"delete from interface_node_request_template;",
		"delete from interface_node_field;",
		"delete from interface_node;",
		"delete from workflow_node;",
		"delete from workflow;",
		"delete from node_config;",
		"delete from template_config;",
		"delete from template;",
		"delete from kv;",
		fmt.Sprintf(`insert into kv (key, value, updated_at) values ('active_profile_id', %s, %s);`, sqlString(catalog.ProfileID), sqlString(indexedAt)),
	}
	for index, service := range catalog.Services {
		statements = append(statements, fmt.Sprintf(`
insert into node_config (id, display_name, role, attached_template_ids, git_url, git_branch, repo_env, source_path, container_name, image, docker_service, service_port, management_port, memory_mb, cpu_milli, startup_command, health_url, log_path, status, sort_order)
values (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %d, %d, %d, %d, %s, %s, %s, %s, %d);`,
			sqlString(service.ID), sqlString(service.DisplayName), sqlString(service.Kind), sqlString(jsonString(service.AttachedTemplateIDs, "[]")),
			sqlString(service.GitURL), sqlString(service.GitBranch), sqlString(service.RepoEnv), sqlString(service.SourcePath), sqlString(service.ContainerName),
			sqlString(service.Image), sqlString(service.DockerService), service.ServicePort, service.ManagementPort, service.MemoryMb, service.CPUMilli,
			sqlString(service.StartupCommand), sqlString(service.HealthURL), sqlString(service.LogPath), sqlString(stringDefault(service.Status, "active")),
			firstNonZero(service.SortOrder, index)))
	}
	for index, workflow := range catalog.Workflows {
		templateID := "workflow/" + workflow.ID
		configID := templateID + "/config"
		statements = append(statements, fmt.Sprintf(`
insert into template (id, name, kind, status, sort_order)
values (%s, %s, 'workflow', 'active', %d);`, sqlString(templateID), sqlString(workflow.DisplayName), index))
		statements = append(statements, fmt.Sprintf(`
insert into template_config (id, template_id, workflow_id, title, description, config_json, status, sort_order)
values (%s, %s, %s, %s, %s, '{}', 'active', %d);`, sqlString(configID), sqlString(templateID), sqlString(workflow.ID), sqlString(workflow.DisplayName), sqlString(workflow.Description), index))
		statements = append(statements, fmt.Sprintf(`
insert into workflow (id, name, template_id, template_config_id, description, status, sort_order, base_step_timeout_ms, timeout_offset_ms)
values (%s, %s, %s, %s, %s, 'active', %d, %d, %d);`, sqlString(workflow.ID), sqlString(workflow.DisplayName), sqlString(templateID), sqlString(configID), sqlString(workflow.Description), index, workflow.BaseStepTimeoutMs, workflow.TimeoutOffsetMs))
	}
	for index, node := range catalog.InterfaceNodes {
		tagsJSON := jsonString(node.Tags, "[]")
		createdAt := stringDefault(node.CreatedAt, indexedAt)
		updatedAt := stringDefault(node.UpdatedAt, indexedAt)
		statements = append(statements, fmt.Sprintf(`
	insert into interface_node (id, display_name, service_id, operation, method, path, template_id, version, status, tags_json, description, timeout_ms, sort_order, created_at, updated_at)
	values (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %d, %d, %s, %s);`,
			sqlString(node.ID), sqlString(node.DisplayName), sqlString(node.ServiceID), sqlString(node.Operation), sqlString(node.Method), sqlString(node.Path),
			sqlString(node.TemplateID), sqlString(stringDefault(node.Version, "v1")), sqlString(stringDefault(node.Status, "active")), sqlString(tagsJSON),
			sqlString(node.Description), node.TimeoutMs, firstNonZero(node.SortOrder, index), sqlString(createdAt), sqlString(updatedAt)))
	}
	for index, field := range catalog.InterfaceFields {
		statements = append(statements, fmt.Sprintf(`
	insert into interface_node_field (id, node_id, direction, field_path, display_name, data_type, required, bindable, port_type, status, sort_order)
	values (%s, %s, %s, %s, %s, %s, %d, %d, %s, %s, %d);`,
			sqlString(field.ID), sqlString(field.NodeID), sqlString(field.Direction), sqlString(field.FieldPath), sqlString(field.DisplayName), sqlString(field.DataType),
			boolInt(field.Required), boolInt(field.Bindable), sqlString(stringDefault(field.PortType, "DATA")), sqlString(stringDefault(field.Status, "active")), firstNonZero(field.SortOrder, index)))
	}
	for index, template := range catalog.RequestTemplates {
		templateID := "request/" + template.ID
		configID := templateID + "/config"
		statements = append(statements, fmt.Sprintf(`
insert into template (id, name, kind, status, sort_order)
values (%s, %s, 'request', 'active', %d);`, sqlString(templateID), sqlString(template.DisplayName), index))
		statements = append(statements, fmt.Sprintf(`
insert into template_config (id, template_id, node_id, scope_type, scope_id, title, config_json, status, sort_order)
values (%s, %s, %s, 'interface_node', %s, %s, %s, 'active', %d);`, sqlString(configID), sqlString(templateID), sqlString(template.NodeID), sqlString(template.NodeID), sqlString(template.DisplayName), sqlString(stringDefault(template.TemplateJSON, "{}")), index))
		statements = append(statements, fmt.Sprintf(`
	insert into interface_node_request_template (id, node_id, name, template_json, status, sort_order, created_at, updated_at)
	values (%s, %s, %s, %s, %s, %d, %s, %s);`,
			sqlString(template.ID), sqlString(template.NodeID), sqlString(template.DisplayName), sqlString(stringDefault(template.TemplateJSON, "{}")),
			sqlString(stringDefault(template.Status, "active")), firstNonZero(template.SortOrder, index), sqlString(indexedAt), sqlString(indexedAt)))
	}
	for index, item := range catalog.APICases {
		statements = append(statements, fmt.Sprintf(`
	insert into interface_node_case (id, node_id, title, description, case_type, scenario, tags_json, priority, owner, payload_template_json, request_template_id, patch_json, render_mode, expected_json, required_for_admission, status, sort_order, created_at, updated_at, case_path, source_kind, source_path, executor_id, base_url, evidence_dir, timeout_seconds, default_overrides_json)
	values (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %d, %s, %d, %s, %s, %s, %s, %s, %s, %s, %s, %d, %s);`,
			sqlString(item.ID), sqlString(item.NodeID), sqlString(item.DisplayName), sqlString(item.Description), sqlString(stringDefault(item.CaseType, "api")), sqlString(item.Scenario),
			sqlString(jsonString(item.Tags, "[]")), sqlString(item.Priority), sqlString(item.Owner),
			sqlString(stringDefault(item.PayloadTemplateJSON, "{}")), sqlString(item.RequestTemplateID), sqlString(stringDefault(item.PatchJSON, "[]")),
			sqlString(stringDefault(item.RenderMode, "legacy_payload")), sqlString(stringDefault(item.ExpectedJSON, "{}")), boolInt(item.RequiredForAdmission),
			sqlString(stringDefault(item.Status, "active")), firstNonZero(item.SortOrder, index), sqlString(indexedAt), sqlString(indexedAt),
			sqlString(item.CasePath), sqlString(item.SourceKind), sqlString(item.SourcePath), sqlString(item.ExecutorID),
			sqlString(item.BaseURL), sqlString(item.EvidenceDir), item.TimeoutSeconds, sqlString(stringDefault(item.DefaultOverridesJSON, "{}"))))
	}
	for index, binding := range catalog.WorkflowBindings {
		statements = append(statements, fmt.Sprintf(`
insert into workflow_interface_node (workflow_id, step_id, node_id, case_id, required, sort_order)
values (%s, %s, %s, %s, %d, %d);`, sqlString(binding.WorkflowID), sqlString(binding.StepID), sqlString(binding.NodeID), sqlString(binding.CaseID), boolInt(binding.Required), firstNonZero(binding.SortOrder, index)))
		if binding.NodeID != "" {
			statements = append(statements, fmt.Sprintf(`
insert into workflow_node (workflow_id, node_id, required, sort_order)
values (%s, %s, %d, %d)
on conflict(workflow_id, node_id, relation_type) do nothing;`, sqlString(binding.WorkflowID), sqlString(binding.NodeID), boolInt(binding.Required), firstNonZero(binding.SortOrder, index)))
		}
	}
	for index, fixture := range catalog.Fixtures {
		statements = append(statements, fmt.Sprintf(`
insert into fixture_profile (id, name, source_type, source_workflow_id, source_until_step, ttl_seconds, status, description, sort_order, created_at, updated_at)
values (%s, %s, %s, %s, %s, %d, %s, %s, %d, %s, %s);`,
			sqlString(fixture.ID), sqlString(fixture.DisplayName), sqlString(fixture.Kind), sqlString(fixture.SourceWorkflowID),
			sqlString(fixture.SourceUntilStep), fixture.TTLSeconds, sqlString(stringDefault(fixture.Status, "active")),
			sqlString(fixture.DataJSON), firstNonZero(fixture.SortOrder, index), sqlString(indexedAt), sqlString(indexedAt)))
	}
	for index, dependency := range catalog.CaseDependencies {
		statements = append(statements, fmt.Sprintf(`
	insert into interface_node_case_dependency (id, case_id, fixture_profile_id, required, mappings_json, status, sort_order)
	values (%s, %s, %s, %d, %s, %s, %d);`,
			sqlString(dependency.ID), sqlString(dependency.CaseID), sqlString(dependency.FixtureID), boolInt(dependency.Required),
			sqlString(stringDefault(dependency.MappingsJSON, "[]")), sqlString(stringDefault(dependency.Status, "active")), firstNonZero(dependency.SortOrder, index)))
	}
	for index, config := range catalog.TemplateConfigs {
		if strings.TrimSpace(config.ID) == "" {
			continue
		}
		templateID := stringDefault(config.TemplateID, "template-config/"+config.ID)
		statements = append(statements, fmt.Sprintf(`
insert into template (id, name, kind, status, sort_order)
values (%s, %s, %s, 'active', %d)
on conflict(id) do nothing;`, sqlString(templateID), sqlString(stringDefault(config.Title, templateID)), sqlString(stringDefault(config.ScopeType, "config")), firstNonZero(config.SortOrder, index)))
		statements = append(statements, fmt.Sprintf(`
insert or replace into template_config (id, template_id, node_id, workflow_id, scope_type, scope_id, title, description, config_json, status, sort_order)
values (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %d);`,
			sqlString(config.ID), sqlString(templateID), sqlString(config.NodeID), sqlString(config.WorkflowID), sqlString(config.ScopeType),
			sqlString(config.ScopeID), sqlString(config.Title), sqlString(config.Description), sqlString(stringDefault(config.ConfigJSON, "{}")),
			sqlString(stringDefault(config.Status, "active")), firstNonZero(config.SortOrder, index)))
	}
	if err := s.exec(ctx, "begin;\n"+strings.Join(statements, "\n")+"\ncommit;"); err != nil {
		return fmt.Errorf("replace profile catalog index %q: %w", catalog.ProfileID, err)
	}
	return nil
}

func (s *Store) GetProfileCatalog(ctx context.Context) (store.ProfileCatalog, error) {
	index, err := s.GetProfileCatalogIndex(ctx)
	if err != nil {
		return store.ProfileCatalog{}, err
	}
	catalog := store.ProfileCatalog{
		ProfileID: index.ProfileID,
		IndexedAt: index.IndexedAt,
	}

	var services []catalogServiceRow
	if err := s.query(ctx, `select id, display_name, role, attached_template_ids, git_url, git_branch, repo_env, source_path, container_name, image, docker_service, service_port, management_port, memory_mb, cpu_milli, startup_command, health_url, log_path, status, sort_order from node_config order by sort_order, id;`, &services); err != nil {
		return store.ProfileCatalog{}, err
	}
	for _, row := range services {
		catalog.Services = append(catalog.Services, store.CatalogService{
			ID: row.ID, DisplayName: row.DisplayName, Kind: row.Role, AttachedTemplateIDs: stringSliceFromJSON(row.AttachedTemplateIDs),
			GitURL: row.GitURL, GitBranch: row.GitBranch, RepoEnv: row.RepoEnv, SourcePath: row.SourcePath, ContainerName: row.ContainerName,
			Image: row.Image, DockerService: row.DockerService, ServicePort: row.ServicePort, ManagementPort: row.ManagementPort,
			MemoryMb: row.MemoryMb, CPUMilli: row.CPUMilli, StartupCommand: row.StartupCommand, HealthURL: row.HealthURL,
			LogPath: row.LogPath, Status: row.Status, SortOrder: row.SortOrder,
		})
	}

	var workflows []catalogWorkflowRow
	if err := s.query(ctx, `select id, name, description, base_step_timeout_ms, timeout_offset_ms from workflow order by sort_order, id;`, &workflows); err != nil {
		return store.ProfileCatalog{}, err
	}
	for _, row := range workflows {
		catalog.Workflows = append(catalog.Workflows, store.CatalogWorkflow{ID: row.ID, DisplayName: row.Name, Description: row.Description, BaseStepTimeoutMs: row.BaseStepTimeoutMs, TimeoutOffsetMs: row.TimeoutOffsetMs})
	}

	var nodes []catalogInterfaceNodeRow
	if err := s.query(ctx, `select id, display_name, service_id, operation, method, path, template_id, version, status, tags_json, description, timeout_ms, sort_order, created_at, updated_at from interface_node order by sort_order, id;`, &nodes); err != nil {
		return store.ProfileCatalog{}, err
	}
	for _, row := range nodes {
		catalog.InterfaceNodes = append(catalog.InterfaceNodes, store.CatalogInterfaceNode{
			ID: row.ID, DisplayName: row.DisplayName, ServiceID: row.ServiceID, Operation: row.Operation,
			Method: row.Method, Path: row.Path, TemplateID: row.TemplateID, Version: row.Version, Status: row.Status,
			Tags: stringSliceFromJSON(row.TagsJSON), Description: row.Description, SortOrder: row.SortOrder,
			TimeoutMs: row.TimeoutMs, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
	}

	var fields []catalogInterfaceNodeFieldRow
	if err := s.query(ctx, `select id, node_id, direction, field_path, display_name, data_type, required, bindable, port_type, status, sort_order from interface_node_field order by node_id, direction, sort_order, id;`, &fields); err != nil {
		return store.ProfileCatalog{}, err
	}
	for _, row := range fields {
		catalog.InterfaceFields = append(catalog.InterfaceFields, store.CatalogInterfaceNodeField{
			ID: row.ID, NodeID: row.NodeID, Direction: row.Direction, FieldPath: row.FieldPath, DisplayName: row.DisplayName,
			DataType: row.DataType, Required: row.Required != 0, Bindable: row.Bindable != 0, PortType: row.PortType,
			Status: row.Status, SortOrder: row.SortOrder,
		})
	}

	var templates []catalogRequestTemplateRow
	if err := s.query(ctx, `select id, node_id, name, template_json, version, status, sort_order from interface_node_request_template order by node_id, sort_order, id;`, &templates); err != nil {
		return store.ProfileCatalog{}, err
	}
	nodeByID := map[string]store.CatalogInterfaceNode{}
	for _, node := range catalog.InterfaceNodes {
		nodeByID[node.ID] = node
	}
	for _, row := range templates {
		node := nodeByID[row.NodeID]
		catalog.RequestTemplates = append(catalog.RequestTemplates, store.CatalogRequestTemplate{
			ID: row.ID, DisplayName: row.Name, NodeID: row.NodeID, Method: node.Method, Path: node.Path,
			TemplateJSON: row.TemplateJSON, Version: row.Version, Status: row.Status, SortOrder: row.SortOrder,
		})
	}

	var cases []catalogAPICaseRow
	if err := s.query(ctx, `select id, node_id, title, description, case_type, scenario, tags_json, priority, owner, payload_template_json, request_template_id, patch_json, render_mode, expected_json, required_for_admission, status, sort_order, case_path, source_kind, source_path, executor_id, base_url, evidence_dir, timeout_seconds, default_overrides_json from interface_node_case order by node_id, sort_order, id;`, &cases); err != nil {
		return store.ProfileCatalog{}, err
	}
	for _, row := range cases {
		catalog.APICases = append(catalog.APICases, store.CatalogAPICase{
			ID: row.ID, DisplayName: row.Title, Description: row.Description, NodeID: row.NodeID, CaseType: row.CaseType, Scenario: row.Scenario,
			Tags: stringSliceFromJSON(row.TagsJSON), Priority: row.Priority, Owner: row.Owner,
			PayloadTemplateJSON: row.PayloadTemplateJSON, RequestTemplateID: row.RequestTemplateID, PatchJSON: row.PatchJSON,
			RenderMode: row.RenderMode, ExpectedJSON: row.ExpectedJSON, RequiredForAdmission: row.RequiredForAdmission != 0,
			Status: row.Status, SortOrder: row.SortOrder, CasePath: row.CasePath, SourceKind: row.SourceKind, SourcePath: row.SourcePath,
			ExecutorID: row.ExecutorID, BaseURL: row.BaseURL, EvidenceDir: row.EvidenceDir, TimeoutSeconds: row.TimeoutSeconds,
			DefaultOverridesJSON: row.DefaultOverridesJSON,
		})
	}

	var dependencies []catalogCaseDependencyRow
	if err := s.query(ctx, `select id, case_id, fixture_profile_id, required, mappings_json, status, sort_order from interface_node_case_dependency order by case_id, sort_order, id;`, &dependencies); err != nil {
		return store.ProfileCatalog{}, err
	}
	for _, row := range dependencies {
		catalog.CaseDependencies = append(catalog.CaseDependencies, store.CatalogCaseDependency{
			ID: row.ID, CaseID: row.CaseID, FixtureID: row.FixtureProfileID, Required: row.Required != 0,
			MappingsJSON: row.MappingsJSON, Status: row.Status, SortOrder: row.SortOrder,
		})
	}

	var fixtures []catalogFixtureRow
	if err := s.query(ctx, `select id, name, source_type, source_workflow_id, source_until_step, ttl_seconds, status, description, sort_order from fixture_profile order by sort_order, id;`, &fixtures); err != nil {
		return store.ProfileCatalog{}, err
	}
	for _, row := range fixtures {
		catalog.Fixtures = append(catalog.Fixtures, store.CatalogFixture{
			ID: row.ID, DisplayName: row.Name, Kind: row.SourceType, DataJSON: row.Description,
			SourceWorkflowID: row.SourceWorkflowID, SourceUntilStep: row.SourceUntilStep, TTLSeconds: row.TTLSeconds,
			Status: row.Status, SortOrder: row.SortOrder,
		})
	}

	var bindings []catalogWorkflowBindingRow
	if err := s.query(ctx, `select workflow_id, step_id, node_id, case_id, required, sort_order from workflow_interface_node order by workflow_id, sort_order, step_id;`, &bindings); err != nil {
		return store.ProfileCatalog{}, err
	}
	for _, row := range bindings {
		catalog.WorkflowBindings = append(catalog.WorkflowBindings, store.CatalogWorkflowBinding{
			WorkflowID: row.WorkflowID, StepID: row.StepID, NodeID: row.NodeID, CaseID: row.CaseID, Required: row.Required != 0,
			SortOrder: row.SortOrder,
		})
	}

	var configs []catalogTemplateConfigRow
	if err := s.query(ctx, `select id, template_id, node_id, workflow_id, scope_type, scope_id, title, description, config_json, status, sort_order from template_config order by workflow_id, scope_type, sort_order, id;`, &configs); err != nil {
		return store.ProfileCatalog{}, err
	}
	for _, row := range configs {
		catalog.TemplateConfigs = append(catalog.TemplateConfigs, store.CatalogTemplateConfig{
			ID: row.ID, TemplateID: row.TemplateID, NodeID: row.NodeID, WorkflowID: row.WorkflowID, ScopeType: row.ScopeType,
			ScopeID: row.ScopeID, Title: row.Title, Description: row.Description, ConfigJSON: row.ConfigJSON,
			Status: row.Status, SortOrder: row.SortOrder,
		})
	}

	return catalog, nil
}

func (s *Store) GetProfileCatalogIndex(ctx context.Context) (store.ProfileCatalogIndex, error) {
	var rows []profileCatalogIndexRow
	if err := s.query(ctx, `
select
  coalesce((select value from kv where key = 'active_profile_id'), '') as profile_id,
  coalesce((select updated_at from kv where key = 'active_profile_id'), '') as indexed_at,
  (select count(*) from node_config) as services,
  (select count(*) from workflow) as workflows,
  (select count(*) from interface_node) as interface_nodes,
  (select count(*) from interface_node_case) as api_cases,
  (select count(*) from interface_node_request_template) as request_templates,
  (select count(*) from workflow_interface_node) as workflow_bindings,
  (select count(*) from interface_node_case_dependency) as case_dependencies,
  (select count(*) from fixture_profile) as fixtures,
  (select count(*) from template) as templates,
  (select count(*) from template_config) as template_configs;`, &rows); err != nil {
		return store.ProfileCatalogIndex{}, err
	}
	if len(rows) == 0 || rows[0].ProfileID == "" {
		return store.ProfileCatalogIndex{}, store.ErrNotFound
	}
	return rows[0].toStore(), nil
}

type catalogServiceRow struct {
	ID                  string `json:"id"`
	DisplayName         string `json:"display_name"`
	Role                string `json:"role"`
	AttachedTemplateIDs string `json:"attached_template_ids"`
	GitURL              string `json:"git_url"`
	GitBranch           string `json:"git_branch"`
	RepoEnv             string `json:"repo_env"`
	SourcePath          string `json:"source_path"`
	ContainerName       string `json:"container_name"`
	Image               string `json:"image"`
	DockerService       string `json:"docker_service"`
	ServicePort         int    `json:"service_port"`
	ManagementPort      int    `json:"management_port"`
	MemoryMb            int    `json:"memory_mb"`
	CPUMilli            int    `json:"cpu_milli"`
	StartupCommand      string `json:"startup_command"`
	HealthURL           string `json:"health_url"`
	LogPath             string `json:"log_path"`
	Status              string `json:"status"`
	SortOrder           int    `json:"sort_order"`
}

type catalogWorkflowRow struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	BaseStepTimeoutMs int    `json:"base_step_timeout_ms"`
	TimeoutOffsetMs   int    `json:"timeout_offset_ms"`
}

type catalogInterfaceNodeRow struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	ServiceID   string `json:"service_id"`
	Operation   string `json:"operation"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	TemplateID  string `json:"template_id"`
	Version     string `json:"version"`
	Status      string `json:"status"`
	TagsJSON    string `json:"tags_json"`
	Description string `json:"description"`
	TimeoutMs   int    `json:"timeout_ms"`
	SortOrder   int    `json:"sort_order"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type catalogInterfaceNodeFieldRow struct {
	ID          string `json:"id"`
	NodeID      string `json:"node_id"`
	Direction   string `json:"direction"`
	FieldPath   string `json:"field_path"`
	DisplayName string `json:"display_name"`
	DataType    string `json:"data_type"`
	Required    int    `json:"required"`
	Bindable    int    `json:"bindable"`
	PortType    string `json:"port_type"`
	Status      string `json:"status"`
	SortOrder   int    `json:"sort_order"`
}

type catalogRequestTemplateRow struct {
	ID           string `json:"id"`
	NodeID       string `json:"node_id"`
	Name         string `json:"name"`
	TemplateJSON string `json:"template_json"`
	Version      string `json:"version"`
	Status       string `json:"status"`
	SortOrder    int    `json:"sort_order"`
}

type catalogAPICaseRow struct {
	ID                   string `json:"id"`
	NodeID               string `json:"node_id"`
	Title                string `json:"title"`
	Description          string `json:"description"`
	CaseType             string `json:"case_type"`
	Scenario             string `json:"scenario"`
	TagsJSON             string `json:"tags_json"`
	Priority             string `json:"priority"`
	Owner                string `json:"owner"`
	PayloadTemplateJSON  string `json:"payload_template_json"`
	RequestTemplateID    string `json:"request_template_id"`
	PatchJSON            string `json:"patch_json"`
	RenderMode           string `json:"render_mode"`
	ExpectedJSON         string `json:"expected_json"`
	RequiredForAdmission int    `json:"required_for_admission"`
	Status               string `json:"status"`
	SortOrder            int    `json:"sort_order"`
	CasePath             string `json:"case_path"`
	SourceKind           string `json:"source_kind"`
	SourcePath           string `json:"source_path"`
	ExecutorID           string `json:"executor_id"`
	BaseURL              string `json:"base_url"`
	EvidenceDir          string `json:"evidence_dir"`
	TimeoutSeconds       int    `json:"timeout_seconds"`
	DefaultOverridesJSON string `json:"default_overrides_json"`
}

type catalogCaseDependencyRow struct {
	ID               string `json:"id"`
	CaseID           string `json:"case_id"`
	FixtureProfileID string `json:"fixture_profile_id"`
	Required         int    `json:"required"`
	MappingsJSON     string `json:"mappings_json"`
	Status           string `json:"status"`
	SortOrder        int    `json:"sort_order"`
}

type catalogFixtureRow struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	SourceType       string `json:"source_type"`
	SourceWorkflowID string `json:"source_workflow_id"`
	SourceUntilStep  string `json:"source_until_step"`
	TTLSeconds       int    `json:"ttl_seconds"`
	Status           string `json:"status"`
	Description      string `json:"description"`
	SortOrder        int    `json:"sort_order"`
}

type catalogWorkflowBindingRow struct {
	WorkflowID string `json:"workflow_id"`
	StepID     string `json:"step_id"`
	NodeID     string `json:"node_id"`
	CaseID     string `json:"case_id"`
	Required   int    `json:"required"`
	SortOrder  int    `json:"sort_order"`
}

type catalogTemplateConfigRow struct {
	ID          string `json:"id"`
	TemplateID  string `json:"template_id"`
	NodeID      string `json:"node_id"`
	WorkflowID  string `json:"workflow_id"`
	ScopeType   string `json:"scope_type"`
	ScopeID     string `json:"scope_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ConfigJSON  string `json:"config_json"`
	Status      string `json:"status"`
	SortOrder   int    `json:"sort_order"`
}

type profileCatalogIndexRow struct {
	ProfileID        string `json:"profile_id"`
	IndexedAt        string `json:"indexed_at"`
	Services         int    `json:"services"`
	Workflows        int    `json:"workflows"`
	InterfaceNodes   int    `json:"interface_nodes"`
	APICases         int    `json:"api_cases"`
	RequestTemplates int    `json:"request_templates"`
	WorkflowBindings int    `json:"workflow_bindings"`
	CaseDependencies int    `json:"case_dependencies"`
	Fixtures         int    `json:"fixtures"`
	Templates        int    `json:"templates"`
	TemplateConfigs  int    `json:"template_configs"`
}

func (r profileCatalogIndexRow) toStore() store.ProfileCatalogIndex {
	return store.ProfileCatalogIndex{
		ProfileID: r.ProfileID,
		IndexedAt: decodeTime(r.IndexedAt),
		Counts: store.ProfileCatalogCounts{
			Services:         r.Services,
			Workflows:        r.Workflows,
			InterfaceNodes:   r.InterfaceNodes,
			APICases:         r.APICases,
			RequestTemplates: r.RequestTemplates,
			WorkflowBindings: r.WorkflowBindings,
			CaseDependencies: r.CaseDependencies,
			Fixtures:         r.Fixtures,
			Templates:        r.Templates,
			TemplateConfigs:  r.TemplateConfigs,
		},
	}
}
