package postgres_test

import (
	"testing"

	"open-test-sandbox/internal/store/postgres"
)

func TestParseConfigFromURLAcceptsPostgresDSN(t *testing.T) {
	cfg, err := postgres.ParseConfigFromURL("postgres://user:secret@example.com:5432/otsandbox?sslmode=disable")
	if err != nil {
		t.Fatalf("parse postgres dsn: %v", err)
	}
	if cfg.URL != "postgres://user:secret@example.com:5432/otsandbox?sslmode=disable" {
		t.Fatalf("postgres config url = %q", cfg.URL)
	}
}

func TestParseConfigFromURLRejectsNonPostgresDSN(t *testing.T) {
	_, err := postgres.ParseConfigFromURL("sqlite://tmp/store.sqlite")
	if err == nil {
		t.Fatal("expected non-postgres dsn to be rejected")
	}
}
