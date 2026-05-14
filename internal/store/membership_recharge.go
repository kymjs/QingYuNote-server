package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// MembershipRechargeRecordParams 会籍充值/核销审计行（支付存订单号与网关流水，兑换存码哈希）。
type MembershipRechargeRecordParams struct {
	UserID                 int64
	Channel                string
	OrderID                sql.NullInt64
	OutTradeNo             sql.NullString
	GatewayTransactionID   sql.NullString
	RedemptionCodeHash     sql.NullString
	PlanID                 string
}

func normalizeRechargeChannel(ch string) (string, error) {
	s := strings.TrimSpace(strings.ToLower(ch))
	switch s {
	case "wechat", "apple", "huawei", "redeem":
		return s, nil
	default:
		return "", fmt.Errorf("invalid membership recharge channel")
	}
}

// InsertMembershipRechargeRecord 在订阅已成功延长后调用，写入可核验凭证。
func (s *Store) InsertMembershipRechargeRecord(ctx context.Context, p *MembershipRechargeRecordParams) error {
	if p == nil {
		return fmt.Errorf("nil params")
	}
	ch, err := normalizeRechargeChannel(p.Channel)
	if err != nil {
		return err
	}
	plan := strings.TrimSpace(p.PlanID)
	if p.UserID <= 0 || plan == "" {
		return fmt.Errorf("invalid user_id or plan_id")
	}
	now := time.Now().UTC()
	q := `INSERT INTO membership_recharge_records (
		user_id, channel, order_id, out_trade_no, gateway_transaction_id, redemption_code_hash, plan_id, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = s.DB.ExecContext(ctx, q,
		p.UserID, ch, p.OrderID, p.OutTradeNo, p.GatewayTransactionID, p.RedemptionCodeHash, plan, now,
	)
	return err
}
