package testutil

import (
	"context"
	"embed"
	"io/fs"
	"testing"

	"github.com/lynxland/appkit/contrib/bunx"
	"github.com/lynxland/appkit/crud"
	"github.com/uptrace/bun"
)

//go:embed testdata/migrations/*.sql
var migFS embed.FS

func testMigrations(t *testing.T) fs.FS {
	t.Helper()
	sub, err := fs.Sub(migFS, "testdata/migrations")
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

type thingModel struct {
	bun.BaseModel `bun:"table:thing,alias:t"`
	ID            string `bun:"id,pk"`
	Name          string `bun:"name,notnull"`
	Seq           int    `bun:"seq,notnull"`
}

// TestNewDBIsolationAndMigrate 验证隔离建库、迁移链就绪与 bunx.List 协作。
// 集成测试：换装 v2 前持续守护编译兼容面。
//
//nolint:staticcheck // v1 管线（ParseListParams/bunx.List/EncodePageToken）的
func TestNewDBIsolationAndMigrate(t *testing.T) {
	db := NewDB(t, Chain{Module: "testmod", FS: testMigrations(t)})

	ctx := context.Background()
	_, err := db.NewInsert().Model(&[]thingModel{
		{ID: "a", Name: "alpha", Seq: 3},
		{ID: "b", Name: "beta", Seq: 1},
		{ID: "c", Name: "gamma", Seq: 2},
	}).Exec(ctx)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	params, _ := crud.ParseListParams(2, "", "", "seq desc")
	res, err := bunx.List[thingModel, string](ctx, db, params, bunx.ListSpec[thingModel, string]{
		Fields:       map[string]string{"seq": "t.seq", "name": "t.name"},
		DefaultOrder: "t.seq DESC",
		Tiebreak:     []string{"t.id"},
		Convert:      func(m *thingModel) string { return m.ID },
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if res.Total != 3 || len(res.Items) != 2 || res.Items[0] != "a" {
		t.Fatalf("list result = total %d items %v", res.Total, res.Items)
	}

	// 第二页。
	tok, err := crud.EncodePageToken(2)
	if err != nil {
		t.Fatal(err)
	}
	params2, _ := crud.ParseListParams(2, tok, "", "seq desc")
	res2, err := bunx.List[thingModel, string](ctx, db, params2, bunx.ListSpec[thingModel, string]{
		Fields:       map[string]string{"seq": "t.seq"},
		DefaultOrder: "t.seq DESC",
		Tiebreak:     []string{"t.id"},
		Convert:      func(m *thingModel) string { return m.ID },
	})
	if err != nil {
		t.Fatalf("list page2: %v", err)
	}
	if len(res2.Items) != 1 || res2.Items[0] != "b" {
		t.Fatalf("page2 = %v", res2.Items)
	}
}
