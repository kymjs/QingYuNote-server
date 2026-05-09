package store

import (
	"context"
	"time"
)

// InsertRedemptionCode 写入一条未兑换记录（codeHash 为规范化明文之 SHA256 hex）。
func (s *Store) InsertRedemptionCode(ctx context.Context, planID, codeHash string) error {
	now := time.Now().UTC()
	q := `INSERT INTO redemption_codes (code_hash, plan_id, created_at) VALUES (?, ?, ?)`
	_, err := s.DB.ExecContext(ctx, q, codeHash, planID, now)
	return err
}
