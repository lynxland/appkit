package periodic

import (
	"context"
	"log/slog"
	"sync"

	"github.com/lynx-go/lynx"
)

// Runner 是可由 RunnerService 驱动的后台消费端口最小形态
// （torchwood bootkit.Runner 的泛化：RealtimeFanout、OutboxWorker 等
// Run(ctx) error 型循环）。
type Runner interface {
	Run(ctx context.Context) error
}

// RunnerService 把 Run(ctx) error 型后台循环包成 lynx.Service actor：
// Start 起协程跑 Run 并阻塞到应用上下文取消（lynx 把 Start 作为
// run.Group actor），Stop 取消后等当前批处理完再退（有界排水）。
type RunnerService struct {
	name   string
	runner Runner
	logger *slog.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewRunnerService 构造后台消费服务壳。
func NewRunnerService(name string, runner Runner, logger *slog.Logger) *RunnerService {
	if logger == nil {
		logger = slog.Default()
	}
	return &RunnerService{name: name, runner: runner, logger: logger}
}

// Name 实现 lynx.Service。
func (s *RunnerService) Name() string { return s.name }

// Init 实现 lynx.Service。
func (s *RunnerService) Init(lynx.AppContext) error { return nil }

// Start 实现 lynx.Service：必须阻塞到应用上下文取消。
func (s *RunnerService) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.runner.Run(runCtx); err != nil && ctx.Err() == nil {
			s.logger.Error(s.name+" stopped with error", "error", err)
		}
	}()

	<-ctx.Done()
	return nil
}

// Stop 实现 lynx.Service：取消 Run 并有界排水（等当前批收尾或 Stop ctx 到期）。
func (s *RunnerService) Stop(ctx context.Context) error {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
	s.logger.Info(s.name + " stopped")
	return nil
}
