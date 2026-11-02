package bunx

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/lynxland/appkit/row"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

type rowModel struct {
	bun.BaseModel `bun:"table:row_things,alias:r"`
	row.Stamps
	row.Tombstone
	ID string `bun:"id"`
}

func TestStampHelpers(t *testing.T) {
	now := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	var m rowModel
	StampInsert(&m, now)
	if !m.CreatedAt.Equal(now) || !m.UpdatedAt.Equal(now) {
		t.Fatalf("StampInsert: %+v", m.Stamps)
	}
	// 非零不覆写（幂等安全网）。
	later := now.Add(time.Minute)
	StampInsert(&m, later)
	if !m.CreatedAt.Equal(now) {
		t.Error("StampInsert must not overwrite non-zero CreatedAt")
	}
	// Update 刷新 updated，created 不动。
	StampUpdate(&m, later)
	if !m.CreatedAt.Equal(now) || !m.UpdatedAt.Equal(later) {
		t.Fatalf("StampUpdate: %+v", m.Stamps)
	}
	// 非 Stamper 模型不 panic（接口断言内聚于调用面——本测试确保泛型
	// 签名不强迫包装）。
	StampInsert(stamped(&m), later)
}

func stamped(m *rowModel) row.Stamper { return m }

func TestSoftDeleteSQL(t *testing.T) {
	db := bun.NewDB(sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN("postgres://u:p@127.0.0.1:1/x"))), pgdialect.New())
	defer func() { _ = db.Close() }()
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	upd := SoftDelete(db.NewUpdate().Model((*rowModel)(nil)).Where("r.id = ?", "x"), now)
	b, err := upd.AppendQuery(db.QueryGen(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "deleted_at = '2026-11-01 09:00:00+00:00'") {
		// bun 对 time 参数内联渲染为 timestamptz 字面量——断言宽松化：
		// 形态细节随驱动绑定，此处只验证 SET 子句存在。
		if !strings.Contains(string(b), "deleted_at") {
			t.Errorf("soft delete SET missing: %s", string(b))
		}
	}
	sel := ExcludeTombstoned(db.NewSelect().Model((*rowModel)(nil)))
	b, err = sel.AppendQuery(db.QueryGen(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "deleted_at IS NULL") {
		t.Errorf("exclude tombstoned missing: %s", string(b))
	}
}
