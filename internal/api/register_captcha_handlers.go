package api

import (
	"bytes"
	"encoding/base64"
	"net/http"

	"github.com/dchest/captcha"
)

const registerCaptchaDigits = 4

// handleRegisterCaptchaNew POST /api/v1/register/captcha/new
// 无需登录：生成图形验证码 PNG（base64），用于发送注册短信前的校验。
func (s *Server) handleRegisterCaptchaNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	id := captcha.NewLen(registerCaptchaDigits)
	var buf bytes.Buffer
	if err := captcha.WriteImage(&buf, id, captcha.StdWidth, captcha.StdHeight); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "captcha_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"captcha_id":   id,
		"image_base64": base64.StdEncoding.EncodeToString(buf.Bytes()),
	})
}
