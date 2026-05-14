// Package huaweiiap 调用华为 IAP 服务端「购买 Token 校验」接口，用于轻羽云订单核销。
// 参考：https://developer.huawei.com/consumer/cn/doc/HMSCore-References/api-order-verify-purchase-token-0000001050746113
package huaweiiap

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const oauthTokenURL = "https://oauth-login.cloud.huawei.com/oauth2/v3/token"

// InAppPurchaseData 与华为返回的 purchaseTokenData JSON 对齐（常用字段）。
type InAppPurchaseData struct {
	ApplicationID int64  `json:"applicationId"`
	OrderID       string `json:"orderId"`
	ProductID     string `json:"productId"`
	PackageName   string `json:"packageName,omitempty"`
	PurchaseState int64  `json:"purchaseState"`
	PurchaseToken string `json:"purchaseToken,omitempty"`
	Price         int64  `json:"price,omitempty"`
}

type orderVerifyResponse struct {
	ResponseCode      string `json:"responseCode"`
	ResponseMessage   string `json:"responseMessage,omitempty"`
	PurchaseTokenData string `json:"purchaseTokenData,omitempty"`
	DataSignature     string `json:"dataSignature,omitempty"`
}

type oauthTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

// Client 华为 IAP 服务端校验客户端（OAuth + 订单域请求）。
type Client struct {
	clientID     string
	clientSecret string
	orderSiteURL string
	httpCli      *http.Client

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

func New(clientID, clientSecret, orderSiteURL string) *Client {
	u := strings.TrimSpace(orderSiteURL)
	if u == "" || !strings.HasPrefix(u, "http") {
		u = "https://orders-drcn.iap.cloud.huawei.com.cn"
	}
	return &Client{
		clientID:     strings.TrimSpace(clientID),
		clientSecret: strings.TrimSpace(clientSecret),
		orderSiteURL: strings.TrimRight(u, "/"),
		httpCli: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) authHeader(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accessToken != "" && time.Now().Before(c.expiresAt) {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte("APPAT:"+c.accessToken)), nil
	}
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	resp, err := c.httpCli.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("huawei oauth http %d: %s", resp.StatusCode, string(body))
	}
	var tr oauthTokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", err
	}
	if strings.TrimSpace(tr.AccessToken) == "" {
		return "", fmt.Errorf("huawei oauth empty token: %s", string(body))
	}
	c.accessToken = tr.AccessToken
	ttl := tr.ExpiresIn
	if ttl <= 0 {
		ttl = 3600
	}
	// 提前 60s 过期，避免边界竞态
	c.expiresAt = time.Now().Add(time.Duration(ttl-60) * time.Second)
	if c.expiresAt.Before(time.Now().Add(30 * time.Second)) {
		c.expiresAt = time.Now().Add(30 * time.Second)
	}
	return "Basic " + base64.StdEncoding.EncodeToString([]byte("APPAT:"+c.accessToken)), nil
}

func orderRootURL(accountFlag int64, fallback string) string {
	switch accountFlag {
	case 1:
		return "https://orders-drcn.iap.cloud.huawei.com.cn"
	case 2:
		return "https://orders-dre.iap.cloud.huawei.eu"
	case 3:
		return "https://orders-dra.iap.cloud.huawei.asia"
	case 4:
		return "https://orders-drru.iap.cloud.huawei.ru"
	default:
		return strings.TrimRight(fallback, "/")
	}
}

// VerifyPurchaseToken 调用 applications/purchases/tokens/verify，成功时返回解析后的购买数据。
func (c *Client) VerifyPurchaseToken(ctx context.Context, accountFlag int64, purchaseToken, productID string) (*InAppPurchaseData, error) {
	pt := strings.TrimSpace(purchaseToken)
	pid := strings.TrimSpace(productID)
	if pt == "" || pid == "" {
		return nil, errors.New("empty purchase_token or product_id")
	}
	auth, err := c.authHeader(ctx)
	if err != nil {
		return nil, fmt.Errorf("huawei iap oauth: %w", err)
	}
	root := orderRootURL(accountFlag, c.orderSiteURL)
	apiURL := root + "/applications/purchases/tokens/verify"
	payload := map[string]string{
		"purchaseToken": pt,
		"productId":     pid,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("Authorization", auth)
	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var ov orderVerifyResponse
	if err := json.Unmarshal(respBody, &ov); err != nil {
		return nil, fmt.Errorf("huawei iap verify decode: %w body=%s", err, string(respBody))
	}
	if strings.TrimSpace(ov.ResponseCode) != "0" {
		msg := strings.TrimSpace(ov.ResponseMessage)
		if msg == "" {
			msg = string(respBody)
		}
		return nil, fmt.Errorf("huawei iap verify responseCode=%s: %s", ov.ResponseCode, msg)
	}
	raw := strings.TrimSpace(ov.PurchaseTokenData)
	if raw == "" {
		return nil, errors.New("huawei iap verify empty purchaseTokenData")
	}
	var data InAppPurchaseData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("huawei iap purchaseTokenData json: %w", err)
	}
	return &data, nil
}

// ErrNotPurchased purchaseState 非已支付。
var ErrNotPurchased = errors.New("huawei purchase not in purchased state")

// ValidateForCredit 校验包名、商品 ID、支付状态，用于入账前闸门。
func ValidateForCredit(data *InAppPurchaseData, expectPackage, expectProductID string) error {
	if data == nil {
		return errors.New("nil purchase data")
	}
	if strings.TrimSpace(data.OrderID) == "" {
		return errors.New("empty huawei orderId")
	}
	pn := strings.TrimSpace(data.PackageName)
	if expectPackage != "" && pn != "" && pn != expectPackage {
		return fmt.Errorf("package mismatch: got %q want %q", pn, expectPackage)
	}
	if strings.TrimSpace(data.ProductID) != strings.TrimSpace(expectProductID) {
		return fmt.Errorf("productId mismatch: got %q want %q", data.ProductID, expectProductID)
	}
	// 0: purchased; -1 initialized 等视为未支付
	if data.PurchaseState != 0 {
		return fmt.Errorf("%w: state=%d", ErrNotPurchased, data.PurchaseState)
	}
	return nil
}
