package release

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const cdnManifestBase = "https://cdn.kymjs.com:8843/note"

// manifestCacheTTL 每个平台 manifest 最多每 24 小时向 CDN 发起一次 HTTP 请求。
const manifestCacheTTL = 24 * time.Hour

// manifestRefreshCheckInterval 后台检查是否有 manifest 已过 TTL，需重新拉取。
const manifestRefreshCheckInterval = 1 * time.Hour

// 与客户端 CDN manifest 字段一致。
type manifestWire struct {
	LatestRelease string `json:"latestRelease"`
	LatestVersion string `json:"latestVersion"`
	DownloadURL   string `json:"downloadUrl"`
}

// GateInfo API 强更门禁下发给客户端的信息。
type GateInfo struct {
	MinVersion  string
	DownloadURL string
}

// Provider 从 CDN 拉取各平台 manifest 并缓存 latestVersion（强更下限）。
type Provider struct {
	client *http.Client

	mu    sync.RWMutex
	cache map[string]cachedManifest // key: normalized platform
}

type cachedManifest struct {
	info          GateInfo
	fetchedAt     time.Time // 最近一次成功拉取时间
	lastAttemptAt time.Time // 最近一次 HTTP 尝试（成功或失败），用于限制每日最多请求一次
}

// NewProvider 创建提供者：先同步拉取六平台 manifest，再后台定期检查是否需刷新。
func NewProvider() *Provider {
	p := &Provider{
		client: &http.Client{Timeout: 10 * time.Second},
		cache:  make(map[string]cachedManifest),
	}
	p.refreshAll(context.Background())
	go p.refreshLoop()
	return p
}

// NormalizePlatform 将 X-Platform 规范为 manifest 平台键；iPad 视为 ios。
func NormalizePlatform(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "android":
		return "android"
	case "ios", "ipad", "iphone":
		return "ios"
	case "harmony":
		return "harmony"
	case "windows":
		return "windows"
	case "linux":
		return "linux"
	case "macos", "mac":
		return "macos"
	default:
		return ""
	}
}

func manifestURLForPlatform(platform string) string {
	switch platform {
	case "android":
		return cdnManifestBase + "/android.json"
	case "ios":
		return cdnManifestBase + "/ios.json"
	case "harmony":
		return cdnManifestBase + "/harmony.json"
	case "windows":
		return cdnManifestBase + "/windows.json"
	case "linux":
		return cdnManifestBase + "/linux.json"
	case "macos":
		return cdnManifestBase + "/mac.json"
	default:
		return ""
	}
}

var allManifestPlatforms = []string{
	"android", "ios", "harmony", "windows", "linux", "macos",
}

// GateInfoForPlatform 返回缓存的强更信息；无缓存或 latestVersion 为空时 ok=false。
func (p *Provider) GateInfoForPlatform(platform string) (GateInfo, bool) {
	key := NormalizePlatform(platform)
	if key == "" {
		return GateInfo{}, false
	}
	p.mu.RLock()
	entry, found := p.cache[key]
	p.mu.RUnlock()
	if !found || strings.TrimSpace(entry.info.MinVersion) == "" {
		return GateInfo{}, false
	}
	return entry.info, true
}

func (p *Provider) refreshLoop() {
	ticker := time.NewTicker(manifestRefreshCheckInterval)
	defer ticker.Stop()
	for range ticker.C {
		p.refreshAll(context.Background())
	}
}

func (p *Provider) refreshAll(ctx context.Context) {
	var wg sync.WaitGroup
	for _, platform := range allManifestPlatforms {
		wg.Add(1)
		go func(plat string) {
			defer wg.Done()
			p.refreshPlatform(ctx, plat)
		}(platform)
	}
	wg.Wait()
}

func (p *Provider) refreshPlatform(ctx context.Context, platform string) {
	if p.shouldSkipCDNFetch(platform) {
		return
	}
	url := manifestURLForPlatform(platform)
	if url == "" {
		return
	}
	now := time.Now()
	info, err := fetchGateInfo(ctx, p.client, url)
	if err != nil {
		log.Printf("[release] fetch %s manifest failed: %v", platform, err)
		p.recordFetchAttempt(platform, now, GateInfo{}, false)
		return
	}
	if strings.TrimSpace(info.MinVersion) == "" {
		log.Printf("[release] %s manifest missing latestVersion", platform)
		p.recordFetchAttempt(platform, now, GateInfo{}, false)
		return
	}
	p.recordFetchAttempt(platform, now, info, true)
}

func (p *Provider) shouldSkipCDNFetch(platform string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	entry, found := p.cache[platform]
	if !found {
		return false
	}
	return !entry.lastAttemptAt.IsZero() &&
		time.Since(entry.lastAttemptAt) < manifestCacheTTL
}

func (p *Provider) recordFetchAttempt(platform string, at time.Time, info GateInfo, success bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry := p.cache[platform]
	entry.lastAttemptAt = at
	if success {
		entry.info = info
		entry.fetchedAt = at
	}
	p.cache[platform] = entry
}

func fetchGateInfo(ctx context.Context, client *http.Client, url string) (GateInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return GateInfo{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return GateInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return GateInfo{}, &httpStatusError{code: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return GateInfo{}, err
	}
	var wire manifestWire
	if err := json.Unmarshal(body, &wire); err != nil {
		return GateInfo{}, err
	}
	return GateInfo{
		MinVersion:  strings.TrimSpace(wire.LatestVersion),
		DownloadURL: strings.TrimSpace(wire.DownloadURL),
	}, nil
}

type httpStatusError struct {
	code int
}

func (e *httpStatusError) Error() string {
	return http.StatusText(e.code)
}
