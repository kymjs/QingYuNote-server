package appleid

import (
	"encoding/base64"
	"encoding/json"
	"strings"
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

func padJWTBase64URL(seg string) string {
	switch len(seg) % 4 {
	case 2:
		return seg + "=="
	case 3:
		return seg + "="
	default:
		return seg
	}
}

// InspectIdentityToken 解析 Apple identity_token 的 header/payload（不验签），供日志使用。
func InspectIdentityToken(raw string) IdentityTokenPeek {
	raw = strings.TrimSpace(raw)
	p := IdentityTokenPeek{TokenLen: len(raw)}
	parts := strings.Split(raw, ".")
	p.NumParts = len(parts)
	if len(parts) < 1 {
		return p
	}
	hdrJSON, err := base64.RawURLEncoding.DecodeString(padJWTBase64URL(parts[0]))
	if err != nil {
		return p
	}
	var hdr map[string]any
	if err := json.Unmarshal(hdrJSON, &hdr); err != nil {
		return p
	}
	p.Alg, _ = hdr["alg"].(string)
	p.Alg = strings.TrimSpace(p.Alg)
	p.Kid, _ = hdr["kid"].(string)
	p.Kid = strings.TrimSpace(p.Kid)

	if len(parts) < 2 {
		return p
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(padJWTBase64URL(parts[1]))
	if err != nil {
		return p
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return p
	}
	p.Iss, _ = claims["iss"].(string)
	p.Iss = strings.TrimSpace(p.Iss)
	p.Aud = flattenAudClaim(claims["aud"])
	sub, _ := claims["sub"].(string)
	sub = strings.TrimSpace(sub)
	if sub != "" {
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
