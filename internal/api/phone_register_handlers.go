package api

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/dchest/captcha"
	"golang.org/x/crypto/bcrypt"

	"github.com/kymjs/noteapi/internal/aliyunsms"
	"github.com/kymjs/noteapi/internal/store"
)

type registerSmsReq struct {
	Phone       string `json:"phone"`
	CaptchaID   string `json:"captcha_id"`
	CaptchaCode string `json:"captcha_code"`
	DeviceID    string `json:"device_id"` // 可选；与 X-Device-Id 一起供公开短信频控 dev 维度，见 TECHNICAL.md §2.11
}

// handleSendRegisterSms POST /api/v1/register/sms/send
// 无需登录：校验图形验证码后，仅当手机号尚未绑定账号时发送注册短信验证码。
// 频控：与重置密码短信共用 sms24:* 计数（同手机号跨接口累计 phone 维度）；发送失败须 undo。详见 TECHNICAL.md §2.11。
func (s *Server) handleSendRegisterSms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if !s.Cfg.AliyunSMSConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sms_not_configured"})
		return
	}
	var req registerSmsReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	cid := strings.TrimSpace(req.CaptchaID)
	cc := strings.TrimSpace(req.CaptchaCode)
	if cid == "" || cc == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "captcha_required"})
		return
	}
	if !captcha.VerifyString(cid, cc) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "captcha_invalid"})
		return
	}
	digits := store.NormalizeLoginPhoneDigits(req.Phone)
	if digits == "" || !validChinaMobileDigits(digits) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_phone"})
		return
	}
	ctx := r.Context()
	_, err := s.Store.GetUserByPhone(ctx, digits)
	if err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already_registered"})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	undo, ok := s.tryReservePublicSMSQuota(w, r, digits, req.DeviceID)
	if !ok {
		return
	}
	if err := s.sendPasswordVerifySMS(digits); err != nil {
		undo()
		apiErr, detail := aliyunsms.ClassifySendError(err)
		log.Printf("register sms send digits=%s: %s", digits, detail)
		status := http.StatusBadGateway
		switch apiErr {
		case "sms_rate_limited":
			status = http.StatusTooManyRequests
		case "sms_limit_exceeded":
			status = http.StatusTooManyRequests
		case "invalid_phone":
			status = http.StatusBadRequest
		case "sms_feature_not_opened", "sms_invalid_params":
			status = http.StatusBadRequest
		case "sms_sign_template_mismatch", "sms_aliyun_auth":
			status = http.StatusBadGateway
		}
		writeJSON(w, status, map[string]string{"error": apiErr})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type registerConfirmReq struct {
	Phone         string `json:"phone"`
	Password      string `json:"password"`
	SmsVerifyCode string `json:"sms_verify_code"`
}

// handleRegisterConfirm POST /api/v1/register
// 无需登录：短信核验通过后创建账号并签发 JWT（与登录响应一致）。
func (s *Server) handleRegisterConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req registerConfirmReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	digits := store.NormalizeLoginPhoneDigits(req.Phone)
	if digits == "" || !validChinaMobileDigits(digits) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_phone"})
		return
	}
	newPw := strings.TrimSpace(req.Password)
	if len(newPw) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_too_short"})
		return
	}
	smsCode := strings.TrimSpace(req.SmsVerifyCode)
	if smsCode == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sms_verify_code_required"})
		return
	}
	if !s.Cfg.AliyunSMSConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sms_not_configured"})
		return
	}
	ctx := r.Context()
	if _, err := s.Store.GetUserByPhone(ctx, digits); err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already_registered"})
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}

	cli, err := aliyunsms.NewClient(s.Cfg.AliyunSMSRegion, s.Cfg.AliyunAccessKeyID, s.Cfg.AliyunAccessKeySecret)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "sms_client_failed"})
		return
	}
	params := aliyunsms.SMSParams{
		SignName:      s.Cfg.AliyunSMSSignName,
		TemplateCode:  s.Cfg.AliyunSMSTemplateCode,
		SchemeName:    s.Cfg.AliyunSMSSchemeName,
		TemplateParam: s.Cfg.AliyunSMSTemplateParam,
	}
	ok, err := aliyunsms.CheckVerifyCode(cli, params, digits, smsCode)
	if err != nil || !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "sms_code_invalid"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPw), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "hash_failed"})
		return
	}
	sHash := string(hash)
	u, err := s.Store.CreateUserWithPhonePassword(ctx, digits, sHash)
	if err != nil {
		if errors.Is(err, store.ErrPhoneAlreadyRegistered) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "already_registered"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	platform, deviceID := extractDeviceInfo(r)
	s.issueAuthToken(w, u.ID, platform, deviceID)
}
