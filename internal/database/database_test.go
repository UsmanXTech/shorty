package database

import (
	"path/filepath"
	"testing"
)

func TestOpenMigratesSchema(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "shorty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var table string
	if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'links'").Scan(&table); err != nil {
		t.Fatal(err)
	}
	if table != "links" {
		t.Fatalf("expected links table, got %q", table)
	}

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Fatalf("expected schema version %d, got %d", schemaVersion, version)
	}
}
