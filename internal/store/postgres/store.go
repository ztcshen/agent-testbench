package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"open-test-sandbox/internal/store"
	"open-test-sandbox/internal/store/schema"
)

type Config struct {
	URL string
}

type Store struct {
	url string
}

func ParseConfigFromURL(storeURL string) (Config, error) {
	storeURL = strings.TrimSpace(storeURL)
	if storeURL == "" {
		return Config{}, errors.New("postgres store url is required")
	}
	parsed, err := url.Parse(storeURL)
	if err != nil {
		return Config{}, err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "postgres", "postgresql":
		return Config{URL: storeURL}, nil
	default:
		return Config{}, fmt.Errorf("unsupported postgres store backend %q", parsed.Scheme)
	}
}

func Open(ctx context.Context, cfg Config) (*Store, error) {
	return nil, errors.New("postgres store open is not implemented yet")
}

func (s *Store) Close() error {
	return nil
}

func (s *Store) CreateRun(context.Context, store.Run) (store.Run, error) {
	return store.Run{}, errPostgresStoreNotImplemented()
}

func (s *Store) GetRun(context.Context, string) (store.Run, error) {
	return store.Run{}, errPostgresStoreNotImplemented()
}

func (s *Store) ListRuns(context.Context) ([]store.Run, error) {
	return nil, errPostgresStoreNotImplemented()
}

func (s *Store) RecordAPICaseRun(context.Context, store.APICaseRun) (store.APICaseRun, error) {
	return store.APICaseRun{}, errPostgresStoreNotImplemented()
}

func (s *Store) ListAPICaseRuns(context.Context, string) ([]store.APICaseRun, error) {
	return nil, errPostgresStoreNotImplemented()
}

func (s *Store) RecordEvidence(context.Context, store.EvidenceRecord) (store.EvidenceRecord, error) {
	return store.EvidenceRecord{}, errPostgresStoreNotImplemented()
}

func (s *Store) ListEvidence(context.Context, string) ([]store.EvidenceRecord, error) {
	return nil, errPostgresStoreNotImplemented()
}

func (s *Store) SaveTraceTopology(context.Context, store.TraceTopology) (store.TraceTopology, error) {
	return store.TraceTopology{}, errPostgresStoreNotImplemented()
}

func (s *Store) ListTraceTopologies(context.Context, string) ([]store.TraceTopology, error) {
	return nil, errPostgresStoreNotImplemented()
}

func (s *Store) RecordPostProcessTask(context.Context, store.PostProcessTask) (store.PostProcessTask, error) {
	return store.PostProcessTask{}, errPostgresStoreNotImplemented()
}

func (s *Store) ListPostProcessTasks(context.Context, string) ([]store.PostProcessTask, error) {
	return nil, errPostgresStoreNotImplemented()
}

func (s *Store) UpsertBaselineGate(context.Context, store.BaselineGate) (store.BaselineGate, error) {
	return store.BaselineGate{}, errPostgresStoreNotImplemented()
}

func (s *Store) GetBaselineGate(context.Context, string, string) (store.BaselineGate, error) {
	return store.BaselineGate{}, errPostgresStoreNotImplemented()
}

func (s *Store) UpsertProfileIndex(context.Context, store.ProfileIndex) (store.ProfileIndex, error) {
	return store.ProfileIndex{}, errPostgresStoreNotImplemented()
}

func (s *Store) GetProfileIndex(context.Context, string) (store.ProfileIndex, error) {
	return store.ProfileIndex{}, errPostgresStoreNotImplemented()
}

func (s *Store) UpsertConfigVersion(context.Context, store.ConfigVersion) (store.ConfigVersion, error) {
	return store.ConfigVersion{}, errPostgresStoreNotImplemented()
}

func (s *Store) GetActiveConfigVersion(context.Context) (store.ConfigVersion, error) {
	return store.ConfigVersion{}, errPostgresStoreNotImplemented()
}

func (s *Store) UpsertReadModel(context.Context, store.ReadModel) (store.ReadModel, error) {
	return store.ReadModel{}, errPostgresStoreNotImplemented()
}

func (s *Store) GetReadModel(context.Context, string, string) (store.ReadModel, error) {
	return store.ReadModel{}, errPostgresStoreNotImplemented()
}

func (s *Store) ReplaceProfileCatalog(context.Context, store.ProfileCatalog) error {
	return errPostgresStoreNotImplemented()
}

func (s *Store) GetProfileCatalog(context.Context) (store.ProfileCatalog, error) {
	return store.ProfileCatalog{}, errPostgresStoreNotImplemented()
}

func (s *Store) GetProfileCatalogIndex(context.Context) (store.ProfileCatalogIndex, error) {
	return store.ProfileCatalogIndex{}, errPostgresStoreNotImplemented()
}

func errPostgresStoreNotImplemented() error {
	return errors.New("postgres store contract is not implemented yet")
}

type SchemaStatusResult struct {
	URL            string
	CurrentVersion int
	TargetVersion  int
	AppliedCount   int
}

func (r SchemaStatusResult) HasPending() bool {
	return r.CurrentVersion < r.TargetVersion
}

func SchemaStatus(ctx context.Context, cfg Config) (SchemaStatusResult, error) {
	current, err := currentSchemaVersion(ctx, cfg.URL)
	if err != nil {
		return SchemaStatusResult{}, err
	}
	return SchemaStatusResult{URL: cfg.URL, CurrentVersion: current, TargetVersion: schema.CurrentVersion}, nil
}

func UpgradeSchema(ctx context.Context, cfg Config) (SchemaStatusResult, error) {
	if err := ensureSchemaVersionTable(ctx, cfg.URL); err != nil {
		return SchemaStatusResult{}, err
	}
	current, err := currentSchemaVersion(ctx, cfg.URL)
	if err != nil {
		return SchemaStatusResult{}, err
	}
	applied := 0
	for _, change := range schema.All() {
		if change.Version <= current {
			continue
		}
		statement := fmt.Sprintf(`
begin;
%s
insert into schema_versions (version, name, applied_at)
values (%d, %s, %s);
commit;`, change.SQL, change.Version, pgString(change.Name), pgString(time.Now().UTC().Format(time.RFC3339Nano)))
		if err := psqlExec(ctx, cfg.URL, statement); err != nil {
			return SchemaStatusResult{}, fmt.Errorf("apply schema change %d %q: %w", change.Version, change.Name, err)
		}
		applied++
	}
	status, err := SchemaStatus(ctx, cfg)
	if err != nil {
		return SchemaStatusResult{}, err
	}
	status.AppliedCount = applied
	return status, nil
}

func ensureSchemaVersionTable(ctx context.Context, dsn string) error {
	return psqlExec(ctx, dsn, `
create table if not exists schema_versions (
  version integer primary key,
  name text not null,
  applied_at timestamptz not null
);`)
}

func currentSchemaVersion(ctx context.Context, dsn string) (int, error) {
	var tableRows []struct {
		Count int `json:"count"`
	}
	if err := psqlQuery(ctx, dsn, `
select case when exists (
  select 1 from information_schema.tables
  where table_schema = current_schema() and table_name = 'schema_versions'
) then 1 else 0 end as count`, &tableRows); err != nil {
		return 0, err
	}
	if len(tableRows) == 0 || tableRows[0].Count == 0 {
		return 0, nil
	}
	var versionRows []struct {
		Version int `json:"version"`
	}
	if err := psqlQuery(ctx, dsn, `select coalesce(max(version), 0) as version from schema_versions`, &versionRows); err != nil {
		return 0, err
	}
	if len(versionRows) == 0 {
		return 0, nil
	}
	return versionRows[0].Version, nil
}

func psqlExec(ctx context.Context, dsn string, statement string) error {
	cmd := exec.CommandContext(ctx, "psql", dsn, "-X", "-q", "-v", "ON_ERROR_STOP=1", "-c", statement)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run psql statement: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func psqlQuery(ctx context.Context, dsn string, statement string, target any) error {
	wrapped := fmt.Sprintf(`copy (select coalesce(json_agg(row_to_json(q)), '[]'::json) from (%s) q) to stdout;`, strings.TrimSuffix(strings.TrimSpace(statement), ";"))
	cmd := exec.CommandContext(ctx, "psql", dsn, "-X", "-q", "-t", "-A", "-v", "ON_ERROR_STOP=1", "-c", wrapped)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run psql query: %w: %s", err, strings.TrimSpace(string(out)))
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		raw = "[]"
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return fmt.Errorf("decode psql query result: %w", err)
	}
	return nil
}

func pgString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
