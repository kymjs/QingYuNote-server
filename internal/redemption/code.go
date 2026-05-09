package redemption

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// NormalizeCode 与用户兑换时一致：去首尾空白并转大写，便于输入容错。
func NormalizeCode(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// HashNormalized 返回 NormalizeCode 后的 SHA256 十六进制字符串（64 字符）。
func HashNormalized(normalized string) string {
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}
