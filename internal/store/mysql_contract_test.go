package store_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"open-test-sandbox/internal/store/mysql"
)

func TestMySQLStoreContractWithExternalDatabase(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("OTSANDBOX_MYSQL_TEST_DSN"))
	if dsn == "" {
		t.Skip("set OTSANDBOX_MYSQL_TEST_DSN to run the MySQL Store contract")
	}
	ctx := context.Background()
	adminCfg, err := mysql.ParseConfigFromURL(dsn)
	if err != nil {
		t.Fatalf("parse mysql admin dsn: %v", err)
	}
	admin, err := sql.Open(adminCfg.DriverName, adminCfg.DSN)
	if err != nil {
		t.Fatalf("open mysql admin connection: %v", err)
	}
	defer admin.Close()
	if err := admin.PingContext(ctx); err != nil {
		t.Fatalf("ping mysql test database: %v", err)
	}
	databaseName := fmt.Sprintf("otsandbox_contract_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, `create database `+quoteMySQLIdent(databaseName)+` character set utf8mb4 collate utf8mb4_unicode_ci`); err != nil {
		t.Fatalf("create mysql test database: %v", err)
	}
	defer func() {
		_, _ = admin.ExecContext(context.Background(), `drop database if exists `+quoteMySQLIdent(databaseName))
	}()

	cfg, err := mysql.ParseConfigFromURL(mysqlTestDSNWithDatabase(t, dsn, databaseName))
	if err != nil {
		t.Fatalf("parse database mysql dsn: %v", err)
	}
	upgraded, err := mysql.UpgradeSchema(ctx, cfg)
	if err != nil {
		t.Fatalf("upgrade mysql schema: %v", err)
	}
	if upgraded.CurrentVersion != upgraded.TargetVersion || upgraded.AppliedCount == 0 {
		t.Fatalf("initial mysql upgrade = %#v", upgraded)
	}

	s, err := mysql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("open mysql store: %v", err)
	}
	defer s.Close()
	exerciseStoreContract(t, ctx, s)

	current, err := mysql.UpgradeSchema(ctx, cfg)
	if err != nil {
		t.Fatalf("repeat mysql upgrade: %v", err)
	}
	if current.CurrentVersion != current.TargetVersion || current.AppliedCount != 0 || current.HasPending() {
		t.Fatalf("repeat mysql upgrade = %#v", current)
	}
}

func mysqlTestDSNWithDatabase(t *testing.T, dsn string, databaseName string) string {
	t.Helper()
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse mysql test dsn: %v", err)
	}
	parsed.Path = "/" + databaseName
	return parsed.String()
}

func quoteMySQLIdent(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}
