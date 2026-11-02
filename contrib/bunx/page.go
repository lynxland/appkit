package bunx

import (
	"fmt"
	"strings"

	"github.com/lynxland/appkit/crud"
	"github.com/uptrace/bun"
)

// ApplyPage 把 crud.PagePlan 编译为 bun 查询：排序（含强制决胜列）+
// 模式形态。过滤不在此处——filter 是调用方 Query hook 的职责（plan 不
// 携带 filter）。
//
// keyset 谓词采用展开 OR 形态（而非紧凑行值比较）：每列按自身方向取
// 比较算子（asc→>、desc→<），以等值前缀链组合——混合方向排序（如
// "state desc, created_at asc"）下唯一正确的形态，PG 亦可走索引。
func ApplyPage(q *bun.SelectQuery, plan crud.PagePlan) *bun.SelectQuery {
	for _, c := range plan.Order {
		q = q.OrderExpr("? ?", bun.Ident(c.Expr), bun.Safe(directionSQL(c.Desc)))
	}
	switch plan.Mode {
	case crud.ModeKeyset:
		if len(plan.CursorValues) > 0 {
			q = q.Where(keysetPredicate(plan.Order), keysetArgs(plan.Order, plan.CursorValues)...)
		}
		if plan.Limit > 0 {
			q = q.Limit(int64(plan.Limit))
		}
	case crud.ModeOffset:
		if plan.Offset > 0 {
			q = q.Offset(int64(plan.Offset))
		}
		if plan.Limit > 0 {
			q = q.Limit(int64(plan.Limit))
		}
	default: // bounded
		if plan.Limit > 0 {
			q = q.Limit(int64(plan.Limit))
		}
	}
	return q
}

// keysetPredicate 生成 (e1 cmp1 ? OR (e1 = ? AND e2 cmp2 ?) OR …) 的
// 展开 OR 谓词；列表达式来自白名单映射，安全内联，值经参数绑定。
func keysetPredicate(order []crud.OrderClause) string {
	parts := make([]string, len(order))
	for i := range order {
		cmp := ">"
		if order[i].Desc {
			cmp = "<"
		}
		eq := make([]string, 0, i)
		for j := 0; j < i; j++ {
			eq = append(eq, fmt.Sprintf("%s = ?", order[j].Expr))
		}
		prefix := strings.Join(eq, " AND ")
		last := fmt.Sprintf("%s %s ?", order[i].Expr, cmp)
		if prefix != "" {
			last = prefix + " AND " + last
		}
		parts[i] = "(" + last + ")"
	}
	return strings.Join(parts, " OR ")
}

// keysetArgs 按 keysetPredicate 的占位符顺序产出绑定参数（每段一个
// 新值 + i 个等值重复值）。
func keysetArgs(order []crud.OrderClause, values []string) []any {
	args := make([]any, 0, len(order)*(len(order)+1)/2)
	for i := range order {
		for j := 0; j < i; j++ {
			args = append(args, values[j])
		}
		args = append(args, values[i])
	}
	return args
}
