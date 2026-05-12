package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/app"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"

	"github.com/kymjs/noteapi/internal/appleid"
	"github.com/kymjs/noteapi/internal/appstoreiap"
	"github.com/kymjs/noteapi/internal/auth"
	"github.com/kymjs/noteapi/internal/config"
	"github.com/kymjs/noteapi/internal/huawei"
	"github.com/kymjs/noteapi/internal/store"
	"github.com/kymjs/noteapi/internal/subscription"
	"github.com/kymjs/noteapi/internal/smsquota"
	"github.com/kymjs/noteapi/internal/wechat"
	"github.com/kymjs/noteapi/internal/wxnotify"
	"github.com/kymjs/noteapi/internal/wxpay"
	"golang.org/x/crypto/bcrypt"
)

type Server struct {
	Cfg       *config.Config
	Store     *store.Store
	PayClient *core.Client
	PayPriv   *rsa.PrivateKey

	notifyHandler *notify.Handler
	qingyuGuard   *qingyuWebDAVGuard
	// smsQuota 公开短信（注册 / 重置密码）发送频控：每 IP、每手机号、每设备 ID 滑动 24h 各最多 3 次；见 sms_public_quota.go 与 TECHNICAL.md §2.11。
	smsQuota *smsquota.Window
}

func NewServer(cfg *config.Config, st *store.Store) (*Server, error) {
	s := &Server{
		Cfg:       cfg,
		Store:     st,
		qingyuGuard: newQingyuWebDAVGuard(),
		smsQuota:  smsquota.New(smsPublicQuotaPerWindow, 24*time.Hour),
	}
	if cfg.WechatPayConfigured() {
		priv, err := utils.LoadPrivateKeyWithPath(cfg.WechatPayPrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("load merchant private key: %w", err)
		}
		s.PayPriv = priv
		ctx := context.Background()
		opts := []core.ClientOption{
			option.WithWechatPayAutoAuthCipher(
				cfg.WechatPayMchID,
				cfg.WechatPayCertSerial,
				priv,
				cfg.WechatPayAPIv3Key,
			),
		}
		client, err := core.NewClient(ctx, opts...)
		if err != nil {
			return nil, err
		}
		s.PayClient = client
		p := strings.TrimSpace(os.Getenv("WECHAT_PAY_PLATFORM_CERT_PEM_PATH"))
		if p != "" {
			v, err := wxnotify.LoadVerifierFromPEMFile(p)
			if err != nil {
				log.Printf("warning: WECHAT_PAY_PLATFORM_CERT_PEM_PATH: %v — notify disabled", err)
			} else {
				h, err := notify.NewRSANotifyHandler(cfg.WechatPayAPIv3Key, v)
				if err != nil {
					log.Printf("warning: notify handler: %v", err)
				} else {
					s.notifyHandler = h
				}
			}
		} else {
			log.Printf("warning: WECHAT_PAY_PLATFORM_CERT_PEM_PATH unset — payment notify verification disabled until configured")
		}
	}
	return s, nil
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/wechat", s.handleAuthWechat)
	mux.HandleFunc("POST /api/v1/auth/huawei", s.handleAuthHuawei)
	mux.HandleFunc("POST /api/v1/auth/apple", s.handleAuthApple)
	mux.HandleFunc("POST /api/v1/auth/phone", s.handleAuthPhone)
	mux.HandleFunc("POST /api/v1/password/reset/check-phone", s.handlePasswordResetCheckPhone)
	mux.HandleFunc("POST /api/v1/password/reset/sms/send", s.handleSendPasswordResetSms)
	mux.HandleFunc("POST /api/v1/password/reset", s.handlePasswordResetConfirm)
	mux.HandleFunc("POST /api/v1/register/captcha/new", s.handleRegisterCaptchaNew)
	mux.HandleFunc("POST /api/v1/register/sms/send", s.handleSendRegisterSms)
	mux.HandleFunc("POST /api/v1/register", s.handleRegisterConfirm)
	mux.HandleFunc("POST /api/v1/me/link/wechat", s.auth(s.handleLinkWechat))
	mux.HandleFunc("POST /api/v1/me/link/huawei", s.auth(s.handleLinkHuawei))
	mux.HandleFunc("POST /api/v1/me/link/apple", s.auth(s.handleLinkApple))
	mux.HandleFunc("POST /api/v1/me/merge/wechat", s.auth(s.handleMergeWechat))
	mux.HandleFunc("POST /api/v1/me/merge/huawei", s.auth(s.handleMergeHuawei))
	mux.HandleFunc("POST /api/v1/me/merge/apple", s.auth(s.handleMergeApple))
	mux.HandleFunc("POST /api/v1/me/rebind/identity/confirm", s.auth(s.handleConfirmIdentityRebind))
	mux.HandleFunc("GET /api/v1/me/subscription", s.auth(s.handleSubscription))
	mux.HandleFunc("GET /api/v1/me/profile", s.auth(s.handleGetProfile))
	mux.HandleFunc("POST /api/v1/me/redeem", s.auth(s.handleRedeem))
	mux.HandleFunc("PATCH /api/v1/me/profile", s.auth(s.handlePatchProfile))
	mux.HandleFunc("POST /api/v1/me/profile/phone/sms/send", s.auth(s.handleSendProfilePhoneSms))
	mux.HandleFunc("POST /api/v1/me/password/sms/send", s.auth(s.handleSendPasswordSms))
	mux.HandleFunc("POST /api/v1/me/avatar", s.auth(s.handlePostAvatar))
	mux.HandleFunc("DELETE /api/v1/me", s.auth(s.handleDeleteAccount))
	mux.HandleFunc("GET /api/v1/qingyu/webdav", s.auth(s.handleQingyuWebDAV))
	mux.HandleFunc("POST /api/v1/orders", s.auth(s.handleCreateOrder))
	mux.HandleFunc("POST /api/v1/orders/{id}/wechat/prepay", s.auth(s.handleWeChatPrepay))
	mux.HandleFunc("POST /api/v1/orders/{id}/apple/verify", s.auth(s.handleAppleVerifyOrder))
	mux.HandleFunc("GET /api/v1/orders/{id}", s.auth(s.handleGetOrder))
	mux.HandleFunc("POST /api/v1/webhooks/wechat/pay", s.handleWeChatPayNotify)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

func (s *Server) auth(next func(http.ResponseWriter, *http.Request, int64)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		raw = strings.TrimSpace(raw)
		if raw == "" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		uid, err := auth.ParseUserID(raw, s.Cfg.JWTSecret)
		if err != nil {
			http.Error(w, `{"error":"invalid_token"}`, http.StatusUnauthorized)
			return
		}
		next(w, r, uid)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON[T any](r *http.Request, dst *T) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

type authWechatReq struct {
	Code string `json:"code"`
}

type authLoginResp struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	UserID      int64  `json:"user_id"`
}

func (s *Server) issueAuthToken(w http.ResponseWriter, userID int64) bool {
	tok, err := auth.SignAccessToken(userID, s.Cfg.JWTSecret, 7*24*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token_failed"})
		return false
	}
	resp := authLoginResp{
		AccessToken: tok,
		ExpiresIn:   int64((7 * 24 * time.Hour).Seconds()),
		UserID:      userID,
	}
	writeJSON(w, http.StatusOK, resp)
	return true
}

func (s *Server) handleAuthWechat(w http.ResponseWriter, r *http.Request) {
	var req authWechatReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Code) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	o, err := wechat.ExchangeOAuthCode(s.Cfg.WechatAppID, s.Cfg.WechatAppSecret, req.Code)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "wechat_oauth_failed", "message": err.Error()})
		return
	}
	ctx := r.Context()
	u, err := s.Store.EnsureUserForIdentity(ctx, store.ProviderWechat, o.OpenID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	s.issueAuthToken(w, u.ID)
}

type authHuaweiReq struct {
	AuthorizationCode string `json:"authorization_code"`
	RedirectURI       string `json:"redirect_uri,omitempty"`
}

func (s *Server) handleAuthHuawei(w http.ResponseWriter, r *http.Request) {
	if !s.Cfg.HuaweiOAuthConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "huawei_oauth_not_configured"})
		return
	}
	var req authHuaweiReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.AuthorizationCode) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	redir := strings.TrimSpace(req.RedirectURI)
	if redir == "" {
		redir = strings.TrimSpace(s.Cfg.HuaweiRedirectURI)
	}
	sub, err := huawei.ExchangeAuthorizationCode(s.Cfg.HuaweiClientID, s.Cfg.HuaweiClientSecret, req.AuthorizationCode, redir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "huawei_oauth_failed", "message": err.Error()})
		return
	}
	ctx := r.Context()
	u, err := s.Store.EnsureUserForIdentity(ctx, store.ProviderHuawei, sub)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	s.issueAuthToken(w, u.ID)
}

type authAppleReq struct {
	IdentityToken string `json:"identity_token"`
}

func (s *Server) handleAuthApple(w http.ResponseWriter, r *http.Request) {
	if !s.Cfg.AppleSignInConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "apple_sign_in_not_configured"})
		return
	}
	var req authAppleReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.IdentityToken) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	sub, err := appleid.VerifyIdentityToken(req.IdentityToken, s.Cfg.AppleClientIDs())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "apple_token_invalid", "message": err.Error()})
		return
	}
	ctx := r.Context()
	u, err := s.Store.EnsureUserForIdentity(ctx, store.ProviderApple, sub)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	s.issueAuthToken(w, u.ID)
}

type authPhoneReq struct {
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

func (s *Server) handleAuthPhone(w http.ResponseWriter, r *http.Request) {
	var req authPhoneReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	if strings.TrimSpace(req.Phone) == "" || strings.TrimSpace(req.Password) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	ctx := r.Context()
	u, err := s.Store.GetUserByPhone(ctx, req.Phone)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	if !u.PasswordHash.Valid || strings.TrimSpace(u.PasswordHash.String) == "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "password_not_set"})
		return
	}
	pw := strings.TrimSpace(req.Password)
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash.String), []byte(pw)); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return
	}
	s.issueAuthToken(w, u.ID)
}

type linkWechatReq struct {
	Code string `json:"code"`
}

func (s *Server) handleLinkWechat(w http.ResponseWriter, r *http.Request, uid int64) {
	var req linkWechatReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Code) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	o, err := wechat.ExchangeOAuthCode(s.Cfg.WechatAppID, s.Cfg.WechatAppSecret, req.Code)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "wechat_oauth_failed", "message": err.Error()})
		return
	}
	ctx := r.Context()
	if err := s.Store.LinkIdentity(ctx, uid, store.ProviderWechat, o.OpenID); err != nil {
		if errors.Is(err, store.ErrIdentityLinkedOtherUser) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "identity_already_linked"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

type linkHuaweiReq struct {
	AuthorizationCode string `json:"authorization_code"`
	RedirectURI       string `json:"redirect_uri,omitempty"`
}

func (s *Server) handleLinkHuawei(w http.ResponseWriter, r *http.Request, uid int64) {
	if !s.Cfg.HuaweiOAuthConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "huawei_oauth_not_configured"})
		return
	}
	var req linkHuaweiReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.AuthorizationCode) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	redir := strings.TrimSpace(req.RedirectURI)
	if redir == "" {
		redir = strings.TrimSpace(s.Cfg.HuaweiRedirectURI)
	}
	sub, err := huawei.ExchangeAuthorizationCode(s.Cfg.HuaweiClientID, s.Cfg.HuaweiClientSecret, req.AuthorizationCode, redir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "huawei_oauth_failed", "message": err.Error()})
		return
	}
	ctx := r.Context()
	if err := s.Store.LinkIdentity(ctx, uid, store.ProviderHuawei, sub); err != nil {
		if errors.Is(err, store.ErrIdentityLinkedOtherUser) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "identity_already_linked"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

type linkAppleReq struct {
	IdentityToken string `json:"identity_token"`
}

func (s *Server) handleLinkApple(w http.ResponseWriter, r *http.Request, uid int64) {
	if !s.Cfg.AppleSignInConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "apple_sign_in_not_configured"})
		return
	}
	var req linkAppleReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.IdentityToken) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	sub, err := appleid.VerifyIdentityToken(req.IdentityToken, s.Cfg.AppleClientIDs())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "apple_token_invalid", "message": err.Error()})
		return
	}
	ctx := r.Context()
	if err := s.Store.LinkIdentity(ctx, uid, store.ProviderApple, sub); err != nil {
		if errors.Is(err, store.ErrIdentityLinkedOtherUser) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "identity_already_linked"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// mergeOrLinkIdentity：凭据对应的身份尚未入库 → 绑定到当前用户；已指向当前用户 → noop；
// 已指向另一用户 → 409 + rebind_ticket（客户端确认后 POST /api/v1/me/rebind/identity/confirm），不再合并或删除对方账号。
func (s *Server) mergeOrLinkIdentity(w http.ResponseWriter, ctx context.Context, survivorID int64, provider, subject string) {
	otherID, err := s.Store.LookupUserIDByIdentity(ctx, provider, subject)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if err := s.Store.LinkIdentity(ctx, survivorID, provider, subject); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": "linked"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	if otherID == survivorID {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": "noop"})
		return
	}
	tik, err := issueIdentityRebindTicket(s.Cfg.JWTSecret, provider, subject, survivorID)
	if err != nil {
		log.Printf("issueIdentityRebindTicket survivor=%d: %v", survivorID, err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "rebind_ticket_unavailable"})
		return
	}
	writeJSON(w, http.StatusConflict, map[string]any{
		"ok":            false,
		"error":         "identity_owned_by_other",
		"rebind_ticket": tik,
	})
}

func (s *Server) handleMergeWechat(w http.ResponseWriter, r *http.Request, uid int64) {
	var req linkWechatReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Code) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	o, err := wechat.ExchangeOAuthCode(s.Cfg.WechatAppID, s.Cfg.WechatAppSecret, req.Code)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "wechat_oauth_failed", "message": err.Error()})
		return
	}
	s.mergeOrLinkIdentity(w, r.Context(), uid, store.ProviderWechat, o.OpenID)
}

func (s *Server) handleMergeHuawei(w http.ResponseWriter, r *http.Request, uid int64) {
	if !s.Cfg.HuaweiOAuthConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "huawei_oauth_not_configured"})
		return
	}
	var req linkHuaweiReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.AuthorizationCode) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	redir := strings.TrimSpace(req.RedirectURI)
	if redir == "" {
		redir = strings.TrimSpace(s.Cfg.HuaweiRedirectURI)
	}
	sub, err := huawei.ExchangeAuthorizationCode(s.Cfg.HuaweiClientID, s.Cfg.HuaweiClientSecret, req.AuthorizationCode, redir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "huawei_oauth_failed", "message": err.Error()})
		return
	}
	s.mergeOrLinkIdentity(w, r.Context(), uid, store.ProviderHuawei, sub)
}

func (s *Server) handleMergeApple(w http.ResponseWriter, r *http.Request, uid int64) {
	if !s.Cfg.AppleSignInConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "apple_sign_in_not_configured"})
		return
	}
	var req linkAppleReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.IdentityToken) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	sub, err := appleid.VerifyIdentityToken(req.IdentityToken, s.Cfg.AppleClientIDs())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "apple_token_invalid", "message": err.Error()})
		return
	}
	s.mergeOrLinkIdentity(w, r.Context(), uid, store.ProviderApple, sub)
}

type subscriptionResp struct {
	State      string `json:"state"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	IsLifetime bool   `json:"is_lifetime"`
}

func (s *Server) handleSubscription(w http.ResponseWriter, r *http.Request, uid int64) {
	ctx := r.Context()
	sub, err := s.Store.GetSubscription(ctx, uid)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
			return
		}
		sub = nil
	}
	state, expYmd, life := subscription.RowToAPIState(sub, time.Now().UTC())
	writeJSON(w, http.StatusOK, subscriptionResp{
		State:      state,
		ExpiresAt:  expYmd,
		IsLifetime: life,
	})
}

type webdavResp struct {
	BaseURL  string `json:"base_url"`
	Username string `json:"username"`
	Password string `json:"password"`
	NotesDir string `json:"notes_dir"`
}

func (s *Server) handleQingyuWebDAV(w http.ResponseWriter, r *http.Request, uid int64) {
	now := time.Now().UTC()
	// 先走短时缓存命中：避免「同一凭据」在 TTL 内重复请求仍吃掉每分钟配额（此前会导致多终端/多入口并发时大量 429）。
	if cached, ok := s.qingyuGuard.getCached(uid, now); ok {
		ctx := r.Context()
		sub, err := s.Store.GetSubscription(ctx, uid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			sub = nil
		}
		state, _, _ := subscription.RowToAPIState(sub, time.Now().UTC())
		if state == "active" || state == "lifetime" {
			writeJSON(w, http.StatusOK, cached)
			return
		}
		s.qingyuGuard.invalidate(uid)
	}
	if !s.qingyuGuard.allow(uid, now) {
		writeTooManyRequests(w)
		return
	}

	ctx := r.Context()
	sub, err := s.Store.GetSubscription(ctx, uid)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
			return
		}
		sub = nil
	}
	state, _, _ := subscription.RowToAPIState(sub, time.Now().UTC())
	if state != "active" && state != "lifetime" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "subscription_required"})
		return
	}
	if !s.Cfg.QingyuWebDAVConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "qingyu_webdav_not_configured"})
		return
	}
	// uid 为 JWT sub，即当前登录用户在 users 表的主键；不同用户 notes_dir 互不相同。
	resp := webdavResp{
		BaseURL:  s.Cfg.QingyuWebDAVBaseURL,
		Username: s.Cfg.QingyuWebDAVUsername,
		Password: s.Cfg.QingyuWebDAVPassword,
		NotesDir: store.QingyuNotesDirForAuthenticatedUser(uid),
	}
	s.qingyuGuard.setCached(uid, resp, now)
	writeJSON(w, http.StatusOK, resp)
}

type createOrderReq struct {
	PlanID string `json:"plan_id"`
}

type orderWire struct {
	ID          int64  `json:"id"`
	OutTradeNo  string `json:"out_trade_no"`
	PlanID      string `json:"plan_id"`
	AmountTotal int    `json:"amount_total"`
	Status      string `json:"status"`
}

func (s *Server) handleCreateOrder(w http.ResponseWriter, r *http.Request, uid int64) {
	var req createOrderReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	plan := strings.TrimSpace(req.PlanID)
	if config.ParsePlanMonths(plan) <= 0 || config.PlanAmountFen(plan) <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_plan"})
		return
	}
	ctx := r.Context()
	out := newOutTradeNo()
	id, err := s.Store.CreateOrder(ctx, uid, out, plan, config.PlanAmountFen(plan))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	writeJSON(w, http.StatusOK, orderWire{
		ID: id, OutTradeNo: out, PlanID: plan, AmountTotal: config.PlanAmountFen(plan), Status: "pending",
	})
}

type prepayResp struct {
	AppID     string `json:"app_id"`
	PartnerID string `json:"partner_id"`
	PrepayID  string `json:"prepay_id"`
	Package   string `json:"package"`
	NonceStr  string `json:"nonce_str"`
	TimeStamp string `json:"timestamp"`
	Sign      string `json:"sign"`
	SignType  string `json:"sign_type"`
}

func (s *Server) handleWeChatPrepay(w http.ResponseWriter, r *http.Request, uid int64) {
	if !s.Cfg.WechatPayConfigured() || s.PayClient == nil || s.PayPriv == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "wechat_pay_not_configured",
			"message": "微信支付商户参数未配置，请稍后在服务端填写商户号与证书",
		})
		return
	}
	idStr := r.PathValue("id")
	oid := config.ParseOrderIDParam(idStr)
	if oid <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_order"})
		return
	}
	ctx := r.Context()
	o, err := s.Store.GetOrderByID(ctx, oid)
	if err != nil || o.UserID != uid {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "order_not_found"})
		return
	}
	if o.Status != "pending" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "order_not_payable"})
		return
	}
	ip := clientIP(r)
	if ip == "" {
		ip = "127.0.0.1"
	}
	tot := int64(o.AmountTotal)
	svc := app.AppApiService{Client: s.PayClient}
	desc := fmt.Sprintf("轻羽云服务-%s", o.PlanID)
	req := app.PrepayRequest{
		Appid:       core.String(s.Cfg.WechatAppID),
		Mchid:       core.String(s.Cfg.WechatPayMchID),
		Description: core.String(desc),
		OutTradeNo:  core.String(o.OutTradeNo),
		NotifyUrl:   core.String(s.Cfg.NotifyURL()),
		Amount: &app.Amount{
			Total:    &tot,
			Currency: core.String("CNY"),
		},
		SceneInfo: &app.SceneInfo{
			PayerClientIp: core.String(ip),
		},
	}
	resp, _, err := svc.Prepay(ctx, req)
	if err != nil {
		log.Printf("prepay: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "wechat_prepay_failed", "message": err.Error()})
		return
	}
	prepayID := ""
	if resp != nil && resp.PrepayId != nil {
		prepayID = *resp.PrepayId
	}
	if prepayID == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "empty_prepay_id"})
		return
	}
	ts, nonce, sign, err := wxpay.AppInvokeSign(s.Cfg.WechatAppID, prepayID, s.PayPriv)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "sign_failed"})
		return
	}
	writeJSON(w, http.StatusOK, prepayResp{
		AppID:     s.Cfg.WechatAppID,
		PartnerID: s.Cfg.WechatPayMchID,
		PrepayID:  prepayID,
		Package:   "Sign=WXPay",
		NonceStr:  nonce,
		TimeStamp: ts,
		Sign:      sign,
		SignType:  "RSA",
	})
}

func (s *Server) handleGetOrder(w http.ResponseWriter, r *http.Request, uid int64) {
	oid := config.ParseOrderIDParam(r.PathValue("id"))
	if oid <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_order"})
		return
	}
	ctx := r.Context()
	o, err := s.Store.GetOrderByID(ctx, oid)
	if err != nil || o.UserID != uid {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "order_not_found"})
		return
	}
	writeJSON(w, http.StatusOK, orderWire{
		ID: o.ID, OutTradeNo: o.OutTradeNo, PlanID: o.PlanID, AmountTotal: o.AmountTotal, Status: o.Status,
	})
}

type appleVerifyReq struct {
	SignedTransaction string `json:"signed_transaction"`
}

func (s *Server) extendQingyuSubscriptionAfterPayment(ctx context.Context, userID int64, planID string, audit *store.MembershipRechargeRecordParams) {
	sub, errSub := s.Store.GetSubscription(ctx, userID)
	if errSub != nil && !errors.Is(errSub, sql.ErrNoRows) {
		log.Printf("subscription read after payment: %v", errSub)
		return
	}
	if errors.Is(errSub, sql.ErrNoRows) {
		sub = nil
	}
	newExp, lifetime := subscription.ExtendAfterPayment(sub, planID, time.Now().UTC())
	if lifetime {
		_ = s.Store.UpsertSubscriptionExpiry(ctx, userID, time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC), true)
	} else {
		_ = s.Store.UpsertSubscriptionExpiry(ctx, userID, newExp, false)
	}
	s.qingyuGuard.invalidate(userID)
	if audit != nil {
		if err := s.Store.InsertMembershipRechargeRecord(ctx, audit); err != nil {
			log.Printf("membership recharge audit: %v", err)
		}
	}
}

func (s *Server) handleAppleVerifyOrder(w http.ResponseWriter, r *http.Request, uid int64) {
	if !s.Cfg.AppleIAPVerifyConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "apple_iap_not_configured",
			"message": "服务端未配置 APPLE_IAP_BUNDLE_ID / APPLE_IAP_PRODUCT_* 内购商品 ID",
		})
		return
	}
	idStr := r.PathValue("id")
	oid := config.ParseOrderIDParam(idStr)
	if oid <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_order"})
		return
	}
	var req appleVerifyReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	jws := strings.TrimSpace(req.SignedTransaction)
	if jws == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_signed_transaction"})
		return
	}
	ctx := r.Context()
	o, err := s.Store.GetOrderByID(ctx, oid)
	if err != nil || o.UserID != uid {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "order_not_found"})
		return
	}
	if o.Status != "pending" {
		if o.Status == "paid" {
			writeJSON(w, http.StatusOK, map[string]string{"status": "already_paid"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "order_not_payable"})
		return
	}

	payload, err := appstoreiap.VerifySignedTransaction(s.Cfg.AppleIAPBundleID, s.Cfg.AppleAppStoreAppID, jws)
	if err != nil {
		log.Printf("apple verify jws: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "apple_jws_invalid", "message": err.Error()})
		return
	}
	if payload.RevocationDate != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "apple_transaction_revoked"})
		return
	}
	tid := strings.TrimSpace(payload.TransactionId)
	if tid == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_transaction_id"})
		return
	}
	planFromApple := s.Cfg.PlanFromAppleProductID(strings.TrimSpace(payload.ProductId))
	if planFromApple == "" || planFromApple != o.PlanID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "apple_product_plan_mismatch"})
		return
	}

	prev, errLookup := s.Store.GetOrderByTransactionID(ctx, tid)
	if errLookup != nil && !errors.Is(errLookup, sql.ErrNoRows) {
		log.Printf("apple verify lookup transaction: %v", errLookup)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	if errLookup == nil && prev != nil {
		if prev.UserID != uid {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "transaction_conflict"})
			return
		}
		if prev.ID != o.ID {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "duplicate_apple_transaction"})
			return
		}
		if prev.Status == "paid" {
			writeJSON(w, http.StatusOK, map[string]string{"status": "paid"})
			return
		}
	}

	if err := s.Store.MarkOrderPaid(ctx, o.OutTradeNo, tid); err != nil {
		refreshed, err2 := s.Store.GetOrderByID(ctx, oid)
		if err2 == nil && refreshed.Status == "paid" &&
			refreshed.TransactionID.Valid && refreshed.TransactionID.String == tid {
			writeJSON(w, http.StatusOK, map[string]string{"status": "paid"})
			return
		}
		log.Printf("apple mark paid: %v", err)
		writeJSON(w, http.StatusConflict, map[string]string{"error": "order_pay_state_conflict"})
		return
	}
	s.extendQingyuSubscriptionAfterPayment(ctx, o.UserID, o.PlanID, &store.MembershipRechargeRecordParams{
		UserID:               o.UserID,
		Channel:              "apple",
		OrderID:              sql.NullInt64{Int64: o.ID, Valid: true},
		OutTradeNo:           sql.NullString{String: o.OutTradeNo, Valid: strings.TrimSpace(o.OutTradeNo) != ""},
		GatewayTransactionID: sql.NullString{String: tid, Valid: strings.TrimSpace(tid) != ""},
		PlanID:               o.PlanID,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "paid"})
}

func (s *Server) handleWeChatPayNotify(w http.ResponseWriter, r *http.Request) {
	if s.notifyHandler == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "notify_not_configured"})
		return
	}
	ctx := r.Context()
	tx := new(payments.Transaction)
	nreq, err := s.notifyHandler.ParseNotifyRequest(ctx, r, tx)
	if err != nil {
		log.Printf("notify parse: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "FAIL", "message": err.Error()})
		return
	}
	_ = nreq
	if tx.OutTradeNo == nil || tx.TransactionId == nil || tx.TradeState == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "FAIL", "message": "bad_payload"})
		return
	}
	if *tx.TradeState != "SUCCESS" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	out := *tx.OutTradeNo
	tid := *tx.TransactionId

	dbo, err := s.Store.GetOrderByOutTradeNo(ctx, out)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("notify: unknown out_trade_no=%s", out)
		} else {
			log.Printf("notify: order lookup %s: %v", out, err)
		}
		writeJSON(w, http.StatusOK, map[string]string{"code": "SUCCESS", "message": "OK"})
		return
	}
	if tx.Amount != nil && tx.Amount.Total != nil && int(*tx.Amount.Total) != dbo.AmountTotal {
		log.Printf("notify: amount mismatch order=%s", out)
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "FAIL", "message": "amount"})
		return
	}
	if err := s.Store.MarkOrderPaid(ctx, out, tid); err != nil {
		log.Printf("notify mark paid (maybe duplicate): %v", err)
		writeJSON(w, http.StatusOK, map[string]string{"code": "SUCCESS", "message": "OK"})
		return
	}
	s.extendQingyuSubscriptionAfterPayment(ctx, dbo.UserID, dbo.PlanID, &store.MembershipRechargeRecordParams{
		UserID:               dbo.UserID,
		Channel:              "wechat",
		OrderID:              sql.NullInt64{Int64: dbo.ID, Valid: true},
		OutTradeNo:           sql.NullString{String: dbo.OutTradeNo, Valid: strings.TrimSpace(dbo.OutTradeNo) != ""},
		GatewayTransactionID: sql.NullString{String: tid, Valid: strings.TrimSpace(tid) != ""},
		PlanID:               dbo.PlanID,
	})
	writeJSON(w, http.StatusOK, map[string]string{"code": "SUCCESS", "message": "OK"})
}

func clientIP(r *http.Request) string {
	x := r.Header.Get("X-Forwarded-For")
	if x != "" {
		return strings.TrimSpace(strings.Split(x, ",")[0])
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

func newOutTradeNo() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return fmt.Sprintf("qy%s", hex.EncodeToString(b))
}
