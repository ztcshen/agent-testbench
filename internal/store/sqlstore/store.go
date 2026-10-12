package sqlstore

import (
	"database/sql"
	"strings"
	"time"

	"agent-testbench/internal/store/assistantstate"
)

type Store struct {
	*assistantstate.Repository
	db      *sql.DB
	dialect Dialect
}

func New(db *sql.DB, dialect Dialect) *Store {
	s := &Store{db: db, dialect: dialect}
	s.Repository = assistantstate.New(db, dialect.BindVar, s.bindVars, func(value time.Time) any { return dbTimeArg(dialect, value) }, decodeDBTime)
	return s
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) bindVars(count int) string {
	vars := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		vars = append(vars, s.dialect.BindVar(i))
	}
	return strings.Join(vars, ", ")
}
