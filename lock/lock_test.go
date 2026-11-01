package lock

import (
	"context"
	"testing"
	"time"
)

func TestMemLockerAcquireRelease(t *testing.T) {
	l := NewMemLocker()
	ctx := context.Background()

	h1, ok := l.TryAcquire(ctx, "job", time.Minute)
	if !ok {
		t.Fatal("first acquire should succeed")
	}
	// 二次获取被拒。
	if _, ok := l.TryAcquire(ctx, "job", time.Minute); ok {
		t.Fatal("second acquire should fail")
	}
	// 释放后可再获取。
	if !h1.Release() {
		t.Fatal("release by holder should succeed")
	}
	h2, ok := l.TryAcquire(ctx, "job", time.Minute)
	if !ok {
		t.Fatal("acquire after release should succeed")
	}
	_ = h2
}

func TestMemLockerTTLExpiry(t *testing.T) {
	l := NewMemLocker()
	ctx := context.Background()
	h1, ok := l.TryAcquire(ctx, "k", 50*time.Millisecond)
	if !ok {
		t.Fatal("acquire")
	}
	time.Sleep(60 * time.Millisecond)
	// 过期后他人可获取。
	h2, ok := l.TryAcquire(ctx, "k", time.Minute)
	if !ok {
		t.Fatal("acquire after ttl expiry")
	}
	// 旧 Handle 的 Release 是无效操作（compare-and-del 不误伤新持有者）。
	if h1.Release() {
		t.Fatal("expired handle release must not succeed")
	}
	// 新持有者正常释放。
	if !h2.Release() {
		t.Fatal("current holder release")
	}
}

func TestMemLockerKeyIsolation(t *testing.T) {
	l := NewMemLocker()
	ctx := context.Background()
	if _, ok := l.TryAcquire(ctx, "a", time.Minute); !ok {
		t.Fatal("a")
	}
	if _, ok := l.TryAcquire(ctx, "b", time.Minute); !ok {
		t.Fatal("b must be independent")
	}
}

func TestMemLockerReleaseIdempotent(t *testing.T) {
	l := NewMemLocker()
	h, _ := l.TryAcquire(context.Background(), "k", time.Minute)
	if !h.Release() {
		t.Fatal("first release")
	}
	if h.Release() {
		t.Fatal("second release must return false")
	}
}
