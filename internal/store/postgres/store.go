package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"agent-testbench/internal/store/sqlstore"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Config struct {
	URL        string
	DriverName string
}

type Store struct {
	core *sqlstore.Store
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
		return Config{URL: storeURL, DriverName: sqlstore.PostgresDialect{}.DriverName()}, nil
	default:
		return Config{}, fmt.Errorf("unsupported postgres store backend %q", parsed.Scheme)
	}
}

func Open(ctx context.Context, cfg Config) (*Store, error) {
	db, err := openDB(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if _, err := sqlstore.UpgradeSchema(ctx, db, sqlstore.PostgresDialect{}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("upgrade postgres store schema: %w", err)
	}
	return &Store{core: sqlstore.New(db, sqlstore.PostgresDialect{})}, nil
}

func openDB(ctx context.Context, cfg Config) (*sql.DB, error) {
	driverName := strings.TrimSpace(cfg.DriverName)
	if driverName == "" {
		driverName = sqlstore.PostgresDialect{}.DriverName()
	}
	db, err := sql.Open(driverName, cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("open postgres store: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres store: %w", err)
	}
	return db, nil
}

func (s *Store) Close() error {
	if s == nil || s.core == nil {
		return nil
	}
	return s.core.Close()
}
