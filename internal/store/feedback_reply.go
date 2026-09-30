package store

import (
	"context"
	"database/sql"
	"time"
)

// FeedbackReplyRow 官方反馈回复一行。
type FeedbackReplyRow struct {
	ID              int64
	UserID          int64
	FeedbackContent string
	FeedbackType    string
	OfficialReply   string
	CreatedAt       time.Time
}

// InsertFeedbackReply 写入一条官方回复，返回自增 id。
func (s *Store) InsertFeedbackReply(
	ctx context.Context,
	userID int64,
	feedbackContent, feedbackType, officialReply string,
	now time.Time,
) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `
INSERT INTO feedback_replies
  (user_id, feedback_content, feedback_type, official_reply, created_at)
VALUES (?, ?, ?, ?, ?)`,
		userID, feedbackContent, feedbackType, officialReply, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetLatestFeedbackReplyByUserID 取该用户 id 最大的一条回复；无数据返回 sql.ErrNoRows。
func (s *Store) GetLatestFeedbackReplyByUserID(ctx context.Context, userID int64) (*FeedbackReplyRow, error) {
	var r FeedbackReplyRow
	err := s.DB.QueryRowContext(ctx, `
SELECT id, user_id, feedback_content, feedback_type, official_reply, created_at
FROM feedback_replies
WHERE user_id = ?
ORDER BY id DESC
LIMIT 1`, userID).Scan(
		&r.ID, &r.UserID, &r.FeedbackContent, &r.FeedbackType, &r.OfficialReply, &r.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ErrNoFeedbackReply 便于调用方判断「无回复」（与 sql.ErrNoRows 等价时可直接用 errors.Is）。
var ErrNoFeedbackReply = sql.ErrNoRows
