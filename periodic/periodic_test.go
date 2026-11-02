package periodic

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

var discard = slog.New(discardHandler{})

type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (discardHandler) WithAttrs([]slog.Attr) slog.Handler        { return discardHandler{} }
func (discardHandler) WithGroup(string) slog.Handler             { return discardHandler{} }

func TestPeriodicTicksAndStops(t *testing.T) {
	var ticks atomic.Int64
	p := New("t1", 20*time.Millisecond, 10*time.Millisecond, func(context.Context) error {
		ticks.Add(1)
		return nil
	}, discard)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = p.Start(ctx) }()
	time.Sleep(120 * time.Millisecond)
	cancel()

	n := ticks.Load()
	if n < 3 {
		t.Fatalf("ticks = %d, want >= 3", n)
	}
	// WithoutCancel 语义（见 TestPeriodicShutdownDoesNotCutInFlight）：cancel
	// 落在 fn 执行窗口内时，在途轮次仍会完成本次 Add——至多 +1；其后调度
	// 停止，计数必须稳定（60ms ≥ 2 个完整间隔，40ms 复核窗）。
	time.Sleep(60 * time.Millisecond)
	after := ticks.Load()
	if after > n+1 {
		t.Fatalf("ticked more than one in-flight round after cancel: %d -> %d", n, after)
	}
	time.Sleep(40 * time.Millisecond)
	if again := ticks.Load(); again != after {
		t.Fatalf("ticks keep growing after cancel: %d -> %d", after, again)
	}
}

// TestPeriodicShutdownDoesNotCutInFlight 关停不腰斩在途轮次：fn 观察到
// 的 ctx 在 parent 取消后仍存活至预算边界（WithoutCancel 语义）。
func TestPeriodicShutdownDoesNotCutInFlight(t *testing.T) {
	inFlightSurvived := make(chan struct{})
	started := make(chan struct{})
	p := New("t2", 50*time.Millisecond, 400*time.Millisecond, func(ctx context.Context) error {
		close(started)
		select {
		case <-ctx.Done(): // 预算到期才会 Done——parent 取消不触发
			return nil
		case <-time.After(200 * time.Millisecond):
			close(inFlightSurvived)
			return nil
		}
	}, discard)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = p.Start(ctx) }()
	<-started
	cancel() // 关停：在途轮继续
	select {
	case <-inFlightSurvived:
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight round was cut by parent cancel")
	}
}

// TestPeriodicPanicRecovers 单轮 panic 不杀循环，下轮照常。
func TestPeriodicPanicRecovers(t *testing.T) {
	var ticks atomic.Int64
	var boom atomic.Bool
	p := New("t3", 20*time.Millisecond, 10*time.Millisecond, func(context.Context) error {
		if ticks.Add(1) == 2 && boom.CompareAndSwap(false, true) {
			panic("round two explodes")
		}
		return nil
	}, discard)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = p.Start(ctx) }()
	time.Sleep(140 * time.Millisecond)
	cancel()
	time.Sleep(30 * time.Millisecond)

	if ticks.Load() < 4 {
		t.Fatalf("loop died after panic: ticks = %d", ticks.Load())
	}
}

// TestPeriodicRunAtStart 启动即跑一轮。
func TestPeriodicRunAtStart(t *testing.T) {
	var ticks atomic.Int64
	p := New("t4", time.Hour, time.Minute, func(context.Context) error {
		ticks.Add(1)
		return nil
	}, discard).RunAtStart()

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = p.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	if ticks.Load() != 1 {
		t.Fatalf("run-at-start ticks = %d, want 1", ticks.Load())
	}
}

// TestPeriodicAfter 首轮延迟：窗口内零 tick，延迟后首轮。
func TestPeriodicAfter(t *testing.T) {
	var ticks atomic.Int64
	p := New("t5", time.Hour, time.Minute, func(context.Context) error {
		ticks.Add(1)
		return nil
	}, discard).After(80 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = p.Start(ctx) }()
	time.Sleep(40 * time.Millisecond)
	if ticks.Load() != 0 {
		t.Fatalf("pre-delay ticks = %d", ticks.Load())
	}
	time.Sleep(100 * time.Millisecond)
	cancel()
	if ticks.Load() != 1 {
		t.Fatalf("post-delay ticks = %d, want 1", ticks.Load())
	}
}

func TestRunnerServiceLifecycle(t *testing.T) {
	entered := make(chan struct{})
	exit := make(chan struct{})
	var r runnerFunc = func(ctx context.Context) error {
		close(entered)
		<-exit
		<-ctx.Done() // 排水语义：Run 在 cancel 后返回
		return nil
	}
	svc := NewRunnerService("r1", r, discard)
	if err := svc.Init(nil); err != nil {
		t.Fatal(err)
	}

	appCtx, appCancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	go func() {
		_ = svc.Start(appCtx)
		close(started)
	}()
	<-entered

	// Stop：取消 + 有界排水——先放行 Run 的 exit，等 Start/Run 收尾。
	close(exit)
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	if err := svc.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	appCancel()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Start did not return after stop")
	}
}

type runnerFunc func(ctx context.Context) error

func (f runnerFunc) Run(ctx context.Context) error { return f(ctx) }
