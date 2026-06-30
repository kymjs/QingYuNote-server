package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kymjs/noteapi/internal/config"
)

// ErrUserIDExists 指定用户 ID 已存在，无法创建。
var ErrUserIDExists = errors.New("user_id_exists")

// AdminUserRow 管理后台用户列表行。
type AdminUserRow struct {
	ID                  int64
	DisplayName         sql.NullString
	AvatarURL           sql.NullString
	Phone               sql.NullString
	CreatedAt           time.Time
	FirstIdentityProv   sql.NullString
	FirstIdentityAt     sql.NullTime
	ExpiresAt           sql.NullTime
	IsLifetime          bool
	TotalRechargeFen    int64
}

const AdminUsersPageSize = 100

// AdminUsersListParams 管理后台用户列表分页与排序参数。
type AdminUsersListParams struct {
	Page  int
	Sort  string // id | last_used | recharge | app_version | qingyu
	Order string // asc | desc
}

// AdminUsersListResult 分页用户列表及总数。
type AdminUsersListResult struct {
	Rows  []AdminUserRow
	Total int64
}

func normalizeAdminUsersListParams(p AdminUsersListParams) AdminUsersListParams {
	if p.Page < 1 {
		p.Page = 1
	}
	p.Sort = strings.ToLower(strings.TrimSpace(p.Sort))
	if p.Sort == "" {
		p.Sort = "id"
	}
	p.Order = strings.ToLower(strings.TrimSpace(p.Order))
	if p.Order != "asc" {
		p.Order = "desc"
	}
	return p
}

func adminUsersOrderExpr(sort, order string) (string, error) {
	desc := order == "desc"
	switch sort {
	case "id":
		if desc {
			return "u.id DESC", nil
		}
		return "u.id ASC", nil
	case "last_used":
		if desc {
			return "dev.last_used_at IS NULL, dev.last_used_at DESC, u.id DESC", nil
		}
		return "dev.last_used_at IS NULL DESC, dev.last_used_at ASC, u.id ASC", nil
	case "recharge":
		if desc {
			return "COALESCE(paid.total_fen, 0) DESC, u.id DESC", nil
		}
		return "COALESCE(paid.total_fen, 0) ASC, u.id ASC", nil
	case "app_version":
		latestVer := `(SELECT d.app_version FROM user_device_sessions d WHERE d.user_id = u.id ORDER BY d.last_active_at DESC, d.id DESC LIMIT 1)`
		if desc {
			return fmt.Sprintf("%s IS NULL, %s DESC, u.id DESC", latestVer, latestVer), nil
		}
		return fmt.Sprintf("%s IS NULL DESC, %s ASC, u.id ASC", latestVer, latestVer), nil
	case "qingyu":
		qingyuExpr := `CASE
    WHEN COALESCE(s.is_lifetime, 0) = 1 THEN 1
    WHEN s.expires_at IS NOT NULL AND DATE(s.expires_at) >= CURDATE() THEN 1
    ELSE 0
  END`
		if desc {
			return qingyuExpr + " DESC, u.id DESC", nil
		}
		return qingyuExpr + " ASC, u.id ASC", nil
	default:
		return "", fmt.Errorf("invalid sort: %s", sort)
	}
}

// CountAdminUsers 返回注册用户总数。
func (s *Store) CountAdminUsers(ctx context.Context) (int64, error) {
	var total int64
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&total)
	return total, err
}

// CountAdminUsersActiveToday 返回指定时刻所在自然日（Asia/Shanghai）内有打开或使用客户端的去重用户数。
func (s *Store) CountAdminUsersActiveToday(ctx context.Context, now time.Time) (int64, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.UTC
	}
	y, m, d := now.In(loc).Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, loc).UTC()
	end := start.Add(24 * time.Hour)
	var n int64
	err = s.DB.QueryRowContext(ctx, `
SELECT COUNT(DISTINCT user_id)
FROM user_device_sessions
WHERE last_active_at >= ? AND last_active_at < ?`, start, end).Scan(&n)
	return n, err
}

// ListAdminUsers 分页返回注册用户及订阅、累计充值（已支付订单金额之和，单位分）。
func (s *Store) ListAdminUsers(ctx context.Context, p AdminUsersListParams) (AdminUsersListResult, error) {
	p = normalizeAdminUsersListParams(p)
	orderExpr, err := adminUsersOrderExpr(p.Sort, p.Order)
	if err != nil {
		return AdminUsersListResult{}, err
	}
	offset := (p.Page - 1) * AdminUsersPageSize
	q := fmt.Sprintf(`
SELECT u.id, u.display_name, u.avatar_url, u.phone, u.created_at,
       (SELECT i.provider FROM user_identities i WHERE i.user_id = u.id ORDER BY i.created_at ASC LIMIT 1),
       (SELECT i.created_at FROM user_identities i WHERE i.user_id = u.id ORDER BY i.created_at ASC LIMIT 1),
       s.expires_at, COALESCE(s.is_lifetime, 0),
       COALESCE(paid.total_fen, 0) AS total_recharge_fen
FROM users u
LEFT JOIN subscriptions s ON s.user_id = u.id
LEFT JOIN (
  SELECT user_id, SUM(amount_total) AS total_fen
  FROM orders
  WHERE status = 'paid'
  GROUP BY user_id
) paid ON paid.user_id = u.id
LEFT JOIN (
  SELECT user_id, MAX(last_active_at) AS last_used_at
  FROM user_device_sessions
  GROUP BY user_id
) dev ON dev.user_id = u.id
ORDER BY %s
LIMIT ? OFFSET ?`, orderExpr)
	rows, err := s.DB.QueryContext(ctx, q, AdminUsersPageSize, offset)
	if err != nil {
		return AdminUsersListResult{}, err
	}
	defer rows.Close()

	var out []AdminUserRow
	for rows.Next() {
		var r AdminUserRow
		var life int
		if err := rows.Scan(
			&r.ID, &r.DisplayName, &r.AvatarURL, &r.Phone, &r.CreatedAt,
			&r.FirstIdentityProv, &r.FirstIdentityAt,
			&r.ExpiresAt, &life, &r.TotalRechargeFen,
		); err != nil {
			return AdminUsersListResult{}, err
		}
		r.IsLifetime = life != 0
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return AdminUsersListResult{}, err
	}
	total, err := s.CountAdminUsers(ctx)
	if err != nil {
		return AdminUsersListResult{}, err
	}
	return AdminUsersListResult{Rows: out, Total: total}, nil
}

// AdminDeviceSession 管理后台展示的用户设备使用信息。
type AdminDeviceSession struct {
	Platform     string
	AppVersion   string
	LastActiveAt time.Time
}

// ListAdminUserDevices 按用户查询设备的最后活跃时间，用于管理后台「使用端口」列。
func (s *Store) ListAdminUserDevices(ctx context.Context, userIDs []int64) (map[int64][]AdminDeviceSession, error) {
	uniq := make([]int64, 0, len(userIDs))
	seen := map[int64]struct{}{}
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	out := make(map[int64][]AdminDeviceSession)
	if len(uniq) == 0 {
		return out, nil
	}
	ph := make([]string, len(uniq))
	args := make([]any, len(uniq))
	for i, id := range uniq {
		ph[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`
SELECT user_id, platform, app_version, last_active_at
FROM user_device_sessions
WHERE user_id IN (%s)
ORDER BY user_id ASC, last_active_at DESC`, strings.Join(ph, ","))
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var uid int64
		var platform string
		var appVersion sql.NullString
		var lastActive time.Time
		if err := rows.Scan(&uid, &platform, &appVersion, &lastActive); err != nil {
			return nil, err
		}
		ver := ""
		if appVersion.Valid {
			ver = strings.TrimSpace(appVersion.String)
		}
		if out[uid] == nil {
			out[uid] = append(out[uid], AdminDeviceSession{
				Platform:     platform,
				AppVersion:   ver,
				LastActiveAt: lastActive,
			})
		} else {
			// 同用户同平台只保留第一条（最新时间）
			seen := false
			for _, e := range out[uid] {
				if e.Platform == platform {
					seen = true
					break
				}
			}
			if !seen {
				out[uid] = append(out[uid], AdminDeviceSession{
					Platform:     platform,
					AppVersion:   ver,
					LastActiveAt: lastActive,
				})
			}
		}
	}
	return out, rows.Err()
}

// AdminRechargeRecordRow 管理后台展示的会籍充值审计记录（新到旧排序由查询保证）。
type AdminRechargeRecordRow struct {
	Channel   string
	CreatedAt time.Time
	AmountFen int64
}

// AdminCreateUser 在指定 ID 不存在时创建用户（手机号 + 初始密码哈希）。
func (s *Store) AdminCreateUser(ctx context.Context, userID int64, rawPhone, passwordHash string) error {
	if userID <= 0 {
		return errors.New("invalid_user_id")
	}
	digits := NormalizeLoginPhoneDigits(rawPhone)
	if digits == "" {
		return errors.New("invalid_phone")
	}
	if passwordHash == "" {
		return errors.New("invalid_password_hash")
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var one int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM users WHERE id = ? LIMIT 1`, userID).Scan(&one)
	if err == nil {
		return ErrUserIDExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if uid, err := s.findUserIDByPhoneTx(ctx, tx, digits); err == nil && uid > 0 {
		return ErrPhoneAlreadyRegistered
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	folderKey := fmt.Sprintf("u%d", userID)
	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `
INSERT INTO users (
  id, folder_key, wechat_openid, created_at, updated_at,
  display_name, avatar_url, phone, email, password_hash
) VALUES (?, ?, NULL, ?, ?, NULL, NULL, ?, NULL, ?)`,
		userID, folderKey, now, now, digits, passwordHash)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SetUserPhone(ctx context.Context, userID int64, phone string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET phone = ? WHERE id = ?`, phone, userID)
	return err
}

func (s *Store) ResetUserPassword(ctx context.Context, userID int64, hash string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, hash, userID)
	return err
}

func (s *Store) DeleteUser(ctx context.Context, userID int64) error {
	if err := s.ArchiveUserRegistrationHistory(ctx, userID); err != nil {
		return err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, `DELETE FROM membership_recharge_records WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM orders WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM subscriptions WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM user_device_sessions WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM user_sync_settings WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM user_identities WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

// ListAdminUserRechargeRecords 按用户查询 membership_recharge_records，用于管理后台「充值记录」列。
func (s *Store) ListAdminUserRechargeRecords(ctx context.Context, userIDs []int64) (map[int64][]AdminRechargeRecordRow, error) {
	uniq := make([]int64, 0, len(userIDs))
	seen := map[int64]struct{}{}
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	out := make(map[int64][]AdminRechargeRecordRow)
	if len(uniq) == 0 {
		return out, nil
	}
	ph := make([]string, len(uniq))
	args := make([]any, len(uniq))
	for i, id := range uniq {
		ph[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`
SELECT m.user_id, m.channel, m.created_at, m.plan_id, o.amount_total
FROM membership_recharge_records m
LEFT JOIN orders o ON o.id = m.order_id
WHERE m.user_id IN (%s)
ORDER BY m.user_id ASC, m.created_at DESC`, strings.Join(ph, ","))
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var uid int64
		var ch string
		var created time.Time
		var planID string
		var orderAmt sql.NullInt64
		if err := rows.Scan(&uid, &ch, &created, &planID, &orderAmt); err != nil {
			return nil, err
		}
		var fen int64
		if orderAmt.Valid {
			fen = orderAmt.Int64
		} else {
			fen = int64(config.PlanAmountFen(planID))
		}
		out[uid] = append(out[uid], AdminRechargeRecordRow{
			Channel:   ch,
			CreatedAt: created,
			AmountFen: fen,
		})
	}
	return out, rows.Err()
}
