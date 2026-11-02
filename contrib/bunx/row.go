package bunx

import (
	"time"

	"github.com/lynxland/appkit/row"
	"github.com/uptrace/bun"
)

// StampInsert 给插入模型补零值双戳（created_at/updated_at）——显式
// 调用，时钟由调用方注入（clock.Clock），时间来源可追溯；非零值不覆写
// （调用方预设的时间被尊重，幂等安全网语义）。
func StampInsert(m row.Stamper, now time.Time) {
	s := m.RowStamps()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = now
	}
}

// StampUpdate 刷新更新戳（updated_at=now；created_at 不动——创建时刻
// 是不可变事实）。
func StampUpdate(m row.Stamper, now time.Time) {
	m.RowStamps().UpdatedAt = now
}

// SoftDelete 把删除语义改写为 tombstone UPDATE：deleted_at=now。
// 模型需带 deleted_at 列（嵌入 row.Tombstone）；配合 ExcludeTombstoned
// 构成完整软删形态。永不物理 DELETE（row.Tombstone 语义）。
func SoftDelete(q *bun.UpdateQuery, now time.Time) *bun.UpdateQuery {
	return q.Set("deleted_at = ?", now)
}

// ExcludeTombstoned 排除软删行（业务查询规约：tombstone 不可见）。
func ExcludeTombstoned(q *bun.SelectQuery) *bun.SelectQuery {
	return q.Where("deleted_at IS NULL")
}
