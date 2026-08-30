package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kymjs/noteapi/internal/config"
)

type feishuTenantTokenResp struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int    `json:"expire"`
}

type feishuBitableCreateResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

var (
	feishuTokenMu     sync.Mutex
	feishuTokenCached string
	feishuTokenExpiry time.Time
)

func feishuTenantAccessToken(cfg *config.Config) (string, error) {
	feishuTokenMu.Lock()
	defer feishuTokenMu.Unlock()
	if feishuTokenCached != "" && time.Now().Before(feishuTokenExpiry) {
		return feishuTokenCached, nil
	}
	body, err := json.Marshal(map[string]string{
		"app_id":     strings.TrimSpace(cfg.FeishuBitableAppID),
		"app_secret": strings.TrimSpace(cfg.FeishuBitableAppSecret),
	})
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out feishuTenantTokenResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("token decode: %w", err)
	}
	if out.Code != 0 || strings.TrimSpace(out.TenantAccessToken) == "" {
		return "", fmt.Errorf("token code=%d msg=%s", out.Code, out.Msg)
	}
	feishuTokenCached = out.TenantAccessToken
	// 提前 5 分钟过期，避免临界失效。
	sec := out.Expire
	if sec <= 0 {
		sec = 7200
	}
	feishuTokenExpiry = time.Now().Add(time.Duration(sec-300) * time.Second)
	return feishuTokenCached, nil
}

func buildFeedbackBitableFields(cfg *config.Config, content string, userID int64, phone, problemType string) map[string]any {
	fields := map[string]any{}
	put := func(key, val string) {
		k := strings.TrimSpace(key)
		if k == "" {
			return
		}
		fields[k] = val
	}
	put(cfg.FeishuBitableFieldContent, content)
	put(cfg.FeishuBitableFieldUserID, fmt.Sprintf("%d", userID))
	if ph := strings.TrimSpace(cfg.FeishuBitableFieldPhone); ph != "" {
		fields[ph] = phone
	}
	if tk := strings.TrimSpace(cfg.FeishuBitableFieldType); tk != "" && strings.TrimSpace(problemType) != "" {
		fields[tk] = strings.TrimSpace(problemType)
	}
	if sk := strings.TrimSpace(cfg.FeishuBitableFieldSubmittedAt); sk != "" {
		fields[sk] = time.Now().UnixMilli()
	}
	return fields
}

func postFeishuBitableFeedback(cfg *config.Config, content string, userID int64, phone, problemType string) error {
	if !cfg.FeishuBitableConfigured() {
		return nil
	}
	token, err := feishuTenantAccessToken(cfg)
	if err != nil {
		return fmt.Errorf("tenant token: %w", err)
	}
	fields := buildFeedbackBitableFields(cfg, content, userID, phone, problemType)
	payload, err := json.Marshal(map[string]any{"fields": fields})
	if err != nil {
		return err
	}
	appToken := strings.TrimSpace(cfg.FeishuBitableAppToken)
	tableID := strings.TrimSpace(cfg.FeishuBitableTableID)
	url := fmt.Sprintf(
		"https://open.feishu.cn/open-apis/bitable/v1/apps/%s/tables/%s/records",
		appToken, tableID,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out feishuBitableCreateResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("create decode http=%d: %w body=%s", resp.StatusCode, err, string(raw))
	}
	if out.Code != 0 {
		return fmt.Errorf("create code=%d msg=%s", out.Code, out.Msg)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("create http=%d body=%s", resp.StatusCode, string(raw))
	}
	return nil
}
