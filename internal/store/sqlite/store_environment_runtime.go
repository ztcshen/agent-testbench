package sqlite

import (
	"context"
	"fmt"
	"strings"

	"agent-testbench/internal/store"
)

func (s *Store) ReplaceEnvironmentServices(ctx context.Context, envID string, services []store.EnvironmentService) error {
	services = store.NormalizeEnvironmentServices(services)
	if err := store.ValidateEnvironmentServices(envID, services); err != nil {
		return err
	}
	now := utcNow()
	statements := []string{fmt.Sprintf("delete from environment_services where env_id = %s;", sqlString(envID))}
	for _, service := range services {
		if service.CreatedAt.IsZero() {
			service.CreatedAt = now
		}
		if service.UpdatedAt.IsZero() {
			service.UpdatedAt = now
		}
		statements = append(statements, fmt.Sprintf(`
insert into environment_services (
  env_id, service_id, repo_url, branch, ref, checkout,
  summary_json, created_at, updated_at
) values (%s, %s, %s, %s, %s, %s, %s, %s, %s);`,
			sqlString(envID), sqlString(service.ServiceID), sqlString(service.RepoURL), sqlString(service.Branch),
			sqlString(service.Ref), sqlString(service.Checkout), sqlString(stringDefault(service.SummaryJSON, "{}")),
			sqlString(encodeTime(service.CreatedAt)), sqlString(encodeTime(service.UpdatedAt))))
	}
	return s.exec(ctx, "begin;\n"+strings.Join(statements, "\n")+"\ncommit;")
}

func (s *Store) ListEnvironmentServices(ctx context.Context, envID string) ([]store.EnvironmentService, error) {
	var rows []environmentServiceRow
	if err := s.query(ctx, fmt.Sprintf(`
select env_id, service_id, repo_url, branch, ref, checkout,
  summary_json, created_at, updated_at
from environment_services
where env_id = %s
order by service_id;`, sqlString(envID)), &rows); err != nil {
		return nil, err
	}
	out := make([]store.EnvironmentService, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toStore())
	}
	return out, nil
}

func (s *Store) ReplaceEnvironmentHealthChecks(ctx context.Context, envID string, checks []store.EnvironmentHealthCheck) error {
	checks = store.NormalizeEnvironmentHealthChecks(checks)
	if err := store.ValidateEnvironmentHealthChecks(envID, checks); err != nil {
		return err
	}
	now := utcNow()
	statements := []string{fmt.Sprintf("delete from environment_health_checks where env_id = %s;", sqlString(envID))}
	for _, check := range checks {
		if check.CreatedAt.IsZero() {
			check.CreatedAt = now
		}
		if check.UpdatedAt.IsZero() {
			check.UpdatedAt = now
		}
		statements = append(statements, fmt.Sprintf(`
insert into environment_health_checks (
  env_id, check_id, check_kind, url, address, command, compose_service,
  expect, apply_order, summary_json, created_at, updated_at
) values (%s, %s, %s, %s, %s, %s, %s, %s, %d, %s, %s, %s);`,
			sqlString(envID), sqlString(check.CheckID), sqlString(check.Kind), sqlString(check.URL),
			sqlString(check.Address), sqlString(check.Command), sqlString(check.ComposeService), sqlString(check.Expect),
			check.ApplyOrder, sqlString(stringDefault(check.SummaryJSON, "{}")),
			sqlString(encodeTime(check.CreatedAt)), sqlString(encodeTime(check.UpdatedAt))))
	}
	return s.exec(ctx, "begin;\n"+strings.Join(statements, "\n")+"\ncommit;")
}

func (s *Store) ListEnvironmentHealthChecks(ctx context.Context, envID string) ([]store.EnvironmentHealthCheck, error) {
	var rows []environmentHealthCheckRow
	if err := s.query(ctx, fmt.Sprintf(`
select env_id, check_id, check_kind, url, address, command, compose_service,
  expect, apply_order, summary_json, created_at, updated_at
from environment_health_checks
where env_id = %s
order by apply_order, check_id;`, sqlString(envID)), &rows); err != nil {
		return nil, err
	}
	out := make([]store.EnvironmentHealthCheck, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toStore())
	}
	return out, nil
}

type environmentServiceRow struct {
	EnvID       string `json:"env_id"`
	ServiceID   string `json:"service_id"`
	RepoURL     string `json:"repo_url"`
	Branch      string `json:"branch"`
	Ref         string `json:"ref"`
	Checkout    string `json:"checkout"`
	SummaryJSON string `json:"summary_json"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func (r environmentServiceRow) toStore() store.EnvironmentService {
	return store.EnvironmentService{
		EnvID:       r.EnvID,
		ServiceID:   r.ServiceID,
		RepoURL:     r.RepoURL,
		Branch:      r.Branch,
		Ref:         r.Ref,
		Checkout:    r.Checkout,
		SummaryJSON: normalizeJSONText(r.SummaryJSON),
		CreatedAt:   decodeTime(r.CreatedAt),
		UpdatedAt:   decodeTime(r.UpdatedAt),
	}
}

type environmentHealthCheckRow struct {
	EnvID          string `json:"env_id"`
	CheckID        string `json:"check_id"`
	Kind           string `json:"check_kind"`
	URL            string `json:"url"`
	Address        string `json:"address"`
	Command        string `json:"command"`
	ComposeService string `json:"compose_service"`
	Expect         string `json:"expect"`
	ApplyOrder     int    `json:"apply_order"`
	SummaryJSON    string `json:"summary_json"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func (r environmentHealthCheckRow) toStore() store.EnvironmentHealthCheck {
	return store.EnvironmentHealthCheck{
		EnvID:          r.EnvID,
		CheckID:        r.CheckID,
		Kind:           r.Kind,
		URL:            r.URL,
		Address:        r.Address,
		Command:        r.Command,
		ComposeService: r.ComposeService,
		Expect:         r.Expect,
		ApplyOrder:     r.ApplyOrder,
		SummaryJSON:    normalizeJSONText(r.SummaryJSON),
		CreatedAt:      decodeTime(r.CreatedAt),
		UpdatedAt:      decodeTime(r.UpdatedAt),
	}
}
