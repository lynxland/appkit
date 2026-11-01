package bunx

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

type upRow struct {
	bun.BaseModel `bun:"table:up_things,alias:u"`
	Key           string `bun:"key"`
	Val           string `bun:"val"`
	Count         int    `bun:"count"`
}

func upsertSQL(t *testing.T, build func(*bun.InsertQuery) *bun.InsertQuery) string {
	t.Helper()
	db := bun.NewDB(sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN("postgres://u:p@127.0.0.1:1/x"))), pgdialect.New())
	defer func() { _ = db.Close() }()
	b, err := build(db.NewInsert().Model(&upRow{Key: "k", Val: "v"})).AppendQuery(db.QueryGen(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestUpsertDoUpdate(t *testing.T) {
	sqlText := upsertSQL(t, func(q *bun.InsertQuery) *bun.InsertQuery {
		return Upsert(q, []string{"key"}, []string{"val", "count"})
	})
	for _, want := range []string{"ON CONFLICT (\"key\") DO UPDATE", "val = EXCLUDED.val", "count = EXCLUDED.count"} {
		if !strings.Contains(sqlText, want) {
			t.Errorf("missing %q in: %s", want, sqlText)
		}
	}
}

func TestUpsertDoNothing(t *testing.T) {
	sqlText := upsertSQL(t, func(q *bun.InsertQuery) *bun.InsertQuery {
		return Upsert(q, []string{"key"}, nil)
	})
	if !strings.Contains(sqlText, "DO NOTHING") {
		t.Errorf("missing DO NOTHING: %s", sqlText)
	}
}

func TestUpsertNoKeys(t *testing.T) {
	sqlText := upsertSQL(t, func(q *bun.InsertQuery) *bun.InsertQuery {
		return Upsert(q, nil, []string{"val"})
	})
	if strings.Contains(sqlText, "ON CONFLICT") {
		t.Errorf("no keys should not add ON CONFLICT: %s", sqlText)
	}
}
