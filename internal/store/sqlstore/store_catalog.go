package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"agent-testbench/internal/store"
)

func (s *Store) ReplaceProfileCatalog(ctx context.Context, catalog store.ProfileCatalog) error {
	if catalog.ProfileID == "" {
		return errors.New("replace profile catalog: profile id is required")
	}
	expectedRevision := int64(0)
	if current, err := s.GetProfileCatalogSnapshot(ctx, catalog.ProfileID); err == nil {
		expectedRevision = current.Revision
	} else if !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("read profile catalog %q before replace: %w", catalog.ProfileID, err)
	}
	_, err := s.CompareAndSwapProfileCatalog(ctx, expectedRevision, catalog, store.ProfileCatalogMutation{
		Operation: "full-replace",
	})
	return err
}

func (s *Store) GetProfileCatalog(ctx context.Context) (store.ProfileCatalog, error) {
	var payload string
	err := s.db.QueryRowContext(ctx, `
select catalog_json
from profile_catalogs
order by indexed_at desc, profile_id desc
limit 1;`).Scan(&payload)
	if err != nil {
		if err == sql.ErrNoRows {
			return store.ProfileCatalog{}, store.ErrNotFound
		}
		return store.ProfileCatalog{}, err
	}
	var catalog store.ProfileCatalog
	if err := json.Unmarshal([]byte(payload), &catalog); err != nil {
		return store.ProfileCatalog{}, fmt.Errorf("decode profile catalog: %w", err)
	}
	return catalog, nil
}

func (s *Store) GetProfileCatalogByID(ctx context.Context, profileID string) (store.ProfileCatalog, error) {
	query := fmt.Sprintf(`
select catalog_json
from profile_catalogs
where profile_id = %s;`, s.dialect.BindVar(1))
	var payload string
	err := s.db.QueryRowContext(ctx, query, profileID).Scan(&payload)
	if err != nil {
		if err == sql.ErrNoRows {
			return store.ProfileCatalog{}, store.ErrNotFound
		}
		return store.ProfileCatalog{}, err
	}
	var catalog store.ProfileCatalog
	if err := json.Unmarshal([]byte(payload), &catalog); err != nil {
		return store.ProfileCatalog{}, fmt.Errorf("decode profile catalog %q: %w", profileID, err)
	}
	return catalog, nil
}

func (s *Store) GetProfileCatalogIndex(ctx context.Context) (store.ProfileCatalogIndex, error) {
	row := s.db.QueryRowContext(ctx, `
select profile_id, indexed_at, services, workflows, interface_nodes, api_cases, request_templates,
  workflow_bindings, case_dependencies, fixtures, templates, template_configs
from profile_catalogs
order by indexed_at desc, profile_id desc
limit 1;`)
	index, err := scanProfileCatalogIndex(row)
	if err != nil {
		return store.ProfileCatalogIndex{}, err
	}
	return index, nil
}

func (s *Store) ListProfileCatalogIndexes(ctx context.Context) (indexes []store.ProfileCatalogIndex, err error) {
	rows, err := s.db.QueryContext(ctx, `
select profile_id, indexed_at, services, workflows, interface_nodes, api_cases, request_templates,
  workflow_bindings, case_dependencies, fixtures, templates, template_configs
from profile_catalogs
order by indexed_at desc, profile_id desc;`)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows, &err)
	for rows.Next() {
		index, err := scanProfileCatalogIndex(rows)
		if err != nil {
			return nil, err
		}
		indexes = append(indexes, index)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return indexes, nil
}

func scanProfileCatalogIndex(row scanner) (store.ProfileCatalogIndex, error) {
	var r store.ProfileCatalogIndex
	var indexedAt any
	if err := row.Scan(
		&r.ProfileID, &indexedAt, &r.Counts.Services, &r.Counts.Workflows, &r.Counts.InterfaceNodes,
		&r.Counts.APICases, &r.Counts.RequestTemplates, &r.Counts.WorkflowBindings, &r.Counts.CaseDependencies,
		&r.Counts.Fixtures, &r.Counts.Templates, &r.Counts.TemplateConfigs,
	); err != nil {
		if err == sql.ErrNoRows {
			return store.ProfileCatalogIndex{}, store.ErrNotFound
		}
		return store.ProfileCatalogIndex{}, err
	}
	r.IndexedAt = decodeDBTime(indexedAt)
	return r, nil
}

func catalogCounts(catalog store.ProfileCatalog) store.ProfileCatalogCounts {
	return store.ProfileCatalogCounts{
		Services:         len(catalog.Services),
		Workflows:        len(catalog.Workflows),
		InterfaceNodes:   len(catalog.InterfaceNodes),
		APICases:         len(catalog.APICases),
		RequestTemplates: len(catalog.RequestTemplates),
		WorkflowBindings: len(catalog.WorkflowBindings),
		CaseDependencies: len(catalog.CaseDependencies),
		Fixtures:         len(catalog.Fixtures),
		Templates:        len(catalog.Workflows) + len(catalog.RequestTemplates) + len(catalog.TemplateConfigs),
		TemplateConfigs:  len(catalog.Workflows) + len(catalog.RequestTemplates) + len(catalog.TemplateConfigs),
	}
}
