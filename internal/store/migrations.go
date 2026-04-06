package store

import "database/sql"

func init() {
	migrations = []migration{
		{version: 1, description: "create services table", fn: migrateV1},
		{version: 2, description: "create notification and monitor tables", fn: migrateV2},
		{version: 3, description: "add confirm_threshold to monitor_settings", fn: migrateV3},
	}
}

func migrateV3(tx *sql.Tx) error {
	return ensureColumnTx(tx, "monitor_settings", "confirm_threshold",
		"ALTER TABLE monitor_settings ADD COLUMN confirm_threshold INTEGER NOT NULL DEFAULT 2")
}

func migrateV2(tx *sql.Tx) error {
	const targets = `
CREATE TABLE IF NOT EXISTS notification_targets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    webhook_url_encrypted TEXT NOT NULL,
    template TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
)`
	if _, err := tx.Exec(targets); err != nil {
		return err
	}
	const settings = `
CREATE TABLE IF NOT EXISTS monitor_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    enabled INTEGER NOT NULL DEFAULT 0,
    interval_seconds INTEGER NOT NULL DEFAULT 60,
    updated_at TEXT NOT NULL
)`
	if _, err := tx.Exec(settings); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO monitor_settings (id, enabled, interval_seconds, updated_at) VALUES (1, 0, 60, datetime('now'))`); err != nil {
		return err
	}
	return nil
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
