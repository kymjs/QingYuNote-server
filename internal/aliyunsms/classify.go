package aliyunsms

import (
	"errors"
	"strings"

	sdkerrors "github.com/aliyun/alibaba-cloud-sdk-go/sdk/errors"
)

// ClassifySendError 将发送验证码阶段的错误映射为 API error 字段使用的短码，便于客户端提示；detail 供服务端日志记录。
func ClassifySendError(err error) (apiCode string, detail string) {
	if err == nil {
		return "", ""
	}
	detail = err.Error()

	var se *sdkerrors.ServerError
	if errors.As(err, &se) {
		code := strings.TrimSpace(se.ErrorCode())
		msg := strings.TrimSpace(se.Message())
		detail = code + ": " + msg
		u := strings.ToUpper(code)
		switch u {
		case "FUNCTION_NOT_OPENED":
			return "sms_feature_not_opened", detail
		case "FREQUENCY_FAIL":
			return "sms_rate_limited", detail
		case "BUSINESS_LIMIT_CONTROL":
			return "sms_limit_exceeded", detail
		case "MOBILE_NUMBER_ILLEGAL":
			return "invalid_phone", detail
		case "INVALID_PARAMETERS":
			return "sms_invalid_params", detail
		case "MissingPhoneNumbers", "IllegalPhoneNumber":
			return "invalid_phone", detail
		}
		ul := strings.ToUpper(code)
		if strings.Contains(ul, "SIGN") || strings.Contains(ul, "TEMPLATE") {
			return "sms_sign_template_mismatch", detail
		}
		if strings.Contains(ul, "ACCESSKEY") || strings.Contains(ul, "SIGNATURE") ||
			strings.Contains(ul, "FORBIDDEN") || strings.Contains(ul, "RAM") {
			return "sms_aliyun_auth", detail
		}
		return "sms_send_failed", detail
	}

	// 本包 SendVerifyCode 返回的 aliyun:业务码:说明
	s := err.Error()
	if strings.HasPrefix(s, "aliyun:") {
		rest := strings.TrimPrefix(s, "aliyun:")
		idx := strings.Index(rest, ":")
		code := rest
		if idx >= 0 {
			code = rest[:idx]
		}
		u := strings.ToUpper(strings.TrimSpace(code))
		switch u {
		case "FREQUENCY_FAIL":
			return "sms_rate_limited", detail
		case "BUSINESS_LIMIT_CONTROL":
			return "sms_limit_exceeded", detail
		case "MOBILE_NUMBER_ILLEGAL":
			return "invalid_phone", detail
		case "FUNCTION_NOT_OPENED":
			return "sms_feature_not_opened", detail
		case "INVALID_PARAMETERS":
			return "sms_invalid_params", detail
		}
		if strings.Contains(u, "SIGN") || strings.Contains(u, "TEMPLATE") {
			return "sms_sign_template_mismatch", detail
		}
	}

	em := strings.ToUpper(s)
	switch {
	case strings.Contains(em, "FREQUENCY_FAIL"):
		return "sms_rate_limited", detail
	case strings.Contains(em, "BUSINESS_LIMIT_CONTROL"):
		return "sms_limit_exceeded", detail
	case strings.Contains(em, "MOBILE_NUMBER_ILLEGAL"):
		return "invalid_phone", detail
	case strings.Contains(em, "FUNCTION_NOT_OPENED"):
		return "sms_feature_not_opened", detail
	}

	return "sms_send_failed", detail
}
