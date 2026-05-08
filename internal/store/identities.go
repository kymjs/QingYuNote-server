package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 与 API / 客户端约定的 provider 取值。
const (
	ProviderWechat = "wechat"
	ProviderHuawei = "huawei"
	ProviderApple  = "apple"
)

// ErrIdentityLinkedOtherUser 该第三方账号已绑定到其他用户。
var ErrIdentityLinkedOtherUser = errors.New("identity_linked_to_other_user")

// LookupUserIDByIdentity 查询 provider+subject 是否已绑定用户（不存在则 sql.ErrNoRows）。
func (s *Store) LookupUserIDByIdentity(ctx context.Context, provider, subject string) (int64, error) {
	p := normalizeProv(provider)
	sub := strings.TrimSpace(subject)
	if p == "" || sub == "" {
		return 0, errors.New("invalid_identity")
	}
	return s.findUserIDByIdentity(ctx, nil, p, sub)
}

func (s *Store) findUserIDByIdentity(ctx context.Context, tx *sql.Tx, provider, subject string) (int64, error) {
	q := `SELECT user_id FROM user_identities WHERE provider = ? AND subject = ? LIMIT 1`
	var uid int64
	var err error
	if tx != nil {
		err = tx.QueryRowContext(ctx, q, provider, subject).Scan(&uid)
	} else {
		err = s.DB.QueryRowContext(ctx, q, provider, subject).Scan(&uid)
	}
	return uid, err
}

// EnsureUserForIdentity 若该 provider+subject 已存在则返回对应用户；否则创建 users + identity。
func (s *Store) EnsureUserForIdentity(ctx context.Context, provider, subject string) (*User, error) {
	provider = normalizeProv(provider)
	subject = strings.TrimSpace(subject)
	if provider == "" || subject == "" {
		return nil, errors.New("invalid_identity")
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	uid, err := s.findUserIDByIdentity(ctx, tx, provider, subject)
	if err == nil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return s.GetUserByID(ctx, uid)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO users (folder_key, wechat_openid, created_at, updated_at) VALUES ('__pending__', NULL, ?, ?)`,
		now, now)
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
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO user_identities (user_id, provider, subject, created_at) VALUES (?, ?, ?, ?)`,
		id, provider, subject, now); err != nil {
		return nil, err
	}
	if provider == ProviderWechat {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET wechat_openid = ?, updated_at = ? WHERE id = ?`, subject, now, id); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetUserByID(ctx, id)
}

// LinkIdentity 将 provider+subject 绑定到当前 user_id（登录后调用）。若已被他人占用则返回 ErrIdentityLinkedOtherUser。
func (s *Store) LinkIdentity(ctx context.Context, userID int64, provider, subject string) error {
	provider = normalizeProv(provider)
	subject = strings.TrimSpace(subject)
	if provider == "" || subject == "" {
		return errors.New("invalid_identity")
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	uid, err := s.findUserIDByIdentity(ctx, tx, provider, subject)
	if err == nil {
		if uid == userID {
			return tx.Commit()
		}
		return ErrIdentityLinkedOtherUser
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO user_identities (user_id, provider, subject, created_at) VALUES (?, ?, ?, ?)`,
		userID, provider, subject, now); err != nil {
		return err
	}
	if provider == ProviderWechat {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET wechat_openid = ?, updated_at = ? WHERE id = ?`, subject, now, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func normalizeProv(p string) string {
	switch strings.TrimSpace(p) {
	case ProviderWechat, ProviderHuawei, ProviderApple:
		return strings.TrimSpace(p)
	default:
		return ""
	}
}
