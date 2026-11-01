package ratelimit

import (
	"sync"
	"time"
)

// SlidingWindow 是滑动窗口内存实现：窗口期内精确计数（每 key 时间戳
// 环），解决固定窗口的边界突刺（两个窗口交界可双倍放行）。内存与
// limit 同阶（每 key 至多 limit 个时间戳）；惰性清扫。
type SlidingWindow struct {
	mu     sync.Mutex
	keys   map[string][]time.Time
	limit  int64
	window time.Duration
}

// NewSlidingWindow 构造：limit 与 window 必须 >0（如 60 / time.Minute）。
func NewSlidingWindow(limit int64, window time.Duration) *SlidingWindow {
	return &SlidingWindow{keys: map[string][]time.Time{}, limit: limit, window: window}
}

// Allow 裁决并计数（allowed 时记录本次时间戳）。
func (s *SlidingWindow) Allow(key string, now time.Time) Decision {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.keys) > 1024 {
		s.sweepLocked(now)
	}
	ts := prune(s.keys[key], now.Add(-s.window))
	if int64(len(ts)) < s.limit {
		s.keys[key] = append(ts, now)
		return Decision{Allowed: true, Limit: s.limit, Remaining: s.limit - int64(len(ts)) - 1}
	}
	// 最早一条滑出窗口的时间即恢复点。
	retryAfter := s.window - now.Sub(ts[0])
	if retryAfter < 0 {
		retryAfter = 0
	}
	s.keys[key] = ts
	return Decision{Allowed: false, RetryAfter: retryAfter, Limit: s.limit, Remaining: 0}
}

func prune(ts []time.Time, cutoff time.Time) []time.Time {
	for len(ts) > 0 && ts[0].Before(cutoff) {
		ts = ts[1:]
	}
	return ts
}

func (s *SlidingWindow) sweepLocked(now time.Time) {
	cutoff := now.Add(-s.window)
	for k, ts := range s.keys {
		ts = prune(ts, cutoff)
		if len(ts) == 0 {
			delete(s.keys, k)
		} else {
			s.keys[k] = ts
		}
	}
}
