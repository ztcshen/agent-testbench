package sqlstore

import (
	"fmt"
	"net/url"
	"strings"
)

type Dialect interface {
	Name() string
	DriverName() string
	BindVar(index int) string
	JSONType() string
	TimeType() string
	BoolType() string
	QuoteIdent(name string) string
	UpsertClause(conflictColumn string, updateColumns []string) string
}

type Config struct {
	Backend    string
	DriverName string
	DSN        string
	Dialect    Dialect
}

type PostgresDialect struct{}
type MySQLDialect struct{}
type SQLiteDialect struct{}

func ConfigFromReference(reference string) (Config, error) {
	dialect, err := DialectFromReference(reference)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Backend:    dialect.Name(),
		DriverName: dialect.DriverName(),
		DSN:        strings.TrimSpace(reference),
		Dialect:    dialect,
	}, nil
}

func DialectFromReference(reference string) (Dialect, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return SQLiteDialect{}, nil
	}
	parsed, err := url.Parse(reference)
	if err != nil || parsed.Scheme == "" {
		if strings.Contains(reference, "://") {
			return nil, fmt.Errorf("invalid store reference %q", reference)
		}
		return SQLiteDialect{}, nil
	}
	switch strings.ToLower(parsed.Scheme) {
	case "postgres", "postgresql":
		return PostgresDialect{}, nil
	case "mysql":
		return MySQLDialect{}, nil
	case "sqlite", "file":
		return SQLiteDialect{}, nil
	default:
		return nil, fmt.Errorf("unsupported store backend %q", parsed.Scheme)
	}
}

func (PostgresDialect) Name() string       { return "postgres" }
func (PostgresDialect) DriverName() string { return "pgx" }
func (PostgresDialect) BindVar(index int) string {
	if index < 1 {
		index = 1
	}
	return fmt.Sprintf("$%d", index)
}
func (PostgresDialect) JSONType() string { return "jsonb" }
func (PostgresDialect) TimeType() string { return "timestamptz" }
func (PostgresDialect) BoolType() string { return "boolean" }
func (PostgresDialect) QuoteIdent(name string) string {
	return quoteDouble(name)
}
func (PostgresDialect) UpsertClause(conflictColumn string, updateColumns []string) string {
	return standardUpsert(conflictColumn, updateColumns)
}

func (MySQLDialect) Name() string       { return "mysql" }
func (MySQLDialect) DriverName() string { return "mysql" }
func (MySQLDialect) BindVar(int) string { return "?" }
func (MySQLDialect) JSONType() string   { return "json" }
func (MySQLDialect) TimeType() string   { return "datetime(6)" }
func (MySQLDialect) BoolType() string   { return "boolean" }
func (MySQLDialect) QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
func (MySQLDialect) UpsertClause(_ string, updateColumns []string) string {
	assignments := make([]string, 0, len(updateColumns))
	for _, column := range updateColumns {
		column = strings.TrimSpace(column)
		if column == "" {
			continue
		}
		assignments = append(assignments, fmt.Sprintf("%s = values(%s)", column, column))
	}
	return "on duplicate key update " + strings.Join(assignments, ", ")
}

func (SQLiteDialect) Name() string       { return "sqlite" }
func (SQLiteDialect) DriverName() string { return "sqlite" }
func (SQLiteDialect) BindVar(int) string { return "?" }
func (SQLiteDialect) JSONType() string   { return "text" }
func (SQLiteDialect) TimeType() string   { return "text" }
func (SQLiteDialect) BoolType() string   { return "integer" }
func (SQLiteDialect) QuoteIdent(name string) string {
	return quoteDouble(name)
}
func (SQLiteDialect) UpsertClause(conflictColumn string, updateColumns []string) string {
	return standardUpsert(conflictColumn, updateColumns)
}

func standardUpsert(conflictColumn string, updateColumns []string) string {
	assignments := make([]string, 0, len(updateColumns))
	for _, column := range updateColumns {
		column = strings.TrimSpace(column)
		if column == "" {
			continue
		}
		assignments = append(assignments, fmt.Sprintf("%s = excluded.%s", column, column))
	}
	return fmt.Sprintf("on conflict(%s) do update set %s", strings.TrimSpace(conflictColumn), strings.Join(assignments, ", "))
}

func quoteDouble(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
