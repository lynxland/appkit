package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/lynx-go/lynx"
	"github.com/lynx-go/lynx-start/outbox"
	"github.com/lynx-go/lynx-start/periodic"
)

// PumpOptions 是泵配置。
type PumpOptions struct {
	// Store 存储（必填）。
	Store outbox.Store
	// Sink 投递端口（必填；进程内用 BusSink，跨进程自带适配）。
	Sink outbox.Sink
	// Interval 轮询间隔（默认 5s；LISTEN/NOTIFY 唤醒是 PG 专属优化，
	// 轮询是可移植基线——torchwood 同款兜底）。
	Interval time.Duration
	// BatchSize 单批最大条数（默认 256，torchwood 实证值）。
	BatchSize int
	// MaxAttempts 重试上限（默认 10，torchwood 实证值；超限死信）。
	MaxAttempts int
	// Logger 日志。
	Logger *slog.Logger
}

// BusSink 把记录投递到进程内事件总线（events.BusPublisher 形态的
// Sink 适配——信封 JSON 反序列化后经 bus.Publish 发布）。
type BusSink struct {
	publish func(ctx context.Context, topic string, payload any) (string, error)
}

// NewBusSink 构造（bus 是 lynx eventbus.Bus）。
func NewBusSink(bus interface {
	Publish(ctx context.Context, topic string, payload any) (string, error)
}) *BusSink {
	return &BusSink{publish: bus.Publish}
}

// Deliver 实现 outbox.Sink。
func (s *BusSink) Deliver(ctx context.Context, records []outbox.Record) error {
	for _, r := range records {
		if _, err := s.publish(ctx, "module."+r.Name[:min(1, len(r.Name))]+topicSuffix(r.Name), r.Payload); err != nil {
			return fmt.Errorf("outbox sink: %s: %w", r.ID, err)
		}
	}
	return nil
}

func topicSuffix(name string) string {
	for i := 0; i < len(name); i++ {
		if name[i] == '.' {
			return name[:i]
		}
	}
	return name
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// NewPump 构造发件箱泵（lynx.Service，直挂 Assembly.Services）。
// 循环骨架经 periodic（预算轮次/WithoutCancel 不腰斩/panic 恢复全继承）。
func NewPump(opts PumpOptions) lynx.Service {
	interval := opts.Interval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	batch := opts.BatchSize
	if batch <= 0 {
		batch = 256
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	return periodic.New("outbox-pump", interval, interval, func(ctx context.Context) error {
		return PumpOnce(ctx, opts.Store, opts.Sink, batch, maxAttempts)
	}, opts.Logger)
}

// pumpOnce 执行一轮：拉取→投递→标记（失败释放重投）。
// PumpOnce 执行一轮（导出：测试与手动运维面）。
func PumpOnce(ctx context.Context, store outbox.Store, sink outbox.Sink, batch, maxAttempts int) error {
	records, err := store.ClaimBatch(ctx, batch, maxAttempts, time.Now().UTC())
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}
	ids := make([]string, len(records))
	for i, r := range records {
		ids[i] = r.ID
	}
	if err := sink.Deliver(ctx, records); err != nil {
		return store.MarkFailed(ctx, ids, time.Now().UTC()) // 释放重投
	}
	return store.MarkPublished(ctx, ids, time.Now().UTC())
}
