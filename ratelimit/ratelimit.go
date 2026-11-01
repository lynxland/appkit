// Package ratelimit 是限流电池（ADR-0009 D13 蓝图桶⑥）：grpcapi 仅有
// 固定窗口参考实现（ratelimit 包自述「参考实现而非基础设施」），本电池
// 补齐令牌桶（burst+平滑补充）与滑动窗口（固定窗口边界突刺的解法），
// 经 interceptor.RateLimitStage 包装入链——命名、Order 40 槽位、
// ResourceExhausted+RetryInfo（gateway 转 Retry-After 头）全部继承
// grpcapi 冻结语义。进程内实现（惰性清扫，无后台协程）；分布式限流走
// Limiter 端口自带适配（Redis，torchwood semaphore 同路线；适配器内部
// 自决 fail-open/fail-closed——torchwood API key 失败节流的 fail-open
// 教训）。
package ratelimit

import (
	"context"
	"time"

	grpcapiinterceptor "github.com/lynx-go/grpcapi/interceptor"
	"github.com/lynx-go/grpcapi/principal"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

// Decision 是一次限流裁决。
type Decision struct {
	Allowed bool
	// RetryAfter 拒绝时的建议等待（本包不做下限整流，Stage 层对齐
	// grpcapi 参考实现的 >=1s 约定）。
	RetryAfter time.Duration
	// Limit / Remaining 供 API 投影（X-RateLimit-*；errstatus 循环消费）。
	Limit     int64
	Remaining int64
}

// Limiter 是限流器端口：按 key 独立裁决，now 由调用方注入（可测）。
// 分布式实现自行决定错误策略（fail-open→Allowed 决策）。
type Limiter interface {
	Allow(key string, now time.Time) Decision
}

// Options 是 Stage 配置。
type Options struct {
	// Limiter 限流器（必填）。
	Limiter Limiter
	// Key 分组键；nil = principal ID 优先、回落 client IP 概念由调用方
	// 提供（grpcapi contextx.ClientInfo 在 ctx 时可用）——本包保持与
	// grpcapi 参考实现同构的默认键序：principal → "anonymous"。
	Key func(ctx context.Context, fullMethod string) string
	// OnLimited 被限流回调（计数/告警）；不得阻塞请求路径。
	OnLimited func(ctx context.Context, key string, d Decision)
}

// Stage 构造限流 Stage（Order 40 槽位，grpcapi 冻结语义复用）。
func Stage(opts Options) grpcapiinterceptor.Stage {
	unary, stream := interceptors(opts)
	return grpcapiinterceptor.RateLimitStage(unary, stream)
}

func interceptors(opts Options) (grpc.UnaryServerInterceptor, grpc.StreamServerInterceptor) {
	check := func(ctx context.Context, fullMethod string) error {
		key := defaultKey(ctx)
		if opts.Key != nil {
			key = opts.Key(ctx, fullMethod)
		}
		d := opts.Limiter.Allow(key, time.Now().UTC())
		if d.Allowed {
			return nil
		}
		retryAfter := d.RetryAfter
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		if opts.OnLimited != nil {
			opts.OnLimited(ctx, key, d)
		}
		st := status.New(codes.ResourceExhausted, "rate limit exceeded")
		if withDetails, err := st.WithDetails(&errdetails.RetryInfo{RetryDelay: durationpb.New(retryAfter)}); err == nil {
			return withDetails.Err()
		}
		return st.Err()
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			if err := check(ctx, info.FullMethod); err != nil {
				return nil, err
			}
			return handler(ctx, req)
		}, func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			if err := check(ss.Context(), info.FullMethod); err != nil {
				return err
			}
			return handler(srv, ss)
		}
}

// defaultKey 与 grpcapi 参考实现同构：principal → anonymous（client IP
// 维度的键由应用经 Options.Key 叠加 grpcapi contextx 提供）。
func defaultKey(ctx context.Context) string {
	if p, ok := principal.From(ctx); ok && p.ID != "" {
		return "principal:" + p.ID
	}
	return "anonymous"
}
