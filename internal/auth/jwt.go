package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const Issuer = "noteapi"

// ScopeH5Pay marks a short-lived ticket usable only for VIP H5 page-pay APIs.
const ScopeH5Pay = "h5_pay"

// H5PayTicketTTL is the default lifetime for H5 pay tickets (external browser).
const H5PayTicketTTL = 5 * time.Minute

func SignAccessToken(userID int64, secret string, ttl time.Duration) (string, error) {
	if secret == "" {
		return "", errors.New("JWT_SECRET missing")
	}
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iss": Issuer,
		"sub": fmt.Sprintf("%d", userID),
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(secret))
}

// SignH5PayTicket issues a short-lived JWT with scope=h5_pay for external-browser VIP checkout.
func SignH5PayTicket(userID int64, secret string, ttl time.Duration) (string, error) {
	if secret == "" {
		return "", errors.New("JWT_SECRET missing")
	}
	if ttl <= 0 {
		ttl = H5PayTicketTTL
	}
	jti, err := randomJTI()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iss":   Issuer,
		"sub":   fmt.Sprintf("%d", userID),
		"iat":   now.Unix(),
		"exp":   now.Add(ttl).Unix(),
		"scope": ScopeH5Pay,
		"jti":   jti,
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(secret))
}

func randomJTI() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// TokenClaims is the parsed access or H5-pay ticket identity.
type TokenClaims struct {
	UserID int64
	Scope  string
}

func ParseTokenClaims(tokenStr, secret string) (TokenClaims, error) {
	var zero TokenClaims
	if secret == "" {
		return zero, errors.New("JWT_SECRET missing")
	}
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !tok.Valid {
		return zero, errors.New("invalid token")
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return zero, errors.New("bad claims")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return zero, errors.New("missing sub")
	}
	id, err := strconv.ParseInt(sub, 10, 64)
	if err != nil {
		return zero, err
	}
	scope, _ := claims["scope"].(string)
	return TokenClaims{UserID: id, Scope: scope}, nil
}

func ParseUserID(tokenStr, secret string) (int64, error) {
	c, err := ParseTokenClaims(tokenStr, secret)
	if err != nil {
		return 0, err
	}
	return c.UserID, nil
}
