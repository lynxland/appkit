// Package outbox 定义事务性发件箱的形状与语义（ADR-0009 D12 数据访问
// 电池族的收官件，证据 2×：torchwood PG LISTEN/NOTIFY+SKIP LOCKED+死信/
// 重领/保留全套、fleetly SQLite 事实流+SSE）。形状层 ORM/存储中立：
// PG/bun 实现见 contrib/bunx/outbox（uow/buntx 同模式）。
//
// # 事务性发件箱（Transactional Outbox）
//
// 业务写与事件发布在同一事务落库（见 uow 包的四件一拍定典）；泵
// （Pump，由 periodic 驱动）拉取待发记录投递到 Sink（进程内 bus 或跨
// 进程 transport），成功即标记。**at-least-once**：崩溃窗口内可能重复
// 投递——消费者必须幂等（事件信封已带 ID）。
//
// 四件一拍（D12 定典，fleetly ADR-0005 泛化）：状态 CAS + tombstone +
// outbox 事件 + 审计，同一 uow.Runner 事务内落库。缺任何一件即部分
// 提交——事务边界外的「事后补发」是本模式要消灭的形态。
package outbox

import (
	"context"
	"encoding/json"
	"time"

	"github.com/lynxland/appkit/events"
)

// Record 是发件箱的存储形态（dialect 中立；PG/bun 表结构见
// contrib/bunx/outbox 的迁移 SQL）。事件信封（events.Envelope）序列化
// 到 Payload；Name/Aggregate 提升为列以便按轴查询（fleetly 同轴过滤）。
type Record struct {
	// ID 是事件 ID（与 events.Envelope.ID 一致——消费者幂等去重的键）。
	ID string
	// Name 是事件名（目录形如 "module.noun.verb"；列级存储便于按轴订阅）。
	Name string
	// Aggregate 是可选聚合轴（如 "organization/<id>"；fleetly team 轴对应）。
	Aggregate string
	// Payload 是序列化信封（JSON events.Envelope——信封格式是跨模块
	// 传输的稳定面，消费侧用 Envelope.ID 幂等去重）。
	Payload []byte
	// CreatedAt 是入箱时间（保留窗口与延迟监控用）。
	CreatedAt time.Time
}

// Sink 是投递端口（transport 可插）：接收一批待发记录并投递。实现
// 必须幂等安全（at-least-once 语义下可能收到重复）。进程内默认实现
// 用 events.BusPublisher（内存 bus）；跨进程走 watermill/Redis 适配。
type Sink interface {
	Deliver(ctx context.Context, records []Record) error
}

// Store 是存储端口（dialect 可插）：同事务追加 + 泵拉取/标记。实现
// 见 contrib/bunx/outbox.Store（PG）。
type Store interface {
	// Append 在调用方事务内写入一条事件记录（四件一拍——必须在
	// uow.Runner 的 ctx 内调用，与业务写共享事务）。
	Append(ctx context.Context, r Record) error
	// ClaimBatch 拉取一批待发记录并标记为在途（FOR UPDATE SKIP LOCKED
	// 语义——多泵并发安全）。返回空批 = 无待发。
	ClaimBatch(ctx context.Context, batchSize int, maxAttempts int, now time.Time) ([]Record, error)
	// MarkPublished 标记一批为已投递（at-least-once：Sink 成功后调用）。
	MarkPublished(ctx context.Context, ids []string, now time.Time) error
	// MarkFailed 递增重试计数；超限即标记死信（不再重投）。
	MarkFailed(ctx context.Context, ids []string, now time.Time) error
}

// Publisher 实现 events.Publisher——业务层零改动：注入 Deps.Events 即
// 把进程内发布替换为事务性发件箱（同事务写入 Store，由泵异步投递）。
type Publisher struct {
	store Store
	now   func() time.Time
}

// NewPublisher 构造发件箱发布器（Store 必须非空）。
func NewPublisher(store Store) *Publisher {
	return &Publisher{store: store, now: time.Now}
}

// Publish 实现 events.Publisher：信封校验后在调用方事务内追加一条
// 发件箱记录。Payload 是信封 JSON（消费侧 Envelope.ID 幂等去重）。
func (p *Publisher) Publish(ctx context.Context, env events.Envelope) error {
	if err := env.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return p.store.Append(ctx, Record{
		ID:        env.ID,
		Name:      env.Name,
		Aggregate: env.Aggregate,
		Payload:   payload,
		CreatedAt: p.now(),
	})
}
