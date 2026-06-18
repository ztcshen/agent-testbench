package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

func (s *Store) SaveTestPlanMapVersion(ctx context.Context, item store.TestPlanMapVersion) (store.TestPlanMapVersion, error) {
	item = prepareTestPlanMapVersion(item, utcNow())
	query := fmt.Sprintf(`
insert into test_map_versions (
  version_id, map_id, version, status, summary, graph_json, created_at
)
values (%s)
%s;`, s.bindVars(7), s.dialect.UpsertClause("version_id", []string{"status", "summary", "graph_json", "created_at"}))
	if _, err := s.db.ExecContext(ctx, query,
		item.ID, item.MapID, item.Version, item.Status, item.Summary, item.GraphJSON, dbTimeArg(s.dialect, item.CreatedAt),
	); err != nil {
		return store.TestPlanMapVersion{}, fmt.Errorf("save test map version %q: %w", item.ID, err)
	}
	return item, nil
}

func (s *Store) ListTestPlanMapVersions(ctx context.Context, mapID string) ([]store.TestPlanMapVersion, error) {
	mapID = strings.TrimSpace(mapID)
	if mapID == "" {
		return nil, store.ErrNotFound
	}
	query := fmt.Sprintf(`
select version_id, map_id, version, status, summary, graph_json, created_at
from test_map_versions
where map_id = %s
order by created_at desc, version_id desc;`, s.dialect.BindVar(1))
	return queryStoreRows(ctx, s.db, query, scanTestPlanMapVersion, mapID)
}

func prepareTestPlanMapVersion(item store.TestPlanMapVersion, now time.Time) store.TestPlanMapVersion {
	item.MapID = strings.TrimSpace(item.MapID)
	item.Version = strings.TrimSpace(item.Version)
	if item.ID == "" {
		item.ID = item.MapID + ":" + item.Version
	}
	if item.Status == "" {
		item.Status = "snapshot"
	}
	item.GraphJSON = jsonForDB(item.GraphJSON, "{}")
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	return item
}

func scanTestPlanMapVersion(row scanner) (store.TestPlanMapVersion, error) {
	var item store.TestPlanMapVersion
	var createdAt any
	if err := row.Scan(&item.ID, &item.MapID, &item.Version, &item.Status, &item.Summary, &item.GraphJSON, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return store.TestPlanMapVersion{}, store.ErrNotFound
		}
		return store.TestPlanMapVersion{}, err
	}
	item.GraphJSON = normalizeJSONText(item.GraphJSON)
	item.CreatedAt = decodeDBTime(createdAt)
	return item, nil
}
