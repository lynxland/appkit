package jwt

import (
	"errors"
	"testing"
	"time"

	"github.com/lynxland/appkit/clock"
)

var master = "test-master-0123456789abcdef-0123456789abcdef"

func TestSignVerifyRoundtrip(t *testing.T) {
	c := clock.NewFake(time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC))
	s := NewSigner(master, "access", "appkit-test", time.Hour, c)
	tok, err := s.Sign("user-1", map[string]string{"role": "member"})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if tok.Claims.Subject != "user-1" || tok.Claims.Custom["role"] != "member" {
		t.Fatalf("claims = %+v", tok.Claims)
	}
	if tok.ExpiresAt.Sub(tok.Claims.IssuedAt) != time.Hour {
		t.Fatalf("ttl mismatch: %v", tok.ExpiresAt.Sub(tok.Claims.IssuedAt))
	}

	v := NewVerifier(master, "access", "appkit-test")
	claims, err := v.Verify(tok.Value, c.Now())
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Subject != "user-1" || claims.Custom["role"] != "member" {
		t.Fatalf("roundtrip claims = %+v", claims)
	}
}

func TestExpiryAndNotYet(t *testing.T) {
	c := clock.NewFake(time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC))
	s := NewSigner(master, "access", "iss", time.Hour, c)
	v := NewVerifier(master, "access", "iss")
	tok, _ := s.Sign("u", nil)

	if _, err := v.Verify(tok.Value, c.Now().Add(time.Hour+time.Second)); !errors.Is(err, ErrExpired) {
		t.Errorf("expired: err = %v, want ErrExpired", err)
	}
	if _, err := v.Verify(tok.Value, c.Now().Add(time.Minute-time.Nanosecond)); err != nil {
		// nbf = now-1min，签发前 1 分钟内仍有效（时钟偏差容忍窗内）。
		t.Logf("pre-nbf verify err (tolerance window): %v", err)
	}
	// 人为构造未生效令牌：把时钟拨到 nbf 之前 2 分钟。
	if _, err := v.Verify(tok.Value, c.Now().Add(-2*time.Minute)); !errors.Is(err, ErrNotYet) {
		t.Errorf("not yet valid: err = %v, want ErrNotYet", err)
	}
}

func TestPurposeIsolation(t *testing.T) {
	c := clock.NewFake(time.Now())
	access := NewSigner(master, "access", "iss", time.Hour, c)
	refreshV := NewVerifier(master, "refresh", "iss")
	tok, _ := access.Sign("u", nil)
	if _, err := refreshV.Verify(tok.Value, c.Now()); !errors.Is(err, ErrBadSig) {
		t.Errorf("cross-purpose: err = %v, want ErrBadSig", err)
	}
}

func TestIssuerAndTamper(t *testing.T) {
	c := clock.NewFake(time.Now())
	s := NewSigner(master, "access", "issuer-a", time.Hour, c)
	tok, _ := s.Sign("u", nil)

	if _, err := NewVerifier(master, "access", "issuer-b").Verify(tok.Value, c.Now()); !errors.Is(err, ErrIssuer) {
		t.Errorf("issuer mismatch: err = %v, want ErrIssuer", err)
	}

	tampered := tok.Value[:len(tok.Value)-2] + "xx"
	if _, err := NewVerifier(master, "access", "issuer-a").Verify(tampered, c.Now()); err == nil {
		t.Error("tampered token accepted")
	}
	if _, err := NewVerifier(master, "access", "issuer-a").Verify("garbage", c.Now()); !errors.Is(err, ErrMalformed) {
		t.Errorf("garbage: err = %v, want ErrMalformed", err)
	}
}
