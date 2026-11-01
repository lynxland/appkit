package bunx

import (
	"database/sql"

	"github.com/lynx-go/lynx-start/occ"
	"github.com/lynx-go/lynx-start/storeerr"
	"github.com/uptrace/bun"
)

// IfVersion 给 UPDATE 追加乐观并发防线：WHERE <col>=expected 且
// SET <col>=<col>+1（版本从 1 起，见 occ.Next）。影响行数为 0 即版本
// 失配或行不存在——判定见 MapGuardedUpdate。
func IfVersion(q *bun.UpdateQuery, column string, expected int64) *bun.UpdateQuery {
	return q.Where("? = ?", bun.Ident(column), expected).
		Set("? = ? + 1", bun.Ident(column), bun.Ident(column))
}

// IfState 给 UPDATE 追加状态机守卫（fleetly Transit 形态）：起点集非空
// 时 WHERE <col> ∈ From，SET <col>=To。起点集空 = 无守卫（调用方自行
// 保证语义）。与 occ.Transition 声明配套使用。
func IfState(q *bun.UpdateQuery, column string, t occ.Transition) *bun.UpdateQuery {
	if len(t.From) > 0 {
		q = q.Where("? IN (?)", bun.Ident(column), bun.List(t.From))
	}
	return q.Set("? = ?", bun.Ident(column), t.To)
}

// MapGuardedUpdate 把守卫式 UPDATE（IfVersion/IfState 追加防线后的）结果
// 归一：驱动错误经 MapError；影响行数为 0 → storeerr.ErrConflict（版本
// 失配/起点非法/行不存在的合并形态——需要区分时由调用方先 Get 再判定，
// 或在 API 面以 409 语义统一表达，见 roadmap ⑨ errstatus）。
func MapGuardedUpdate(res sql.Result, err error) error {
	if err != nil {
		return MapError(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return storeerr.ErrConflict
	}
	return nil
}
