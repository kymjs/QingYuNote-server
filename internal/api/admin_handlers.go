package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kymjs/noteapi/internal/auth"
	"github.com/kymjs/noteapi/internal/redemption"
	"github.com/kymjs/noteapi/internal/store"
	"github.com/kymjs/noteapi/internal/subscription"
)

type adminLoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type adminLoginResp struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if !s.Cfg.AdminConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "admin_not_configured"})
		return
	}
	ip := clientIP(r)
	ctx := r.Context()
	banned, err := s.Store.IsAdminIPBanned(ctx, ip)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	if banned {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "ip_banned"})
		return
	}
	var req adminLoginReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	user := strings.TrimSpace(req.Username)
	pass := strings.TrimSpace(req.Password)
	if user == "" || pass == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	if user != s.Cfg.AdminUsername || pass != s.Cfg.AdminPassword {
		justBanned, errFail := s.Store.RecordAdminLoginFailure(ctx, ip)
		if errFail != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
			return
		}
		if justBanned {
			log.Printf("admin login: IP %s permanently banned after %d consecutive failures", ip, store.AdminLoginMaxConsecutiveFailures)
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "ip_banned"})
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return
	}
	if err := s.Store.ResetAdminLoginFailures(ctx, ip); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	ttl := 24 * time.Hour
	tok, err := auth.SignAdminToken(s.Cfg.JWTSecret, ttl)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token_failed"})
		return
	}
	writeJSON(w, http.StatusOK, adminLoginResp{
		AccessToken: tok,
		ExpiresIn:   int64(ttl.Seconds()),
	})
}

func (s *Server) adminAuth(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		raw = strings.TrimSpace(raw)
		if raw == "" || !auth.IsAdminToken(raw, s.Cfg.JWTSecret) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

type adminRechargeRecordWire struct {
	Time       string  `json:"time"`
	Channel    string  `json:"channel"`
	AmountYuan float64 `json:"amount_yuan"`
}

type adminDeviceUsageWire struct {
	Platform string `json:"platform"`
	LastTime string `json:"last_time"`
}

type adminUserWire struct {
	ID                 int64                     `json:"id"`
	RegisterSource     string                    `json:"register_source"`
	Phone              *string                   `json:"phone"`
	Nickname           *string                   `json:"nickname"`
	AvatarURL          *string                   `json:"avatar_url"`
	QingyuActive       bool                      `json:"qingyu_active"`
	QingyuExpiresAt    string                    `json:"qingyu_expires_at,omitempty"`
	QingyuIsLifetime   bool                      `json:"qingyu_is_lifetime"`
	TotalRechargeYuan  float64                   `json:"total_recharge_yuan"`
	RechargeRecords    []adminRechargeRecordWire `json:"recharge_records"`
	DeviceUsage        []adminDeviceUsageWire     `json:"device_usage"`
}

// 同一事务内创建用户与首条 OAuth identity 时 created_at 应几乎相同；手机号注册后再绑定第三方则 identity 更晚，仍视为验证码注册。
const adminRegisterSourceOAuthSyncWindow = 2 * time.Second

func adminRegisterSource(userCreated time.Time, prov sql.NullString, identAt sql.NullTime) string {
	if !prov.Valid || !identAt.Valid {
		return "sms"
	}
	delta := identAt.Time.Sub(userCreated)
	if delta > adminRegisterSourceOAuthSyncWindow || delta < -adminRegisterSourceOAuthSyncWindow {
		return "sms"
	}
	switch strings.ToLower(strings.TrimSpace(prov.String)) {
	case "huawei":
		return "huawei"
	case "apple":
		return "apple"
	case "wechat":
		return "wechat"
	default:
		return "sms"
	}
}

func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := s.Store.ListAdminUsers(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	userIDs := make([]int64, len(rows))
	for i, row := range rows {
		userIDs[i] = row.ID
	}
	recByUser, err := s.Store.ListAdminUserRechargeRecords(ctx, userIDs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	devByUser, err := s.Store.ListAdminUserDevices(ctx, userIDs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().UTC()
	out := make([]adminUserWire, 0, len(rows))
	for _, row := range rows {
		var sub *store.SubscriptionRow
		if row.ExpiresAt.Valid || row.IsLifetime {
			sub = &store.SubscriptionRow{
				UserID:     row.ID,
				ExpiresAt:  row.ExpiresAt,
				IsLifetime: row.IsLifetime,
			}
		}
		state, expYmd, life := subscription.RowToAPIState(sub, now)
		qingyuOK := state == "active" || state == "lifetime"
		recs := recByUser[row.ID]
		wireRecs := make([]adminRechargeRecordWire, 0, len(recs))
		for _, r := range recs {
			wireRecs = append(wireRecs, adminRechargeRecordWire{
				Time:       r.CreatedAt.In(loc).Format("2006-01-02 15:04:05"),
				Channel:    r.Channel,
				AmountYuan: float64(r.AmountFen) / 100.0,
			})
		}
		devs := devByUser[row.ID]
		wireDevs := make([]adminDeviceUsageWire, 0, len(devs))
		for _, d := range devs {
			wireDevs = append(wireDevs, adminDeviceUsageWire{
				Platform: d.Platform,
				LastTime: d.LastActiveAt.In(loc).Format("2006-01-02 15:04:05"),
			})
		}
		wire := adminUserWire{
			ID:                row.ID,
			RegisterSource:    adminRegisterSource(row.CreatedAt, row.FirstIdentityProv, row.FirstIdentityAt),
			Phone:             strPtrOrNil(row.Phone),
			Nickname:          strPtrOrNil(row.DisplayName),
			AvatarURL:         strPtrOrNil(row.AvatarURL),
			QingyuActive:      qingyuOK,
			QingyuExpiresAt:   expYmd,
			QingyuIsLifetime:  life,
			TotalRechargeYuan: float64(row.TotalRechargeFen) / 100.0,
			RechargeRecords:   wireRecs,
			DeviceUsage:       wireDevs,
		}
		out = append(out, wire)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

type adminCreateRedemptionReq struct {
	PlanID string `json:"plan_id"`
	Count  int    `json:"count"`
}

type adminRedemptionCodeWire struct {
	Code string `json:"code"`
}

type adminCreateRedemptionResp struct {
	PlanID string                    `json:"plan_id"`
	Codes  []adminRedemptionCodeWire `json:"codes"`
}

func validRedemptionPlan(p string) bool {
	switch strings.TrimSpace(p) {
	case "monthly", "half_year", "yearly", "lifetime_vip":
		return true
	default:
		return false
	}
}

func newRandomRedemptionCode() (plain, hash string, err error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plain = "QY-" + strings.ToUpper(hex.EncodeToString(b))
	h := redemption.HashNormalized(redemption.NormalizeCode(plain))
	return plain, h, nil
}

func planTitleCN(plan string) string {
	switch plan {
	case "monthly":
		return "月卡"
	case "half_year":
		return "半年卡"
	case "yearly":
		return "年卡"
	case "lifetime_vip":
		return "终身 VIP"
	default:
		return plan
	}
}

func feishuRedemptionBody(plan, code string) string {
	return fmt.Sprintf("【轻羽云兑换码】\n类型：%s（%s）\n兑换码：%s\n（仅限一次性使用，请勿转发给无关人员）",
		planTitleCN(plan), plan, code)
}

type feishuTextPayload struct {
	MsgType string `json:"msg_type"`
	Content struct {
		Text string `json:"text"`
	} `json:"content"`
}

func postFeishuText(webhookURL, text string) error {
	var p feishuTextPayload
	p.MsgType = "text"
	p.Content.Text = text
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	return nil
}

func (s *Server) handleAdminCreateRedemptionCodes(w http.ResponseWriter, r *http.Request) {
	if !s.Cfg.RedemptionIssueConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "redemption_issue_not_configured"})
		return
	}
	var req adminCreateRedemptionReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	plan := strings.TrimSpace(req.PlanID)
	if !validRedemptionPlan(plan) || req.Count < 1 || req.Count > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	ctx := r.Context()
	webhook := strings.TrimSpace(s.Cfg.FeishuRedemptionWebhook)
	codes := make([]adminRedemptionCodeWire, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		plain, hash, errGen := newRandomRedemptionCode()
		if errGen != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "code_gen_failed"})
			return
		}
		if err := s.Store.InsertRedemptionCode(ctx, plan, hash); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
			return
		}
		codes = append(codes, adminRedemptionCodeWire{Code: plain})
		if webhook != "" {
			if err := postFeishuText(webhook, feishuRedemptionBody(plan, plain)); err != nil {
				log.Printf("warning: feishu webhook failed for code ending …%s: %v", plain[len(plain)-4:], err)
			}
		}
	}
	writeJSON(w, http.StatusOK, adminCreateRedemptionResp{PlanID: plan, Codes: codes})
}
