package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenRestrictsPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "shorty.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if fi, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if mode := fi.Mode().Perm(); mode != 0o600 {
		t.Fatalf("database file should be 0600, got %o", mode)
	}
	if fi, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	} else if mode := fi.Mode().Perm(); mode != 0o700 {
		t.Fatalf("database dir should be 0700, got %o", mode)
	}
}
