package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type Store struct {
	DB *sql.DB
}

func OpenMySQL(dsn string) (*Store, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("MYSQL_DSN is required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{DB: db}, nil
}

// QingyuNotesDirForAuthenticatedUser 返回当前登录用户在 NAS 上的笔记根路径。
// userID 须与 JWT sub 一致（即 users.id）；每名用户目录不同，例如用户 42 → "/42/"。
func QingyuNotesDirForAuthenticatedUser(userID int64) string {
	return fmt.Sprintf("/%d/", userID)
}

type User struct {
	ID           int64
	FolderKey    string
	WechatOID    sql.NullString
	CreatedAt    time.Time
	DisplayName  sql.NullString
	AvatarURL    sql.NullString
	Phone        sql.NullString
	Email        sql.NullString
	PasswordHash sql.NullString
}

func (s *Store) UpsertUserByWechat(ctx context.Context, openID string) (*User, error) {
	return s.EnsureUserForIdentity(ctx, ProviderWechat, openID)
}

func (s *Store) GetUserByWechatOpenID(ctx context.Context, openID string) (*User, error) {
	var u User
	q := `SELECT id, folder_key, wechat_openid, created_at,
		display_name, avatar_url, phone, email, password_hash
		FROM users WHERE wechat_openid = ? LIMIT 1`
	err := s.DB.QueryRowContext(ctx, q, openID).Scan(
		&u.ID, &u.FolderKey, &u.WechatOID, &u.CreatedAt,
		&u.DisplayName, &u.AvatarURL, &u.Phone, &u.Email, &u.PasswordHash,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (*User, error) {
	var u User
	q := `SELECT id, folder_key, wechat_openid, created_at,
		display_name, avatar_url, phone, email, password_hash
		FROM users WHERE id = ? LIMIT 1`
	err := s.DB.QueryRowContext(ctx, q, id).Scan(
		&u.ID, &u.FolderKey, &u.WechatOID, &u.CreatedAt,
		&u.DisplayName, &u.AvatarURL, &u.Phone, &u.Email, &u.PasswordHash,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// NormalizeLoginPhoneDigits 去掉首尾空白、可选 +86 前缀，仅保留 ASCII 数字，用于手机号登录比对。
func NormalizeLoginPhoneDigits(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "+86") {
		s = strings.TrimSpace(s[3:])
	}
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func phoneLoginLookupKeys(digits string) []string {
	if digits == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var keys []string
	add := func(k string) {
		if k == "" {
			return
		}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	add(digits)
	if len(digits) == 11 && digits[0] == '1' {
		add("+86" + digits)
		add("86" + digits)
	}
	return keys
}

// GetUserByPhone 按资料中的手机号查找用户（兼容存为 11 位、+86、86 前缀）。
func (s *Store) GetUserByPhone(ctx context.Context, rawPhone string) (*User, error) {
	digits := NormalizeLoginPhoneDigits(rawPhone)
	for _, key := range phoneLoginLookupKeys(digits) {
		var u User
		q := `SELECT id, folder_key, wechat_openid, created_at,
			display_name, avatar_url, phone, email, password_hash
			FROM users WHERE phone = ? LIMIT 1`
		err := s.DB.QueryRowContext(ctx, q, key).Scan(
			&u.ID, &u.FolderKey, &u.WechatOID, &u.CreatedAt,
			&u.DisplayName, &u.AvatarURL, &u.Phone, &u.Email, &u.PasswordHash,
		)
		if err == nil {
			return &u, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	return nil, sql.ErrNoRows
}

type SubscriptionRow struct {
	UserID      int64
	ExpiresAt   sql.NullTime
	IsLifetime  bool
}

func (s *Store) GetSubscription(ctx context.Context, userID int64) (*SubscriptionRow, error) {
	var r SubscriptionRow
	q := `SELECT user_id, expires_at, is_lifetime FROM subscriptions WHERE user_id = ?`
	err := s.DB.QueryRowContext(ctx, q, userID).Scan(&r.UserID, &r.ExpiresAt, &r.IsLifetime)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) UpsertSubscriptionExpiry(ctx context.Context, userID int64, expiresAt time.Time, lifetime bool) error {
	now := time.Now().UTC()
	q := `INSERT INTO subscriptions (user_id, expires_at, is_lifetime, updated_at)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE expires_at = VALUES(expires_at), is_lifetime = VALUES(is_lifetime), updated_at = VALUES(updated_at)`
	_, err := s.DB.ExecContext(ctx, q, userID, expiresAt, lifetime, now)
	return err
}

type Order struct {
	ID           int64
	UserID       int64
	OutTradeNo   string
	PlanID       string
	AmountTotal  int
	Status       string
	CreatedAt    time.Time
	PaidAt       sql.NullTime
	TransactionID sql.NullString
}

func (s *Store) CreateOrder(ctx context.Context, userID int64, outTradeNo, planID string, amountFen int) (int64, error) {
	now := time.Now().UTC()
	q := `INSERT INTO orders (user_id, out_trade_no, plan_id, amount_total, status, created_at)
		VALUES (?, ?, ?, ?, 'pending', ?)`
	res, err := s.DB.ExecContext(ctx, q, userID, outTradeNo, planID, amountFen, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) GetOrderByID(ctx context.Context, id int64) (*Order, error) {
	var o Order
	q := `SELECT id, user_id, out_trade_no, plan_id, amount_total, status, created_at, paid_at, transaction_id
		FROM orders WHERE id = ?`
	err := s.DB.QueryRowContext(ctx, q, id).Scan(
		&o.ID, &o.UserID, &o.OutTradeNo, &o.PlanID, &o.AmountTotal, &o.Status, &o.CreatedAt, &o.PaidAt, &o.TransactionID,
	)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *Store) GetOrderByOutTradeNo(ctx context.Context, out string) (*Order, error) {
	var o Order
	q := `SELECT id, user_id, out_trade_no, plan_id, amount_total, status, created_at, paid_at, transaction_id
		FROM orders WHERE out_trade_no = ?`
	err := s.DB.QueryRowContext(ctx, q, out).Scan(
		&o.ID, &o.UserID, &o.OutTradeNo, &o.PlanID, &o.AmountTotal, &o.Status, &o.CreatedAt, &o.PaidAt, &o.TransactionID,
	)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// GetOrderByTransactionID 用于苹果 / 微信等网关 transaction id 幂等与冲突检测。
func (s *Store) GetOrderByTransactionID(ctx context.Context, txID string) (*Order, error) {
	txID = strings.TrimSpace(txID)
	if txID == "" {
		return nil, sql.ErrNoRows
	}
	var o Order
	q := `SELECT id, user_id, out_trade_no, plan_id, amount_total, status, created_at, paid_at, transaction_id
		FROM orders WHERE transaction_id = ?`
	err := s.DB.QueryRowContext(ctx, q, txID).Scan(
		&o.ID, &o.UserID, &o.OutTradeNo, &o.PlanID, &o.AmountTotal, &o.Status, &o.CreatedAt, &o.PaidAt, &o.TransactionID,
	)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *Store) MarkOrderPaid(ctx context.Context, outTradeNo, transactionID string) error {
	now := time.Now().UTC()
	q := `UPDATE orders SET status = 'paid', paid_at = ?, transaction_id = ? WHERE out_trade_no = ? AND status = 'pending'`
	res, err := s.DB.ExecContext(ctx, q, now, transactionID, outTradeNo)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("order not updated")
	}
	return nil
}

// UpsertUserDeviceSession 记录或更新用户设备会话的活跃时间。
// platform: android/ios/harmony/windows/linux/macos
// deviceID: 客户端生成的设备唯一ID
// appVersion: 客户端版本号（X-App-Version），空则不覆盖已有版本
func (s *Store) UpsertUserDeviceSession(ctx context.Context, userID int64, platform, deviceID, appVersion string) error {
	if userID <= 0 || platform == "" || deviceID == "" {
		return nil
	}
	now := time.Now().UTC()
	q := `INSERT INTO user_device_sessions (user_id, platform, device_id, app_version, last_active_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			last_active_at = VALUES(last_active_at),
			app_version = IF(VALUES(app_version) <> '', VALUES(app_version), app_version)`
	_, err := s.DB.ExecContext(ctx, q, userID, platform, deviceID, appVersion, now, now)
	return err
}
