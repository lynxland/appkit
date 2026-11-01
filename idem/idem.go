// Package idem 是幂等执法拦截器（ADR-0009 D13 考古桶⑤，证据 2×：
// fleetly application/idem 通用版——claim TTL 90s/replay 24h/fingerprint
// sha256(方法+确定性序列化)/principal 作用域，torchwood documents 域内
// 手搓 IDEMPOTENCY.KEY_CONFLICT/IN_PROGRESS）。grpcapi 依赖电池（同
// contexts 层位）：拦截器词汇是 grpcapi 冻结 SPI（interceptor.Stage）。
//
// 语义（fleetly 形态泛化）：
//   - 携带 Idempotency-Key 的已登记方法请求先 claim：
//     新键 → 放行执行；执行成功 Complete 缓存响应（replay 窗口内同键
//     同指纹 → 原样回放），失败 Release（可重试）；
//     同键同指纹在途 → InProgress（409 语义）；
//     同键不同指纹 → Mismatch（键复用不同载荷，422 语义）。
//   - 指纹 = sha256(FullMethod + proto 确定性序列化)；作用域 = principal
//     （匿名回退 "anon"）——键按调用方隔离。
//   - 回放经 dynamicpb 从缓存字节重建响应（进程内全局 proto 注册表），
//     短路于 authz 之前——同 principal 同指纹的请求已通过一次授权。
//   - 单节点 v1：Store 端口可插（MemStore 内置，惰性清扫）；分布式
//     声明（Redis）走 Store 适配，形态即 torchwood semaphore 路线。
package idem

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/lynx-go/lynx-start/clock"
)

// State 是 claim 的裁决结果。
type State uint8

const (
	// StateClaimed 新键：放行执行，成功后 Complete。
	StateClaimed State = iota
	// StateReplayed 命中已完成：附带缓存的响应原样回放。
	StateReplayed
	// StateInProgress 同键在途（前次未决）：拒绝并发重复。
	StateInProgress
	// StateMismatch 同键不同指纹：键复用不同载荷。
	StateMismatch
)

func (s State) String() string {
	switch s {
	case StateClaimed:
		return "claimed"
	case StateReplayed:
		return "replayed"
	case StateInProgress:
		return "in-progress"
	case StateMismatch:
		return "mismatch"
	default:
		return fmt.Sprintf("State(%d)", uint8(s))
	}
}

// Cached 是 Complete 缓存的响应形态：proto 消息全名 + 确定性序列化字节
// （回放经 dynamicpb 重建，见 stage）。
type Cached struct {
	MessageName string
	Body        []byte
}

// Request 是一次幂等声明的输入。
type Request struct {
	Key         string
	Scope       string
	Fingerprint string
}

// Result 是 Claim 的裁决（StateReplayed 时携带缓存响应）。
type Result struct {
	State  State
	Cached Cached
	Finger string // 冲突方的指纹（Mismatch 诊断）
}

// Store 是幂等声明的存储端口。实现须并发安全；Claim 是原子裁决。
type Store interface {
	Claim(req Request, claimTTL time.Duration, now time.Time) (Result, error)
	Complete(key, scope string, cached Cached, replayTTL time.Duration, now time.Time) error
	Release(key, scope string, now time.Time) error
}

// MemStore 是单进程实现：map + 互斥 + 惰性清扫（Claim 顺带清理过期条目；
// 无后台协程——单节点语义，条目量与 Enforced 方法写入频度同阶）。
type MemStore struct {
	mu sync.Mutex
	m  map[string]memEntry // scope+"\x00"+key
}

type memEntry struct {
	fingerprint string
	done        bool
	cached      Cached
	expiresAt   time.Time
}

// NewMemStore 构造空存储。
func NewMemStore() *MemStore { return &MemStore{m: map[string]memEntry{}} }

func memKey(scope, key string) string { return scope + "\x00" + key }

// Claim 原子裁决：过期条目视为不存在（顺带惰性清扫）。
func (s *MemStore) Claim(req Request, claimTTL time.Duration, now time.Time) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 惰性清扫：控制 map 规模（全量扫描只在条目超阈值时触发）。
	if len(s.m) > 1024 {
		for k, e := range s.m {
			if now.After(e.expiresAt) {
				delete(s.m, k)
			}
		}
	}
	k := memKey(req.Scope, req.Key)
	if e, ok := s.m[k]; ok && now.Before(e.expiresAt) {
		if e.fingerprint != req.Fingerprint {
			return Result{State: StateMismatch, Finger: e.fingerprint}, nil
		}
		if e.done {
			return Result{State: StateReplayed, Cached: e.cached}, nil
		}
		return Result{State: StateInProgress, Finger: e.fingerprint}, nil
	}
	s.m[k] = memEntry{fingerprint: req.Fingerprint, expiresAt: now.Add(claimTTL)}
	return Result{State: StateClaimed}, nil
}

// Complete 缓存响应并切换到 replay 窗口。
func (s *MemStore) Complete(key, scope string, cached Cached, replayTTL time.Duration, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[memKey(scope, key)]
	if !ok {
		return fmt.Errorf("idem: complete for unknown key %q (claim expired?)", key)
	}
	e.done, e.cached, e.expiresAt = true, cached, now.Add(replayTTL)
	s.m[memKey(scope, key)] = e
	return nil
}

// Release 释放在途声明（失败可重试）。
func (s *MemStore) Release(key, scope string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[memKey(scope, key)]
	if !ok {
		return nil // 已过期：无需动作
	}
	e.done, e.cached, e.expiresAt = false, Cached{}, now.Add(time.Duration(0))
	s.m[memKey(scope, key)] = e
	// 立即过期：下次 Claim 视为新键。
	delete(s.m, memKey(scope, key))
	return nil
}

// Options 是 Stage 的配置。
type Options struct {
	// Enforced 判定 FullMethod 是否执法（fleetly EnforcedMethods 的
	// 泛化；建议经 grpcapi policy.Set 按方法类别生成 + 守卫反向审计）。
	Enforced func(fullMethod string) bool
	// ClaimTTL 在途窗口（默认 90s，fleetly 实证值）。
	ClaimTTL time.Duration
	// ReplayTTL 回放窗口（默认 24h，fleetly 实证值）。
	ReplayTTL time.Duration
	// Clock 时间源（测试注入 Fake；默认 clock.System）。
	Clock clock.Clock
}

const (
	// DefaultClaimTTL 是在途窗口缺省（fleetly 实证值）。
	DefaultClaimTTL = 90 * time.Second
	// DefaultReplayTTL 是回放窗口缺省（fleetly 实证值）。
	DefaultReplayTTL = 24 * time.Hour
)

// HeaderKey 是幂等键的标准传输头。
const HeaderKey = "idempotency-key"

// Fingerprint 计算请求指纹：sha256(FullMethod + 确定性载荷字节)。
// proto.Marshal 对同一二进制版本是确定性的（文档化保证）。
func Fingerprint(fullMethod string, payload []byte) string {
	h := sha256.New()
	h.Write([]byte(fullMethod))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}
