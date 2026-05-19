package open

import (
	"context"
	"errors"
	"fmt"

	"open-test-sandbox/internal/store"
	"open-test-sandbox/internal/store/postgres"
	"open-test-sandbox/internal/store/sqlite"
	"open-test-sandbox/internal/store/sqlstore"
)

type Backend string

const (
	BackendSQLite   Backend = "sqlite"
	BackendPostgres Backend = "postgres"
	BackendMySQL    Backend = "mysql"
)

var ErrBackendUnavailable = errors.New("store backend unavailable")

func BackendFromReference(reference string) (Backend, error) {
	dialect, err := sqlstore.DialectFromReference(reference)
	if err != nil {
		return "", err
	}
	switch dialect.Name() {
	case "sqlite":
		return BackendSQLite, nil
	case "postgres":
		return BackendPostgres, nil
	case "mysql":
		return BackendMySQL, nil
	default:
		return "", fmt.Errorf("unsupported store backend %q", dialect.Name())
	}
}

func Open(ctx context.Context, reference string) (store.Store, error) {
	backend, err := BackendFromReference(reference)
	if err != nil {
		return nil, err
	}
	switch backend {
	case BackendSQLite:
		cfg, err := sqlite.ParseConfigFromURL(reference)
		if err != nil {
			return nil, err
		}
		return sqlite.Open(ctx, cfg)
	case BackendPostgres:
		cfg, err := postgres.ParseConfigFromURL(reference)
		if err != nil {
			return nil, err
		}
		return postgres.Open(ctx, cfg)
	case BackendMySQL:
		return nil, fmt.Errorf("%w: mysql store backend is recognized but not implemented yet", ErrBackendUnavailable)
	default:
		return nil, fmt.Errorf("unsupported store backend %q", backend)
	}
}
