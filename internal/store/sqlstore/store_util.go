package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

type sqlExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func utcNow() time.Time {
	return time.Now().UTC()
}

func dbTimeArg(d Dialect, t time.Time) any {
	if t.IsZero() {
		if d.Name() == "sqlite" {
			return ""
		}
		return nil
	}
	if d.Name() == "sqlite" {
		return encodeTime(t)
	}
	return t.UTC()
}

func encodeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func stringDefault(value string, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func decodeTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func decodeDBTime(value any) time.Time {
	switch v := value.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return v.UTC()
	case string:
		return decodeTime(v)
	case []byte:
		return decodeTime(string(v))
	default:
		return time.Time{}
	}
}

func applyAuditTimeDefaults(createdAt *time.Time, updatedAt *time.Time, now time.Time) {
	if createdAt.IsZero() {
		*createdAt = now
	}
	if updatedAt.IsZero() {
		*updatedAt = now
	}
}

func applyDecodedAuditTimes(createdAt any, updatedAt any, targetCreatedAt *time.Time, targetUpdatedAt *time.Time) {
	*targetCreatedAt = decodeDBTime(createdAt)
	*targetUpdatedAt = decodeDBTime(updatedAt)
}

func scanRowWithAuditTimes(row scanner, fields []any, createdAt *time.Time, updatedAt *time.Time, jsonFields ...*string) error {
	var createdRaw, updatedRaw any
	targets := make([]any, 0, len(fields)+2)
	targets = append(targets, fields...)
	targets = append(targets, &createdRaw, &updatedRaw)
	if err := row.Scan(targets...); err != nil {
		return err
	}
	for _, field := range jsonFields {
		if field != nil {
			*field = normalizeJSONText(*field)
		}
	}
	applyDecodedAuditTimes(createdRaw, updatedRaw, createdAt, updatedAt)
	return nil
}

func scanStoreRowError(err error) error {
	if err == sql.ErrNoRows {
		return store.ErrNotFound
	}
	return err
}

func rollbackTxOnError(tx *sql.Tx, errp *error) {
	if *errp == nil {
		return
	}
	if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
		*errp = errors.Join(*errp, rollbackErr)
	}
}

func (s *Store) runEnvironmentReplaceTx(ctx context.Context, replace func(sqlExecer) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackTxOnError(tx, &err)
	if err := replace(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func normalizeJSONText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return value
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return value
	}
	return string(encoded)
}

func closeRows(rows *sql.Rows, errp *error) {
	if rows == nil {
		return
	}
	if closeErr := rows.Close(); closeErr != nil && *errp == nil {
		*errp = closeErr
	}
}

func queryStoreRows[T any](ctx context.Context, db *sql.DB, query string, scan func(scanner) (T, error), args ...any) (out []T, err error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows, &err)
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
