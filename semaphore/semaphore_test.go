package semaphore

import (
	"context"
	"testing"
)

func TestInMemory(t *testing.T) {
	s := NewInMemory(2)
	ctx := context.Background()

	ok1, r1, _ := s.TryAcquire(ctx)
	ok2, r2, _ := s.TryAcquire(ctx)
	ok3, _, _ := s.TryAcquire(ctx)
	if !ok1 || !ok2 || ok3 {
		t.Fatalf("acquire: %v %v %v", ok1, ok2, ok3)
	}
	r1() // 释放一个
	ok4, _, _ := s.TryAcquire(ctx)
	if !ok4 {
		t.Fatal("acquire after release")
	}
	r2()
}

func TestNoop(t *testing.T) {
	s := Noop{}
	ok, release, err := s.TryAcquire(context.Background())
	if !ok || err != nil || release == nil {
		t.Fatalf("noop: %v %v", ok, err)
	}
	release()
}
