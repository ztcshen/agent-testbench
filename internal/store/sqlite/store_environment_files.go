package sqlite

import (
	"context"
	"fmt"
	"strings"

	"agent-testbench/internal/store"
)

func (s *Store) ReplaceEnvironmentFiles(ctx context.Context, envID string, files []store.EnvironmentFile) error {
	files = store.NormalizeEnvironmentFiles(files)
	if err := store.ValidateEnvironmentFiles(envID, files); err != nil {
		return err
	}
	now := utcNow()
	statements := []string{fmt.Sprintf("delete from environment_files where env_id = %s;", sqlString(envID))}
	for _, file := range files {
		if file.CreatedAt.IsZero() {
			file.CreatedAt = now
		}
		if file.UpdatedAt.IsZero() {
			file.UpdatedAt = now
		}
		statements = append(statements, fmt.Sprintf(`
insert into environment_files (
  env_id, file_path, file_kind, content_inline, required, apply_order,
  summary_json, created_at, updated_at
) values (%s, %s, %s, %s, %d, %d, %s, %s, %s);`,
			sqlString(envID), sqlString(file.Path), sqlString(file.Kind), sqlString(file.ContentInline),
			boolInt(file.Required), file.ApplyOrder, sqlString(stringDefault(file.SummaryJSON, "{}")),
			sqlString(encodeTime(file.CreatedAt)), sqlString(encodeTime(file.UpdatedAt))))
	}
	return s.exec(ctx, "begin;\n"+strings.Join(statements, "\n")+"\ncommit;")
}

func (s *Store) ListEnvironmentFiles(ctx context.Context, envID string) ([]store.EnvironmentFile, error) {
	var rows []environmentFileRow
	if err := s.query(ctx, fmt.Sprintf(`
select env_id, file_path, file_kind, content_inline, required, apply_order,
  summary_json, created_at, updated_at
from environment_files
where env_id = %s
order by apply_order, file_kind, file_path;`, sqlString(envID)), &rows); err != nil {
		return nil, err
	}
	out := make([]store.EnvironmentFile, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toStore())
	}
	return out, nil
}

type environmentFileRow struct {
	EnvID         string `json:"env_id"`
	Path          string `json:"file_path"`
	Kind          string `json:"file_kind"`
	ContentInline string `json:"content_inline"`
	Required      int    `json:"required"`
	ApplyOrder    int    `json:"apply_order"`
	SummaryJSON   string `json:"summary_json"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

func (r environmentFileRow) toStore() store.EnvironmentFile {
	return store.EnvironmentFile{
		EnvID:         r.EnvID,
		Path:          r.Path,
		Kind:          r.Kind,
		ContentInline: r.ContentInline,
		Required:      r.Required != 0,
		ApplyOrder:    r.ApplyOrder,
		SummaryJSON:   normalizeJSONText(r.SummaryJSON),
		CreatedAt:     decodeTime(r.CreatedAt),
		UpdatedAt:     decodeTime(r.UpdatedAt),
	}
}
