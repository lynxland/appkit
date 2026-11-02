// Package buntx 是 uow 端口的 bun 实现：事务经 context 传播，repo 适配器
// 用 Conn(ctx) 取当前连接（事务内=事务连接，否则=根连接）。
package buntx

import (
	"context"

	"github.com/lynxland/appkit/uow"
	"github.com/uptrace/bun"
)

type txKey struct{}

// WithTx 把 bun 事务放入 ctx（Run/RunInNewTx 内部使用；测试可直接构造）。
func WithTx(ctx context.Context, tx bun.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// InTx 报告 ctx 是否携带活动事务。
func InTx(ctx context.Context) bool {
	_, ok := ctx.Value(txKey{}).(bun.Tx)
	return ok
}

// Conn 返回 ctx 携带的连接：有事务用事务连接，否则用根数据库。repo 的
// 一切读写必须经此入口，保证加入调用方的工作单元。
func Conn(ctx context.Context, db *bun.DB) bun.IDB {
	if tx, ok := ctx.Value(txKey{}).(bun.Tx); ok {
		return tx
	}
	return db
}

// Manager 实现 uow.Runner 与 uow.Isolator（wire 提供，绑定到 uow 端口）。
type Manager struct {
	db *bun.DB
}

// New 构造 Manager。
func New(db *bun.DB) *Manager { return &Manager{db: db} }

var (
	_ uow.Runner   = (*Manager)(nil)
	_ uow.Isolator = (*Manager)(nil)
)

// Run 实现 uow.Runner：已在事务内则加入（同一提交），否则开启新事务。
// 嵌套传播而非嵌套事务：独立连接上的嵌套事务会因外层持有的锁死锁。
func (m *Manager) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	if InTx(ctx) {
		return fn(ctx)
	}
	return m.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return fn(WithTx(ctx, tx))
	})
}

// RunInNewTx 实现 uow.Isolator：无视 ctx 已有事务，强制开独立事务并提交
// （子操作先于外层提交的路径）。调用方自行确保两事务不触碰互加锁的行。
func (m *Manager) RunInNewTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return m.db.RunInTx(context.WithoutCancel(ctx), nil, func(ctx context.Context, tx bun.Tx) error {
		return fn(WithTx(ctx, tx))
	})
}
