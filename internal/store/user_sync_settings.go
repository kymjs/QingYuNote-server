package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// UserSyncSettingsRow 用户云同步设置（管理后台展示用，密码等为解密后明文）。
type UserSyncSettingsRow struct {
	UserID       int64
	SyncProvider string
	Enabled      bool
	BaseURL      sql.NullString
	Username     sql.NullString
	Password     sql.NullString
	NotesDir     sql.NullString
	UpdatedAt    time.Time
}

// UpsertUserSyncSettings 写入或更新用户云同步设置。
func (s *Store) UpsertUserSyncSettings(ctx context.Context, row UserSyncSettingsRow) error {
	now := time.Now().UTC()
	_, err := s.DB.ExecContext(ctx, `
INSERT INTO user_sync_settings (
  user_id, sync_provider, enabled, base_url, username, password, notes_dir, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  sync_provider = VALUES(sync_provider),
  enabled = VALUES(enabled),
  base_url = VALUES(base_url),
  username = VALUES(username),
  password = VALUES(password),
  notes_dir = VALUES(notes_dir),
  updated_at = VALUES(updated_at)`,
		row.UserID,
		strings.TrimSpace(row.SyncProvider),
		boolToTiny(row.Enabled),
		nullStringOrNil(row.BaseURL),
		nullStringOrNil(row.Username),
		nullStringOrNil(row.Password),
		nullStringOrNil(row.NotesDir),
		now,
	)
	return err
}

func boolToTiny(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullStringOrNil(v sql.NullString) any {
	if v.Valid {
		return strings.TrimSpace(v.String)
	}
	return nil
}

// ListUserSyncSettingsByUserIDs 批量读取用户云同步设置。
func (s *Store) ListUserSyncSettingsByUserIDs(ctx context.Context, userIDs []int64) (map[int64]UserSyncSettingsRow, error) {
	uniq := make([]int64, 0, len(userIDs))
	seen := map[int64]struct{}{}
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	out := make(map[int64]UserSyncSettingsRow)
	if len(uniq) == 0 {
		return out, nil
	}
	ph := make([]string, len(uniq))
	args := make([]any, len(uniq))
	for i, id := range uniq {
		ph[i] = "?"
		args[i] = id
	}
	q := `SELECT user_id, sync_provider, enabled, base_url, username, password, notes_dir, updated_at
FROM user_sync_settings
WHERE user_id IN (` + strings.Join(ph, ",") + `)`
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r UserSyncSettingsRow
		var enabled int
		if err := rows.Scan(
			&r.UserID, &r.SyncProvider, &enabled,
			&r.BaseURL, &r.Username, &r.Password, &r.NotesDir, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		r.Enabled = enabled != 0
		out[r.UserID] = r
	}
	return out, rows.Err()
}
