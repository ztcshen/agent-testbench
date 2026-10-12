// Package assistantstate owns SQL persistence for agent conversations, separate
// from execution records and shared SQL-store migration/runner plumbing.
package assistantstate

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-testbench/internal/store"
)

type Repository struct {
	db         *sql.DB
	bind       func(int) string
	bindMany   func(int) string
	timeArg    func(time.Time) any
	decodeTime func(any) time.Time
}

func New(db *sql.DB, bind func(int) string, bindMany func(int) string, timeArg func(time.Time) any, decodeTime func(any) time.Time) *Repository {
	return &Repository{db: db, bind: bind, bindMany: bindMany, timeArg: timeArg, decodeTime: decodeTime}
}

const columns = "id, owner_id, map_id, profile_id, title, status, revision, state_json, created_at, updated_at"

func (s *Repository) CreateAssistantSession(ctx context.Context, item store.AssistantSession) error {
	_, err := s.db.ExecContext(ctx, "insert into assistant_sessions ("+columns+") values ("+s.bindMany(10)+");", item.ID, item.OwnerID, item.MapID, item.ProfileID, item.Title, item.Status, item.Revision, item.StateJSON, s.timeArg(item.CreatedAt), s.timeArg(item.UpdatedAt))
	return err
}

func (s *Repository) GetAssistantSession(ctx context.Context, id string) (store.AssistantSession, error) {
	return s.scan(s.db.QueryRowContext(ctx, "select "+columns+" from assistant_sessions where id = "+s.bind(1)+";", id))
}

func (s *Repository) ListAssistantSessions(ctx context.Context, ownerID, mapID string) ([]store.AssistantSession, error) {
	query := fmt.Sprintf("select %s from assistant_sessions where owner_id = %s and map_id = %s order by updated_at desc, id desc limit 50;", columns, s.bind(1), s.bind(2))
	rows, err := s.db.QueryContext(ctx, query, ownerID, mapID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []store.AssistantSession{}
	for rows.Next() {
		item, scanErr := s.scan(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Repository) UpdateAssistantSession(ctx context.Context, item store.AssistantSession, expected int64) (bool, error) {
	query := fmt.Sprintf(`update assistant_sessions set title = %s, status = %s, revision = revision + 1, state_json = %s, updated_at = %s
where id = %s and owner_id = %s and map_id = %s and profile_id = %s and revision = %s;`, s.bind(1), s.bind(2), s.bind(3), s.bind(4), s.bind(5), s.bind(6), s.bind(7), s.bind(8), s.bind(9))
	result, err := s.db.ExecContext(ctx, query, item.Title, item.Status, item.StateJSON, s.timeArg(item.UpdatedAt), item.ID, item.OwnerID, item.MapID, item.ProfileID, expected)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (s *Repository) scan(row interface{ Scan(...any) error }) (store.AssistantSession, error) {
	var item store.AssistantSession
	var created, updated any
	err := row.Scan(&item.ID, &item.OwnerID, &item.MapID, &item.ProfileID, &item.Title, &item.Status, &item.Revision, &item.StateJSON, &created, &updated)
	if err == sql.ErrNoRows {
		return item, store.ErrNotFound
	}
	item.CreatedAt, item.UpdatedAt = s.decodeTime(created), s.decodeTime(updated)
	return item, err
}
