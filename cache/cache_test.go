package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMemCacheGetSetDelete(t *testing.T) {
	c := NewMemCache()
	ctx := context.Background()

	if _, err := c.Get(ctx, "k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if err := c.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatal(err)
	}
	v, err := c.Get(ctx, "k")
	if err != nil || string(v) != "v" {
		t.Fatalf("get: %q %v", v, err)
	}
	if err := c.Delete(ctx, "k", "other"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, "k"); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted")
	}
}

func TestMemCacheTTLExpiry(t *testing.T) {
	c := NewMemCache()
	ctx := context.Background()
	_ = c.Set(ctx, "k", []byte("v"), 50*time.Millisecond)
	if _, err := c.Get(ctx, "k"); err != nil {
		t.Fatalf("before ttl: %v", err)
	}
	time.Sleep(60 * time.Millisecond)
	if _, err := c.Get(ctx, "k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after ttl: %v", err)
	}
	// NoTTL：永不过期。
	_ = c.Set(ctx, "forever", []byte("v"), NoTTL)
	time.Sleep(10 * time.Millisecond)
	if _, err := c.Get(ctx, "forever"); err != nil {
		t.Fatalf("no-ttl expired: %v", err)
	}
}

func TestLoadOrStoreSingleflight(t *testing.T) {
	c := NewMemCache()
	ctx := context.Background()
	var calls atomic.Int64
	loader := func(ctx context.Context) ([]byte, error) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond) // 模拟慢回源
		return []byte("loaded"), nil
	}

	// 并发 10 个同 Key LoadOrStore——loader 只执行 1 次。
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.LoadOrStore(ctx, "hot", time.Minute, loader)
			if err != nil || string(v) != "loaded" {
				t.Errorf("goroutine: %q %v", v, err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("loader called %d times, want 1 (singleflight)", calls.Load())
	}
	// 二次命中不再回源。
	if _, err := c.LoadOrStore(ctx, "hot", time.Minute, loader); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("cached hit still loads: %d", calls.Load())
	}
}

func TestLoadOrStoreLoaderNotFound(t *testing.T) {
	c := NewMemCache()
	ctx := context.Background()
	if _, err := c.LoadOrStore(ctx, "miss", time.Minute, func(context.Context) ([]byte, error) { return nil, ErrNotFound }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("loader miss: %v", err)
	}
	// 未写缓存——下次再调 loader。
	if _, err := c.Get(ctx, "miss"); !errors.Is(err, ErrNotFound) {
		t.Fatal("loader miss should not populate cache")
	}
}
