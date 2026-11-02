// Package bunx 是 bun 适配器（ADR-0009 D5：核心不绑 ORM，bun 是可选实现）：
// 连接引导（DSN→*bun.DB）、Deps.DB 中立句柄的类型化出口（DB/MustDB）、
// 白名单排序应用与泛型列表执行（count + order + 分页 + 领域转换）。
// 排序解析（ParseOrderBy/OrderClause）是 ORM 中立件，住在核心 crud 包。
// 过滤（AIP-160）不内置——动态查询语义属于模块，经 queryModifier 注入。
package bunx

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lynxland/appkit"
	"github.com/lynxland/appkit/crud"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// Options 是 OpenDB 的连接选项。
type Options struct {
	MaxOpenConns int // <=0 不设置（驱动默认）
	Ping         bool
}

// OpenDB 从 Postgres DSN 构造 *bun.DB（pgdriver + pgdialect）。
// Ping=true 时探活失败即返回错误（组合根 fail-fast）。
func OpenDB(dsn string, opts Options) (*bun.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("bunx: dsn is empty")
	}
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	if opts.MaxOpenConns > 0 {
		sqldb.SetMaxOpenConns(opts.MaxOpenConns)
	}
	if opts.Ping {
		if err := sqldb.Ping(); err != nil {
			_ = sqldb.Close()
			return nil, fmt.Errorf("bunx: ping: %w", err)
		}
	}
	return bun.NewDB(sqldb, pgdialect.New()), nil
}

// DB 从 Deps 取出 *bun.DB。Deps.DB 是 ORM 中立句柄（any），bun 应用的
// 具体类型由本适配包约定；类型不符即报错（装配错配应在组合根暴露）。
func DB(deps appkit.Deps) (*bun.DB, error) {
	db, ok := deps.DB.(*bun.DB)
	if !ok {
		return nil, fmt.Errorf("bunx: deps.DB is %T, want *bun.DB", deps.DB)
	}
	return db, nil
}

// MustDB 同 DB，但类型不符时 panic——仅供组合根装配期使用。
func MustDB(deps appkit.Deps) *bun.DB {
	db, err := DB(deps)
	if err != nil {
		panic(err)
	}
	return db
}

// ApplyOrder 把解析结果应用到查询（等值列自动追加主键决胜，保证分页稳定）。
func ApplyOrder(q *bun.SelectQuery, clauses []crud.OrderClause, tiebreak ...string) *bun.SelectQuery {
	for _, c := range clauses {
		q = q.OrderExpr("? ?", bun.Ident(c.Expr), bun.Safe(directionSQL(c.Desc)))
	}
	for _, t := range tiebreak {
		q = q.OrderExpr("? ASC", bun.Ident(t))
	}
	return q
}

func directionSQL(desc bool) string {
	if desc {
		return "DESC"
	}
	return "ASC"
}

// ListSpec 描述一次模块列表查询的执行配置。
type ListSpec[M any, D any] struct {
	// Fields 是 API 字段名 → 列表达式白名单（排序用）。
	Fields map[string]string
	// DefaultOrder 是无排序请求时的兜底子句（如 "d.created_at DESC"）；空则必须由调用方保证确定性。
	DefaultOrder string
	// Tiebreak 列（幂等分页的决胜列，如主键）。
	Tiebreak []string
	// Query 在 count/scan 前注入过滤与关联（where/join）。
	Query func(*bun.SelectQuery) *bun.SelectQuery
	// Convert 是 PO → 领域对象转换。
	Convert func(*M) D
}

// Result 是列表执行产物（分页收尾由调用方用 crud.FinalizeOffsetPage 完成，
// 或直接用带 token 的 List）。
type Result[D any] struct {
	Items []D
	Total int
}

// List 执行 count + 排序 + 分页扫描 + 领域转换；params 来自
// crud.ParseListParams（token 一致性与 offset 上限已校验）。
//
// Deprecated: v1 分页形态（ADR-0009 D11）。新代码用 Spec.Parse 产出
// PagePlan 后经 ApplyPage 编译，配合 crud.Cursor 编码游标；本函数保持
// 编译兼容，随模块换装退场。
func List[M any, D any](ctx context.Context, db *bun.DB, params crud.ListParams, spec ListSpec[M, D]) (*Result[D], error) {
	base := func() *bun.SelectQuery {
		q := db.NewSelect().Model((*M)(nil))
		if spec.Query != nil {
			q = spec.Query(q)
		}
		return q
	}

	total, err := base().Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("bunx: count: %w", err)
	}

	q := base()
	clauses, err := crud.ParseOrderBy(params.OrderBy, spec.Fields)
	if err != nil {
		return nil, err
	}
	if len(clauses) > 0 {
		q = ApplyOrder(q, clauses, spec.Tiebreak...)
	} else if spec.DefaultOrder != "" {
		q = q.OrderExpr("? ", bun.Safe(spec.DefaultOrder))
		for _, t := range spec.Tiebreak {
			q = q.OrderExpr("? ASC", bun.Ident(t))
		}
	}
	if params.Offset > 0 {
		q = q.Offset(int64(params.Offset))
	}
	if params.PageSize > 0 {
		q = q.Limit(int64(params.PageSize))
	}

	var models []M
	if err := q.Scan(ctx, &models); err != nil {
		return nil, fmt.Errorf("bunx: scan: %w", err)
	}
	out := &Result[D]{Items: make([]D, 0, len(models)), Total: int(total)}
	for i := range models {
		out.Items = append(out.Items, spec.Convert(&models[i]))
	}
	return out, nil
}
