package idem

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func TestMemStoreStateMachine(t *testing.T) {
	s := NewMemStore()
	req := Request{Key: "k1", Scope: "u1", Fingerprint: "fp-a"}

	// 新键 → Claimed。
	if r, err := s.Claim(req, 90*time.Second, t0); err != nil || r.State != StateClaimed {
		t.Fatalf("claim: %+v %v", r, err)
	}
	// 同键同指纹在途 → InProgress。
	if r, _ := s.Claim(req, 90*time.Second, t0); r.State != StateInProgress {
		t.Fatalf("in-progress: %+v", r)
	}
	// 同键不同指纹 → Mismatch。
	if r, _ := s.Claim(Request{Key: "k1", Scope: "u1", Fingerprint: "fp-b"}, 90*time.Second, t0); r.State != StateMismatch {
		t.Fatalf("mismatch: %+v", r)
	}
	// Complete → Replayed（携带缓存）。
	cached := Cached{MessageName: "google.protobuf.Empty", Body: []byte{}}
	if err := s.Complete("k1", "u1", cached, 24*time.Hour, t0); err != nil {
		t.Fatal(err)
	}
	r, _ := s.Claim(req, 90*time.Second, t0)
	if r.State != StateReplayed || r.Cached.MessageName != cached.MessageName {
		t.Fatalf("replayed: %+v", r)
	}
	// 作用域隔离：他人同键 = 新声明。
	if r, _ := s.Claim(Request{Key: "k1", Scope: "u2", Fingerprint: "fp-a"}, 90*time.Second, t0); r.State != StateClaimed {
		t.Fatalf("scope isolation: %+v", r)
	}
	// replay 窗口过期 → 新键。
	if r, _ := s.Claim(req, 90*time.Second, t0.Add(25*time.Hour)); r.State != StateClaimed {
		t.Fatalf("replay expiry: %+v", r)
	}
	// claim 窗口过期（在途未决）→ 新键可重试。
	s2 := NewMemStore()
	_, _ = s2.Claim(req, 90*time.Second, t0)
	if r, _ := s2.Claim(req, 90*time.Second, t0.Add(91*time.Second)); r.State != StateClaimed {
		t.Fatalf("claim expiry: %+v", r)
	}
	// Complete 未知键 → 错误（claim 已过期）。
	if err := s2.Complete("ghost", "u1", cached, time.Hour, t0); err == nil {
		t.Error("complete unknown key must error")
	}
}

func TestMemStoreRelease(t *testing.T) {
	s := NewMemStore()
	req := Request{Key: "k1", Scope: "u1", Fingerprint: "fp"}
	_, _ = s.Claim(req, 90*time.Second, t0)
	if err := s.Release("k1", "u1", t0); err != nil {
		t.Fatal(err)
	}
	// 释放后同键 → 全新声明。
	if r, _ := s.Claim(req, 90*time.Second, t0); r.State != StateClaimed {
		t.Fatalf("after release: %+v", r)
	}
	// 幂等释放（已不存在）。
	if err := s.Release("k1", "u1", t0); err != nil {
		t.Fatal(err)
	}
}

func TestFingerprint(t *testing.T) {
	a := Fingerprint("/svc.Method", []byte("payload"))
	b := Fingerprint("/svc.Method", []byte("payload"))
	c := Fingerprint("/svc.Method", []byte("payload2"))
	d := Fingerprint("/svc.Other", []byte("payload"))
	if a != b || a == c || a == d {
		t.Errorf("fingerprint discrimination broken: %s %s %s", a, c, d)
	}
	if len(a) != 64 { // sha256 hex
		t.Errorf("fingerprint length = %d", len(a))
	}
}
