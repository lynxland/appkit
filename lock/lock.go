// Package lock 是分布式锁电池（ADR-0009 D13 蓝图桶，torchwood semaphore
// 同族 + advisory-lock 经验）：TryLocker 端口（非阻塞获取+TTL 过期+
// 唯一持有者释放）与单进程内存实现。分布式形态（Redis SET NX+Lua
// compare-and-del / PG advisory lock）走 TryLocker 适配——torchwood
// pkg/semaphore 的 RedisSemaphore 是现成蓝本。
//
// 语义（torchwood 信号量同款）：
//   - TryAcquire 非阻塞：获取到返回 (Handle, true)；被占返回 (nil, false)。
//   - Handle.Release 只允许持有者释放（compare-and-del：后来的持有者
//     不得释放前任的锁）。
//   - TTL 过期：超时未释放即自动失效（单进程实现惰性清扫；分布式实现
//     靠 KV 原生 TTL）。
//   - 可重入语义明确不做（Redis/PG 原生均不支持；需要时用信号量形态）。
package lock

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Handle 是一次成功获取的锁持有句柄。Release 只允许持有者释放
// （唯一持有者语义：TTL 过期后被他人获取时，旧 Handle 的 Release 是
// 无效操作——不误伤新持有者）。
type Handle interface {
	// Release 释放锁；返回是否真正释放（唯一持有者时 true，已过期或
	// 已被他人获取时 false）。幂等：二次调用返回 false。
	Release() bool
}

// TryLocker 是非阻塞锁端口（key 独立锁语义）。
type TryLocker interface {
	// TryAcquire 尝试获取 key 的锁（TTL 内有效）；获取到返回句柄+true。
	TryAcquire(ctx context.Context, key string, ttl time.Duration) (Handle, bool)
}

// -- 单进程内存实现 --

type memLock struct {
	holder  string    // 持有者 token（唯一释放权）
	expires time.Time // TTL 过期时刻
}

type memHandle struct {
	locker   *MemLocker
	key      string
	holder   string
	released bool
}

// Release 实现 Handle：compare-and-del（holder 匹配才释放）。
func (h *memHandle) Release() bool {
	if h.released {
		return false
	}
	h.released = true
	h.locker.mu.Lock()
	defer h.locker.mu.Unlock()
	if cur, ok := h.locker.locks[h.key]; ok && cur.holder == h.holder {
		delete(h.locker.locks, h.key)
		return true
	}
	return false // 已过期或被他人获取
}

// MemLocker 是单进程实现：map+互斥+惰性清扫（TryAcquire 顺带清理）。
// 分布式部署下各实例独立（非跨进程互斥）——跨进程用 Redis/PG 适配。
type MemLocker struct {
	mu    sync.Mutex
	locks map[string]memLock
	seq   uint64 // 持有者 token 单调计数
}

// NewMemLocker 构造。
func NewMemLocker() *MemLocker { return &MemLocker{locks: map[string]memLock{}} }

// TryAcquire 实现 TryLocker。
func (m *MemLocker) TryAcquire(_ context.Context, key string, ttl time.Duration) (Handle, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	// 惰性清扫：全局条目超阈值时清过期。
	if len(m.locks) > 1024 {
		for k, l := range m.locks {
			if now.After(l.expires) {
				delete(m.locks, k)
			}
		}
	}
	if cur, ok := m.locks[key]; ok && now.Before(cur.expires) {
		return nil, false // 被占且未过期
	}
	m.seq++
	h := &memHandle{locker: m, key: key, holder: fmt.Sprintf("%d", m.seq)}
	m.locks[key] = memLock{holder: h.holder, expires: now.Add(ttl)}
	return h, true
}
