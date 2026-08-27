package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	GrantSourceWelcome        = "welcome"
	GrantSourceInvited        = "invited"
	GrantSourceInviteFriend   = "invite_friend"
	GrantSourceInviteRecharge = "invite_recharge"
	GrantSourceAppStoreReview = "app_store_review"
)

// MembershipGrantRecordParams 会籍赠送审计行。
type MembershipGrantRecordParams struct {
	UserID        int64
	Source        string
	GrantDays     int
	GrantMonths   int
	RelatedUserID int64
	OrderID       int64
}

func normalizeGrantSource(src string) (string, error) {
	s := strings.TrimSpace(strings.ToLower(src))
	switch s {
	case GrantSourceWelcome, GrantSourceInvited, GrantSourceInviteFriend, GrantSourceInviteRecharge, GrantSourceAppStoreReview:
		return s, nil
	default:
		return "", fmt.Errorf("invalid grant source")
	}
}

// InsertMembershipGrantRecord 在会籍赠送成功后写入审计。
func (s *Store) InsertMembershipGrantRecord(ctx context.Context, p *MembershipGrantRecordParams) error {
	if p == nil {
		return fmt.Errorf("nil params")
	}
	src, err := normalizeGrantSource(p.Source)
	if err != nil {
		return err
	}
	if p.UserID <= 0 {
		return fmt.Errorf("invalid user_id")
	}
	if p.GrantDays <= 0 && p.GrantMonths <= 0 {
		return fmt.Errorf("invalid grant duration")
	}
	now := time.Now().UTC()
	var related any
	if p.RelatedUserID > 0 {
		related = p.RelatedUserID
	}
	var orderID any
	if p.OrderID > 0 {
		orderID = p.OrderID
	}
	var days any
	if p.GrantDays > 0 {
		days = p.GrantDays
	}
	var months any
	if p.GrantMonths > 0 {
		months = p.GrantMonths
	}
	q := `INSERT INTO membership_grant_records
		(user_id, source, grant_days, grant_months, related_user_id, order_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err = s.DB.ExecContext(ctx, q, p.UserID, src, days, months, related, orderID, now)
	return err
}

// AdminMembershipGrantRow 管理后台展示的会籍赠送记录。
type AdminMembershipGrantRow struct {
	Source      string
	GrantDays   int
	GrantMonths int
	CreatedAt   time.Time
}

// ListAdminMembershipGrantRecords 按用户查询会籍赠送审计。
func (s *Store) ListAdminMembershipGrantRecords(ctx context.Context, userIDs []int64) (map[int64][]AdminMembershipGrantRow, error) {
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
	out := make(map[int64][]AdminMembershipGrantRow)
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
SELECT user_id, source, COALESCE(grant_days, 0), COALESCE(grant_months, 0), created_at
FROM membership_grant_records
WHERE user_id IN (%s)
ORDER BY user_id ASC, created_at DESC`, strings.Join(ph, ","))
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var uid int64
		var r AdminMembershipGrantRow
		if err := rows.Scan(&uid, &r.Source, &r.GrantDays, &r.GrantMonths, &r.CreatedAt); err != nil {
			return nil, err
		}
		out[uid] = append(out[uid], r)
	}
	return out, rows.Err()
}

// FormatMembershipGrantLabel 生成管理后台展示文案。
func FormatMembershipGrantLabel(source string, grantDays, grantMonths int) string {
	switch source {
	case GrantSourceWelcome:
		if grantDays > 0 {
			return fmt.Sprintf("首次安装赠送%d天", grantDays)
		}
		return "首次安装赠送"
	case GrantSourceInvited:
		if grantMonths > 0 {
			return fmt.Sprintf("被好友邀请赠送%d个月", grantMonths)
		}
		if grantDays > 0 {
			return fmt.Sprintf("被好友邀请赠送%d天", grantDays)
		}
		return "被好友邀请赠送"
	case GrantSourceInviteFriend:
		if grantDays > 0 {
			return fmt.Sprintf("邀请好友赠送%d天", grantDays)
		}
		return "邀请好友赠送"
	case GrantSourceInviteRecharge:
		if grantDays > 0 {
			return fmt.Sprintf("邀请好友充值赠送%d天", grantDays)
		}
		return "邀请好友充值赠送"
	case GrantSourceAppStoreReview:
		if grantMonths > 0 {
			return fmt.Sprintf("应用市场评价赠送%d个月", grantMonths)
		}
		if grantDays > 0 {
			return fmt.Sprintf("应用市场评价赠送%d天", grantDays)
		}
		return "应用市场评价赠送"
	default:
		return ""
	}
}
