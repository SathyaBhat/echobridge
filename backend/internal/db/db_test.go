package db

import (
	"testing"
)

// newTestDB opens an in-memory SQLite database and runs migrations.
// The DB is automatically closed when the test finishes.
func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := New(":memory:")
	if err != nil {
		t.Fatalf("newTestDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigrate_CreatesAllTables(t *testing.T) {
	db := newTestDB(t)

	tables := []string{"accounts", "mastodon_apps", "media"}
	for _, table := range tables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found after migration: %v", table, err)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	db := newTestDB(t)
	// Running migrate() again on an existing schema must not error.
	if err := db.migrate(); err != nil {
		t.Errorf("second migrate() call returned error: %v", err)
	}
}
