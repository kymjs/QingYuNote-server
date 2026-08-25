package appmarket

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kymjs/noteapi/internal/store"
)

const (
	huaweiWebEdgeBase  = "https://web-drcn.hispace.dbankcloud.com/edge"
	harmonyPackageName = "com.kymjs.note"
)

// FetchHarmonyLatestVersion 查询 AppGallery 网页使用的详情接口。请求目标和包名均固定，
// 不接受外部输入，避免客户端借此间接访问任意地址。
func FetchHarmonyLatestVersion(ctx context.Context) (string, error) {
	identity, err := randomID()
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	interfaceCode, cookie, err := bootstrapInterfaceCode(ctx, client, identity)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(map[string]any{
		"pageNum":       1,
		"pageSize":      100,
		"pageId":        "webAgPackage|" + harmonyPackageName,
		"clientPackage": "com.huawei.hmsapp.appgallery",
		"accountZone":   "CN",
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, huaweiWebEdgeBase+"/harmony/page-detail", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	setWebEdgeHeaders(req, identity, interfaceCode, cookie)
	req.Header.Set("Net-Msg-Id", mustRandomID())
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("appgallery_status_%d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if len(raw) == 2<<20 {
		return "", fmt.Errorf("appgallery_response_too_large")
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		return "", fmt.Errorf("decode_appgallery: %w", err)
	}
	version := findVersion(document)
	if !store.IsSemanticVersion(version) {
		return "", fmt.Errorf("appgallery_version_missing_or_invalid")
	}
	return version, nil
}

func bootstrapInterfaceCode(ctx context.Context, client *http.Client, identity string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, huaweiWebEdgeBase+"/webedge/getInterfaceCode",
		strings.NewReader(`{"params":{}}`))
	if err != nil {
		return "", "", err
	}
	setWebEdgeHeaders(req, identity, "null", "")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("appgallery_interface_code_status_%d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", "", err
	}
	var code string
	if err := json.Unmarshal(raw, &code); err != nil || strings.TrimSpace(code) == "" {
		return "", "", fmt.Errorf("appgallery_interface_code_invalid")
	}
	cookies := resp.Cookies()
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return code, strings.Join(parts, "; "), nil
}

func setWebEdgeHeaders(req *http.Request, identity, code, cookie string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://appgallery.huawei.com")
	req.Header.Set("Referer", "https://appgallery.huawei.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; QingyuVersionSync/1.0)")
	req.Header.Set("Identity-Id", identity)
	req.Header.Set("Interface-Code", code+"_"+fmt.Sprintf("%d", time.Now().UnixMilli()))
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func mustRandomID() string {
	value, err := randomID()
	if err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return value
}

func findVersion(value any) string {
	root, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	// Web Edge page-detail 在部分版本直接返回 packageName / version。
	if packageName, _ := root["packageName"].(string); packageName == harmonyPackageName {
		if version, _ := root["version"].(string); store.IsSemanticVersion(version) {
			return strings.TrimSpace(version)
		}
	}
	// 网页 detailappinfocard 卡片中的 version 是当前前端使用的版本字段。
	return findDetailCardVersion(root)
}

func findDetailCardVersion(value any) string {
	switch item := value.(type) {
	case map[string]any:
		if layoutName, _ := item["layoutName"].(string); layoutName == "detailappinfocard" {
			if list, ok := item["dataList"].([]any); ok && len(list) > 0 {
				if card, ok := list[0].(map[string]any); ok {
					if version, _ := card["version"].(string); store.IsSemanticVersion(version) {
						return strings.TrimSpace(version)
					}
				}
			}
		}
		for _, child := range item {
			if version := findDetailCardVersion(child); version != "" {
				return version
			}
		}
	case []any:
		for _, child := range item {
			if version := findDetailCardVersion(child); version != "" {
				return version
			}
		}
	}
	return ""
}
