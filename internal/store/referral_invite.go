package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const ReferralDailyInviteLimit = 5

const (
	ReferralClaimStatusPending = "pending"
	ReferralClaimStatusSuccess = "success"
	ReferralClaimStatusFailed  = "failed"
)

// ReferralInviteHistoryRow 邀请活动页展示的被邀请记录。
type ReferralInviteHistoryRow struct {
	MaskedPhone string
	Status      string
	ClaimedAt   time.Time
	CompletedAt sql.NullTime
}

// RecordPhoneRegistrationHistory 记录曾注册过的手机号（注销后仍保留，用于邀请校验）。
func (s *Store) RecordPhoneRegistrationHistory(ctx context.Context, phone string, at time.Time) error {
	digits := NormalizeLoginPhoneDigits(phone)
	if digits == "" {
		return nil
	}
	q := `INSERT IGNORE INTO registered_phone_history (phone, first_registered_at) VALUES (?, ?)`
	_, err := s.DB.ExecContext(ctx, q, digits, at)
	return err
}

// PhoneEverRegistered 手机号是否历史上曾注册过（含已注销账号）。
func (s *Store) PhoneEverRegistered(ctx context.Context, phone string) (bool, error) {
	digits := NormalizeLoginPhoneDigits(phone)
	if digits == "" {
		return false, nil
	}
	var n int
	err := s.DB.QueryRowContext(ctx,
		`SELECT 1 FROM registered_phone_history WHERE phone = ? LIMIT 1`, digits).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		_, err := s.GetUserByPhone(ctx, digits)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return err == nil, err
}

// RecordIdentityRegistrationHistory 记录曾注册过的第三方身份。
func (s *Store) RecordIdentityRegistrationHistory(ctx context.Context, provider, subject string, at time.Time) error {
	p := strings.TrimSpace(provider)
	sub := strings.TrimSpace(subject)
	if p == "" || sub == "" {
		return nil
	}
	q := `INSERT IGNORE INTO registered_identity_history (provider, subject, first_registered_at) VALUES (?, ?, ?)`
	_, err := s.DB.ExecContext(ctx, q, p, sub, at)
	return err
}

// IdentityEverRegistered 第三方身份是否历史上曾注册过。
func (s *Store) IdentityEverRegistered(ctx context.Context, provider, subject string) (bool, error) {
	p := strings.TrimSpace(provider)
	sub := strings.TrimSpace(subject)
	if p == "" || sub == "" {
		return false, nil
	}
	var n int
	err := s.DB.QueryRowContext(ctx,
		`SELECT 1 FROM registered_identity_history WHERE provider = ? AND subject = ? LIMIT 1`,
		p, sub).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		_, err := s.LookupUserIDByIdentity(ctx, p, sub)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return err == nil, err
}

// ArchiveUserRegistrationHistory 注销前将手机号与第三方身份写入历史表。
func (s *Store) ArchiveUserRegistrationHistory(ctx context.Context, userID int64) error {
	u, err := s.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if u.Phone.Valid && strings.TrimSpace(u.Phone.String) != "" {
		if err := s.RecordPhoneRegistrationHistory(ctx, u.Phone.String, now); err != nil {
			return err
		}
	}
	rows, err := s.DB.QueryContext(ctx,
		`SELECT provider, subject FROM user_identities WHERE user_id = ?`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var prov, sub string
		if err := rows.Scan(&prov, &sub); err != nil {
			return err
		}
		if err := s.RecordIdentityRegistrationHistory(ctx, prov, sub, now); err != nil {
			return err
		}
	}
	return rows.Err()
}

// UserHasActiveQingyuMembership 是否为有效轻羽云会员（含终身）。
func (s *Store) UserHasActiveQingyuMembership(ctx context.Context, userID int64) (bool, error) {
	sub, err := s.GetSubscription(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if sub.IsLifetime {
		return true, nil
	}
	if !sub.ExpiresAt.Valid {
		return false, nil
	}
	today := time.Now().UTC()
	exp := sub.ExpiresAt.Time
	y1, m1, d1 := today.Date()
	y2, m2, d2 := exp.Date()
	todayDay := time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC)
	expDay := time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC)
	return !expDay.Before(todayDay), nil
}

// CountReferralClaimsByInviterSince 统计邀请人在某时刻之后发起的邀请次数。
func (s *Store) CountReferralClaimsByInviterSince(ctx context.Context, inviterID int64, since time.Time) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM referral_claims WHERE inviter_user_id = ? AND claimed_at >= ?`,
		inviterID, since).Scan(&n)
	return n, err
}

// InviteePhoneAlreadyReferred 该手机号是否已有成功邀请或有效待处理邀请。
func (s *Store) InviteePhoneAlreadyReferred(ctx context.Context, inviteePhone string, now time.Time) (bool, error) {
	digits := NormalizeLoginPhoneDigits(inviteePhone)
	if digits == "" {
		return false, errors.New("invalid_phone")
	}
	var n int
	err := s.DB.QueryRowContext(ctx, `
SELECT COUNT(*) FROM referral_claims
WHERE invitee_phone = ? AND (
  status = ? OR (status = ? AND claimed_at >= ?)
)`, digits, ReferralClaimStatusSuccess, ReferralClaimStatusPending, now.Add(-ReferralClaimValidWindow)).Scan(&n)
	return n > 0, err
}

// MaskPhoneCN 13812345678 -> 138****5678
func MaskPhoneCN(digits string) string {
	d := NormalizeLoginPhoneDigits(digits)
	if len(d) != 11 {
		return "****"
	}
	return d[:3] + "****" + d[7:]
}

// InsertReferralClaimV2 创建邀请记录（含被邀请人手机号）。
func (s *Store) InsertReferralClaimV2(ctx context.Context, token string, inviterUserID int64, inviteePhone string, claimedAt time.Time, status string) error {
	digits := NormalizeLoginPhoneDigits(inviteePhone)
	if digits == "" {
		return fmt.Errorf("invalid invitee phone")
	}
	st := strings.TrimSpace(status)
	if st == "" {
		st = ReferralClaimStatusPending
	}
	q := `INSERT INTO referral_claims (token, inviter_user_id, invitee_phone, status, claimed_at) VALUES (?, ?, ?, ?, ?)`
	_, err := s.DB.ExecContext(ctx, q, token, inviterUserID, digits, st, claimedAt)
	return err
}

// ListReferralInviteHistory 邀请人的邀请历史（含成功/失败/进行中）。
func (s *Store) ListReferralInviteHistory(ctx context.Context, inviterUserID int64, now time.Time) ([]ReferralInviteHistoryRow, error) {
	q := `
SELECT invitee_phone, status, claimed_at, used_at
FROM referral_claims
WHERE inviter_user_id = ?
ORDER BY claimed_at DESC
LIMIT 100`
	rows, err := s.DB.QueryContext(ctx, q, inviterUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReferralInviteHistoryRow
	for rows.Next() {
		var phone string
		var status string
		var claimedAt time.Time
		var usedAt sql.NullTime
		if err := rows.Scan(&phone, &status, &claimedAt, &usedAt); err != nil {
			return nil, err
		}
		displayStatus := status
		if usedAt.Valid {
			displayStatus = ReferralClaimStatusSuccess
		} else if status == ReferralClaimStatusPending && now.Sub(claimedAt) > ReferralClaimValidWindow {
			displayStatus = ReferralClaimStatusFailed
		}
		out = append(out, ReferralInviteHistoryRow{
			MaskedPhone: MaskPhoneCN(phone),
			Status:      displayStatus,
			ClaimedAt:   claimedAt,
			CompletedAt: usedAt,
		})
	}
	return out, rows.Err()
}

// MarkReferralClaimSuccess 邀请成功。
func (s *Store) MarkReferralClaimSuccess(ctx context.Context, claimID, inviteeUserID int64, usedAt time.Time) error {
	q := `UPDATE referral_claims SET used_by_user_id = ?, used_at = ?, status = ? WHERE id = ? AND status = ?`
	_, err := s.DB.ExecContext(ctx, q, inviteeUserID, usedAt, ReferralClaimStatusSuccess, claimID, ReferralClaimStatusPending)
	return err
}

// MarkReferralClaimFailed 邀请失败（提交时或过期）。
func (s *Store) MarkReferralClaimFailed(ctx context.Context, claimID int64) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE referral_claims SET status = ? WHERE id = ? AND status = ?`,
		ReferralClaimStatusFailed, claimID, ReferralClaimStatusPending)
	return err
}

// GetPendingReferralClaimByInviteePhone 按被邀请人手机号查找有效待处理邀请（24h 内、未使用）。
func (s *Store) GetPendingReferralClaimByInviteePhone(ctx context.Context, inviteePhone string, now time.Time) (*ReferralClaimRow, error) {
	digits := NormalizeLoginPhoneDigits(inviteePhone)
	if digits == "" {
		return nil, sql.ErrNoRows
	}
	since := now.Add(-ReferralClaimValidWindow)
	var r ReferralClaimRow
	q := `SELECT id, token, inviter_user_id, claimed_at, used_by_user_id, used_at, invitee_phone, status
		FROM referral_claims
		WHERE invitee_phone = ? AND status = ? AND used_at IS NULL AND claimed_at >= ?
		ORDER BY claimed_at DESC LIMIT 1`
	var inviteePhoneCol sql.NullString
	var status string
	err := s.DB.QueryRowContext(ctx, q, digits, ReferralClaimStatusPending, since).Scan(
		&r.ID, &r.Token, &r.InviterUserID, &r.ClaimedAt, &r.UsedByUserID, &r.UsedAt, &inviteePhoneCol, &status,
	)
	if err != nil {
		return nil, err
	}
	r.InviteePhone = inviteePhoneCol
	r.Status = status
	return &r, nil
}

// GetReferralClaimByTokenForInvitee 按 token 读取邀请记录（含手机号）。
func (s *Store) GetReferralClaimByTokenForInvitee(ctx context.Context, token string) (*ReferralClaimRow, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, sql.ErrNoRows
	}
	var r ReferralClaimRow
	q := `SELECT id, token, inviter_user_id, claimed_at, used_by_user_id, used_at, invitee_phone, status
		FROM referral_claims WHERE token = ? LIMIT 1`
	var inviteePhone sql.NullString
	var status string
	err := s.DB.QueryRowContext(ctx, q, token).Scan(
		&r.ID, &r.Token, &r.InviterUserID, &r.ClaimedAt, &r.UsedByUserID, &r.UsedAt, &inviteePhone, &status,
	)
	if err != nil {
		return nil, err
	}
	r.InviteePhone = inviteePhone
	r.Status = status
	return &r, nil
}
