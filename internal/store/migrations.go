package store

import "database/sql"

func init() {
	migrations = []migration{
		{version: 1, description: "create services table", fn: migrateV1},
	}
}

func migrateV1(tx *sql.Tx) error {
	const schema = `
CREATE TABLE IF NOT EXISTS services (
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
)`
	if _, err := tx.Exec(schema); err != nil {
		return err
	}
	return ensureColumnTx(tx, "services", "managed_files_json",
		"ALTER TABLE services ADD COLUMN managed_files_json TEXT NOT NULL DEFAULT '[]'")
}
