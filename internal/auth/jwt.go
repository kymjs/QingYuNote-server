package auth

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const Issuer = "noteapi"

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

func ParseUserID(tokenStr, secret string) (int64, error) {
	if secret == "" {
		return 0, errors.New("JWT_SECRET missing")
	}
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !tok.Valid {
		return 0, errors.New("invalid token")
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return 0, errors.New("bad claims")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return 0, errors.New("missing sub")
	}
	id, err := strconv.ParseInt(sub, 10, 64)
	if err != nil {
		return 0, err
	}
	return id, nil
}
