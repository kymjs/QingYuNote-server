package api

import (
	"log"
	"net/http"

	"github.com/kymjs/noteapi/internal/aliyunsms"
	"github.com/kymjs/noteapi/internal/store"
)

type sendProfilePhoneSmsReq struct {
	Phone string `json:"phone"`
}

// handleSendProfilePhoneSms POST /api/v1/me/profile/phone/sms/send
// 向请求中的目标手机号发送验证码，用于绑定/修改资料中的手机号（与 PATCH phone + sms_verify_code 配对）。
func (s *Server) handleSendProfilePhoneSms(w http.ResponseWriter, r *http.Request, uid int64) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if !s.Cfg.AliyunSMSConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sms_not_configured"})
		return
	}
	var req sendProfilePhoneSmsReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	digits := store.NormalizeLoginPhoneDigits(req.Phone)
	if digits == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_phone"})
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
	if err := aliyunsms.SendVerifyCode(cli, params, digits); err != nil {
		apiErr, detail := aliyunsms.ClassifySendError(err)
		log.Printf("profile phone sms send uid=%d: %s", uid, detail)
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
