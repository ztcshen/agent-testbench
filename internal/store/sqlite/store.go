package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"agent-testbench/internal/store/sqlstore"

	_ "modernc.org/sqlite"
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
	*sqlstore.Store
	db   *sql.DB
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
	return r.CurrentVersion != r.TargetVersion
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

	db, err := openDB(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &Store{Store: sqlstore.New(db, sqlstore.SQLiteDialect{}), db: db, path: cfg.Path}, nil
}

func (s *Store) Close() error {
	if s == nil || s.Store == nil {
		return nil
	}
	return s.Store.Close()
}

func openDB(ctx context.Context, cfg Config) (*sql.DB, error) {
	cfg = cfg.Resolve()
	db, err := sql.Open(sqlstore.SQLiteDialect{}.DriverName(), cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite store: %w", err)
	}
	if err := configureDB(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite store: %w", err)
	}
	return db, nil
}

func configureDB(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;
PRAGMA journal_mode = WAL;`); err != nil {
		return fmt.Errorf("configure sqlite store: %w", err)
	}
	return nil
}

func (s *Store) upgradeSchema(ctx context.Context) (SchemaStatusResult, error) {
	normalized, err := normalizeLegacySchema(ctx, s.db)
	if err != nil {
		return SchemaStatusResult{}, err
	}
	status, err := sqlstore.UpgradeSchema(ctx, s.db, sqlstore.SQLiteDialect{})
	if normalized && err == nil {
		status.AppliedCount++
	}
	return sqliteSchemaStatus(s.path, status), err
}

func (s *Store) schemaStatus(ctx context.Context, applied int) (SchemaStatusResult, error) {
	status, err := sqlstore.SchemaStatus(ctx, s.db, sqlstore.SQLiteDialect{})
	status.AppliedCount = applied
	return sqliteSchemaStatus(s.path, status), err
}

func sqliteSchemaStatus(path string, status sqlstore.SchemaStatusResult) SchemaStatusResult {
	return SchemaStatusResult{
		Path:           path,
		CurrentVersion: status.CurrentVersion,
		TargetVersion:  status.TargetVersion,
		AppliedCount:   status.AppliedCount,
	}
}
