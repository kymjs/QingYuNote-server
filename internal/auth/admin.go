package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const adminSubject = "admin"

// SignAdminToken 签发管理后台 JWT（sub=admin，含 role 声明）。
func SignAdminToken(secret string, ttl time.Duration) (string, error) {
	if secret == "" {
		return "", errors.New("JWT_SECRET missing")
	}
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iss":  Issuer,
		"sub":  adminSubject,
		"role": "admin",
		"iat":  now.Unix(),
		"exp":  now.Add(ttl).Unix(),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(secret))
}

// IsAdminToken 校验 token 是否为管理后台签发。
func IsAdminToken(tokenStr, secret string) bool {
	if secret == "" || tokenStr == "" {
		return false
	}
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil || !tok.Valid {
		return false
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return false
	}
	sub, _ := claims["sub"].(string)
	role, _ := claims["role"].(string)
	return sub == adminSubject && role == "admin"
}
