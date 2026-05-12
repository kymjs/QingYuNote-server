// 本文件实现「公开短信」发送前的进程内频控辅助逻辑（包仍为 api）。
//
// # 规则摘要（给维护者 / 排障；勿写入面向用户的文案）
//
// 对以下接口在调用阿里云发短信之前统一占位（与 phone_register_handlers、password_reset_handlers 配合）：
//   - POST /api/v1/register/sms/send
//   - POST /api/v1/password/reset/sms/send
//
// 三个统计维度彼此独立，每个维度在滑动 24h 窗口内最多允许 3 次成功占位（常量 smsPublicQuotaPerWindow）：
//   1) 客户端 IP（X-Forwarded-For 首段，否则 RemoteAddr，见 server.go 的 clientIP）
//   2) 规范化后的 11 位手机号（与 store.NormalizeLoginPhoneDigits 一致）
//   3) 设备：HTTP 头 X-Device-Id 优先，否则 JSON device_id；若皆空则用 sha256(IP+"\n"+User-Agent) 前 12 字节 hex 前缀 fb:，避免「全匿名」共用一个桶
//
// 注册与重置密码共用 key 前缀 sms24:*，故同一手机号在两条链路上共享 phone 维度的计数。
// 占位由 smsquota.Window.BeginSend 完成；若阿里云发送失败，调用方必须执行返回的 undo 以回滚三轴占位。
// 超限：HTTP 429 + error 字段 sms_quota_exceeded（客户端应对用户展示泛化提示，不写具体阈值与维度）。
package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// smsPublicQuotaPerWindow 单维度（ip / phone / dev）在滑动窗口内允许的最大成功占位次数。
// 与 NewServer 中 smsquota.New(smsPublicQuotaPerWindow, 24*time.Hour) 一致；改此处须同步改窗口时长或文档 TECHNICAL.md §2.11。
const smsPublicQuotaPerWindow = 3

// normalizeSMSDeviceID 解析请求中的设备标识，供 sms24:dev:* 计数。
// 优先级：X-Device-Id > JSON device_id；经 clampSMSDeviceKey（trim、最长 128）。
// 若仍无有效值：返回 "fb:"+hex(sha256(clientIP+"\n"+User-Agent))[:12]，使无头客户端仍按「近似终端」分桶，而非全局单桶。
func normalizeSMSDeviceID(r *http.Request, bodyDevice string) string {
	if v := strings.TrimSpace(r.Header.Get("X-Device-Id")); v != "" {
		return clampSMSDeviceKey(v)
	}
	if v := strings.TrimSpace(bodyDevice); v != "" {
		return clampSMSDeviceKey(v)
	}
	sum := sha256.Sum256([]byte(clientIP(r) + "\n" + r.UserAgent()))
	return "fb:" + hex.EncodeToString(sum[:12])
}

func clampSMSDeviceKey(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 128 {
		s = s[:128]
	}
	if s == "" {
		return ""
	}
	return s
}

// publicSMSQuotaKeys 构造一次发短信尝试在三个维度上的计数器 key（与 smsquota.Window 内 map 键一一对应）。
func (s *Server) publicSMSQuotaKeys(r *http.Request, phoneDigits string, bodyDevice string) []string {
	ip := clientIP(r)
	dev := normalizeSMSDeviceID(r, bodyDevice)
	return []string{
		"sms24:ip:" + ip,
		"sms24:phone:" + phoneDigits,
		"sms24:dev:" + dev,
	}
}

// tryReservePublicSMSQuota 在发短信前占位；ok 为 false 时已向 w 写入 429 + sms_quota_exceeded。
// 成功时返回非 nil 的 undo，必须在阿里云发送失败时调用以释放三轴占位；发送成功则不得调用 undo。
func (s *Server) tryReservePublicSMSQuota(w http.ResponseWriter, r *http.Request, phoneDigits, bodyDevice string) (undo func(), ok bool) {
	if s.smsQuota == nil {
		return func() {}, true
	}
	keys := s.publicSMSQuotaKeys(r, phoneDigits, bodyDevice)
	allowed, undo := s.smsQuota.BeginSend(keys)
	if !allowed {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "sms_quota_exceeded"})
		return nil, false
	}
	return undo, true
}
