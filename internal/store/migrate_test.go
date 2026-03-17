package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrateFromScratch(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer st.Close()

	var version int
	if err := st.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version); err != nil {
		t.Fatalf("query version: %v", err)
	}
	if version != len(migrations) {
		t.Fatalf("expected version %d, got %d", len(migrations), version)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	st1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	st1.Close()

	st2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer st2.Close()

	var version int
	if err := st2.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version); err != nil {
		t.Fatalf("query version: %v", err)
	}
	if version != len(migrations) {
		t.Fatalf("expected version %d, got %d", len(migrations), version)
	}
}

func TestMigrateFromExistingDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Simulate a pre-migration database: create services table directly without schema_migrations
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE services (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		repo_url TEXT NOT NULL,
		branch TEXT NOT NULL,
		work_dir TEXT NOT NULL,
		compose_files_json TEXT NOT NULL,
		env_json TEXT NOT NULL,
		managed_files_json TEXT NOT NULL DEFAULT '[]',
		ssh_key_encrypted TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	db.Close()

	// Open via Store — migrate() should bootstrap schema_migrations and apply v1 idempotently
	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer st.Close()

	var version int
	if err := st.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version); err != nil {
		t.Fatalf("query version: %v", err)
	}
	if version != len(migrations) {
		t.Fatalf("expected version %d, got %d", len(migrations), version)
	}
}
