package sqlite

import (
	"context"
	"fmt"
	"strings"

	"agent-testbench/internal/store"
)

func (s *Store) UpsertEnvironment(ctx context.Context, e store.Environment) (store.Environment, error) {
	var err error
	e, err = store.PrepareEnvironmentForStructuredUpsert(ctx, s, e, utcNow())
	if err != nil {
		return store.Environment{}, err
	}
	if err := s.exec(ctx, fmt.Sprintf(`
insert into environments (
  id, display_name, description, status, verified, services_json, repos_json, compose_json,
  health_checks_json, verification_workflow_id, last_verification_run_id, last_verification_status,
  evidence_complete, topology_complete, last_verified_at, summary_json, created_at, updated_at
)
values (%s, %s, %s, %s, %d, %s, %s, %s, %s, %s, %s, %s, %d, %d, %s, %s, %s, %s)
on conflict(id) do update set
  display_name = excluded.display_name,
  description = excluded.description,
  status = excluded.status,
  verified = excluded.verified,
  services_json = excluded.services_json,
  repos_json = excluded.repos_json,
  compose_json = excluded.compose_json,
  health_checks_json = excluded.health_checks_json,
  verification_workflow_id = excluded.verification_workflow_id,
  last_verification_run_id = excluded.last_verification_run_id,
  last_verification_status = excluded.last_verification_status,
  evidence_complete = excluded.evidence_complete,
  topology_complete = excluded.topology_complete,
  last_verified_at = excluded.last_verified_at,
  summary_json = excluded.summary_json,
  updated_at = excluded.updated_at;`,
		sqlString(e.ID), sqlString(e.DisplayName), sqlString(e.Description), sqlString(stringDefault(e.Status, "draft")),
		boolInt(e.Verified), sqlString(stringDefault(e.ServicesJSON, "[]")), sqlString(stringDefault(e.ReposJSON, "{}")),
		sqlString(stringDefault(e.ComposeJSON, "{}")), sqlString(stringDefault(e.HealthChecksJSON, "[]")), sqlString(e.VerificationWorkflowID),
		sqlString(e.LastVerificationRunID), sqlString(e.LastVerificationStatus), boolInt(e.EvidenceComplete), boolInt(e.TopologyComplete),
		sqlString(encodeTime(e.LastVerifiedAt)), sqlString(stringDefault(e.SummaryJSON, "{}")), sqlString(encodeTime(e.CreatedAt)), sqlString(encodeTime(e.UpdatedAt)))); err != nil {
		return store.Environment{}, fmt.Errorf("upsert environment %q: %w", e.ID, err)
	}
	return e, nil
}

func (s *Store) GetEnvironment(ctx context.Context, id string) (store.Environment, error) {
	var rows []environmentRow
	if err := s.query(ctx, fmt.Sprintf(`
select id, display_name, description, status, verified, services_json, repos_json, compose_json,
  health_checks_json, verification_workflow_id, last_verification_run_id, last_verification_status,
  evidence_complete, topology_complete, last_verified_at, summary_json, created_at, updated_at
from environments where id = %s;`, sqlString(id)), &rows); err != nil {
		return store.Environment{}, err
	}
	if len(rows) == 0 {
		return store.Environment{}, store.ErrNotFound
	}
	return store.HydrateEnvironmentStructuredState(ctx, s, rows[0].toStore())
}

func (s *Store) ListEnvironments(ctx context.Context) ([]store.Environment, error) {
	var rows []environmentRow
	if err := s.query(ctx, `
select id, display_name, description, status, verified, services_json, repos_json, compose_json,
  health_checks_json, verification_workflow_id, last_verification_run_id, last_verification_status,
  evidence_complete, topology_complete, last_verified_at, summary_json, created_at, updated_at
from environments order by verified desc, updated_at desc, id;`, &rows); err != nil {
		return nil, err
	}
	out := make([]store.Environment, 0, len(rows))
	for _, row := range rows {
		env, err := store.HydrateEnvironmentStructuredState(ctx, s, row.toStore())
		if err != nil {
			return nil, err
		}
		out = append(out, env)
	}
	return out, nil
}

func (s *Store) ReplaceEnvironmentComponentGraph(ctx context.Context, envID string, graph store.EnvironmentComponentGraph) error {
	if err := store.ValidateEnvironmentComponentGraph(envID, graph); err != nil {
		return err
	}
	now := utcNow()
	statements := []string{
		fmt.Sprintf("delete from component_config_assets where env_id = %s;", sqlString(envID)),
		fmt.Sprintf("delete from component_dependencies where env_id = %s;", sqlString(envID)),
		fmt.Sprintf("delete from environment_components where env_id = %s;", sqlString(envID)),
	}
	for _, component := range graph.Components {
		if component.CreatedAt.IsZero() {
			component.CreatedAt = now
		}
		if component.UpdatedAt.IsZero() {
			component.UpdatedAt = now
		}
		statements = append(statements, fmt.Sprintf(`
insert into environment_components (
  env_id, component_id, display_name, kind, role, compose_service, image, required,
  runtime_json, healthcheck_json, summary_json, created_at, updated_at
) values (%s, %s, %s, %s, %s, %s, %s, %d, %s, %s, %s, %s, %s);`,
			sqlString(envID), sqlString(component.ComponentID), sqlString(component.DisplayName), sqlString(component.Kind),
			sqlString(component.Role), sqlString(component.ComposeService), sqlString(component.Image), boolInt(component.Required),
			sqlString(stringDefault(component.RuntimeJSON, "{}")), sqlString(stringDefault(component.HealthCheckJSON, "{}")),
			sqlString(stringDefault(component.SummaryJSON, "{}")), sqlString(encodeTime(component.CreatedAt)), sqlString(encodeTime(component.UpdatedAt))))
	}
	for _, dep := range graph.Dependencies {
		if dep.CreatedAt.IsZero() {
			dep.CreatedAt = now
		}
		if dep.UpdatedAt.IsZero() {
			dep.UpdatedAt = now
		}
		statements = append(statements, fmt.Sprintf(`
insert into component_dependencies (
  env_id, consumer_component_id, provider_component_id, phase, capability, required,
  profile_json, created_at, updated_at
) values (%s, %s, %s, %s, %s, %d, %s, %s, %s);`,
			sqlString(envID), sqlString(dep.ConsumerComponentID), sqlString(dep.ProviderComponentID),
			sqlString(dep.Phase), sqlString(dep.Capability), boolInt(dep.Required),
			sqlString(stringDefault(dep.ProfileJSON, "{}")), sqlString(encodeTime(dep.CreatedAt)), sqlString(encodeTime(dep.UpdatedAt))))
	}
	for _, asset := range graph.Assets {
		if asset.CreatedAt.IsZero() {
			asset.CreatedAt = now
		}
		if asset.UpdatedAt.IsZero() {
			asset.UpdatedAt = now
		}
		if strings.TrimSpace(asset.TargetComponentID) == "" {
			asset.TargetComponentID = asset.OwnerComponentID
		}
		statements = append(statements, fmt.Sprintf(`
insert into component_config_assets (
  env_id, owner_component_id, asset_id, asset_kind, target_component_id, target_path,
  content_inline, remote_ref_json, sha256, size_bytes, apply_order, sensitive,
  summary_json, created_at, updated_at
) values (%s, %s, %s, %s, %s, %s, %s, %s, %s, %d, %d, %d, %s, %s, %s);`,
			sqlString(envID), sqlString(asset.OwnerComponentID), sqlString(asset.AssetID), sqlString(asset.AssetKind),
			sqlString(asset.TargetComponentID), sqlString(asset.TargetPath), sqlString(asset.ContentInline),
			sqlString(stringDefault(asset.RemoteRefJSON, "{}")), sqlString(asset.SHA256), asset.SizeBytes,
			asset.ApplyOrder, boolInt(asset.Sensitive), sqlString(stringDefault(asset.SummaryJSON, "{}")),
			sqlString(encodeTime(asset.CreatedAt)), sqlString(encodeTime(asset.UpdatedAt))))
	}
	return s.exec(ctx, "begin;\n"+strings.Join(statements, "\n")+"\ncommit;")
}

func (s *Store) GetEnvironmentComponentGraph(ctx context.Context, envID string) (store.EnvironmentComponentGraph, error) {
	var componentRows []environmentComponentRow
	if err := s.query(ctx, fmt.Sprintf(`
select env_id, component_id, display_name, kind, role, compose_service, image, required,
  runtime_json, healthcheck_json, summary_json, created_at, updated_at
from environment_components
where env_id = %s
order by component_id;`, sqlString(envID)), &componentRows); err != nil {
		return store.EnvironmentComponentGraph{}, err
	}
	var dependencyRows []componentDependencyRow
	if err := s.query(ctx, fmt.Sprintf(`
select env_id, consumer_component_id, provider_component_id, phase, capability, required,
  profile_json, created_at, updated_at
from component_dependencies
where env_id = %s
order by consumer_component_id, provider_component_id, phase, capability;`, sqlString(envID)), &dependencyRows); err != nil {
		return store.EnvironmentComponentGraph{}, err
	}
	var assetRows []componentConfigAssetRow
	if err := s.query(ctx, fmt.Sprintf(`
select env_id, owner_component_id, asset_id, asset_kind, target_component_id, target_path,
  content_inline, remote_ref_json, sha256, size_bytes, apply_order, sensitive,
  summary_json, created_at, updated_at
from component_config_assets
where env_id = %s
order by owner_component_id, apply_order, asset_id;`, sqlString(envID)), &assetRows); err != nil {
		return store.EnvironmentComponentGraph{}, err
	}
	graph := store.EnvironmentComponentGraph{
		Components:   make([]store.EnvironmentComponent, 0, len(componentRows)),
		Dependencies: make([]store.ComponentDependency, 0, len(dependencyRows)),
		Assets:       make([]store.ComponentConfigAsset, 0, len(assetRows)),
	}
	for _, row := range componentRows {
		graph.Components = append(graph.Components, row.toStore())
	}
	for _, row := range dependencyRows {
		graph.Dependencies = append(graph.Dependencies, row.toStore())
	}
	for _, row := range assetRows {
		graph.Assets = append(graph.Assets, row.toStore())
	}
	return graph, nil
}

type environmentRow struct {
	ID                     string `json:"id"`
	DisplayName            string `json:"display_name"`
	Description            string `json:"description"`
	Status                 string `json:"status"`
	Verified               int    `json:"verified"`
	ServicesJSON           string `json:"services_json"`
	ReposJSON              string `json:"repos_json"`
	ComposeJSON            string `json:"compose_json"`
	HealthChecksJSON       string `json:"health_checks_json"`
	VerificationWorkflowID string `json:"verification_workflow_id"`
	LastVerificationRunID  string `json:"last_verification_run_id"`
	LastVerificationStatus string `json:"last_verification_status"`
	EvidenceComplete       int    `json:"evidence_complete"`
	TopologyComplete       int    `json:"topology_complete"`
	LastVerifiedAt         string `json:"last_verified_at"`
	SummaryJSON            string `json:"summary_json"`
	CreatedAt              string `json:"created_at"`
	UpdatedAt              string `json:"updated_at"`
}

type environmentComponentRow struct {
	EnvID           string `json:"env_id"`
	ComponentID     string `json:"component_id"`
	DisplayName     string `json:"display_name"`
	Kind            string `json:"kind"`
	Role            string `json:"role"`
	ComposeService  string `json:"compose_service"`
	Image           string `json:"image"`
	Required        int    `json:"required"`
	RuntimeJSON     string `json:"runtime_json"`
	HealthCheckJSON string `json:"healthcheck_json"`
	SummaryJSON     string `json:"summary_json"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

func (r environmentComponentRow) toStore() store.EnvironmentComponent {
	return store.EnvironmentComponent{
		EnvID:           r.EnvID,
		ComponentID:     r.ComponentID,
		DisplayName:     r.DisplayName,
		Kind:            r.Kind,
		Role:            r.Role,
		ComposeService:  r.ComposeService,
		Image:           r.Image,
		Required:        r.Required != 0,
		RuntimeJSON:     normalizeJSONText(r.RuntimeJSON),
		HealthCheckJSON: normalizeJSONText(r.HealthCheckJSON),
		SummaryJSON:     normalizeJSONText(r.SummaryJSON),
		CreatedAt:       decodeTime(r.CreatedAt),
		UpdatedAt:       decodeTime(r.UpdatedAt),
	}
}

type componentDependencyRow struct {
	EnvID               string `json:"env_id"`
	ConsumerComponentID string `json:"consumer_component_id"`
	ProviderComponentID string `json:"provider_component_id"`
	Phase               string `json:"phase"`
	Capability          string `json:"capability"`
	Required            int    `json:"required"`
	ProfileJSON         string `json:"profile_json"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

func (r componentDependencyRow) toStore() store.ComponentDependency {
	return store.ComponentDependency{
		EnvID:               r.EnvID,
		ConsumerComponentID: r.ConsumerComponentID,
		ProviderComponentID: r.ProviderComponentID,
		Phase:               r.Phase,
		Capability:          r.Capability,
		Required:            r.Required != 0,
		ProfileJSON:         normalizeJSONText(r.ProfileJSON),
		CreatedAt:           decodeTime(r.CreatedAt),
		UpdatedAt:           decodeTime(r.UpdatedAt),
	}
}

type componentConfigAssetRow struct {
	EnvID             string `json:"env_id"`
	OwnerComponentID  string `json:"owner_component_id"`
	AssetID           string `json:"asset_id"`
	AssetKind         string `json:"asset_kind"`
	TargetComponentID string `json:"target_component_id"`
	TargetPath        string `json:"target_path"`
	ContentInline     string `json:"content_inline"`
	RemoteRefJSON     string `json:"remote_ref_json"`
	SHA256            string `json:"sha256"`
	SizeBytes         int64  `json:"size_bytes"`
	ApplyOrder        int    `json:"apply_order"`
	Sensitive         int    `json:"sensitive"`
	SummaryJSON       string `json:"summary_json"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

func (r componentConfigAssetRow) toStore() store.ComponentConfigAsset {
	return store.ComponentConfigAsset{
		EnvID:             r.EnvID,
		OwnerComponentID:  r.OwnerComponentID,
		AssetID:           r.AssetID,
		AssetKind:         r.AssetKind,
		TargetComponentID: r.TargetComponentID,
		TargetPath:        r.TargetPath,
		ContentInline:     r.ContentInline,
		RemoteRefJSON:     normalizeJSONText(r.RemoteRefJSON),
		SHA256:            r.SHA256,
		SizeBytes:         r.SizeBytes,
		ApplyOrder:        r.ApplyOrder,
		Sensitive:         r.Sensitive != 0,
		SummaryJSON:       normalizeJSONText(r.SummaryJSON),
		CreatedAt:         decodeTime(r.CreatedAt),
		UpdatedAt:         decodeTime(r.UpdatedAt),
	}
}

func (r environmentRow) toStore() store.Environment {
	return store.Environment{
		ID:                     r.ID,
		DisplayName:            r.DisplayName,
		Description:            r.Description,
		Status:                 r.Status,
		Verified:               r.Verified != 0,
		ServicesJSON:           r.ServicesJSON,
		ReposJSON:              r.ReposJSON,
		ComposeJSON:            r.ComposeJSON,
		HealthChecksJSON:       r.HealthChecksJSON,
		VerificationWorkflowID: r.VerificationWorkflowID,
		LastVerificationRunID:  r.LastVerificationRunID,
		LastVerificationStatus: r.LastVerificationStatus,
		EvidenceComplete:       r.EvidenceComplete != 0,
		TopologyComplete:       r.TopologyComplete != 0,
		LastVerifiedAt:         decodeTime(r.LastVerifiedAt),
		SummaryJSON:            r.SummaryJSON,
		CreatedAt:              decodeTime(r.CreatedAt),
		UpdatedAt:              decodeTime(r.UpdatedAt),
	}
}
