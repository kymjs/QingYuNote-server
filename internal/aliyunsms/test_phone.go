package aliyunsms

import "strings"

// TestSMSVerifyCode 为 100 开头测试手机号的固定短信验证码（不实际下发短信）。
const TestSMSVerifyCode = "8200"

// IsTestPhone 判断是否为 11 位、100 开头的测试手机号。
func IsTestPhone(phoneDigits string) bool {
	d := strings.TrimSpace(phoneDigits)
	if len(d) != 11 || !strings.HasPrefix(d, "100") {
		return false
	}
	for i := 0; i < 11; i++ {
		if d[i] < '0' || d[i] > '9' {
			return false
		}
	}
	return true
}
