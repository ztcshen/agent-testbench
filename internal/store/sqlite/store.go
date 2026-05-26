package sqlite

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"agent-testbench/internal/store/schema"
)

type Config struct {
	Path    string
	BaseDir string
}

func (c Config) Resolve() Config {
	if c.Path != "" {
		return c
	}
	baseDir := c.BaseDir
	if baseDir == "" {
		baseDir = "."
	}
	c.Path = filepath.Join(baseDir, "runtime", "store.sqlite")
	return c
}

func ConfigFromURL(storeURL string) Config {
	if storeURL == "" {
		return Config{}.Resolve()
	}
	for _, prefix := range []string{"sqlite://", "file:"} {
		if strings.HasPrefix(storeURL, prefix) {
			return Config{Path: strings.TrimPrefix(storeURL, prefix)}.Resolve()
		}
	}
	return Config{Path: storeURL}.Resolve()
}

func ParseConfigFromURL(storeURL string) (Config, error) {
	if storeURL == "" {
		return ConfigFromURL(storeURL), nil
	}
	if isUnsupportedBackendURL(storeURL) {
		return Config{}, fmt.Errorf("unsupported store backend %q; supported forms are local paths, sqlite://PATH, and file:PATH", backendScheme(storeURL))
	}
	return ConfigFromURL(storeURL), nil
}

func isUnsupportedBackendURL(storeURL string) bool {
	scheme := backendScheme(storeURL)
	if scheme == "" {
		return false
	}
	return scheme != "sqlite" && scheme != "file"
}

func backendScheme(storeURL string) string {
	match := regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]*):`).FindStringSubmatch(storeURL)
	if len(match) != 2 {
		return ""
	}
	return strings.ToLower(match[1])
}

type Store struct {
	path string
}

func Open(ctx context.Context, cfg Config) (*Store, error) {
	if sqliteStoreDisabled() {
		return nil, errors.New("SQLite Store is disabled by AGENT_TESTBENCH_DISABLE_SQLITE_STORE; use a PostgreSQL or MySQL Store for this run")
	}
	s, err := openRaw(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if _, err := s.upgradeSchema(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

type SchemaStatusResult struct {
	Path           string
	CurrentVersion int
	TargetVersion  int
	AppliedCount   int
}

func (r SchemaStatusResult) HasPending() bool {
	return r.CurrentVersion < r.TargetVersion
}

func SchemaStatus(ctx context.Context, cfg Config) (SchemaStatusResult, error) {
	if sqliteStoreDisabled() {
		return SchemaStatusResult{}, errors.New("SQLite Store is disabled by AGENT_TESTBENCH_DISABLE_SQLITE_STORE; use a PostgreSQL or MySQL Store for this run")
	}
	s, err := openRaw(ctx, cfg)
	if err != nil {
		return SchemaStatusResult{}, err
	}
	defer s.Close()
	return s.schemaStatus(ctx, 0)
}

func UpgradeSchema(ctx context.Context, cfg Config) (SchemaStatusResult, error) {
	if sqliteStoreDisabled() {
		return SchemaStatusResult{}, errors.New("SQLite Store is disabled by AGENT_TESTBENCH_DISABLE_SQLITE_STORE; use a PostgreSQL or MySQL Store for this run")
	}
	s, err := openRaw(ctx, cfg)
	if err != nil {
		return SchemaStatusResult{}, err
	}
	defer s.Close()
	return s.upgradeSchema(ctx)
}

func sqliteStoreDisabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("AGENT_TESTBENCH_DISABLE_SQLITE_STORE")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func openRaw(ctx context.Context, cfg Config) (*Store, error) {
	cfg = cfg.Resolve()
	if cfg.Path == "" {
		return nil, errors.New("sqlite store path is required")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite store directory: %w", err)
	}

	s := &Store{path: cfg.Path}
	if err := s.configure(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return nil
}

func (s *Store) configure(ctx context.Context) error {
	return s.exec(ctx, `
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;
PRAGMA journal_mode = WAL;`)
}

func (s *Store) upgradeSchema(ctx context.Context) (SchemaStatusResult, error) {
	if err := s.ensureSchemaVersionTable(ctx); err != nil {
		return SchemaStatusResult{}, err
	}
	current, err := s.currentSchemaVersion(ctx)
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
commit;`, change.SQL, change.Version, sqlString(change.Name), sqlString(encodeTime(utcNow())))
		if err := s.exec(ctx, statement); err != nil {
			return SchemaStatusResult{}, fmt.Errorf("apply schema change %d %q: %w", change.Version, change.Name, err)
		}
		applied++
	}
	return s.schemaStatus(ctx, applied)
}

func (s *Store) schemaStatus(ctx context.Context, applied int) (SchemaStatusResult, error) {
	current, err := s.currentSchemaVersion(ctx)
	if err != nil {
		return SchemaStatusResult{}, err
	}
	return SchemaStatusResult{
		Path:           s.path,
		CurrentVersion: current,
		TargetVersion:  schema.CurrentVersion,
		AppliedCount:   applied,
	}, nil
}

func (s *Store) ensureSchemaVersionTable(ctx context.Context) error {
	return s.exec(ctx, `
create table if not exists schema_versions (
  version integer primary key,
  name text not null,
  applied_at text not null
);`)
}

func (s *Store) currentSchemaVersion(ctx context.Context) (int, error) {
	var tableRows []struct {
		Count int `json:"count"`
	}
	if err := s.query(ctx, `
select count(*) as count from sqlite_master
where type = 'table' and name = 'schema_versions';`, &tableRows); err != nil {
		return 0, err
	}
	if len(tableRows) == 0 || tableRows[0].Count == 0 {
		return 0, nil
	}

	var versionRows []struct {
		Version int `json:"version"`
	}
	if err := s.query(ctx, `select coalesce(max(version), 0) as version from schema_versions;`, &versionRows); err != nil {
		return 0, err
	}
	if len(versionRows) == 0 {
		return 0, nil
	}
	return versionRows[0].Version, nil
}
