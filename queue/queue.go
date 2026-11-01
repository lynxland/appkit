// Package queue 是 at-least-once 任务队列端口（ADR-2024 D13 第二梯队，
// torchwood Queue 端口[Redis Stream XREADGROUP]的形态泛化）：
// Enqueue/Dequeue/Ack/Fail 四操作 + 单进程内存实现。Redis Stream/
// watermill 适配走 Queue 端口自带（首个消费者时动工——D13 触发器）。
//
// 语义：
//   - at-least-once：Dequeue 后未 Ack 的消息在 visibility-timeout 后
//     可被再次 Dequeue（redelivery）——消费者必须幂等。
//   - Ack 确认处理完成；Fail 显式退回（立即或延迟重投由实现策略决定）。
//   - Trim 清理已完成消息（保留窗口语义由实现决定；内存实现即清空）。
package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrEmpty 是队列空哨兵（Dequeue 无消息时的非错误信号）。
var ErrEmpty = errors.New("queue: empty")

// Message 是一条队列消息。
type Message struct {
	ID       string
	Topic    string
	Payload  []byte
	Attempts int
}

// Queue 是 at-least-once 任务队列端口。
type Queue interface {
	// Enqueue 入队（立即对 Dequeue 可见）。
	Enqueue(ctx context.Context, topic string, payload []byte) (string, error)
	// Dequeue 按 topic 拉取一条；无消息返回 ErrEmpty。
	// visibilityTimeout 内未 Ack 的消息可被再次拉取（redelivery）。
	Dequeue(ctx context.Context, topic string, visibilityTimeout time.Duration) (Message, error)
	// Ack 确认完成（消息从队列移除）。
	Ack(ctx context.Context, id string) error
	// Fail 显式退回（消息立即可被再次拉取）。
	Fail(ctx context.Context, id string) error
}

// MemQueue 是单进程内存实现（测试/开发/轻量内嵌；分布式用 Redis/
// watermill 适配）。
type MemQueue struct {
	mu       sync.Mutex
	seq      int
	pending  map[string]*memMsg // ID → 待处理
	inFlight map[string]*memMsg // ID → 已拉取未 Ack
}

type memMsg struct {
	Message
	claimedAt time.Time
	visibleAt time.Time     // 可见时刻（Fail/超时后回退）
	timeout   time.Duration // 领取时的 visibility timeout（超时判定用原值）
}

// NewMemQueue 构造。
func NewMemQueue() *MemQueue {
	return &MemQueue{pending: map[string]*memMsg{}, inFlight: map[string]*memMsg{}}
}

// Enqueue 实现 Queue。
func (q *MemQueue) Enqueue(_ context.Context, topic string, payload []byte) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.seq++
	id := fmt.Sprintf("m%d", q.seq)
	now := time.Now().UTC()
	q.pending[id] = &memMsg{Message: Message{ID: id, Topic: topic, Payload: payload}, visibleAt: now}
	return id, nil
}

// Dequeue 实现 Queue：按 FIFO 拉取最早可见的消息；in-flight 消息的
// visibilityTimeout 到期后自动回退为可拉取。
func (q *MemQueue) Dequeue(_ context.Context, topic string, visibilityTimeout time.Duration) (Message, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now().UTC()

	// 超时回退（用领取时的原 timeout——后续 Dequeue 的新值不追溯生效）。
	for id, m := range q.inFlight {
		if now.Sub(m.claimedAt) > m.timeout {
			m.visibleAt = now
			m.Attempts++
			q.pending[id] = m
			delete(q.inFlight, id)
		}
	}

	// FIFO 扫描同 topic 最早可见。
	var bestID string
	var bestAt time.Time
	for id, m := range q.pending {
		if m.Topic != topic || now.Before(m.visibleAt) {
			continue
		}
		if bestID == "" || m.visibleAt.Before(bestAt) {
			bestID, bestAt = id, m.visibleAt
		}
	}
	if bestID == "" {
		return Message{}, ErrEmpty
	}
	m := q.pending[bestID]
	delete(q.pending, bestID)
	m.claimedAt = now
	m.timeout = visibilityTimeout
	q.inFlight[bestID] = m
	return m.Message, nil
}

// Ack 实现 Queue。
func (q *MemQueue) Ack(_ context.Context, id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.pending, id)
	delete(q.inFlight, id)
	return nil
}

// Fail 实现 Queue（立即可再拉取）。
func (q *MemQueue) Fail(_ context.Context, id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if m, ok := q.inFlight[id]; ok {
		m.visibleAt = time.Now().UTC()
		m.Attempts++
		q.pending[id] = m
		delete(q.inFlight, id)
	}
	return nil
}

// Len 返回待处理+在途消息数（测试辅助）。
func (q *MemQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending) + len(q.inFlight)
}
