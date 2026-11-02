package idem

import (
	"context"
	"fmt"

	"github.com/lynxland/appkit/clock"
	grpcapiinterceptor "github.com/lynx-go/grpcapi/interceptor"
	"github.com/lynx-go/grpcapi/principal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

// StageOrder 是幂等执法在拦截链的位置：authn(30) 之后、authz(70) 之前
// （fleetly 链位同构——回放短路早于 authz：同 principal 同指纹的请求已
// 通过一次授权）。占用 45（grpcapi Order 表的空位）。
const StageOrder = 45

// New 构造幂等执法 Stage（挂到 Assembly.Stages / chain.DefaultChain）。
// 仅处理 unary（幂等语义对 stream 不成立）；非 proto 请求载荷直接放行
// （指纹不可计算时保守不执法，不拒请求）。
func New(store Store, opts Options) grpcapiinterceptor.Stage {
	if opts.ClaimTTL <= 0 {
		opts.ClaimTTL = DefaultClaimTTL
	}
	if opts.ReplayTTL <= 0 {
		opts.ReplayTTL = DefaultReplayTTL
	}
	if opts.Clock == nil {
		opts.Clock = clock.System{}
	}
	if opts.Enforced == nil {
		opts.Enforced = func(string) bool { return false }
	}
	return grpcapiinterceptor.Stage{
		Name:  "idem",
		Order: StageOrder,
		Unary: func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			key, ok := keyFromContext(ctx)
			if !ok || !opts.Enforced(info.FullMethod) {
				return handler(ctx, req)
			}
			msg, ok := req.(proto.Message)
			if !ok {
				return handler(ctx, req) // 非 proto 载荷：不执法（保守放行）
			}
			payload, err := proto.Marshal(msg)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "idem: marshal request: %v", err)
			}
			result, err := store.Claim(Request{
				Key:         key,
				Scope:       scopeFromContext(ctx),
				Fingerprint: Fingerprint(info.FullMethod, payload),
			}, opts.ClaimTTL, opts.Clock.Now())
			if err != nil {
				return nil, status.Errorf(codes.Internal, "idem: claim: %v", err)
			}
			switch result.State {
			case StateReplayed:
				replayed, err := rebuild(result.Cached)
				if err != nil {
					return nil, status.Errorf(codes.Internal, "idem: replay: %v", err)
				}
				return replayed, nil
			case StateInProgress:
				return nil, status.Error(codes.Aborted, "idempotency key is in progress")
			case StateMismatch:
				return nil, status.Error(codes.InvalidArgument, "idempotency key was already used with a different payload")
			}
			resp, err := handler(ctx, req)
			if err != nil {
				// 失败释放：同键可重试（fleetly failure-release 语义）。
				if releaseErr := store.Release(key, scopeFromContext(ctx), opts.Clock.Now()); releaseErr != nil {
					// 释放失败不吞业务错误；Internal 拼接供排障。
					return nil, fmt.Errorf("%w (idem release: %v)", err, releaseErr)
				}
				return nil, err
			}
			respMsg, ok := resp.(proto.Message)
			if !ok {
				return resp, nil // 非 proto 响应：放行但声明不缓存（下次同键=InProgress→过期重放缺席）
			}
			body, err := proto.Marshal(respMsg)
			if err != nil {
				return resp, nil
			}
			if err := store.Complete(key, scopeFromContext(ctx), Cached{
				MessageName: string(respMsg.ProtoReflect().Descriptor().FullName()),
				Body:        body,
			}, opts.ReplayTTL, opts.Clock.Now()); err != nil {
				return resp, nil // 缓存失败不影响本次响应
			}
			return resp, nil
		},
		Stream: func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return handler(srv, ss) // stream 不执法（幂等语义不成立），直通
		},
	}
}

// keyFromContext 从入站 metadata 取幂等键（大小写不敏感）。
func keyFromContext(ctx context.Context) (string, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}
	vals := md.Get(HeaderKey)
	if len(vals) == 0 || vals[0] == "" {
		return "", false
	}
	return vals[0], true
}

// scopeFromContext 取声明作用域：principal ID（grpcapi 语境权威），
// 匿名回退 "anon"（创建型动词通常已 authenticated——回退仅供宽松面）。
func scopeFromContext(ctx context.Context) string {
	if p, ok := principal.From(ctx); ok && p.ID != "" {
		return p.ID
	}
	return "anon"
}

// rebuild 经 dynamicpb 从缓存重建响应实例（进程内全局 proto 注册表：
// 消息描述符随 proto 导入注册，FullName 全局可寻）。
func rebuild(c Cached) (proto.Message, error) {
	if c.MessageName == "" {
		return nil, fmt.Errorf("empty cached response")
	}
	// Body 可为零字节（如 google.protobuf.Empty 的合法序列化）。
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(c.MessageName))
	if err != nil {
		return nil, fmt.Errorf("find %s: %w", c.MessageName, err)
	}
	msgDesc, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, fmt.Errorf("%s is not a message", c.MessageName)
	}
	msg := dynamicpb.NewMessage(msgDesc)
	if err := proto.Unmarshal(c.Body, msg); err != nil {
		return nil, err
	}
	return msg, nil
}
