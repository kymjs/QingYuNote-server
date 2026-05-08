package wechat

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type OAuthAccess struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	OpenID       string `json:"openid"`
	Scope        string `json:"scope"`
	UnionID      string `json:"unionid"`
	ErrCode      int    `json:"errcode"`
	ErrMsg       string `json:"errmsg"`
}

func ExchangeOAuthCode(appID, secret, code string) (*OAuthAccess, error) {
	appID = strings.TrimSpace(appID)
	secret = strings.TrimSpace(secret)
	code = strings.TrimSpace(code)
	if appID == "" || secret == "" || code == "" {
		return nil, errors.New("missing app id, secret or code")
	}
	u := "https://api.weixin.qq.com/sns/oauth2/access_token?" + url.Values{
		"appid":      {appID},
		"secret":     {secret},
		"code":       {code},
		"grant_type": {"authorization_code"},
	}.Encode()
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var o OAuthAccess
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, err
	}
	if o.ErrCode != 0 {
		return nil, fmt.Errorf("wechat oauth err %d: %s", o.ErrCode, o.ErrMsg)
	}
	if strings.TrimSpace(o.OpenID) == "" {
		return nil, errors.New("empty openid")
	}
	return &o, nil
}
