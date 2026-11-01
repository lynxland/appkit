// Package cache 是缓存端点电池（ADR-0009 D13 蓝图桶，web 基建标配，
// 首个消费者=本批）：Cache 端口（Get/Set/Delete+TTL）+ singleflight
// 防击透（Loader 语义：并发同 Key 回源只执行一次）+ 单进程内存实现。
// Redis 适配走 Cache 端口自带实现（首个分布式消费者时动工——
// D13 触发器纪律）。
//
// 语义：
//   - Get 命中返回值字节；未命中/过期返回 ErrNotFound（调用方回源）。
//   - Set 带 TTL（<=0 永不过期；正洁癖者可传 cache.NoTTL）。
//   - LoadOrStore：singleflight——并发同 Key 回源合并为一次 loader
//     调用（防缓存击透：热点 Key 失效瞬间不压垮后端）。
//   - Delete 幂等（不存在不报错）。
package cache

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrNotFound 是未命中哨兵（Get 未命中与 LoadOrStore 的 loader 未命中
// 共用；调用方按回源信号处理）。
var ErrNotFound = errors.New("cache: key not found")

// NoTTL 是永不过期标记（Set 的 ttl 参数传 0 即此语义）。
const NoTTL = time.Duration(0)

// Cache 是缓存端口（键→字节值；调用方自选序列化——JSON/proto/裸文本）。
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
}

// Loader 是回源函数：返回值写入缓存；返回 ErrNotFound 则不写
// （负缓存属调用方策略，由调用方自行 Set 空值实现）。
type Loader func(ctx context.Context) ([]byte, error)

// MemCache 是单进程内存实现：map+RWMutex+惰性过期+singleflight。
type MemCache struct {
	mu    sync.RWMutex
	items map[string]item
	// singleflight：in-flight loader 按 key 去重。
	flightMu sync.Mutex
	flights  map[string]*flight
}

type item struct {
	value    []byte
	expires  time.Time // zero = 永不过期
	hasValue bool
}

type flight struct {
	done  chan struct{}
	value []byte
	err   error
}

// NewMemCache 构造。
func NewMemCache() *MemCache {
	return &MemCache{items: map[string]item{}, flights: map[string]*flight{}}
}

// Get 实现 Cache。
func (c *MemCache) Get(_ context.Context, key string) ([]byte, error) {
	c.mu.RLock()
	it, ok := c.items[key]
	c.mu.RUnlock()
	if !ok || (it.hasValue && !it.expires.IsZero() && time.Now().After(it.expires)) {
		return nil, ErrNotFound
	}
	if !it.hasValue {
		return nil, ErrNotFound
	}
	return it.value, nil
}

// Set 实现 Cache。
func (c *MemCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	var expires time.Time
	if ttl > 0 {
		expires = time.Now().Add(ttl)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) > 4096 { // 惰性清扫
		now := time.Now()
		for k, it := range c.items {
			if it.hasValue && !it.expires.IsZero() && now.After(it.expires) {
				delete(c.items, k)
			}
		}
	}
	c.items[key] = item{value: value, expires: expires, hasValue: true}
	return nil
}

// Delete 实现 Cache。
func (c *MemCache) Delete(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range keys {
		delete(c.items, k)
	}
	return nil
}

// LoadOrStore 命中即返回；未命中则 loader 回源（并发同 Key 合并为一次
// loader 调用——singleflight 防击透），成功后写缓存。
// loader 返回 ErrNotFound 时不写缓存（穿透属正常回源）。
func (c *MemCache) LoadOrStore(ctx context.Context, key string, ttl time.Duration, loader Loader) ([]byte, error) {
	if v, err := c.Get(ctx, key); err == nil {
		return v, nil
	}

	c.flightMu.Lock()
	if f, ok := c.flights[key]; ok { // 并发等待者：等首个 loader 结果
		c.flightMu.Unlock()
		select {
		case <-f.done:
			return f.value, f.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f := &flight{done: make(chan struct{})}
	c.flights[key] = f
	c.flightMu.Unlock()

	f.value, f.err = loader(ctx)
	close(f.done)

	c.flightMu.Lock()
	delete(c.flights, key)
	c.flightMu.Unlock()

	if f.err != nil {
		return nil, f.err
	}
	if err := c.Set(ctx, key, f.value, ttl); err != nil {
		return nil, err
	}
	return f.value, nil
}
