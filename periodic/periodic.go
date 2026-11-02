// Package periodic 是周期作业骨架（ADR-0009 D13 考古桶⑧，torchwood
// Periodic 的泛化提取——其 worker 侧 12+ ticker 循环的单一实现，fleetly
// engine.Loop 的 panic 恢复语义并入）：ticker+select 循环、启动即跑/
// 首轮延迟、单轮预算、panic 恢复续跑、宿主电池层位（实现 lynx.Service
// actor 契约，可直挂 Assembly.Services）。
//
// 单轮预算 = context.WithTimeout(context.WithoutCancel(ctx), budget)：
// WithoutCancel 使优雅关停不腰斩在途轮次（轮次在预算边界自然收束，
// 作业幂等由下一轮收敛）；budget 必须 > 0 且短于 interval（关系由
// 调用方测试断言）。fn 返回 error 记 Error 日志（关停引发的失败以
// parent ctx.Err() 为哨兵抑制）；fn panic 记 Error 后继续下轮（单轮
// 自毁不放大为进程死亡——fleetly supervisor 语义的骨架内化）。
package periodic

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/lynxland/lynx"
)

// Periodic 是周期作业。零值不可用，经 New 构造。
type Periodic struct {
	name         string
	interval     time.Duration
	budget       time.Duration
	runAtStart   bool
	initialDelay time.Duration
	logger       *slog.Logger
	fn           func(ctx context.Context) error
}

// New 构造周期作业。name 用于日志前缀；fn 在每轮收到带预算的派生 ctx。
func New(name string, interval, budget time.Duration, fn func(ctx context.Context) error, logger *slog.Logger) *Periodic {
	if interval <= 0 {
		panic("periodic: interval must be positive")
	}
	if budget <= 0 {
		panic("periodic: budget must be positive")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Periodic{name: name, interval: interval, budget: budget, fn: fn, logger: logger}
}

// RunAtStart 启动即跑一轮（重启补齐错过的窗口——幂等作业适用）。
func (p *Periodic) RunAtStart() *Periodic {
	p.runAtStart = true
	return p
}

// After 把首轮推迟 initialDelay（此后仍按 interval 周期）。独占 RunAtStart。
func (p *Periodic) After(initialDelay time.Duration) *Periodic {
	p.initialDelay = initialDelay
	return p
}

// Name 实现 lynx.Service。
func (p *Periodic) Name() string { return p.name }

// Init 实现 lynx.Service（周期作业无初始化动作）。
func (p *Periodic) Init(lynx.AppContext) error { return nil }

// Start 阻塞运行周期循环到 ctx 取消（lynx actor 契约：Start 不得立即返回）。
func (p *Periodic) Start(ctx context.Context) error {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	switch {
	case p.initialDelay > 0:
		delay := time.NewTimer(p.initialDelay)
		defer delay.Stop()
		select {
		case <-ctx.Done():
			p.logger.Info(p.name + " stopped")
			return nil
		case <-delay.C:
			p.runOnce(ctx)
		}
	case p.runAtStart:
		p.runOnce(ctx)
	}

	for {
		select {
		case <-ctx.Done():
			p.logger.Info(p.name + " stopped")
			return nil
		case <-ticker.C:
			p.runOnce(ctx)
		}
	}
}

// Stop 实现 lynx.Service（在途轮次由预算自然收束，无需显式动作）。
func (p *Periodic) Stop(context.Context) error { return nil }

// runOnce 执行单轮：派生预算 ctx、recover panic、抑制关停期失败日志。
func (p *Periodic) runOnce(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error(p.name+" round panicked", "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
		}
	}()
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.budget)
	defer cancel()
	if err := p.fn(rctx); err != nil && ctx.Err() == nil {
		p.logger.Error(p.name+" round failed", "error", err)
	}
}
