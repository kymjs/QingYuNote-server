package api

import (
	"sync"
	"time"
)

const (
	appMarketReportLimit  = 10
	appMarketReportWindow = time.Minute
)

type appMarketReportGuard struct {
	mu   sync.Mutex
	rate map[string][]time.Time
}

func newAppMarketReportGuard() *appMarketReportGuard {
	return &appMarketReportGuard{rate: make(map[string][]time.Time)}
}

func (g *appMarketReportGuard) allow(ip string, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	cutoff := now.Add(-appMarketReportWindow)
	times := g.rate[ip]
	kept := times[:0]
	for _, at := range times {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) >= appMarketReportLimit {
		g.rate[ip] = kept
		return false
	}
	g.rate[ip] = append(kept, now)
	return true
}
