package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	AppStoreReviewReviewing = "reviewing"
	AppStoreReviewApproved  = "approved"
	AppStoreReviewRejected  = "rejected"
)

// AppStoreReviewCampaignRow 应用市场好评活动状态。
type AppStoreReviewCampaignRow struct {
	UserID             int64
	Status             string
	RejectReason       string
	ScreenshotFilename string
	GrantedAt          sql.NullTime
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (s *Store) GetAppStoreReviewCampaign(ctx context.Context, userID int64) (*AppStoreReviewCampaignRow, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("invalid user_id")
	}
	q := `SELECT user_id, status, COALESCE(reject_reason, ''), COALESCE(screenshot_filename, ''),
		granted_at, created_at, updated_at
		FROM app_store_review_campaign WHERE user_id = ?`
	var row AppStoreReviewCampaignRow
	err := s.DB.QueryRowContext(ctx, q, userID).Scan(
		&row.UserID, &row.Status, &row.RejectReason, &row.ScreenshotFilename,
		&row.GrantedAt, &row.CreatedAt, &row.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// BeginAppStoreReviewUpload 将状态置为 reviewing 并保存截图文件名。
// 已通过则拒绝；审核中拒绝重复提交；驳回后允许覆盖。
func (s *Store) BeginAppStoreReviewUpload(ctx context.Context, userID int64, filename string) error {
	if userID <= 0 {
		return fmt.Errorf("invalid user_id")
	}
	fn := strings.TrimSpace(filename)
	if fn == "" {
		return fmt.Errorf("empty filename")
	}
	now := time.Now().UTC()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM app_store_review_campaign WHERE user_id = ? FOR UPDATE`,
		userID,
	).Scan(&status)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		switch status {
		case AppStoreReviewApproved:
			return errAppStoreReviewAlreadyApproved
		case AppStoreReviewReviewing:
			return errAppStoreReviewAlreadyReviewing
		}
		_, err = tx.ExecContext(ctx, `
UPDATE app_store_review_campaign
SET status = ?, reject_reason = NULL, screenshot_filename = ?, granted_at = NULL, updated_at = ?
WHERE user_id = ?`, AppStoreReviewReviewing, fn, now, userID)
	} else {
		_, err = tx.ExecContext(ctx, `
INSERT INTO app_store_review_campaign
	(user_id, status, reject_reason, screenshot_filename, granted_at, created_at, updated_at)
VALUES (?, ?, NULL, ?, NULL, ?, ?)`, userID, AppStoreReviewReviewing, fn, now, now)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

var (
	errAppStoreReviewAlreadyApproved  = errors.New("app_store_review_already_approved")
	errAppStoreReviewAlreadyReviewing = errors.New("app_store_review_already_reviewing")
)

func IsAppStoreReviewAlreadyApproved(err error) bool {
	return errors.Is(err, errAppStoreReviewAlreadyApproved)
}

func IsAppStoreReviewAlreadyReviewing(err error) bool {
	return errors.Is(err, errAppStoreReviewAlreadyReviewing)
}

func (s *Store) MarkAppStoreReviewApproved(ctx context.Context, userID int64) error {
	now := time.Now().UTC()
	_, err := s.DB.ExecContext(ctx, `
UPDATE app_store_review_campaign
SET status = ?, reject_reason = NULL, granted_at = ?, updated_at = ?
WHERE user_id = ? AND status = ?`,
		AppStoreReviewApproved, now, now, userID, AppStoreReviewReviewing)
	return err
}

func (s *Store) MarkAppStoreReviewRejected(ctx context.Context, userID int64, reason string) error {
	now := time.Now().UTC()
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) > 200 {
		reason = string([]rune(reason)[:200])
	}
	_, err := s.DB.ExecContext(ctx, `
UPDATE app_store_review_campaign
SET status = ?, reject_reason = ?, updated_at = ?
WHERE user_id = ? AND status = ?`,
		AppStoreReviewRejected, reason, now, userID, AppStoreReviewReviewing)
	return err
}
