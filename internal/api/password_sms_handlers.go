package api

import (
	"net/http"
	"strings"

	"github.com/kymjs/noteapi/internal/aliyunsms"
	"github.com/kymjs/noteapi/internal/store"
)

type sendPasswordSmsReq struct {
	// Phone 须与账号资料手机号一致（用于防止串号）；可与客户端输入框对齐校验。
	Phone string `json:"phone"`
}

func (s *Server) handleSendPasswordSms(w http.ResponseWriter, r *http.Request, uid int64) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if !s.Cfg.AliyunSMSConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sms_not_configured"})
		return
	}
	var req sendPasswordSmsReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	ctx := r.Context()
	u, err := s.Store.GetUserByID(ctx, uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	if !u.Phone.Valid || strings.TrimSpace(u.Phone.String) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "phone_not_bound"})
		return
	}
	dbDigits := store.NormalizeLoginPhoneDigits(u.Phone.String)
	if dbDigits == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "phone_not_bound"})
		return
	}
	clientDigits := store.NormalizeLoginPhoneDigits(req.Phone)
	if clientDigits == "" || clientDigits != dbDigits {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "phone_mismatch"})
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
	if err := aliyunsms.SendVerifyCode(cli, params, dbDigits); err != nil {
		em := strings.ToUpper(err.Error())
		switch {
		case strings.Contains(em, "FREQUENCY_FAIL"):
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "sms_rate_limited"})
		case strings.Contains(em, "BUSINESS_LIMIT_CONTROL"):
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "sms_limit_exceeded"})
		case strings.Contains(em, "MOBILE_NUMBER_ILLEGAL"):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_phone"})
		default:
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "sms_send_failed"})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
