package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"

	"composepilot/internal/models"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
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
);`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate db: %w", err)
	}
	if err := s.ensureColumn("services", "managed_files_json", "ALTER TABLE services ADD COLUMN managed_files_json TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureColumn(table, column, alter string) error {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return fmt.Errorf("pragma table_info(%s): %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid       int
			name      string
			typ       string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("scan table_info(%s): %w", table, err)
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate table_info(%s): %w", table, err)
	}
	if _, err := s.db.Exec(alter); err != nil {
		return fmt.Errorf("alter table %s add %s: %w", table, column, err)
	}
	return nil
}

func (s *Store) CreateService(ctx context.Context, svc models.Service) (models.Service, error) {
	now := time.Now().UTC()
	composeJSON, err := json.Marshal(svc.ComposeFiles)
	if err != nil {
		return models.Service{}, fmt.Errorf("marshal compose files: %w", err)
	}
	envJSON, err := json.Marshal(svc.Environment)
	if err != nil {
		return models.Service{}, fmt.Errorf("marshal environment: %w", err)
	}
	managedFilesJSON, err := json.Marshal(svc.ManagedFiles)
	if err != nil {
		return models.Service{}, fmt.Errorf("marshal managed files: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO services (name, repo_url, branch, work_dir, compose_files_json, env_json, managed_files_json, ssh_key_encrypted, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		svc.Name, svc.RepoURL, svc.Branch, svc.WorkDir, string(composeJSON), string(envJSON), string(managedFilesJSON), svc.EncryptedSSHKey, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return models.Service{}, fmt.Errorf("insert service: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return models.Service{}, fmt.Errorf("get inserted id: %w", err)
	}
	svc.ID = id
	svc.CreatedAt = now
	svc.UpdatedAt = now
	return svc, nil
}

func (s *Store) UpdateService(ctx context.Context, svc models.Service) (models.Service, error) {
	composeJSON, err := json.Marshal(svc.ComposeFiles)
	if err != nil {
		return models.Service{}, fmt.Errorf("marshal compose files: %w", err)
	}
	envJSON, err := json.Marshal(svc.Environment)
	if err != nil {
		return models.Service{}, fmt.Errorf("marshal environment: %w", err)
	}
	managedFilesJSON, err := json.Marshal(svc.ManagedFiles)
	if err != nil {
		return models.Service{}, fmt.Errorf("marshal managed files: %w", err)
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
UPDATE services
SET name = ?, repo_url = ?, branch = ?, work_dir = ?, compose_files_json = ?, env_json = ?, managed_files_json = ?, ssh_key_encrypted = ?, updated_at = ?
WHERE id = ?`,
		svc.Name, svc.RepoURL, svc.Branch, svc.WorkDir, string(composeJSON), string(envJSON), string(managedFilesJSON), svc.EncryptedSSHKey, now.Format(time.RFC3339Nano), svc.ID)
	if err != nil {
		return models.Service{}, fmt.Errorf("update service: %w", err)
	}
	svc.UpdatedAt = now
	return svc, nil
}

func (s *Store) DeleteService(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM services WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	return nil
}

func (s *Store) ListServices(ctx context.Context) ([]models.Service, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, repo_url, branch, work_dir, compose_files_json, env_json, managed_files_json, ssh_key_encrypted, created_at, updated_at
FROM services ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("query services: %w", err)
	}
	defer rows.Close()

	var services []models.Service
	for rows.Next() {
		svc, err := scanService(rows)
		if err != nil {
			return nil, err
		}
		services = append(services, svc)
	}
	return services, rows.Err()
}

func (s *Store) GetService(ctx context.Context, id int64) (models.Service, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, repo_url, branch, work_dir, compose_files_json, env_json, managed_files_json, ssh_key_encrypted, created_at, updated_at
FROM services WHERE id = ?`, id)
	svc, err := scanService(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return models.Service{}, os.ErrNotExist
		}
		return models.Service{}, err
	}
	return svc, nil
}

func scanService(scanner interface{ Scan(dest ...any) error }) (models.Service, error) {
	var svc models.Service
	var composeJSON string
	var envJSON string
	var managedFilesJSON string
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(&svc.ID, &svc.Name, &svc.RepoURL, &svc.Branch, &svc.WorkDir, &composeJSON, &envJSON, &managedFilesJSON, &svc.EncryptedSSHKey, &createdAt, &updatedAt); err != nil {
		return models.Service{}, err
	}
	if err := json.Unmarshal([]byte(composeJSON), &svc.ComposeFiles); err != nil {
		return models.Service{}, fmt.Errorf("unmarshal compose files: %w", err)
	}
	if err := json.Unmarshal([]byte(envJSON), &svc.Environment); err != nil {
		return models.Service{}, fmt.Errorf("unmarshal environment: %w", err)
	}
	if err := json.Unmarshal([]byte(managedFilesJSON), &svc.ManagedFiles); err != nil {
		return models.Service{}, fmt.Errorf("unmarshal managed files: %w", err)
	}
	var err error
	svc.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return models.Service{}, fmt.Errorf("parse created_at: %w", err)
	}
	svc.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return models.Service{}, fmt.Errorf("parse updated_at: %w", err)
	}
	if svc.Environment == nil {
		svc.Environment = map[string]string{}
	}
	if svc.ManagedFiles == nil {
		svc.ManagedFiles = []models.ManagedFile{}
	}
	return svc, nil
}
