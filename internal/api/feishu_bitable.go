package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
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
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

var (
	feishuTokenMu     sync.Mutex
	feishuTokenCached string
	feishuTokenExpiry time.Time
)

func maskFeishuID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "(empty)"
	}
	r := []rune(s)
	n := len(r)
	if n <= 8 {
		return string(r[:1]) + "***"
	}
	return string(r[:4]) + "***" + string(r[n-4:])
}

func fieldKeysSorted(fields map[string]any) []string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func truncateForLog(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("…(%d bytes total)", len(s))
}

func feishuTenantAccessToken(cfg *config.Config) (string, error) {
	feishuTokenMu.Lock()
	defer feishuTokenMu.Unlock()
	appID := strings.TrimSpace(cfg.FeishuBitableAppID)
	if feishuTokenCached != "" && time.Now().Before(feishuTokenExpiry) {
		log.Printf("info: feedback bitable token cache hit app_id=%s expires_in=%s",
			maskFeishuID(appID), time.Until(feishuTokenExpiry).Round(time.Second))
		return feishuTokenCached, nil
	}
	body, err := json.Marshal(map[string]string{
		"app_id":     appID,
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
		return "", fmt.Errorf("token decode http=%d: %w body=%s", resp.StatusCode, err, truncateForLog(string(raw), 512))
	}
	if out.Code != 0 || strings.TrimSpace(out.TenantAccessToken) == "" {
		return "", fmt.Errorf("token code=%d msg=%s http=%d body=%s", out.Code, out.Msg, resp.StatusCode, truncateForLog(string(raw), 512))
	}
	feishuTokenCached = out.TenantAccessToken
	// 提前 5 分钟过期，避免临界失效。
	sec := out.Expire
	if sec <= 0 {
		sec = 7200
	}
	feishuTokenExpiry = time.Now().Add(time.Duration(sec-300) * time.Second)
	log.Printf("info: feedback bitable token refreshed app_id=%s expire=%ds http=%d",
		maskFeishuID(appID), sec, resp.StatusCode)
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

// feishuBitableGET 用同一 token 探测表元数据，帮助区分「token 无效」与「无写权限」。
func feishuBitableGET(ctx context.Context, token, url string) (httpStatus int, code int, msg string, body string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, 0, "", "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out.Code, out.Msg, string(raw), nil
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
	fieldMode := strings.TrimSpace(cfg.FeishuBitableFieldMode)
	if fieldMode == "" {
		fieldMode = "name"
	}
	url := fmt.Sprintf(
		"https://open.feishu.cn/open-apis/bitable/v1/apps/%s/tables/%s/records",
		appToken, tableID,
	)
	log.Printf("info: feedback bitable create begin app_id=%s app_token=%s table_id=%s field_mode=%s field_keys=%v user_id=%d content_runes=%d payload_bytes=%d url=%s",
		maskFeishuID(cfg.FeishuBitableAppID),
		maskFeishuID(appToken),
		maskFeishuID(tableID),
		fieldMode,
		fieldKeysSorted(fields),
		userID,
		len([]rune(content)),
		len(payload),
		url,
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
	bodyStr := truncateForLog(string(raw), 2048)
	var out feishuBitableCreateResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("create decode http=%d: %w body=%s", resp.StatusCode, err, bodyStr)
	}
	if out.Code != 0 {
		// 失败时探测：能否读表元数据 / 字段列表（帮助区分协作者缺失 vs 仅缺写权限 vs token/表 ID 错）。
		probeCtx, probeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer probeCancel()
		metaURL := fmt.Sprintf("https://open.feishu.cn/open-apis/bitable/v1/apps/%s/tables/%s", appToken, tableID)
		fieldsURL := fmt.Sprintf("https://open.feishu.cn/open-apis/bitable/v1/apps/%s/tables/%s/fields", appToken, tableID)
		mh, mc, mm, mb, merr := feishuBitableGET(probeCtx, token, metaURL)
		fh, fc, fm, fb, ferr := feishuBitableGET(probeCtx, token, fieldsURL)
		metaPart := "meta_probe=err"
		if merr == nil {
			metaPart = fmt.Sprintf("meta_probe http=%d code=%d msg=%s body=%s", mh, mc, mm, truncateForLog(mb, 512))
		} else {
			metaPart = fmt.Sprintf("meta_probe err=%v", merr)
		}
		fieldsPart := "fields_probe=err"
		if ferr == nil {
			fieldsPart = fmt.Sprintf("fields_probe http=%d code=%d msg=%s body=%s", fh, fc, fm, truncateForLog(fb, 1024))
		} else {
			fieldsPart = fmt.Sprintf("fields_probe err=%v", ferr)
		}
		log.Printf("warning: feedback bitable create denied code=%d msg=%s http=%d body=%s %s %s",
			out.Code, out.Msg, resp.StatusCode, bodyStr, metaPart, fieldsPart)
		return fmt.Errorf("create code=%d msg=%s http=%d body=%s; %s; %s",
			out.Code, out.Msg, resp.StatusCode, bodyStr, metaPart, fieldsPart)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("create http=%d body=%s", resp.StatusCode, bodyStr)
	}
	log.Printf("info: feedback bitable create ok app_token=%s table_id=%s user_id=%d",
		maskFeishuID(appToken), maskFeishuID(tableID), userID)
	return nil
}
