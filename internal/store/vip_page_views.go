package store

import (
	"context"
	"time"
)

// VipPageViewRow 充值页埋点聚合行。
type VipPageViewRow struct {
	UserID       int64
	ViewCount    int64
	LastViewedAt time.Time
}

// UpsertVipPageView 同一用户多次打开：次数 +1，刷新最后查看时间。
func (s *Store) UpsertVipPageView(ctx context.Context, userID int64, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `
INSERT INTO vip_page_views (user_id, view_count, last_viewed_at)
VALUES (?, 1, ?)
ON DUPLICATE KEY UPDATE
  view_count = view_count + 1,
  last_viewed_at = VALUES(last_viewed_at)`,
		userID, now)
	return err
}

// ListVipPageViews 按最后查看时间倒序返回全部聚合行。
func (s *Store) ListVipPageViews(ctx context.Context) ([]VipPageViewRow, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT user_id, view_count, last_viewed_at
FROM vip_page_views
ORDER BY last_viewed_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VipPageViewRow
	for rows.Next() {
		var r VipPageViewRow
		if err := rows.Scan(&r.UserID, &r.ViewCount, &r.LastViewedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
