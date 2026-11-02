// Package jwt 提供最小 HS256 JWT 签发与验证（标准库实现，无外部依赖）。
// 密钥经 secretkey.Derive(master, purpose) 域隔离——一个 purpose 的令牌
// 在其他 purpose 下不可验证。签发依赖 appkit/clock 注入时间源。
//
// 面向 identity/session 类模块的访问令牌与轮换令牌； grpcapi 的
// authn/jwt 负责请求侧解析，本包补齐签发侧。
package jwt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lynxland/appkit/clock"
	"github.com/lynxland/appkit/idgen"
	"github.com/lynxland/appkit/secretkey"
)

// Claims 是受支持的标准声明 + 自定义字符串声明集。
type Claims struct {
	Subject   string            `json:"sub,omitempty"`
	Issuer    string            `json:"iss,omitempty"`
	Audience  string            `json:"aud,omitempty"`
	ID        string            `json:"jti,omitempty"`
	ExpiresAt time.Time         `json:"exp,omitempty"`
	NotBefore time.Time         `json:"nbf,omitempty"`
	IssuedAt  time.Time         `json:"iat,omitempty"`
	Custom    map[string]string `json:"cus,omitempty"`
}

// Signer 签发令牌（组合根构造，应用进程内共享）。
type Signer struct {
	key    []byte
	issuer string
	ttl    time.Duration
	clock  clock.Clock
}

// NewSigner 以主密钥与 purpose 派生签名钥；ttl<=0 是编程错误。
func NewSigner(master, purpose, issuer string, ttl time.Duration, c clock.Clock) *Signer {
	if c == nil {
		c = clock.System{}
	}
	if ttl <= 0 {
		panic("jwt: ttl must be positive")
	}
	return &Signer{key: secretkey.Derive(master, purpose), issuer: issuer, ttl: ttl, clock: c}
}

// Token 是一次签发的产物。
type Token struct {
	Value     string
	Claims    Claims
	ExpiresAt time.Time
}

// Sign 签发访问令牌：subject 为主体 ID，custom 为附加声明。
func (s *Signer) Sign(subject string, custom map[string]string) (*Token, error) {
	now := s.clock.Now()
	claims := Claims{
		Subject:   subject,
		Issuer:    s.issuer,
		ID:        idgen.New(),
		ExpiresAt: now.Add(s.ttl),
		NotBefore: now.Add(-time.Minute), // 容忍 1min 时钟偏差
		IssuedAt:  now,
		Custom:    custom,
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return nil, fmt.Errorf("jwt: marshal claims: %w", err)
	}
	body := b64(`{"alg":"HS256","typ":"JWT"}`) + "." + b64(string(payload))
	sig := sign(s.key, body)
	return &Token{Value: body + "." + sig, Claims: claims, ExpiresAt: claims.ExpiresAt}, nil
}

// Verifier 验证令牌（与 Signer 共享 master/purpose/issuer 才能通过）。
type Verifier struct {
	key    []byte
	issuer string
}

// NewVerifier 构造验证器。
func NewVerifier(master, purpose, issuer string) *Verifier {
	return &Verifier{key: secretkey.Derive(master, purpose), issuer: issuer}
}

// 验证错误哨兵：调用方 errors.Is 映射 InvalidArgument/Unauthenticated。
var (
	ErrMalformed = errors.New("jwt: malformed token")
	ErrBadSig    = errors.New("jwt: signature mismatch")
	ErrExpired   = errors.New("jwt: token expired")
	ErrNotYet    = errors.New("jwt: token not yet valid")
	ErrIssuer    = errors.New("jwt: issuer mismatch")
)

// Verify 验证签名与时效（exp/nbf），返回声明。
func (v *Verifier) Verify(token string, now time.Time) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrMalformed
	}
	if !hmac.Equal([]byte(sign(v.key, parts[0]+"."+parts[1])), []byte(parts[2])) {
		return nil, ErrBadSig
	}
	payload, err := unb64(parts[1])
	if err != nil {
		return nil, ErrMalformed
	}
	var claims Claims
	if err := json.Unmarshal([]byte(payload), &claims); err != nil {
		return nil, ErrMalformed
	}
	if v.issuer != "" && claims.Issuer != v.issuer {
		return nil, ErrIssuer
	}
	if !claims.ExpiresAt.IsZero() && now.After(claims.ExpiresAt) {
		return nil, ErrExpired
	}
	if !claims.NotBefore.IsZero() && now.Before(claims.NotBefore) {
		return nil, ErrNotYet
	}
	return &claims, nil
}

func sign(key []byte, body string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func b64(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

func unb64(s string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	return string(raw), err
}
