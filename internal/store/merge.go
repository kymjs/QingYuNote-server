package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MergeUserAbsorb 将 sourceUserID 对应账号并入 survivorUserID（保留当前登录用户 JWT 指向的一方）。
// 合并订阅（取终身优先，否则取较晚到期日）、订单归属、全部 user_identities，然后删除源用户行。
func (s *Store) MergeUserAbsorb(ctx context.Context, survivorUserID, sourceUserID int64) error {
	if survivorUserID == sourceUserID {
		return nil
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, id := range []int64{survivorUserID, sourceUserID} {
		var dummy int64
		e := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id = ? LIMIT 1`, id).Scan(&dummy)
		if errors.Is(e, sql.ErrNoRows) {
			return fmt.Errorf("user %d not found", id)
		}
		if e != nil {
			return e
		}
	}

	if err := mergeSubscriptionsTx(ctx, tx, survivorUserID, sourceUserID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE orders SET user_id = ? WHERE user_id = ?`, survivorUserID, sourceUserID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE user_identities SET user_id = ? WHERE user_id = ?`, survivorUserID, sourceUserID); err != nil {
		return err
	}

	if err := syncWechatOpenIDColumnTx(ctx, tx, survivorUserID); err != nil {
		return err
	}

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE users SET updated_at = ? WHERE id = ?`, now, survivorUserID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, sourceUserID); err != nil {
		return err
	}

	return tx.Commit()
}

func mergeSubscriptionsTx(ctx context.Context, tx *sql.Tx, survivorID, sourceID int64) error {
	a, errA := getSubscriptionTx(ctx, tx, survivorID)
	b, errB := getSubscriptionTx(ctx, tx, sourceID)
	hasA := errA == nil
	hasB := errB == nil
	if errA != nil && !errors.Is(errA, sql.ErrNoRows) {
		return errA
	}
	if errB != nil && !errors.Is(errB, sql.ErrNoRows) {
		return errB
	}

	if !hasA && !hasB {
		return nil
	}

	life := (hasA && a.IsLifetime) || (hasB && b.IsLifetime)
	now := time.Now().UTC()

	if _, err := tx.ExecContext(ctx, `DELETE FROM subscriptions WHERE user_id IN (?, ?)`, survivorID, sourceID); err != nil {
		return err
	}

	if life {
		q := `INSERT INTO subscriptions (user_id, expires_at, is_lifetime, updated_at) VALUES (?, NULL, 1, ?)`
		if _, err := tx.ExecContext(ctx, q, survivorID, now); err != nil {
			return err
		}
		return nil
	}

	maxDay, ok := pickLaterExpiry(hasA, a, hasB, b)
	if !ok {
		return nil
	}
	q := `INSERT INTO subscriptions (user_id, expires_at, is_lifetime, updated_at) VALUES (?, ?, 0, ?)`
	if _, err := tx.ExecContext(ctx, q, survivorID, maxDay, now); err != nil {
		return err
	}
	return nil
}

func pickLaterExpiry(hasA bool, a *SubscriptionRow, hasB bool, b *SubscriptionRow) (time.Time, bool) {
	var best time.Time
	found := false
	if hasA && a.ExpiresAt.Valid && !a.IsLifetime {
		best = dateUTC(a.ExpiresAt.Time)
		found = true
	}
	if hasB && b.ExpiresAt.Valid && !b.IsLifetime {
		t := dateUTC(b.ExpiresAt.Time)
		if !found || t.After(best) {
			best = t
			found = true
		}
	}
	return best, found
}

func dateUTC(t time.Time) time.Time {
	y, m, d := t.In(time.UTC).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func getSubscriptionTx(ctx context.Context, tx *sql.Tx, userID int64) (*SubscriptionRow, error) {
	var r SubscriptionRow
	q := `SELECT user_id, expires_at, is_lifetime FROM subscriptions WHERE user_id = ?`
	err := tx.QueryRowContext(ctx, q, userID).Scan(&r.UserID, &r.ExpiresAt, &r.IsLifetime)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func syncWechatOpenIDColumnTx(ctx context.Context, tx *sql.Tx, userID int64) error {
	var sub string
	err := tx.QueryRowContext(ctx,
		`SELECT subject FROM user_identities WHERE user_id = ? AND provider = ? LIMIT 1`,
		userID, ProviderWechat).Scan(&sub)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `UPDATE users SET wechat_openid = NULL WHERE id = ?`, userID)
		return err
	}
	if err != nil {
		return err
	}
	sub = strings.TrimSpace(sub)
	if sub == "" {
		_, err = tx.ExecContext(ctx, `UPDATE users SET wechat_openid = NULL WHERE id = ?`, userID)
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE users SET wechat_openid = ? WHERE id = ?`, sub, userID)
	return err
}
