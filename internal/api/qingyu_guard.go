package api

import (
	"net/http"
	"sync"
	"time"
)

// 轻羽 WebDAV 下发：按用户限流 + 短时结果缓存，避免海量客户端同时冷启动打爆 DB。
// 数值可通过环境调整时，可再接到 config；当前保持简单默认可运营。

const (
	qingyuWebDAVCacheTTL     = 45 * time.Second
	qingyuMaxReqPerWindow    = 60
	qingyuRateWindow         = time.Minute
)

type qingyuWebDAVGuard struct {
	mu       sync.Mutex
	rate     map[int64][]time.Time
	cache    map[int64]qingyuWebDAVCacheEntry
}

type qingyuWebDAVCacheEntry struct {
	resp  webdavResp
	until time.Time
}

func newQingyuWebDAVGuard() *qingyuWebDAVGuard {
	return &qingyuWebDAVGuard{
		rate:  make(map[int64][]time.Time),
		cache: make(map[int64]qingyuWebDAVCacheEntry),
	}
}

func (g *qingyuWebDAVGuard) allow(uid int64, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	cutoff := now.Add(-qingyuRateWindow)
	times := g.rate[uid]
	kept := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= qingyuMaxReqPerWindow {
		g.rate[uid] = kept
		return false
	}
	kept = append(kept, now)
	g.rate[uid] = kept
	return true
}

func (g *qingyuWebDAVGuard) getCached(uid int64, now time.Time) (webdavResp, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	e, ok := g.cache[uid]
	if !ok || !now.Before(e.until) {
		return webdavResp{}, false
	}
	return e.resp, true
}

func (g *qingyuWebDAVGuard) setCached(uid int64, resp webdavResp, now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cache[uid] = qingyuWebDAVCacheEntry{resp: resp, until: now.Add(qingyuWebDAVCacheTTL)}
}

func writeTooManyRequests(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "30")
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
}
