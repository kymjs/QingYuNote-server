package appmarket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kymjs/noteapi/internal/store"
)

const iosAppStoreURL = "https://itunes.apple.com/cn/lookup?id=6764479359"

// FetchIOSLatestVersion 从固定 iTunes Lookup JSON 的 version 字段提取版本。
// 任意网络、响应结构或版本格式异常都会返回错误，调用方不得推进本地记录或写 WebDAV。
func FetchIOSLatestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, iosAppStoreURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; QingyuVersionSync/1.0)")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("appstore_status_%d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if len(raw) == 2<<20 {
		return "", fmt.Errorf("appstore_response_too_large")
	}
	return parseIOSVersion(raw)
}

func parseIOSVersion(payload []byte) (string, error) {
	var response struct {
		ResultCount int `json:"resultCount"`
		Results     []struct {
			Version string `json:"version"`
		} `json:"results"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return "", fmt.Errorf("decode_appstore: %w", err)
	}
	if response.ResultCount < 1 || len(response.Results) == 0 || !store.IsSemanticVersion(response.Results[0].Version) {
		return "", fmt.Errorf("appstore_version_missing_or_invalid")
	}
	return response.Results[0].Version, nil
}
