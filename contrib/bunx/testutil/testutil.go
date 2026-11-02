// Package testutil 是模块与应用共享的测试基座：
//   - PgDSN/SkipUnlessPg：测试 Postgres DSN 的环境变量约定；
//   - NewDB：每个测试一个隔离数据库（自动建库 → 逐迁移链 goose Up →
//     t.Cleanup 自动 FORCE DROP），配合 lynxtest 组装层测试使用。
package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/lynxland/appkit/contrib/bunx"
	"github.com/lynxland/appkit/goosekit"
	"github.com/lynxland/appkit/idgen"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/driver/pgdriver"
)

// EnvPgDSN 是集成测试 Postgres DSN 的环境变量名。
const EnvPgDSN = "APPKIT_TEST_PG_DSN"

// PgDSN 返回测试数据库 DSN；未设置时返回空串。
func PgDSN() string { return os.Getenv(EnvPgDSN) }

// SkipUnlessPg 无 DSN 时跳过测试（本地无依赖属常态，Skip 而非 Fail）。
func SkipUnlessPg(t testing.TB) string {
	t.Helper()
	dsn := PgDSN()
	if dsn == "" {
		t.Skipf("%s not set; skipping database test", EnvPgDSN)
	}
	return dsn
}

// Chain 声明测试库需要就绪的一条迁移链。
type Chain struct {
	Module string
	FS     fs.FS
}

// NewDB 起隔离测试库：CREATE DATABASE lstest_<rand> → 逐链 goosekit.Up →
// 返回就绪的 *bun.DB（github.com/uptrace/bun）。t.Cleanup 关连接并 FORCE
// DROP 整库。
func NewDB(t testing.TB, chains ...Chain) *bun.DB {
	t.Helper()
	dsn := SkipUnlessPg(t)

	dbName := "lstest_" + idgen.New()[:12]
	admin := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	if _, err := admin.Exec(fmt.Sprintf(`CREATE DATABASE %s`, dbName)); err != nil {
		_ = admin.Close()
		t.Fatalf("testutil: create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.ExecContext(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, dbName))
		_ = admin.Close()
	})

	testDSN, err := withDatabase(dsn, dbName)
	if err != nil {
		t.Fatalf("testutil: rewrite dsn: %v", err)
	}
	db, err := bunx.OpenDB(testDSN, bunx.Options{})
	if err != nil {
		t.Fatalf("testutil: open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	for _, ch := range chains {
		if err := goosekit.Up(ctx, db.DB, ch.FS, ch.Module); err != nil {
			t.Fatalf("testutil: migrate %s: %v", ch.Module, err)
		}
	}
	return db
}

// withDatabase 把 DSN 的库名替换为 name（保持其余成分不变）。
func withDatabase(dsn, name string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	u.Path = "/" + name
	return u.String(), nil
}
