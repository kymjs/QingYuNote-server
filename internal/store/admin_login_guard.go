package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const AdminLoginMaxConsecutiveFailures = 5

// IsAdminIPBanned 查询 IP 是否已被永久封禁。
func (s *Store) IsAdminIPBanned(ctx context.Context, ip string) (bool, error) {
	if ip == "" {
		return false, nil
	}
	var bannedAt sql.NullTime
	err := s.DB.QueryRowContext(ctx,
		`SELECT banned_at FROM admin_login_ip_guards WHERE ip = ? LIMIT 1`, ip,
	).Scan(&bannedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return bannedAt.Valid, nil
}

// RecordAdminLoginFailure 记录一次登录失败；连续失败达上限时永久封禁并返回 banned=true。
func (s *Store) RecordAdminLoginFailure(ctx context.Context, ip string) (banned bool, err error) {
	if ip == "" {
		return false, nil
	}
	now := time.Now().UTC()
	res, err := s.DB.ExecContext(ctx, `
INSERT INTO admin_login_ip_guards (ip, consecutive_failures, banned_at, updated_at)
VALUES (?, 1, NULL, ?)
ON DUPLICATE KEY UPDATE
  consecutive_failures = IF(banned_at IS NOT NULL, consecutive_failures, consecutive_failures + 1),
  banned_at = IF(
    banned_at IS NOT NULL,
    banned_at,
    IF(consecutive_failures + 1 >= ?, ?, NULL)
  ),
  updated_at = ?`,
		ip, now, AdminLoginMaxConsecutiveFailures, now, now,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, nil
	}
	isBanned, err := s.IsAdminIPBanned(ctx, ip)
	return isBanned, err
}

// ResetAdminLoginFailures 登录成功后清零连续失败计数（已封禁 IP 不受影响）。
func (s *Store) ResetAdminLoginFailures(ctx context.Context, ip string) error {
	if ip == "" {
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `
UPDATE admin_login_ip_guards
SET consecutive_failures = 0, updated_at = UTC_TIMESTAMP(3)
WHERE ip = ? AND banned_at IS NULL`, ip)
	return err
}
