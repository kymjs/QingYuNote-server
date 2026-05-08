package huawei

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const tokenEndpoint = "https://oauth-login.cloud.huawei.com/oauth2/v3/token"

type rawTokenResp struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	OpenID      string `json:"open_id"` // 部分响应直接在顶层给出
	ExpiresIn   int    `json:"expires_in"`
	SubError    int    `json:"sub_error"` // 华为错误码（若有）
	ErrorDesc   string `json:"error_description"`
	Error       string `json:"error"`
}

// ExchangeAuthorizationCode 用客户端传来的 authorization_code 换令牌并解析稳定 subject（优先 id_token.sub）。
func ExchangeAuthorizationCode(clientID, clientSecret, code, redirectURI string) (subject string, err error) {
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	code = strings.TrimSpace(code)
	redirectURI = strings.TrimSpace(redirectURI)
	if clientID == "" || clientSecret == "" || code == "" {
		return "", errors.New("missing huawei oauth params")
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	if redirectURI != "" {
		form.Set("redirect_uri", redirectURI)
	}

	req, err := http.NewRequest(http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("huawei token http %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}

	var tr rawTokenResp
	if err := json.Unmarshal(raw, &tr); err != nil {
		return "", fmt.Errorf("huawei token json: %w", err)
	}
	if strings.TrimSpace(tr.Error) != "" || tr.SubError != 0 {
		return "", fmt.Errorf("huawei oauth error: %s %s", tr.Error, tr.ErrorDesc)
	}
	sub := strings.TrimSpace(tr.OpenID)
	if sub == "" && strings.TrimSpace(tr.IDToken) != "" {
		sub, err = subjectFromJWTUnsafePayload(tr.IDToken)
		if err != nil {
			return "", fmt.Errorf("huawei id_token: %w", err)
		}
		sub = strings.TrimSpace(sub)
	}
	if sub == "" {
		return "", errors.New("huawei token response missing open_id / id_token.sub")
	}
	return sub, nil
}

func subjectFromJWTUnsafePayload(jwt string) (string, error) {
	parts := strings.Split(strings.TrimSpace(jwt), ".")
	if len(parts) != 3 {
		return "", errors.New("not a jwt")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", err
	}
	if strings.TrimSpace(claims.Sub) == "" {
		return "", errors.New("jwt missing sub")
	}
	return strings.TrimSpace(claims.Sub), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
