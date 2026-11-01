// Package testkit 是 bun 栈的进程内全链测试夹具（ADR-0009 D13 蓝图桶⑦，
// fleetly apitest 蓝本）：一组 InstalledModule 在隔离测试库（真 PG 门控）
// 上装配——Deps 组装（CapturePublisher/内存 bus/viper Config）→
// compose.AssembleAll → policy 集 → chain.DefaultChain →
// grpcapiserver.New（fail-closed 校验全继承）→ GRPCServer()/HTTPHandler()
// 逃生口经 bufconn/httptest 无端口起服。测试拿到真实 gRPC ClientConn
// 与 REST base URL，走 register→login→业务 的真实链路。
//
// 定位于 contrib/bunx（偏向装配层的测试面）：依赖真 PG 与 bun 适配，
// root 核心不反向引用。模块作者用它写跨层测试，应用作者用它替代
// docker+curl 冒烟的可回归形态。
package testkit

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"

	grpcapiauthn "github.com/lynx-go/grpcapi/authn"
	grpcapiinterceptor "github.com/lynx-go/grpcapi/interceptor"
	grpcapipolicy "github.com/lynx-go/grpcapi/policy"
	grpcapiprotopolicy "github.com/lynx-go/grpcapi/policy/protopolicy"
	grpcapiserver "github.com/lynx-go/grpcapi/server"
	"github.com/lynx-go/lynx"
	kernel "github.com/lynx-go/lynx-start"
	"github.com/lynx-go/lynx-start/chain"
	"github.com/lynx-go/lynx-start/compose"
	"github.com/lynx-go/lynx-start/contrib/bunx/buntx"
	"github.com/lynx-go/lynx-start/contrib/bunx/testutil"
	"github.com/lynx-go/lynx/eventbus"
	"github.com/spf13/viper"
	"net/http/httptest"

	"github.com/uptrace/bun"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// Options 是 Harness 配置。
type Options struct {
	// Installed 参与装配的已装模块（迁移链与策略集由此派生）。
	Installed []kernel.InstalledModule
	// Config 是配置面（viper 键值；模块配置段如 identity.secret 在此给值）。
	Config map[string]any
	// Logger 测试日志；默认丢弃（slog.DiscardHandler 的等价物）。
	Logger *slog.Logger
	// ExtraStages 是应用级拦截 Stage（idem/ratelimit 等），追加进
	// DefaultChain（模块贡献 Stage 之后）。
	ExtraStages []grpcapiinterceptor.Stage
	// Gateway 启用 REST 面（httptest 无端口；HTTPURL 生效）。
	Gateway bool
}

// Harness 是进程内全链夹具。字段均为测试可注入/可断言面。
type Harness struct {
	// Conn 是已拨号的 gRPC 客户端（bufconn，无端口）。
	Conn *grpc.ClientConn
	// HTTPURL 是 REST 面 base URL（Options.Gateway 时有效）。
	HTTPURL string
	// DB 是隔离测试库（t.Cleanup 整库 FORCE DROP）。
	DB *bun.DB
	// Deps 是装配用的共享依赖（Events=Capture 可断言）。
	Deps kernel.Deps
	// Events 是事件捕获器（发布断言用）。
	Events *testutil.CapturePublisher
	// Bus 是内存事件总线（订阅侧）。
	Bus eventbus.Bus
}

// New 装配并起服。PG 门控同 testutil 约定：无 LYNXSTART_TEST_PG_DSN
// 时整测跳过。
func New(t testing.TB, opts Options) *Harness {
	t.Helper()
	logger := opts.Logger
	if logger == nil {
		logger = discardLogger()
	}

	var chains []testutil.Chain
	for _, m := range opts.Installed {
		if m.Module.Migrations != nil {
			chains = append(chains, testutil.Chain{Module: m.Module.Name, FS: m.Module.Migrations})
		}
	}
	db := testutil.NewDB(t, chains...)

	events := testutil.NewCapturePublisher()
	bus := eventbus.NewMemoryBus(eventbus.Options{})

	v := viper.New()
	for k, val := range opts.Config {
		v.Set(k, val)
	}
	cfg := lynx.NewViperConfig(v)

	deps := kernel.Deps{
		DB:     db,
		Tx:     buntx.New(db),
		Events: events,
		Bus:    bus,
		Config: cfg,
		Logger: logger,
	}

	agg, err := compose.AssembleAll(deps, opts.Installed)
	if err != nil {
		t.Fatalf("testkit: assemble: %v", err)
	}
	agg.Stages = append(agg.Stages, opts.ExtraStages...)

	ctx := context.Background()
	files := compose.PolicyFiles(compose.Descriptors(opts.Installed))
	set, err := grpcapipolicy.NewSet()
	if err != nil {
		t.Fatalf("testkit: policy set: %v", err)
	}
	if len(files) > 0 {
		if set, err = grpcapipolicy.Build(ctx, grpcapipolicy.FromFiles(files...), grpcapipolicy.Options{
			Provider: grpcapiprotopolicy.New(grpcapiprotopolicy.Options{}),
			Registry: grpcapipolicy.DefaultRegistry(),
		}); err != nil {
			t.Fatalf("testkit: policy build: %v", err)
		}
	}

	authnOpts := grpcapiauthn.Options{Enrichers: agg.Enrichers}
	fullChain, err := chain.DefaultChain(logger, set, agg, authnOpts)
	if err != nil {
		t.Fatalf("testkit: chain: %v", err)
	}

	srv, err := grpcapiserver.New(ctx, grpcapiserver.Config{
		Policy:               set,
		Services:             agg.Registrations,
		ExtraRoutes:          agg.ExtraRoutes,
		Authenticators:       agg.Authenticators,
		AuthnOptions:         authnOpts,
		Chain:                fullChain,
		AllowNoAuthenticator: len(agg.Authenticators) == 0,
		Logger:               logger,
	})
	if err != nil {
		t.Fatalf("testkit: server: %v", err)
	}

	// gRPC：bufconn 无端口起服（校验与注册语义全继承 server.New）。
	grpcSrv := srv.GRPCServer()
	lis := bufconn.Listen(1 << 20)
	go func() { _ = grpcSrv.Serve(lis) }()
	t.Cleanup(func() { _ = lis.Close() })
	t.Cleanup(func() { grpcSrv.GracefulStop() })

	conn, err := grpc.NewClient("passthrough:///testkit",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("testkit: dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	h := &Harness{Conn: conn, DB: db, Deps: deps, Events: events, Bus: bus}
	if opts.Gateway {
		hs := httptest.NewServer(srv.HTTPHandler())
		t.Cleanup(hs.Close)
		h.HTTPURL = hs.URL
	}
	return h
}

// Eventually 轮询等待检查通过（50ms 间隔；torchwood 同款——内存总线
// 异步投递等场景的确定性等待，替代固定 sleep）。
func Eventually(t testing.TB, timeout time.Duration, check func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := check(); err == nil {
			return
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("testkit: eventually: %v", lastErr)
}

// discardLogger 返回丢弃输出的测试日志（等价 slog.DiscardHandler 的
// 显式形态，兼容更早工具链的可读写法）。
func discardLogger() *slog.Logger {
	return slog.New(discardHandler{})
}

type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (discardHandler) WithAttrs(attrs []slog.Attr) slog.Handler  { return discardHandler{} }
func (discardHandler) WithGroup(name string) slog.Handler        { return discardHandler{} }
