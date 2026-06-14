package sqlstore

import (
	"context"
	"fmt"

	"agent-testbench/internal/store"
)

func (s *Store) ReplaceEnvironmentServices(ctx context.Context, envID string, services []store.EnvironmentService) (err error) {
	services = store.NormalizeEnvironmentServices(services)
	if err := store.ValidateEnvironmentServices(envID, services); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackTxOnError(tx, &err)
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`delete from environment_services where env_id = %s;`, s.dialect.BindVar(1)), envID); err != nil {
		return fmt.Errorf("clear environment services for %q: %w", envID, err)
	}
	now := utcNow()
	for _, service := range services {
		applyAuditTimeDefaults(&service.CreatedAt, &service.UpdatedAt, now)
		query := fmt.Sprintf(`
insert into environment_services (
  env_id, service_id, repo_url, branch, ref, checkout,
  summary_json, created_at, updated_at
) values (%s);`, s.bindVars(9))
		if _, err := tx.ExecContext(ctx, query,
			envID, service.ServiceID, service.RepoURL, service.Branch, service.Ref, service.Checkout,
			stringDefault(service.SummaryJSON, "{}"), dbTimeArg(s.dialect, service.CreatedAt), dbTimeArg(s.dialect, service.UpdatedAt),
		); err != nil {
			return fmt.Errorf("insert environment service %q: %w", service.ServiceID, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *Store) ListEnvironmentServices(ctx context.Context, envID string) (items []store.EnvironmentService, err error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
select env_id, service_id, repo_url, branch, ref, checkout,
  summary_json, created_at, updated_at
from environment_services
where env_id = %s
order by service_id;`, s.dialect.BindVar(1)), envID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows, &err)
	for rows.Next() {
		item, err := scanEnvironmentService(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ReplaceEnvironmentHealthChecks(ctx context.Context, envID string, checks []store.EnvironmentHealthCheck) (err error) {
	checks = store.NormalizeEnvironmentHealthChecks(checks)
	if err := store.ValidateEnvironmentHealthChecks(envID, checks); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackTxOnError(tx, &err)
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`delete from environment_health_checks where env_id = %s;`, s.dialect.BindVar(1)), envID); err != nil {
		return fmt.Errorf("clear environment health checks for %q: %w", envID, err)
	}
	now := utcNow()
	for _, check := range checks {
		applyAuditTimeDefaults(&check.CreatedAt, &check.UpdatedAt, now)
		query := fmt.Sprintf(`
insert into environment_health_checks (
  env_id, check_id, check_kind, url, address, command, compose_service,
  expect, apply_order, summary_json, created_at, updated_at
) values (%s);`, s.bindVars(12))
		if _, err := tx.ExecContext(ctx, query,
			envID, check.CheckID, check.Kind, check.URL, check.Address, check.Command, check.ComposeService,
			check.Expect, check.ApplyOrder, stringDefault(check.SummaryJSON, "{}"),
			dbTimeArg(s.dialect, check.CreatedAt), dbTimeArg(s.dialect, check.UpdatedAt),
		); err != nil {
			return fmt.Errorf("insert environment health check %q: %w", check.CheckID, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *Store) ListEnvironmentHealthChecks(ctx context.Context, envID string) (items []store.EnvironmentHealthCheck, err error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
select env_id, check_id, check_kind, url, address, command, compose_service,
  expect, apply_order, summary_json, created_at, updated_at
from environment_health_checks
where env_id = %s
order by apply_order, check_id;`, s.dialect.BindVar(1)), envID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows, &err)
	for rows.Next() {
		item, err := scanEnvironmentHealthCheck(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanEnvironmentService(row scanner) (store.EnvironmentService, error) {
	var item store.EnvironmentService
	if err := scanRowWithAuditTimes(row, []any{
		&item.EnvID, &item.ServiceID, &item.RepoURL, &item.Branch, &item.Ref, &item.Checkout,
		&item.SummaryJSON,
	}, &item.CreatedAt, &item.UpdatedAt, &item.SummaryJSON); err != nil {
		return store.EnvironmentService{}, err
	}
	return item, nil
}

func scanEnvironmentHealthCheck(row scanner) (store.EnvironmentHealthCheck, error) {
	var item store.EnvironmentHealthCheck
	if err := scanRowWithAuditTimes(row, []any{
		&item.EnvID, &item.CheckID, &item.Kind, &item.URL, &item.Address, &item.Command, &item.ComposeService,
		&item.Expect, &item.ApplyOrder, &item.SummaryJSON,
	}, &item.CreatedAt, &item.UpdatedAt, &item.SummaryJSON); err != nil {
		return store.EnvironmentHealthCheck{}, err
	}
	return item, nil
}
