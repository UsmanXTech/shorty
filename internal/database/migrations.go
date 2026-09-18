package database

import (
	"database/sql"
	"fmt"
)

const schemaVersion = 4

func Migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version >= schemaVersion {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()
	if version < 1 {
		const schema = `
CREATE TABLE IF NOT EXISTS links (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	slug TEXT NOT NULL UNIQUE,
	url TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	expires_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_links_slug ON links(slug);
CREATE INDEX IF NOT EXISTS idx_links_expires_at ON links(expires_at);
PRAGMA user_version = 1;
`
		if _, err := tx.Exec(schema); err != nil {
			return fmt.Errorf("apply schema v1: %w", err)
		}
	}
	if version < 2 {
		if _, err := tx.Exec(`ALTER TABLE links ADD COLUMN max_clicks INTEGER;`); err != nil {
			return fmt.Errorf("apply schema v2: %w", err)
		}
		if _, err := tx.Exec(`ALTER TABLE links ADD COLUMN clicks INTEGER NOT NULL DEFAULT 0;`); err != nil {
			return fmt.Errorf("apply schema v2 clicks: %w", err)
		}
		if _, err := tx.Exec(`PRAGMA user_version = 2;`); err != nil {
			return fmt.Errorf("set schema v2: %w", err)
		}
	}
	if version < 3 {
		const schema = `
CREATE TABLE IF NOT EXISTS click_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	slug TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_click_events_slug ON click_events(slug);
CREATE INDEX IF NOT EXISTS idx_click_events_created_at ON click_events(created_at);
PRAGMA user_version = 3;
`
		if _, err := tx.Exec(schema); err != nil {
			return fmt.Errorf("apply schema v3: %w", err)
		}
	}
	if version < 4 {
		if _, err := tx.Exec(`ALTER TABLE click_events ADD COLUMN referrer TEXT;`); err != nil {
			return fmt.Errorf("apply schema v4 referrer: %w", err)
		}
		if _, err := tx.Exec(`ALTER TABLE click_events ADD COLUMN user_agent TEXT;`); err != nil {
			return fmt.Errorf("apply schema v4 user agent: %w", err)
		}
		if _, err := tx.Exec(`PRAGMA user_version = 4;`); err != nil {
			return fmt.Errorf("set schema v4: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}
