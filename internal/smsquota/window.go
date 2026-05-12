// smsquota 包提供进程内、滑动时间窗的计数器，用于公开短信接口发送前的占位与回滚。
//
// 设计要点（详见仓库 server/TECHNICAL.md §2.11）：
//   - 窗口默认 24h，与 api.smsPublicQuotaPerWindow（每 key 最大次数）组合使用。
//   - BeginSend 对传入的多个 key 原子地「先全部检查再全部 +1」；任一侧已满则整笔失败且不占位。
//   - 返回的 undo 会按 key 列表各 pop 掉最后一次追加的时间戳；须在下游（如阿里云）失败时调用。
//   - 存储为内存 map，多副本部署时每进程独立计数；要全局一致需外置 Redis 等。
//   - 空字符串 key 会被跳过（不计入、不占位），便于调用方统一传三元组。
package smsquota

import (
	"sync"
	"time"
)

// Window 为每个逻辑 key 维护一条发送时间序列；仅保留窗口内的记录。
type Window struct {
	mu     sync.Mutex
	limit  int           // 每个 key 在 window 内允许的最大事件数
	window time.Duration // 滑动窗口长度
	events map[string][]time.Time
}

// New 构造计数器。limit 最小为 1；window 若 <=0 则强制为 24h。
func New(limit int, window time.Duration) *Window {
	if limit < 1 {
		limit = 1
	}
	if window <= 0 {
		window = 24 * time.Hour
	}
	return &Window{
		limit:  limit,
		window: window,
		events: make(map[string][]time.Time),
	}
}

func (w *Window) pruneKey(key string, cutoff time.Time) {
	ts := w.events[key]
	i := 0
	for _, t := range ts {
		if t.After(cutoff) {
			ts[i] = t
			i++
		}
	}
	ts = ts[:i]
	if len(ts) == 0 {
		delete(w.events, key)
	} else {
		w.events[key] = ts
	}
}

// BeginSend 对 keys 中每个非空 key：先剪掉窗口外时间戳，再若已有条数 >= limit 则整笔失败（不占位）。
// 否则对每个 key append 当前时间。返回 ok=false 时 undo 为 nil；ok=true 时必须保留 undo 直至发送成功或调用 undo。
func (w *Window) BeginSend(keys []string) (ok bool, undo func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-w.window)
	for _, k := range keys {
		if k == "" {
			continue
		}
		w.pruneKey(k, cutoff)
		if len(w.events[k]) >= w.limit {
			return false, nil
		}
	}
	for _, k := range keys {
		if k == "" {
			continue
		}
		w.events[k] = append(w.events[k], now)
	}
	return true, func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		for _, k := range keys {
			if k == "" {
				continue
			}
			arr := w.events[k]
			if len(arr) == 0 {
				continue
			}
			w.events[k] = arr[:len(arr)-1]
			if len(w.events[k]) == 0 {
				delete(w.events, k)
			}
		}
	}
}
