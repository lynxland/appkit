package bunx

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/lynxland/appkit/crud"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

type pageRow struct {
	bun.BaseModel `bun:"table:page_rows,alias:d"`
	ID            string `bun:"id"`
	State         string `bun:"state"`
}

func pageSQL(t *testing.T, plan crud.PagePlan) string {
	t.Helper()
	db := bun.NewDB(sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN("postgres://u:p@127.0.0.1:1/x"))), pgdialect.New())
	defer func() { _ = db.Close() }()
	q := ApplyPage(db.NewSelect().Model((*pageRow)(nil)), plan)
	b, err := q.AppendQuery(db.QueryGen(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func spec() crud.Spec {
	return crud.Spec{
		Modes:        crud.ModeSetBounded | crud.ModeSetOffset | crud.ModeSetKeyset,
		Sortable:     map[string]string{"id": "d.id", "state": "d.state"},
		Tiebreak:     "id",
		DefaultLimit: 50,
		MaxLimit:     200,
	}
}

func TestApplyPageKeysetFirstPage(t *testing.T) {
	q, err := crud.Parse[any](spec(), crud.RawParams{OrderBy: "id"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sqlText := pageSQL(t, q.PagePlan)
	for _, want := range []string{`"d"."id" ASC`, "LIMIT 50"} {
		if !strings.Contains(sqlText, want) {
			t.Errorf("missing %q in: %s", want, sqlText)
		}
	}
	if strings.Contains(sqlText, "WHERE") {
		t.Errorf("first page must not add WHERE: %s", sqlText)
	}
}

func TestApplyPageKeysetAfterSingleColumn(t *testing.T) {
	q, err := crud.Parse[any](spec(), crud.RawParams{OrderBy: "id"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	q.CursorValues = []string{"row-42"}
	sqlText := pageSQL(t, q.PagePlan)
	if !strings.Contains(sqlText, `d.id > 'row-42'`) {
		t.Errorf("single-column keyset predicate wrong: %s", sqlText)
	}
}

// 混合方向排序的展开 OR 谓词：desc 列取 <，等值前缀链接决胜列。
func TestApplyPageKeysetMixedDirection(t *testing.T) {
	s := spec()
	s.Sortable = map[string]string{"id": "d.id", "state": "d.state"}
	s.Tiebreak = "id"
	q, err := crud.Parse[any](s, crud.RawParams{OrderBy: "state desc"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	q.CursorValues = []string{"open", "row-42"}
	sqlText := pageSQL(t, q.PagePlan)
	want := `(d.state < 'open') OR (d.state = 'open' AND d.id > 'row-42')`
	if !strings.Contains(sqlText, want) {
		t.Errorf("mixed-direction predicate wrong: %s", sqlText)
	}
	if !strings.Contains(sqlText, `"d"."state" DESC`) || !strings.Contains(sqlText, `"d"."id" ASC`) {
		t.Errorf("order direction wrong: %s", sqlText)
	}
}

func TestApplyPageOffsetAndBounded(t *testing.T) {
	q, err := crud.Parse[any](spec(), crud.RawParams{Offset: 40, Limit: 20}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sqlText := pageSQL(t, q.PagePlan)
	if !strings.Contains(sqlText, "LIMIT 20 OFFSET 40") {
		t.Errorf("offset shape wrong: %s", sqlText)
	}
	bounded := crud.Spec{Modes: crud.ModeSetBounded, Sortable: s0(), DefaultLimit: 10, MaxLimit: 10}
	qb, err := crud.Parse[any](bounded, crud.RawParams{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sqlText = pageSQL(t, qb.PagePlan)
	if !strings.Contains(sqlText, "LIMIT 10") || strings.Contains(sqlText, "OFFSET") {
		t.Errorf("bounded shape wrong: %s", sqlText)
	}
}

func s0() map[string]string { return map[string]string{"id": "d.id"} }
