package database

import (
	"database/sql"
	"fmt"
)

const schemaVersion = 7

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
	if version < 5 {
		if _, err := tx.Exec(`ALTER TABLE click_events ADD COLUMN country TEXT;`); err != nil {
			return fmt.Errorf("apply schema v5 country: %w", err)
		}
		if _, err := tx.Exec(`PRAGMA user_version = 5;`); err != nil {
			return fmt.Errorf("set schema v5: %w", err)
		}
	}
	if version < 6 {
		const schema = `
CREATE TABLE IF NOT EXISTS teams (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS api_keys (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	key_prefix TEXT NOT NULL,
	key_hash TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL,
	last_used_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);
CREATE TABLE IF NOT EXISTS domains (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	domain TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS link_variants (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	link_id INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE,
	url TEXT NOT NULL,
	weight INTEGER NOT NULL DEFAULT 1,
	clicks INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_link_variants_link ON link_variants(link_id);
CREATE TABLE IF NOT EXISTS webhooks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	url TEXT NOT NULL,
	events TEXT NOT NULL,
	secret TEXT NOT NULL,
	active INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS webhook_deliveries (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	webhook_id INTEGER NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
	event TEXT NOT NULL,
	payload TEXT NOT NULL,
	status_code INTEGER,
	success INTEGER NOT NULL DEFAULT 0,
	error TEXT,
	attempted_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_hook ON webhook_deliveries(webhook_id);
ALTER TABLE links ADD COLUMN password_hash TEXT;
ALTER TABLE links ADD COLUMN team_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE links ADD COLUMN domain_id INTEGER REFERENCES domains(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_links_team ON links(team_id);
CREATE INDEX IF NOT EXISTS idx_links_domain ON links(domain_id);
INSERT INTO teams (name, created_at) SELECT 'Default', strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE NOT EXISTS (SELECT 1 FROM teams);
PRAGMA user_version = 6;
`
		if _, err := tx.Exec(schema); err != nil {
			return fmt.Errorf("apply schema v6: %w", err)
		}
	}
	if version < 7 {
		const schema = `
ALTER TABLE api_keys ADD COLUMN is_admin INTEGER NOT NULL DEFAULT 0;
ALTER TABLE domains ADD COLUMN team_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE webhooks ADD COLUMN team_id INTEGER NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS idx_domains_team ON domains(team_id);
CREATE INDEX IF NOT EXISTS idx_webhooks_team ON webhooks(team_id);
PRAGMA user_version = 7;
`
		if _, err := tx.Exec(schema); err != nil {
			return fmt.Errorf("apply schema v7: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}
