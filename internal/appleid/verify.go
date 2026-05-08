package appleid

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const appleIssuer = "https://appleid.apple.com"

var (
	jwksMu      sync.RWMutex
	jwksCached  *jwksDoc
	jwksFetched time.Time
	jwksTTL     = 24 * time.Hour
)

type jwksDoc struct {
	Keys []map[string]any `json:"keys"`
}

// VerifyIdentityToken 校验 Apple identity_token（ES256），并返回 token 中的 sub。
func VerifyIdentityToken(rawToken, audience string) (sub string, err error) {
	rawToken = strings.TrimSpace(rawToken)
	audience = strings.TrimSpace(audience)
	if rawToken == "" || audience == "" {
		return "", errors.New("missing apple token or audience")
	}

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodES256.Alg()}),
		jwt.WithIssuer(appleIssuer),
		jwt.WithAudience(audience),
		jwt.WithLeeway(10*time.Second),
	)

	var ac jwt.RegisteredClaims
	_, err = parser.ParseWithClaims(rawToken, &ac, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		kid = strings.TrimSpace(kid)
		if kid == "" {
			return nil, errors.New("missing jwt kid")
		}
		return publicKeyForKid(kid)
	})
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(ac.Subject)
	if s == "" {
		return "", errors.New("missing sub")
	}
	return s, nil
}

func publicKeyForKid(kid string) (*ecdsa.PublicKey, error) {
	doc, err := fetchAppleJWKS()
	if err != nil {
		return nil, err
	}
	for _, k := range doc.Keys {
		kidVal, _ := k["kid"].(string)
		if strings.TrimSpace(kidVal) != kid {
			continue
		}
		return mapECJWKToPublicKey(k)
	}
	return nil, fmt.Errorf("apple jwks: no key for kid %q", kid)
}

func fetchAppleJWKS() (*jwksDoc, error) {
	jwksMu.RLock()
	if jwksCached != nil && time.Since(jwksFetched) < jwksTTL {
		doc := jwksCached
		jwksMu.RUnlock()
		return doc, nil
	}
	jwksMu.RUnlock()

	jwksMu.Lock()
	defer jwksMu.Unlock()
	if jwksCached != nil && time.Since(jwksFetched) < jwksTTL {
		return jwksCached, nil
	}

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Get("https://appleid.apple.com/auth/keys")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("apple jwks http %d", resp.StatusCode)
	}
	var doc jwksDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	jwksCached = &doc
	jwksFetched = time.Now()
	return jwksCached, nil
}

func mapECJWKToPublicKey(k map[string]any) (*ecdsa.PublicKey, error) {
	kty, _ := k["kty"].(string)
	if strings.TrimSpace(kty) != "EC" {
		return nil, errors.New("expected EC key")
	}
	crv, _ := k["crv"].(string)
	if strings.TrimSpace(crv) != "P-256" {
		return nil, fmt.Errorf("unsupported curve %q", crv)
	}
	xStr, _ := k["x"].(string)
	yStr, _ := k["y"].(string)
	xb, err := base64.RawURLEncoding.DecodeString(xStr)
	if err != nil {
		return nil, err
	}
	yb, err := base64.RawURLEncoding.DecodeString(yStr)
	if err != nil {
		return nil, err
	}
	x := new(big.Int).SetBytes(xb)
	y := new(big.Int).SetBytes(yb)
	curve := elliptic.P256()
	if !curve.IsOnCurve(x, y) {
		return nil, errors.New("invalid EC point")
	}
	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
}
