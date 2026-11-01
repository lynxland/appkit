package idem

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lynx-go/grpcapi/interceptor"
	"github.com/lynx-go/lynx-start/clock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

func ctxWithKey(k string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(HeaderKey, k))
}

func runUnary(t *testing.T, st interceptor.Stage, ctx context.Context, req any, handler grpc.UnaryHandler) (any, error) {
	t.Helper()
	return st.Unary(ctx, req, &grpc.UnaryServerInfo{FullMethod: "/demo.Svc/Create"}, handler)
}

func stageFor(t *testing.T, enforced bool) interceptor.Stage {
	t.Helper()
	fake := clock.NewFake(t0)
	return New(NewMemStore(), Options{
		Enforced:  func(string) bool { return enforced },
		Clock:     fake,
		ClaimTTL:  90 * time.Second,
		ReplayTTL: 24 * time.Hour,
	})
}

func TestStagePassthrough(t *testing.T) {
	st := stageFor(t, true)
	calls := 0
	h := func(ctx context.Context, req any) (any, error) { calls++; return &emptypb.Empty{}, nil }

	// 无键 → 直通。
	if _, err := runUnary(t, st, context.Background(), &emptypb.Empty{}, h); err != nil || calls != 1 {
		t.Fatalf("no key: %v calls=%d", err, calls)
	}
	// 未登记方法 → 直通。
	if _, err := runUnary(t, stageFor(t, false), ctxWithKey("k"), &emptypb.Empty{}, h); err != nil || calls != 2 {
		t.Fatalf("not enforced: %v calls=%d", err, calls)
	}
	// 非 proto 载荷 → 保守放行。
	if _, err := runUnary(t, st, ctxWithKey("k"), struct{}{}, h); err != nil || calls != 3 {
		t.Fatalf("non-proto: %v calls=%d", err, calls)
	}
}

func TestStageReplayFlow(t *testing.T) {
	st := stageFor(t, true)
	calls := 0
	h := func(ctx context.Context, req any) (any, error) {
		calls++
		return &emptypb.Empty{}, nil
	}
	// 首次：执行并缓存。
	resp, err := runUnary(t, st, ctxWithKey("k1"), &emptypb.Empty{}, h)
	if err != nil || calls != 1 {
		t.Fatalf("first: %v calls=%d", err, calls)
	}
	if _, ok := resp.(*emptypb.Empty); !ok {
		t.Fatalf("first resp type %T", resp)
	}
	// 重复（同键同载荷）：回放——handler 不再执行，返回 dynamicpb 重建的等价消息。
	resp2, err := runUnary(t, st, ctxWithKey("k1"), &emptypb.Empty{}, h)
	if err != nil || calls != 1 {
		t.Fatalf("replay must not re-execute: %v calls=%d", err, calls)
	}
	msg, ok := resp2.(proto.Message)
	if !ok || string(msg.ProtoReflect().Descriptor().FullName()) != "google.protobuf.Empty" {
		t.Fatalf("replayed resp %T", resp2)
	}
	// emptypb 载荷恒同（无可变字段）——指纹差异的拦截器出口经
	// TestStageMismatchAndInProgress 的显式预置验证。
}

func TestStageMismatchAndInProgress(t *testing.T) {
	// 直接预置 store 状态验证拦截器出口分支。
	store := NewMemStore()
	fake := clock.NewFake(t0)
	st := New(store, Options{Enforced: func(string) bool { return true }, Clock: fake})
	h := func(ctx context.Context, req any) (any, error) { return &emptypb.Empty{}, nil }

	// 预置：k-mis 已被不同指纹 claim。
	_, _ = store.Claim(Request{Key: "k-mis", Scope: "anon", Fingerprint: "other"}, 90*time.Second, t0)
	_, err := runUnary(t, st, ctxWithKey("k-mis"), &emptypb.Empty{}, h)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("mismatch code = %v", status.Code(err))
	}
	// 预置：k-run 在途（同指纹——首请求的 proto 序列化）。
	fp := Fingerprint("/demo.Svc/Create", mustMarshal(&emptypb.Empty{}))
	_, _ = store.Claim(Request{Key: "k-run", Scope: "anon", Fingerprint: fp}, 90*time.Second, t0)
	_, err = runUnary(t, st, ctxWithKey("k-run"), &emptypb.Empty{}, h)
	if status.Code(err) != codes.Aborted {
		t.Fatalf("in-progress code = %v", status.Code(err))
	}
}

func TestStageFailureRelease(t *testing.T) {
	store := NewMemStore()
	fake := clock.NewFake(t0)
	st := New(store, Options{Enforced: func(string) bool { return true }, Clock: fake})
	boom := errors.New("boom")
	h := func(ctx context.Context, req any) (any, error) { return nil, boom }

	// 失败：释放声明。
	if _, err := runUnary(t, st, ctxWithKey("k-err"), &emptypb.Empty{}, h); !errors.Is(err, boom) {
		t.Fatalf("handler error must pass through: %v", err)
	}
	// 同键重试：全新声明（再次执行 handler）。
	calls := 0
	h2 := func(ctx context.Context, req any) (any, error) { calls++; return &emptypb.Empty{}, nil }
	if _, err := runUnary(t, st, ctxWithKey("k-err"), &emptypb.Empty{}, h2); err != nil || calls != 1 {
		t.Fatalf("retry after release: %v calls=%d", err, calls)
	}
}

func mustMarshal(m proto.Message) []byte {
	b, err := proto.Marshal(m)
	if err != nil {
		panic(err)
	}
	return b
}
