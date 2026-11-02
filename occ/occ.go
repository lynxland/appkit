// Package occ 是乐观并发与状态机 CAS（ADR-0009 D12 数据访问电池族③，
// 证据 2×：torchwood current_version+expected_version 字段违规回传、
// fleetly Transit——state∈from + generation 单调）。与 storeerr 咬合：
// 版本/状态失配统一落 storeerr.ErrConflict（传输层投影含两侧值的要求
// 见 roadmap ⑨ errstatus）。
//
// 两种写侧防线：
//
//   - 版本防线（version 列，乐观并发）：API 携带 expected_version，
//     UPDATE 经适配器 IfVersion 附加 WHERE version=expected 与自增；
//     影响行数 0 即失配（见 bunx.MapGuardedUpdate）。
//   - 状态防线（状态机列，fleetly Transit 形态）：声明迁移的合法起点集
//     与目标，UPDATE 经 IfState 附加起点守卫。
package occ

import (
	"fmt"

	"github.com/lynxland/appkit/storeerr"
)

// Next 返回下一版本。约定：版本从 1 起、单调递增（current<=0 视为尚未
// 建版，首版为 1）；generation/重试语义由调用方叠加。
func Next(current int64) int64 {
	if current <= 0 {
		return 1
	}
	return current + 1
}

// Check 校验实际版本与期望一致；失配返回携带两侧值的 ErrConflict 包裹
// （errors.Is 命中 storeerr.ErrConflict；消息含 have/want 便于投影）。
func Check(actual, expected int64) error {
	if actual != expected {
		return fmt.Errorf("%w: version mismatch (have %d, want %d)", storeerr.ErrConflict, actual, expected)
	}
	return nil
}

// Expect 是 API 字段 expected_version 的领域形态（torchwood
// expected_version 的泛化）：携带期望版本的值对象。
type Expect struct {
	Version int64
}

// Matches 报告实际版本是否满足期望。
func (e Expect) Matches(actual int64) bool { return actual == e.Version }

// Transition 声明一次状态机迁移：合法起点集 From（空 = 任意起点）与
// 目标 To。写侧守卫（WHERE state ∈ From）由适配器编译（bunx.IfState）。
type Transition struct {
	From []string
	To   string
}

// Allows 报告 from 是否在合法起点集（From 空 = 任意起点）。
func (t Transition) Allows(from string) bool {
	if len(t.From) == 0 {
		return true
	}
	for _, f := range t.From {
		if f == from {
			return true
		}
	}
	return false
}

// Apply 校验起点合法性并返回目标状态；非法起点返回携带 from→to 的
// ErrConflict 包裹（并发下的状态迁移竞争在此显形）。
func (t Transition) Apply(from string) (string, error) {
	if !t.Allows(from) {
		return "", fmt.Errorf("%w: illegal transition %q -> %q (allowed from %v)", storeerr.ErrConflict, from, t.To, t.From)
	}
	return t.To, nil
}
