package api

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/kymjs/noteapi/internal/aliyunsms"
	"github.com/kymjs/noteapi/internal/store"
)

type passwordResetCheckReq struct {
	Phone string `json:"phone"`
}

type passwordResetCheckResp struct {
	Registered bool `json:"registered"`
}

type passwordResetSmsReq struct {
	Phone string `json:"phone"`
}

type passwordResetConfirmReq struct {
	Phone         string `json:"phone"`
	NewPassword   string `json:"new_password"`
	SmsVerifyCode string `json:"sms_verify_code"`
}

func validChinaMobileDigits(d string) bool {
	if len(d) != 11 || d[0] != '1' {
		return false
	}
	// 中国大陆号段第二位通常为 3–9
	if d[1] < '3' || d[1] > '9' {
		return false
	}
	for i := 2; i < 11; i++ {
		if d[i] < '0' || d[i] > '9' {
			return false
		}
	}
	return true
}

// handlePasswordResetCheckPhone POST /api/v1/password/reset/check-phone
// 无需登录：判断规范化后的 11 位手机号是否在系统中存在绑定账号。
func (s *Server) handlePasswordResetCheckPhone(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req passwordResetCheckReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	digits := store.NormalizeLoginPhoneDigits(req.Phone)
	if digits == "" || !validChinaMobileDigits(digits) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_phone"})
		return
	}
	ctx := r.Context()
	_, err := s.Store.GetUserByPhone(ctx, digits)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusOK, passwordResetCheckResp{Registered: false})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	writeJSON(w, http.StatusOK, passwordResetCheckResp{Registered: true})
}

// handleSendPasswordResetSms POST /api/v1/password/reset/sms/send
// 无需登录：若手机号已绑定账号则发送重置密码验证码。
func (s *Server) handleSendPasswordResetSms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if !s.Cfg.AliyunSMSConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sms_not_configured"})
		return
	}
	var req passwordResetSmsReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	digits := store.NormalizeLoginPhoneDigits(req.Phone)
	if digits == "" || !validChinaMobileDigits(digits) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_phone"})
		return
	}
	ctx := r.Context()
	u, err := s.Store.GetUserByPhone(ctx, digits)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no_account"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	if !u.Phone.Valid || strings.TrimSpace(u.Phone.String) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no_account"})
		return
	}
	dbDigits := store.NormalizeLoginPhoneDigits(u.Phone.String)
	if dbDigits == "" || dbDigits != digits {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no_account"})
		return
	}
	if err := s.sendPasswordVerifySMS(dbDigits); err != nil {
		apiErr, detail := aliyunsms.ClassifySendError(err)
		log.Printf("password reset sms send digits=%s: %s", digits, detail)
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

func (s *Server) sendPasswordVerifySMS(dbDigits string) error {
	cli, err := aliyunsms.NewClient(s.Cfg.AliyunSMSRegion, s.Cfg.AliyunAccessKeyID, s.Cfg.AliyunAccessKeySecret)
	if err != nil {
		return err
	}
	params := aliyunsms.SMSParams{
		SignName:      s.Cfg.AliyunSMSSignName,
		TemplateCode:  s.Cfg.AliyunSMSTemplateCode,
		SchemeName:    s.Cfg.AliyunSMSSchemeName,
		TemplateParam: s.Cfg.AliyunSMSTemplateParam,
	}
	return aliyunsms.SendVerifyCode(cli, params, dbDigits)
}

// handlePasswordResetConfirm POST /api/v1/password/reset
// 无需登录：短信核验通过后设置该手机号对应账号的新密码。
func (s *Server) handlePasswordResetConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req passwordResetConfirmReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	digits := store.NormalizeLoginPhoneDigits(req.Phone)
	if digits == "" || !validChinaMobileDigits(digits) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_phone"})
		return
	}
	newPw := strings.TrimSpace(req.NewPassword)
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
	u, err := s.Store.GetUserByPhone(ctx, digits)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no_account"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	if !u.Phone.Valid || strings.TrimSpace(u.Phone.String) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no_account"})
		return
	}
	dbDigits := store.NormalizeLoginPhoneDigits(u.Phone.String)
	if dbDigits == "" || dbDigits != digits {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no_account"})
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
	ok, err := aliyunsms.CheckVerifyCode(cli, params, dbDigits, smsCode)
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
	if err := s.Store.SetUserPasswordHash(ctx, u.ID, &sHash); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
