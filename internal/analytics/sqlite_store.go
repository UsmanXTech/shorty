package analytics

import (
	"database/sql"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func (s *SQLiteStore) RecordEvent(event Event) error {
	_, err := s.db.Exec(
		"INSERT INTO click_events (slug, created_at) VALUES (?, ?)",
		event.Slug, event.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
	)
	return err
}
