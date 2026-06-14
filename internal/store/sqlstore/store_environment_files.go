package sqlstore

import (
	"context"
	"fmt"

	"agent-testbench/internal/store"
)

func (s *Store) ReplaceEnvironmentFiles(ctx context.Context, envID string, files []store.EnvironmentFile) (err error) {
	files = store.NormalizeEnvironmentFiles(files)
	if err := store.ValidateEnvironmentFiles(envID, files); err != nil {
		return err
	}
	return s.runEnvironmentReplaceTx(ctx, func(tx sqlExecer) error {
		return s.replaceEnvironmentFilesTx(ctx, tx, envID, files)
	})
}

func (s *Store) replaceEnvironmentFilesTx(ctx context.Context, execer sqlExecer, envID string, files []store.EnvironmentFile) error {
	if _, err := execer.ExecContext(ctx, fmt.Sprintf(`delete from environment_files where env_id = %s;`, s.dialect.BindVar(1)), envID); err != nil {
		return fmt.Errorf("clear environment files for %q: %w", envID, err)
	}
	now := utcNow()
	for _, file := range files {
		applyAuditTimeDefaults(&file.CreatedAt, &file.UpdatedAt, now)
		query := fmt.Sprintf(`
insert into environment_files (
  env_id, file_path, file_kind, content_inline, required, apply_order,
  summary_json, created_at, updated_at
) values (%s);`, s.bindVars(9))
		if _, err := execer.ExecContext(ctx, query,
			envID, file.Path, file.Kind, file.ContentInline, file.Required, file.ApplyOrder,
			stringDefault(file.SummaryJSON, "{}"), dbTimeArg(s.dialect, file.CreatedAt), dbTimeArg(s.dialect, file.UpdatedAt),
		); err != nil {
			return fmt.Errorf("insert environment file %q kind=%q: %w", file.Path, file.Kind, err)
		}
	}
	return nil
}

func (s *Store) ListEnvironmentFiles(ctx context.Context, envID string) (items []store.EnvironmentFile, err error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
select env_id, file_path, file_kind, content_inline, required, apply_order,
  summary_json, created_at, updated_at
from environment_files
where env_id = %s
order by apply_order, file_kind, file_path;`, s.dialect.BindVar(1)), envID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows, &err)
	for rows.Next() {
		item, err := scanEnvironmentFile(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanEnvironmentFile(row scanner) (store.EnvironmentFile, error) {
	var item store.EnvironmentFile
	if err := scanRowWithAuditTimes(row, []any{
		&item.EnvID, &item.Path, &item.Kind, &item.ContentInline, &item.Required, &item.ApplyOrder,
		&item.SummaryJSON,
	}, &item.CreatedAt, &item.UpdatedAt, &item.SummaryJSON); err != nil {
		return store.EnvironmentFile{}, err
	}
	return item, nil
}
