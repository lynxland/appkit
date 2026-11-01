package blob

import (
	"context"
	"errors"
	"testing"
)

func TestMemStorePutGetDelete(t *testing.T) {
	s := NewMemStore()
	ctx := context.Background()

	if err := s.Put(ctx, Object{Key: "a/1.txt", Data: []byte("hello"), ContentType: "text/plain"}); err != nil {
		t.Fatal(err)
	}
	obj, err := s.Get(ctx, "a/1.txt")
	if err != nil || string(obj.Data) != "hello" || obj.Size != 5 || obj.ContentType != "text/plain" {
		t.Fatalf("get: %+v %v", obj, err)
	}
	if _, err := s.Get(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if err := s.Delete(ctx, "a/1.txt"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "a/1.txt"); err != nil { // 幂等
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "a/1.txt"); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted")
	}
}

func TestMemStoreListPrefix(t *testing.T) {
	s := NewMemStore()
	ctx := context.Background()
	for _, k := range []string{"org/1/a", "org/1/b", "org/2/c", "other/d"} {
		_ = s.Put(ctx, Object{Key: k, Data: []byte("x")})
	}
	items, err := s.List(ctx, "org/1/")
	if err != nil || len(items) != 2 || items[0].Key != "org/1/a" {
		t.Fatalf("list: %v %+v", err, items)
	}
	if len(items[0].Data) != 0 {
		t.Error("list must return metadata only (no data)")
	}
	all, _ := s.List(ctx, "")
	if len(all) != 4 {
		t.Fatalf("empty prefix lists all: %d", len(all))
	}
}

func TestMemStoreOverwrite(t *testing.T) {
	s := NewMemStore()
	ctx := context.Background()
	_ = s.Put(ctx, Object{Key: "k", Data: []byte("v1")})
	_ = s.Put(ctx, Object{Key: "k", Data: []byte("v22")})
	obj, _ := s.Get(ctx, "k")
	if string(obj.Data) != "v22" || obj.Size != 3 {
		t.Fatalf("overwrite: %+v", obj)
	}
}

func TestMemStoreEmptyKey(t *testing.T) {
	if err := NewMemStore().Put(context.Background(), Object{}); err == nil {
		t.Fatal("empty key must be rejected")
	}
}
