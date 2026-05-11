package aliyunsms

import (
	"errors"
	"strings"

	sdkerrors "github.com/aliyun/alibaba-cloud-sdk-go/sdk/errors"
)

// ClassifySendError 将发送验证码阶段的错误映射为 API error 字段使用的短码；detail 供服务端日志记录。
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
		case "MISSINGPHONENUMBERS", "ILLEGALPHONENUMBER":
			return "invalid_phone", detail
		}

		// ① AccessKey / RPC 签名（与「短信签名 SignName」无关）：须先于下面「模板/短信签名」判断，
		// 否则 SignatureDoesNotMatch 会因含有 SIGN 被误判为 sms_sign_template_mismatch。
		if isAccessOrRpcSignatureError(u) {
			return "sms_aliyun_auth", detail
		}

		// ② 控制台赠送短信签名、模板 CODE、模板变量
		if isSmsProductSignOrTemplateError(u) {
			return "sms_sign_template_mismatch", detail
		}

		return "sms_send_failed", detail
	}

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
		if isAccessOrRpcSignatureError(u) {
			return "sms_aliyun_auth", detail
		}
		if isSmsProductSignOrTemplateError(u) {
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

// AccessKey、RPC 请求签名（HMAC）、RAM 拒绝；不含短信业务「签名名称」类错误。
func isAccessOrRpcSignatureError(codeUpper string) bool {
	switch codeUpper {
	case "SIGNATUREDOESNOTMATCH", "INCOMPLETESIGNATURE", "SIGNATURENONCEEXPIRED",
		"INVALIDTIMESTAMP", "NONCEEXPIRED":
		return true
	}
	if strings.HasPrefix(codeUpper, "INVALIDACCESSKEYID") {
		return true
	}
	if strings.Contains(codeUpper, "SUBUSER") && strings.Contains(codeUpper, "PERMISSION") {
		return true
	}
	if strings.HasPrefix(codeUpper, "FORBIDDEN") || strings.HasPrefix(codeUpper, "NOPERMISSION") {
		return true
	}
	if strings.Contains(codeUpper, "ACCESSDENIED") && strings.Contains(codeUpper, "AUTHORIZATION") {
		return true
	}
	return false
}

// 号码认证控制台侧：签名名称、模板 CODE、模板与变量不匹配等。
func isSmsProductSignOrTemplateError(codeUpper string) bool {
	switch codeUpper {
	case "INVALIDSIGNNAME", "SIGNNAMENOTMATCH", "INVALIDTEMPLATE",
		"TEMPLATENOTEXIST":
		return true
	}
	return strings.Contains(codeUpper, "SIGNNAME") ||
		strings.Contains(codeUpper, "TEMPLATECODE") ||
		strings.Contains(codeUpper, "TEMPLATEPARAM")
}
