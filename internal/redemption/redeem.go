package redemption

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kymjs/noteapi/internal/store"
	"github.com/kymjs/noteapi/internal/subscription"
)

var ErrInvalid = errors.New("redemption_invalid")

func getSubscriptionTx(ctx context.Context, tx *sql.Tx, userID int64) (*store.SubscriptionRow, error) {
	var r store.SubscriptionRow
	q := `SELECT user_id, expires_at, is_lifetime FROM subscriptions WHERE user_id = ?`
	err := tx.QueryRowContext(ctx, q, userID).Scan(&r.UserID, &r.ExpiresAt, &r.IsLifetime)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func upsertSubscriptionTx(ctx context.Context, tx *sql.Tx, userID int64, expiresAt time.Time, lifetime bool) error {
	now := time.Now().UTC()
	q := `INSERT INTO subscriptions (user_id, expires_at, is_lifetime, updated_at)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE expires_at = VALUES(expires_at), is_lifetime = VALUES(is_lifetime), updated_at = VALUES(updated_at)`
	_, err := tx.ExecContext(ctx, q, userID, expiresAt, lifetime, now)
	return err
}

// RedeemAtomically 核销兑换码并写入订阅（与支付成功逻辑一致）。
func RedeemAtomically(st *store.Store, ctx context.Context, uid int64, codeHash string, nowUTC time.Time) (planID string, err error) {
	tx, err := st.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var plan string
	err = tx.QueryRowContext(ctx,
		`SELECT plan_id FROM redemption_codes WHERE code_hash = ? AND redeemed_at IS NULL FOR UPDATE`,
		codeHash,
	).Scan(&plan)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalid
	}
	if err != nil {
		return "", err
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE redemption_codes SET redeemed_at = ?, redeemed_by_user_id = ? WHERE code_hash = ? AND redeemed_at IS NULL`,
		nowUTC, uid, codeHash,
	)
	if err != nil {
		return "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", ErrInvalid
	}

	sub, err := getSubscriptionTx(ctx, tx, uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if errors.Is(err, sql.ErrNoRows) {
		sub = nil
	}

	newExp, lifetime := subscription.ApplyRedemptionPlan(sub, plan, nowUTC)
	if lifetime {
		exp := time.Date(subscription.LifetimeYearUTC, 12, 31, 0, 0, 0, 0, time.UTC)
		if err := upsertSubscriptionTx(ctx, tx, uid, exp, true); err != nil {
			return "", err
		}
	} else {
		if err := upsertSubscriptionTx(ctx, tx, uid, newExp, false); err != nil {
			return "", err
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return plan, nil
}
