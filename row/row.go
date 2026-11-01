// Package row 是行规约（ADR-0009 D12 数据访问电池族④，证据 2×：
// fleetly FormatTime 单源+FakeClock 黄金时间戳+tombstone never-reuse、
// torchwood nullzero 约定+审计表）：时间三件（created_at/updated_at/
// deleted_at）的列形态与语义约定 + 规范时间文本形态。ORM 中立——模型
// 嵌入本包类型即声明列存在；自动维护归适配器（contrib/bunx 的
// StampInsert/StampUpdate——显式调用、时钟由调用方注入）。
//
// 刻意不做魔法钩子（bun 的 DB 级 QueryHook 拿不到模型、模型级
// BeforeAppendModel 会把 ORM 词汇渗进中立契约，且隐式时间源不可追溯）：
// 时间戳维护保持显式一句式——与 torchwood 显式列白名单、fleetly 显式
// commitTransition 同哲学。
package row

import (
	"fmt"
	"time"
)

// Stamps 是审计时间列组：created_at / updated_at（适配器按 bun 默认
// snake_case 列名识别；存储形态 timestamptz 或 TEXT 皆可，TEXT 用本包
// 规范形态）。
type Stamps struct {
	CreatedAt time.Time
	UpdatedAt time.Time
}

// RowStamps 暴露行规约戳记——嵌入 Stamps 的模型即满足 Stamper，
// 适配器经此识别并写回。
func (s *Stamps) RowStamps() *Stamps { return s }

// Stamper 由嵌入 Stamps 的模型满足（适配器识别口）。
type Stamper interface{ RowStamps() *Stamps }

// Tombstone 是软删列组：deleted_at。语义（fleetly 教训）：
//   - 非零 = 已删，对业务查询不可见（ExcludeTombstoned 适配器件）；
//   - **永不复用**——tombstone 行占位使 seq/ID 不回卷，keyset 游标稳定；
//   - 删除语义改写为 UPDATE（bunx.SoftDelete），不做物理 DELETE。
type Tombstone struct {
	DeletedAt *time.Time
}

// MarkDeleted 打 tombstone（幂等：已删不覆写原时间——首次删除时刻是
// 事实，不可变）。
func (t *Tombstone) MarkDeleted(now time.Time) {
	if t.DeletedAt == nil {
		t.DeletedAt = &now
	}
}

// IsDeleted 报告软删状态。
func (t *Tombstone) IsDeleted() bool { return t.DeletedAt != nil }

// TimeLayout 是规范时间文本形态：RFC3339、UTC、秒精度。
const TimeLayout = time.RFC3339

// FormatTime 输出规范时间文本（fleetly FormatTime 的泛化）——TEXT 存储
// 的单源，也是 crud keyset 游标时间轴值的规范编码（字典序 = 时间序，
// tiebreak 稳定的前提）。
func FormatTime(t time.Time) string { return t.UTC().Format(TimeLayout) }

// ParseTime 解析规范时间文本（接受任意时区偏移形式，归一 UTC）。
func ParseTime(s string) (time.Time, error) {
	t, err := time.Parse(TimeLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("row: parse time %q: %w", s, err)
	}
	return t.UTC(), nil
}
