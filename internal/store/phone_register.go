package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrPhoneAlreadyRegistered 该手机号已绑定账号。
var ErrPhoneAlreadyRegistered = errors.New("phone_already_registered")

// CreateUserWithPhonePassword 创建仅手机号+密码登录的新用户（phone 存规范化 11 位数字）。
func (s *Store) CreateUserWithPhonePassword(ctx context.Context, rawPhone, passwordHash string) (*User, error) {
	digits := NormalizeLoginPhoneDigits(rawPhone)
	if digits == "" {
		return nil, errors.New("invalid_phone")
	}
	hash := passwordHash
	if hash == "" {
		return nil, errors.New("invalid_password_hash")
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// 事务内再次确认无占用，降低并发下重复注册窗口。
	if uid, err := s.findUserIDByPhoneTx(ctx, tx, digits); err == nil && uid > 0 {
		return nil, ErrPhoneAlreadyRegistered
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO users (folder_key, wechat_openid, phone, password_hash, created_at, updated_at) VALUES ('__pending__', NULL, ?, ?, ?, ?)`,
		digits, hash, now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	folderKey := fmt.Sprintf("u%d", id)
	if _, err := tx.ExecContext(ctx, `UPDATE users SET folder_key = ? WHERE id = ?`, folderKey, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetUserByID(ctx, id)
}

func (s *Store) findUserIDByPhoneTx(ctx context.Context, tx *sql.Tx, digits string) (int64, error) {
	for _, key := range phoneLoginLookupKeys(digits) {
		var uid int64
		q := `SELECT id FROM users WHERE phone = ? LIMIT 1`
		err := tx.QueryRowContext(ctx, q, key).Scan(&uid)
		if err == nil {
			return uid, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	}
	return 0, sql.ErrNoRows
}
