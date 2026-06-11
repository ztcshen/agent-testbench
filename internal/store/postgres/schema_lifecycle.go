package postgres

import (
	"context"
	"database/sql"

	"agent-testbench/internal/store/sqlstore"
)

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
	status, err := sqlstore.SchemaStatusForStore(ctx, cfg.URL, dbOpener(cfg), sqlstore.PostgresDialect{})
	return SchemaStatusResult(status), err
}

func UpgradeSchema(ctx context.Context, cfg Config) (SchemaStatusResult, error) {
	status, err := sqlstore.UpgradeSchemaForStore(ctx, cfg.URL, dbOpener(cfg), sqlstore.PostgresDialect{})
	return SchemaStatusResult(status), err
}

func dbOpener(cfg Config) sqlstore.DBOpener {
	return func(ctx context.Context) (*sql.DB, error) {
		return openDB(ctx, cfg)
	}
}
