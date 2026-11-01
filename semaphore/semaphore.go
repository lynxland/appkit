// Package semaphore 是计数信号量电池（ADR-0009 D13 第二梯队，torchwood
// pkg/semaphore 蓝本）：TryAcquire 端口（非阻塞获取一个许可+释放回调），
// 与 lock（互斥语义）同族不同形态——信号量限并发数，锁保唯一性。
// Redis 形态（SET NX 槽位数组 + Lua compare-and-del 防误删他人槽位）
// 走 Semaphore 端口适配——torchwood RedisSemaphore 是现成蓝本。
package semaphore

import "context"

// Semaphore 是计数信号量端口：TryAcquire 非阻塞尝试获取一个许可；
// acquired=true 时返回的 release 必须调用（defer 语义——漏调即泄漏一个
// 许可直到进程重启/TTL 过期[分布式实现]）。
type Semaphore interface {
	TryAcquire(ctx context.Context) (acquired bool, release func(), err error)
}

// InMemory 是单进程实现（chan 缓冲）：构造时固定许可数，获取扣一/释放还一。
type InMemory struct{ ch chan struct{} }

// NewInMemory 构造 permits 个许可的信号量（必须 > 0）。
func NewInMemory(permits int) *InMemory {
	return &InMemory{ch: make(chan struct{}, permits)}
}

// TryAcquire 实现 Semaphore（非阻塞：缓冲满即 false）。
func (s *InMemory) TryAcquire(context.Context) (bool, func(), error) {
	select {
	case s.ch <- struct{}{}:
		return true, func() { <-s.ch }, nil
	default:
		return false, nil, nil
	}
}

// Noop 永远获取成功且释放无操作——限流关闭/降级路径的显式哨兵实现。
type Noop struct{}

// TryAcquire 实现 Semaphore。
func (Noop) TryAcquire(context.Context) (bool, func(), error) { return true, func() {}, nil }
