package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"composepilot/internal/models"
)

func (s *Store) ListNotificationTargets(ctx context.Context) ([]models.NotificationTarget, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, type, name, webhook_url_encrypted, template, enabled, created_at, updated_at
FROM notification_targets ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("query notification targets: %w", err)
	}
	defer rows.Close()

	var targets []models.NotificationTarget
	for rows.Next() {
		t, err := scanNotificationTarget(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

func (s *Store) GetNotificationTarget(ctx context.Context, id int64) (models.NotificationTarget, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, type, name, webhook_url_encrypted, template, enabled, created_at, updated_at
FROM notification_targets WHERE id = ?`, id)
	t, err := scanNotificationTarget(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return models.NotificationTarget{}, os.ErrNotExist
		}
		return models.NotificationTarget{}, err
	}
	return t, nil
}

func (s *Store) CreateNotificationTarget(ctx context.Context, t models.NotificationTarget) (models.NotificationTarget, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
INSERT INTO notification_targets (type, name, webhook_url_encrypted, template, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.Type, t.Name, t.EncryptedWebhookURL, t.Template, boolToInt(t.Enabled),
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return models.NotificationTarget{}, fmt.Errorf("insert notification target: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return models.NotificationTarget{}, err
	}
	t.ID = id
	t.CreatedAt = now
	t.UpdatedAt = now
	return t, nil
}

func (s *Store) UpdateNotificationTarget(ctx context.Context, t models.NotificationTarget) (models.NotificationTarget, error) {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
UPDATE notification_targets
SET type = ?, name = ?, webhook_url_encrypted = ?, template = ?, enabled = ?, updated_at = ?
WHERE id = ?`,
		t.Type, t.Name, t.EncryptedWebhookURL, t.Template, boolToInt(t.Enabled),
		now.Format(time.RFC3339Nano), t.ID)
	if err != nil {
		return models.NotificationTarget{}, fmt.Errorf("update notification target: %w", err)
	}
	t.UpdatedAt = now
	return t, nil
}

func (s *Store) DeleteNotificationTarget(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM notification_targets WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete notification target: %w", err)
	}
	return nil
}

func (s *Store) GetMonitorSettings(ctx context.Context) (models.MonitorSettings, error) {
	row := s.db.QueryRowContext(ctx, `SELECT enabled, interval_seconds, confirm_threshold FROM monitor_settings WHERE id = 1`)
	var enabled int
	var interval int
	var threshold int
	if err := row.Scan(&enabled, &interval, &threshold); err != nil {
		if err == sql.ErrNoRows {
			return models.MonitorSettings{Enabled: false, IntervalSeconds: 60, ConfirmThreshold: 2}, nil
		}
		return models.MonitorSettings{}, err
	}
	if threshold < 1 {
		threshold = 1
	}
	return models.MonitorSettings{Enabled: enabled != 0, IntervalSeconds: interval, ConfirmThreshold: threshold}, nil
}

func (s *Store) SaveMonitorSettings(ctx context.Context, settings models.MonitorSettings) error {
	if settings.IntervalSeconds <= 0 {
		settings.IntervalSeconds = 60
	}
	if settings.ConfirmThreshold < 1 {
		settings.ConfirmThreshold = 1
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO monitor_settings (id, enabled, interval_seconds, confirm_threshold, updated_at)
VALUES (1, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET enabled = excluded.enabled, interval_seconds = excluded.interval_seconds, confirm_threshold = excluded.confirm_threshold, updated_at = excluded.updated_at`,
		boolToInt(settings.Enabled), settings.IntervalSeconds, settings.ConfirmThreshold, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("save monitor settings: %w", err)
	}
	return nil
}

func scanNotificationTarget(scanner interface{ Scan(dest ...any) error }) (models.NotificationTarget, error) {
	var t models.NotificationTarget
	var enabled int
	var createdAt, updatedAt string
	if err := scanner.Scan(&t.ID, &t.Type, &t.Name, &t.EncryptedWebhookURL, &t.Template, &enabled, &createdAt, &updatedAt); err != nil {
		return models.NotificationTarget{}, err
	}
	t.Enabled = enabled != 0
	var err error
	t.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return models.NotificationTarget{}, fmt.Errorf("parse created_at: %w", err)
	}
	t.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return models.NotificationTarget{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
