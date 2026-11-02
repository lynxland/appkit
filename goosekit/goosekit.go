// Package goosekit 实现迁移链隔离（ADR-0004 契约④）：每模块独立 goose 版本表
// （goose_<module>_db），链间永不重排号，跨模块引用只存 ID、不建外键。
//
// 模块把迁移 embed 为 fs.FS（Module.Migrations），应用侧 migrate-all 遍历
// 已装模块逐链执行；版本表名由模块名派生，安装器与 doctor 用同一函数对账。
package goosekit

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"github.com/pressly/goose/v3"
)

// TableName 返回模块的 goose 版本表名：goose_<module>_db（中划线转下划线）。
func TableName(module string) string {
	return "goose_" + strings.ReplaceAll(module, "-", "_") + "_db"
}

// tableNameRe 是派生表名的就近防注入防御：只许小写字母/数字/下划线。
// 与模块命名规约（appkit.ValidateName）相互独立——本包是 ORM 中立电池，
// 不 import 根契约。
var tableNameRe = regexp.MustCompile(`^[a-z0-9_]+$`)

// provider 构造模块迁移链的 goose provider（root=迁移目录本身）。
func provider(db *sql.DB, fsys fs.FS, module string) (*goose.Provider, error) {
	table := TableName(module)
	if !tableNameRe.MatchString(table) {
		return nil, fmt.Errorf("goosekit: module name %q derives unsafe table name", module)
	}
	return goose.NewProvider(goose.DialectPostgres, db, fsys,
		goose.WithTableName(table),
		// 应用侧逐链执行时放宽乱序（不同链无共享序），链内仍严格递增。
		goose.WithAllowOutofOrder(true),
	)
}

// Up 执行模块迁移链的全部未应用迁移。
func Up(ctx context.Context, db *sql.DB, fsys fs.FS, module string) error {
	p, err := provider(db, fsys, module)
	if err != nil {
		return err
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("goosekit: migrate %s up: %w", module, err)
	}
	return nil
}

// Down 回退模块迁移链最近 n 个迁移（n<0 = 全部回退）。
func Down(ctx context.Context, db *sql.DB, fsys fs.FS, module string, n int) error {
	p, err := provider(db, fsys, module)
	if err != nil {
		return err
	}
	if _, err := p.DownTo(ctx, int64(n)); err != nil {
		return fmt.Errorf("goosekit: migrate %s down: %w", module, err)
	}
	return nil
}

// Version 返回模块迁移链的当前版本（0 = 未迁移）。
func Version(ctx context.Context, db *sql.DB, fsys fs.FS, module string) (int64, error) {
	p, err := provider(db, fsys, module)
	if err != nil {
		return 0, err
	}
	v, err := p.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("goosekit: migrate %s version: %w", module, err)
	}
	return v, nil
}
