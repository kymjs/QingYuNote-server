package store

import (
	"context"
	"fmt"
	"strings"
)

// PatchUserProfile 按非 nil 指针更新字段；指向空字符串时表示清空该列（SQL NULL）。
func (s *Store) PatchUserProfile(ctx context.Context, userID int64, displayName, avatarURL, phone, email *string) error {
	var sets []string
	var args []interface{}
	if displayName != nil {
		sets = append(sets, "display_name = ?")
		args = append(args, optionalTrimmedPtr(displayName))
	}
	if avatarURL != nil {
		sets = append(sets, "avatar_url = ?")
		args = append(args, optionalTrimmedPtr(avatarURL))
	}
	if phone != nil {
		sets = append(sets, "phone = ?")
		args = append(args, optionalTrimmedPtr(phone))
	}
	if email != nil {
		sets = append(sets, "email = ?")
		args = append(args, optionalTrimmedPtr(email))
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, userID)
	q := fmt.Sprintf(
		`UPDATE users SET %s, updated_at = UTC_TIMESTAMP(3) WHERE id = ?`,
		strings.Join(sets, ", "),
	)
	_, err := s.DB.ExecContext(ctx, q, args...)
	return err
}

func optionalTrimmedPtr(p *string) interface{} {
	if p == nil {
		return nil
	}
	t := strings.TrimSpace(*p)
	if t == "" {
		return nil
	}
	return t
}

// SetUserPasswordHash 设置或清空密码哈希（nil 或空串表示清空）。
func (s *Store) SetUserPasswordHash(ctx context.Context, userID int64, hash *string) error {
	var v interface{}
	if hash != nil && strings.TrimSpace(*hash) != "" {
		v = strings.TrimSpace(*hash)
	} else {
		v = nil
	}
	_, err := s.DB.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = UTC_TIMESTAMP(3) WHERE id = ?`,
		v, userID,
	)
	return err
}

// DeleteUserByID 删除用户行（外键级联 identities / subscriptions / orders）。
func (s *Store) DeleteUserByID(ctx context.Context, userID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, userID)
	return err
}
