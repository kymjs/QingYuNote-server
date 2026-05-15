package store

import (
	"context"
	"database/sql"
)

// AdminUserRow 管理后台用户列表行。
type AdminUserRow struct {
	ID               int64
	DisplayName      sql.NullString
	AvatarURL        sql.NullString
	Phone            sql.NullString
	ExpiresAt        sql.NullTime
	IsLifetime       bool
	TotalRechargeFen int64
}

// ListAdminUsers 返回全部注册用户及订阅、累计充值（已支付订单金额之和，单位分）。
func (s *Store) ListAdminUsers(ctx context.Context) ([]AdminUserRow, error) {
	q := `
SELECT u.id, u.display_name, u.avatar_url, u.phone,
       s.expires_at, COALESCE(s.is_lifetime, 0),
       COALESCE(paid.total_fen, 0)
FROM users u
LEFT JOIN subscriptions s ON s.user_id = u.id
LEFT JOIN (
  SELECT user_id, SUM(amount_total) AS total_fen
  FROM orders
  WHERE status = 'paid'
  GROUP BY user_id
) paid ON paid.user_id = u.id
ORDER BY u.id DESC`
	rows, err := s.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AdminUserRow
	for rows.Next() {
		var r AdminUserRow
		var life int
		if err := rows.Scan(
			&r.ID, &r.DisplayName, &r.AvatarURL, &r.Phone,
			&r.ExpiresAt, &life, &r.TotalRechargeFen,
		); err != nil {
			return nil, err
		}
		r.IsLifetime = life != 0
		out = append(out, r)
	}
	return out, rows.Err()
}
