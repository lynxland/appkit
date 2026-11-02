package testutil

import (
	"context"
	"sync"

	"github.com/lynxland/appkit/events"
	"github.com/lynxland/appkit/uow"
)

// 编译期断言:fake 形状与端口一致。
var (
	_ events.Publisher = (*CapturePublisher)(nil)
	_ uow.Runner       = InlineRunner{}
)

// CapturePublisher 是 events.Publisher 的测试 fake:记录全部发布的事件。
// 与 BusPublisher 同语义——信封不合法即拒绝(事件目录约定护栏;手抄 fake
// 漏 Validate 曾让非法事件名在测试期漏网,见 ADR-0005)。Fail 置 true 时
// Publish 返回错误且不记录(测错误路径)。
type CapturePublisher struct {
	Fail bool

	mu   sync.Mutex
	evts []events.Envelope
}

// NewCapturePublisher 构造空记录器。
func NewCapturePublisher() *CapturePublisher { return &CapturePublisher{} }

// Publish 实现 events.Publisher。
func (p *CapturePublisher) Publish(_ context.Context, ev events.Envelope) error {
	if err := ev.Validate(); err != nil {
		return err
	}
	if p.Fail {
		return errCaptureFail
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.evts = append(p.evts, ev)
	return nil
}

// Envelopes 返回已记录事件的副本(时间序)。
func (p *CapturePublisher) Envelopes() []events.Envelope {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]events.Envelope, len(p.evts))
	copy(out, p.evts)
	return out
}

// Names 返回已记录事件的名字(时间序)。
func (p *CapturePublisher) Names() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.evts))
	for i, e := range p.evts {
		out[i] = e.Name
	}
	return out
}

type captureError struct{}

func (captureError) Error() string { return "capture publisher: fail flag set" }

var errCaptureFail error = captureError{}

// InlineRunner 是 uow.Runner 的测试 fake:立即执行 fn、不开事务
// (仓储 fake 自带记忆,无需真事务)。
type InlineRunner struct{}

// Run 实现 uow.Runner。
func (InlineRunner) Run(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
