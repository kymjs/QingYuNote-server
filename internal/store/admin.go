package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kymjs/noteapi/internal/config"
)

// AdminUserRow 管理后台用户列表行。
type AdminUserRow struct {
	ID                  int64
	DisplayName         sql.NullString
	AvatarURL           sql.NullString
	Phone               sql.NullString
	CreatedAt           time.Time
	FirstIdentityProv   sql.NullString
	FirstIdentityAt     sql.NullTime
	ExpiresAt           sql.NullTime
	IsLifetime          bool
	TotalRechargeFen    int64
}

// ListAdminUsers 返回全部注册用户及订阅、累计充值（已支付订单金额之和，单位分）。
func (s *Store) ListAdminUsers(ctx context.Context) ([]AdminUserRow, error) {
	q := `
SELECT u.id, u.display_name, u.avatar_url, u.phone, u.created_at,
       (SELECT i.provider FROM user_identities i WHERE i.user_id = u.id ORDER BY i.created_at ASC LIMIT 1),
       (SELECT i.created_at FROM user_identities i WHERE i.user_id = u.id ORDER BY i.created_at ASC LIMIT 1),
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
			&r.ID, &r.DisplayName, &r.AvatarURL, &r.Phone, &r.CreatedAt,
			&r.FirstIdentityProv, &r.FirstIdentityAt,
			&r.ExpiresAt, &life, &r.TotalRechargeFen,
		); err != nil {
			return nil, err
		}
		r.IsLifetime = life != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// AdminDeviceSession 管理后台展示的用户设备使用信息。
type AdminDeviceSession struct {
	Platform     string
	LastActiveAt time.Time
}

// ListAdminUserDevices 按用户查询设备的最后活跃时间，用于管理后台「使用端口」列。
func (s *Store) ListAdminUserDevices(ctx context.Context, userIDs []int64) (map[int64][]AdminDeviceSession, error) {
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
	out := make(map[int64][]AdminDeviceSession)
	if len(uniq) == 0 {
		return out, nil
	}
	ph := make([]string, len(uniq))
	args := make([]any, len(uniq))
	for i, id := range uniq {
		ph[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`
SELECT user_id, platform, last_active_at
FROM user_device_sessions
WHERE user_id IN (%s)
ORDER BY user_id ASC, last_active_at DESC`, strings.Join(ph, ","))
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var uid int64
		var platform string
		var lastActive time.Time
		if err := rows.Scan(&uid, &platform, &lastActive); err != nil {
			return nil, err
		}
		out[uid] = append(out[uid], AdminDeviceSession{
			Platform:     platform,
			LastActiveAt: lastActive,
		})
	}
	return out, rows.Err()
}

// AdminRechargeRecordRow 管理后台展示的会籍充值审计记录（新到旧排序由查询保证）。
type AdminRechargeRecordRow struct {
	Channel   string
	CreatedAt time.Time
	AmountFen int64
}

// ListAdminUserRechargeRecords 按用户查询 membership_recharge_records，用于管理后台「充值记录」列。
func (s *Store) ListAdminUserRechargeRecords(ctx context.Context, userIDs []int64) (map[int64][]AdminRechargeRecordRow, error) {
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
	out := make(map[int64][]AdminRechargeRecordRow)
	if len(uniq) == 0 {
		return out, nil
	}
	ph := make([]string, len(uniq))
	args := make([]any, len(uniq))
	for i, id := range uniq {
		ph[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`
SELECT m.user_id, m.channel, m.created_at, m.plan_id, o.amount_total
FROM membership_recharge_records m
LEFT JOIN orders o ON o.id = m.order_id
WHERE m.user_id IN (%s)
ORDER BY m.user_id ASC, m.created_at DESC`, strings.Join(ph, ","))
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var uid int64
		var ch string
		var created time.Time
		var planID string
		var orderAmt sql.NullInt64
		if err := rows.Scan(&uid, &ch, &created, &planID, &orderAmt); err != nil {
			return nil, err
		}
		var fen int64
		if orderAmt.Valid {
			fen = orderAmt.Int64
		} else {
			fen = int64(config.PlanAmountFen(planID))
		}
		out[uid] = append(out[uid], AdminRechargeRecordRow{
			Channel:   ch,
			CreatedAt: created,
			AmountFen: fen,
		})
	}
	return out, rows.Err()
}
