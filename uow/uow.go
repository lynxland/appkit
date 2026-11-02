// Package uow 定义工作单元端口（torchwood pkg/uow 的移植，去掉 RLS 执行身份）。
//
// 调用方用 Runner.Run 包住需要同一提交的写。fn 仍接收 ctx，适配器可从
// ctx 读取事务；domain 端口不得把驱动类型写进签名。bun 实现见 contrib/bunx/buntx（appkit/contrib/bunx 子模块）。
package uow

import "context"

// Runner 执行 fn：已在工作单元内则加入，否则开启新单元。
type Runner interface {
	Run(ctx context.Context, fn func(ctx context.Context) error) error
}

// Isolator 在 Runner 之上提供强制独立子事务（不并入 ctx 已有事务），
// 供「子操作必须先于外层事务提交」的路径使用。
type Isolator interface {
	Runner
	RunInNewTx(ctx context.Context, fn func(ctx context.Context) error) error
}
