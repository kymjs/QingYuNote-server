package appleid

import (
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// IdentityTokenPeek 仅从 JWT 解析公开字段（不验证签名），用于运维日志。
// 绝不包含完整 identity_token；sub 仅保留前缀以便关联。
type IdentityTokenPeek struct {
	Alg       string
	Kid       string
	Iss       string
	Aud       string // 多个 audience 时用逗号连接
	SubPrefix string // sub 前若干字符
	TokenLen  int
	NumParts  int // JWT 分段数，正常为 3
}

// InspectIdentityToken 解析 Apple identity_token 的 header/payload（不验签），供日志使用。
// 解码逻辑与 golang-jwt 校验路径对齐（含可选 padding），避免出现日志里 alg 为空而校验阶段仍能读出 RS256 的不一致。
func InspectIdentityToken(raw string) IdentityTokenPeek {
	raw = strings.TrimSpace(raw)
	p := IdentityTokenPeek{TokenLen: len(raw)}
	parts := strings.Split(raw, ".")
	p.NumParts = len(parts)

	var tok *jwt.Token
	var err error
	// 与常见「非标准 padding」实现兼容；校验失败时再退回默认 Parser（与 VerifyIdentityToken 内 ParseWithClaims 一致）。
	for _, parser := range []*jwt.Parser{
		jwt.NewParser(jwt.WithPaddingAllowed()),
		jwt.NewParser(),
	} {
		tok, _, err = parser.ParseUnverified(raw, jwt.MapClaims{})
		if err == nil && tok != nil {
			break
		}
		tok = nil
	}
	if tok == nil || tok.Header == nil {
		return p
	}

	p.Alg = anyString(tok.Header["alg"])
	p.Kid = anyString(tok.Header["kid"])

	mc, ok := tok.Claims.(jwt.MapClaims)
	if !ok || mc == nil {
		return p
	}
	p.Iss = anyString(mc["iss"])
	p.Aud = flattenAudClaim(mc["aud"])
	if sub := anyString(mc["sub"]); sub != "" {
		r := []rune(sub)
		const max = 12
		if len(r) > max {
			p.SubPrefix = string(r[:max]) + "…"
		} else {
			p.SubPrefix = sub
		}
	}
	return p
}

func anyString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func flattenAudClaim(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case []any:
		var out []string
		for _, x := range t {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return strings.Join(out, ",")
	default:
		return ""
	}
}
