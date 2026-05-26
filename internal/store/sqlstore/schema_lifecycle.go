package sqlstore

import (
	"context"
	"database/sql"
)

type DBOpener func(context.Context) (*sql.DB, error)

type StoreSchemaStatusResult struct {
	URL            string
	CurrentVersion int
	TargetVersion  int
	AppliedCount   int
}

func SchemaStatusForStore(ctx context.Context, storeURL string, open DBOpener, d Dialect) (StoreSchemaStatusResult, error) {
	status, err := SchemaStatusFromOpener(ctx, open, d)
	if err != nil {
		return StoreSchemaStatusResult{}, err
	}
	return storeSchemaStatus(storeURL, status), nil
}

func UpgradeSchemaForStore(ctx context.Context, storeURL string, open DBOpener, d Dialect) (StoreSchemaStatusResult, error) {
	status, err := UpgradeSchemaFromOpener(ctx, open, d)
	if err != nil {
		return StoreSchemaStatusResult{}, err
	}
	return storeSchemaStatus(storeURL, status), nil
}

func SchemaStatusFromOpener(ctx context.Context, open DBOpener, d Dialect) (SchemaStatusResult, error) {
	db, err := open(ctx)
	if err != nil {
		return SchemaStatusResult{}, err
	}
	defer closeOpenedDB(db)
	return SchemaStatus(ctx, db, d)
}

func UpgradeSchemaFromOpener(ctx context.Context, open DBOpener, d Dialect) (SchemaStatusResult, error) {
	db, err := open(ctx)
	if err != nil {
		return SchemaStatusResult{}, err
	}
	defer closeOpenedDB(db)
	return UpgradeSchema(ctx, db, d)
}

func closeOpenedDB(db *sql.DB) {
	if err := db.Close(); err != nil {
		return
	}
}

func storeSchemaStatus(storeURL string, status SchemaStatusResult) StoreSchemaStatusResult {
	return StoreSchemaStatusResult{
		URL:            storeURL,
		CurrentVersion: status.CurrentVersion,
		TargetVersion:  status.TargetVersion,
		AppliedCount:   status.AppliedCount,
	}
}
