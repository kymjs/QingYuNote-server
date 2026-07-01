package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const ReferralClaimValidWindow = 24 * time.Hour

func shanghaiDayStartUTC(now time.Time) time.Time {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.UTC
	}
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc).UTC()
}

const (
	WelcomeBonusDays    = 7
	ReferralInviterDays = 7
	ReferralInviteeMonths = 1
)

// ReferralClaimRow 官网提交的待绑定邀请。
type ReferralClaimRow struct {
	ID             int64
	Token          string
	InviterUserID  int64
	ClaimedAt      time.Time
	UsedByUserID   sql.NullInt64
	UsedAt         sql.NullTime
	InviteePhone   sql.NullString
	Status         string
}

// UserReferralRow 已完成的邀请关系。
type UserReferralRow struct {
	ID            int64
	InviterUserID int64
	InviteeUserID int64
	CreatedAt     time.Time
}

// AdminReferralInviteeRow 管理后台邀请详情。
type AdminReferralInviteeRow struct {
	InviteeUserID    int64
	InviteePhone     sql.NullString
	InviteeNickname  sql.NullString
	RegisteredAt     time.Time
	TotalRechargeFen int64
	RechargeCount    int
}

// InvitePopupStatsRow 弹窗曝光/点击统计。
type InvitePopupStatsRow struct {
	Variant     int
	Impressions int64
	Clicks      int64
}

func (s *Store) GrantWelcomeBonusPending(ctx context.Context, userID int64, now time.Time) error {
	q := `UPDATE users SET welcome_bonus_granted_at = ?, updated_at = ?
		WHERE id = ? AND welcome_bonus_granted_at IS NULL`
	res, err := s.DB.ExecContext(ctx, q, now, now, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil
	}
	return nil
}

// UserHasInvitePopupImpression 用户是否已有邀请弹窗曝光记录（服务端唯一「已展示」依据）。
func (s *Store) UserHasInvitePopupImpression(ctx context.Context, userID int64) (bool, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx,
		`SELECT id FROM invite_popup_events
		 WHERE user_id = ? AND event_type = 'impression' LIMIT 1`,
		userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// UserHasInvitePopupImpressionSince 用户在 since 之后是否已有邀请弹窗曝光记录。
func (s *Store) UserHasInvitePopupImpressionSince(ctx context.Context, userID int64, since time.Time) (bool, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx,
		`SELECT id FROM invite_popup_events
		 WHERE user_id = ? AND event_type = 'impression' AND created_at >= ? LIMIT 1`,
		userID, since).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// ShouldShowInvitePopup 是否应向该用户展示邀请弹窗：
// 轻羽云会员 — 历史上从未曝光过；非会员 — 当日（上海时区）未曝光过。
func (s *Store) ShouldShowInvitePopup(ctx context.Context, userID int64, isQingyuMember bool) (bool, error) {
	if isQingyuMember {
		has, err := s.UserHasInvitePopupImpression(ctx, userID)
		if err != nil {
			return false, err
		}
		return !has, nil
	}
	todayStart := shanghaiDayStartUTC(time.Now().UTC())
	has, err := s.UserHasInvitePopupImpressionSince(ctx, userID, todayStart)
	if err != nil {
		return false, err
	}
	return !has, nil
}

func (s *Store) GetReferralClaimByToken(ctx context.Context, token string) (*ReferralClaimRow, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, sql.ErrNoRows
	}
	var r ReferralClaimRow
	q := `SELECT id, token, inviter_user_id, claimed_at, used_by_user_id, used_at
		FROM referral_claims WHERE token = ? LIMIT 1`
	err := s.DB.QueryRowContext(ctx, q, token).Scan(
		&r.ID, &r.Token, &r.InviterUserID, &r.ClaimedAt, &r.UsedByUserID, &r.UsedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) InsertReferralClaim(ctx context.Context, token string, inviterUserID int64, claimedAt time.Time) error {
	q := `INSERT INTO referral_claims (token, inviter_user_id, claimed_at) VALUES (?, ?, ?)`
	_, err := s.DB.ExecContext(ctx, q, token, inviterUserID, claimedAt)
	return err
}

// GetLatestUnusedReferralClaimForInviter 取邀请人最近一条未使用且在 since 之后创建的领取记录。
func (s *Store) GetLatestUnusedReferralClaimForInviter(ctx context.Context, inviterUserID int64, since time.Time) (*ReferralClaimRow, error) {
	var r ReferralClaimRow
	q := `SELECT id, token, inviter_user_id, claimed_at, used_by_user_id, used_at
		FROM referral_claims
		WHERE inviter_user_id = ? AND used_at IS NULL AND claimed_at >= ?
		ORDER BY claimed_at DESC LIMIT 1`
	err := s.DB.QueryRowContext(ctx, q, inviterUserID, since).Scan(
		&r.ID, &r.Token, &r.InviterUserID, &r.ClaimedAt, &r.UsedByUserID, &r.UsedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) MarkReferralClaimUsed(ctx context.Context, claimID, inviteeUserID int64, usedAt time.Time) error {
	q := `UPDATE referral_claims SET used_by_user_id = ?, used_at = ? WHERE id = ? AND used_at IS NULL`
	_, err := s.DB.ExecContext(ctx, q, inviteeUserID, usedAt, claimID)
	return err
}

func (s *Store) InsertUserReferral(ctx context.Context, inviterUserID, inviteeUserID int64, createdAt time.Time) error {
	if inviterUserID <= 0 || inviteeUserID <= 0 || inviterUserID == inviteeUserID {
		return fmt.Errorf("invalid referral users")
	}
	q := `INSERT INTO user_referrals (inviter_user_id, invitee_user_id, created_at) VALUES (?, ?, ?)`
	_, err := s.DB.ExecContext(ctx, q, inviterUserID, inviteeUserID, createdAt)
	return err
}

func (s *Store) GetInviterUserIDForInvitee(ctx context.Context, inviteeUserID int64) (int64, error) {
	var inviterID int64
	err := s.DB.QueryRowContext(ctx,
		`SELECT inviter_user_id FROM user_referrals WHERE invitee_user_id = ? LIMIT 1`,
		inviteeUserID).Scan(&inviterID)
	return inviterID, err
}

func (s *Store) CountReferralsByInviter(ctx context.Context, inviterUserID int64) (int64, error) {
	var n int64
	err := s.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_referrals WHERE inviter_user_id = ?`, inviterUserID).Scan(&n)
	return n, err
}

func (s *Store) CountReferralsByInviters(ctx context.Context, inviterIDs []int64) (map[int64]int64, error) {
	out := make(map[int64]int64)
	if len(inviterIDs) == 0 {
		return out, nil
	}
	placeholders := strings.Repeat("?,", len(inviterIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(inviterIDs))
	for i, id := range inviterIDs {
		args[i] = id
	}
	q := fmt.Sprintf(`SELECT inviter_user_id, COUNT(*) FROM user_referrals
		WHERE inviter_user_id IN (%s) GROUP BY inviter_user_id`, placeholders)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var uid, cnt int64
		if err := rows.Scan(&uid, &cnt); err != nil {
			return nil, err
		}
		out[uid] = cnt
	}
	return out, rows.Err()
}

func (s *Store) ListAdminReferralInvitees(ctx context.Context, inviterUserID int64) ([]AdminReferralInviteeRow, error) {
	q := `
SELECT u.id, u.phone, u.display_name, u.created_at,
       COALESCE(paid.total_fen, 0), COALESCE(paid.cnt, 0)
FROM user_referrals r
JOIN users u ON u.id = r.invitee_user_id
LEFT JOIN (
  SELECT user_id, SUM(amount_total) AS total_fen, COUNT(*) AS cnt
  FROM orders WHERE status = 'paid' GROUP BY user_id
) paid ON paid.user_id = u.id
WHERE r.inviter_user_id = ?
ORDER BY r.created_at DESC`
	rows, err := s.DB.QueryContext(ctx, q, inviterUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminReferralInviteeRow
	for rows.Next() {
		var r AdminReferralInviteeRow
		if err := rows.Scan(
			&r.InviteeUserID, &r.InviteePhone, &r.InviteeNickname, &r.RegisteredAt,
			&r.TotalRechargeFen, &r.RechargeCount,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) InsertReferralRechargeReward(ctx context.Context, inviterID, inviteeID, orderID int64, planID string, rewardDays int, createdAt time.Time) error {
	q := `INSERT INTO referral_recharge_rewards
		(inviter_user_id, invitee_user_id, order_id, plan_id, reward_days, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`
	_, err := s.DB.ExecContext(ctx, q, inviterID, inviteeID, orderID, planID, rewardDays, createdAt)
	return err
}

func (s *Store) ReferralRechargeRewardExists(ctx context.Context, orderID int64) (bool, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx,
		`SELECT id FROM referral_recharge_rewards WHERE order_id = ? LIMIT 1`, orderID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) InsertInvitePopupEvent(ctx context.Context, variant int, eventType string, userID int64, createdAt time.Time) error {
	q := `INSERT INTO invite_popup_events (variant, event_type, user_id, created_at) VALUES (?, ?, ?, ?)`
	var uid any
	if userID > 0 {
		uid = userID
	}
	_, err := s.DB.ExecContext(ctx, q, variant, eventType, uid, createdAt)
	return err
}

func (s *Store) InvitePopupStatsSince(ctx context.Context, since time.Time) ([]InvitePopupStatsRow, error) {
	q := `
SELECT variant,
       SUM(CASE WHEN event_type = 'impression' THEN 1 ELSE 0 END),
       SUM(CASE WHEN event_type = 'click' THEN 1 ELSE 0 END)
FROM invite_popup_events
WHERE created_at >= ?
GROUP BY variant`
	rows, err := s.DB.QueryContext(ctx, q, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InvitePopupStatsRow
	for rows.Next() {
		var r InvitePopupStatsRow
		if err := rows.Scan(&r.Variant, &r.Impressions, &r.Clicks); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) InvitePopupStatsAll(ctx context.Context) ([]InvitePopupStatsRow, error) {
	q := `
SELECT variant,
       SUM(CASE WHEN event_type = 'impression' THEN 1 ELSE 0 END),
       SUM(CASE WHEN event_type = 'click' THEN 1 ELSE 0 END)
FROM invite_popup_events
GROUP BY variant`
	rows, err := s.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InvitePopupStatsRow
	for rows.Next() {
		var r InvitePopupStatsRow
		if err := rows.Scan(&r.Variant, &r.Impressions, &r.Clicks); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
