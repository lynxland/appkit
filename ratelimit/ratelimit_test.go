package ratelimit

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

var t0 = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func TestTokenBucketBurstAndRefill(t *testing.T) {
	tb := NewTokenBucket(3, 2) // burst 3，2 token/s
	// 首拍可用满 burst。
	for i := 0; i < 3; i++ {
		if d := tb.Allow("k", t0); !d.Allowed {
			t.Fatalf("burst[%d] denied: %+v", i, d)
		}
	}
	// 第 4 次拒绝，建议等待 = 半秒（缺 1 token、2/s 补充）。
	if d := tb.Allow("k", t0); d.Allowed || d.RetryAfter != 500*time.Millisecond {
		t.Fatalf("4th: %+v", d)
	}
	// 0.4s 后仍不足；0.6s 后恢复 1 token。
	if d := tb.Allow("k", t0.Add(400*time.Millisecond)); d.Allowed {
		t.Fatalf("0.4s: %+v", d)
	}
	if d := tb.Allow("k", t0.Add(600*time.Millisecond)); !d.Allowed {
		t.Fatalf("0.6s: %+v", d)
	}
	// 每 key 隔离。
	if d := tb.Allow("other", t0.Add(600*time.Millisecond)); !d.Allowed || d.Remaining != 2 {
		t.Fatalf("isolation: %+v", d)
	}
}

func TestTokenBucketCap(t *testing.T) {
	tb := NewTokenBucket(2, 1000) // 极快补充，验证容量封顶
	tb.Allow("k", t0)
	tb.Allow("k", t0)
	if d := tb.Allow("k", t0); d.Allowed {
		t.Fatal("burst exhausted")
	}
	// 10ms 补 10 token 但容量封顶 2 → 恢复后仅 2 可用。
	tb.Allow("k", t0.Add(10*time.Millisecond)) // 用 1，剩 1
	if d := tb.Allow("k", t0.Add(10*time.Millisecond)); !d.Allowed || d.Remaining != 0 {
		t.Fatalf("cap: %+v", d)
	}
	if d := tb.Allow("k", t0.Add(10*time.Millisecond)); d.Allowed {
		t.Fatalf("cap overflow: %+v", d)
	}
}

func TestSlidingWindowNoBoundarySpike(t *testing.T) {
	sw := NewSlidingWindow(3, time.Minute)
	// 窗口尾部打满 3。
	for i := 0; i < 3; i++ {
		if d := sw.Allow("k", t0.Add(time.Duration(i)*time.Second)); !d.Allowed {
			t.Fatalf("fill[%d]: %+v", i, d)
		}
	}
	// 跨入"下一固定窗口"（滑动语义下仍被限——固定窗口边界突刺不复现）。
	if d := sw.Allow("k", t0.Add(59*time.Second)); d.Allowed {
		t.Fatalf("boundary spike: %+v", d)
	}
	// 最早一条（t0）滑出后恢复。
	if d := sw.Allow("k", t0.Add(61*time.Second)); !d.Allowed {
		t.Fatalf("slide-out: %+v", d)
	}
}

func TestSlidingWindowRemaining(t *testing.T) {
	sw := NewSlidingWindow(3, time.Minute)
	sw.Allow("k", t0)
	if d := sw.Allow("k", t0); !d.Allowed || d.Remaining != 1 {
		t.Fatalf("remaining: %+v", d)
	}
}

func TestStageAllowsAndLimits(t *testing.T) {
	sw := NewSlidingWindow(2, time.Hour) // 测试期不会滑出
	limited := ""
	st := Stage(Options{
		Limiter:   sw,
		Key:       func(context.Context, string) string { return "fixed" },
		OnLimited: func(_ context.Context, key string, _ Decision) { limited = key },
	})
	if st.Name != "ratelimit" || st.Order != 40 {
		t.Fatalf("stage meta: %s %d", st.Name, st.Order)
	}
	h := func(ctx context.Context, req any) (any, error) { return "ok", nil }
	call := func() error {
		_, err := st.Unary(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/x.Y/Z"}, h)
		return err
	}
	if err := call(); err != nil {
		t.Fatalf("1st: %v", err)
	}
	if err := call(); err != nil {
		t.Fatalf("2nd: %v", err)
	}
	err := call()
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("3rd code = %v", status.Code(err))
	}
	if limited != "fixed" {
		t.Errorf("OnLimited key = %q", limited)
	}
	// RetryInfo detail 存在且 >=1s（对齐 grpcapi 参考实现整流）。
	st2 := status.Convert(err)
	found := false
	for _, d := range st2.Details() {
		if _, ok := d.(interface{ GetRetryDelay() *durationpb.Duration }); ok {
			found = true
		}
	}
	if !found {
		t.Error("RetryInfo detail missing")
	}
	// 另一 key 不受影响。
	st3 := Stage(Options{Limiter: sw, Key: func(context.Context, string) string { return "other" }})
	if _, err := st3.Unary(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/x.Y/Z"}, h); err != nil {
		t.Fatalf("other key: %v", err)
	}
	// stream 同走限流（新键验证放行形态；流建立计一次）。
	fresh := Stage(Options{Limiter: sw, Key: func(context.Context, string) string { return "fresh" }})
	if err := fresh.Stream(nil, &fakeStream{}, &grpc.StreamServerInfo{}, func(srv any, ss grpc.ServerStream) error { return nil }); err != nil {
		t.Fatalf("stream: %v", err)
	}
}

type fakeStream struct{ grpc.ServerStream }

func (f *fakeStream) Context() context.Context { return context.Background() }
