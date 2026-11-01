package ratelimit

import (
	"sync"
	"time"
)

// TokenBucket 是令牌桶内存实现：每 key 独立桶，容量 burst、按
// refillPerSecond 平滑补充。突发友好（首拍可用满 burst），长期均值
// 收敛到补充速率；惰性清扫（无后台协程）。
type TokenBucket struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	burst   int64
	refill  float64 // tokens per second
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewTokenBucket 构造：burst 必须 >0，refillPerSecond 必须 >0
// （常见取 limit/window，如 60/60s=1/s——先满桶再平滑补）。
func NewTokenBucket(burst int64, refillPerSecond float64) *TokenBucket {
	return &TokenBucket{
		buckets: map[string]*bucket{},
		burst:   burst,
		refill:  refillPerSecond,
	}
}

// Allow 裁决并扣减（allowed 时扣 1 token）。
func (t *TokenBucket) Allow(key string, now time.Time) Decision {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.buckets) > 1024 {
		t.sweepLocked(now)
	}
	b, ok := t.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(t.burst), last: now}
		t.buckets[key] = b
	} else if d := now.Sub(b.last); d > 0 {
		b.tokens += d.Seconds() * t.refill
		if b.tokens > float64(t.burst) {
			b.tokens = float64(t.burst)
		}
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return Decision{Allowed: true, Limit: t.burst, Remaining: int64(b.tokens)}
	}
	// 补到 1 token 的等待时间。
	deficit := 1 - b.tokens
	return Decision{
		Allowed:    false,
		RetryAfter: time.Duration(deficit / t.refill * float64(time.Second)),
		Limit:      t.burst,
		Remaining:  0,
	}
}

// sweepLocked 清扫超过一个补满周期未见活动的桶（省略精确判定：
// 1 小时未见即回收——桶重建成本为零拍满桶，语义无损）。
func (t *TokenBucket) sweepLocked(now time.Time) {
	idle := time.Hour
	for k, b := range t.buckets {
		if now.Sub(b.last) > idle {
			delete(t.buckets, k)
		}
	}
}
