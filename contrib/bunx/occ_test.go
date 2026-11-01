package bunx

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/lynx-go/lynx-start/occ"
	"github.com/lynx-go/lynx-start/storeerr"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

type occRow struct {
	bun.BaseModel `bun:"table:occ_rows,alias:o"`
	ID            string `bun:"id"`
	Version       int64  `bun:"version"`
	Status        string `bun:"status"`
}

func occSQL(t *testing.T, build func(*bun.UpdateQuery) *bun.UpdateQuery) string {
	t.Helper()
	db := bun.NewDB(sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN("postgres://u:p@127.0.0.1:1/x"))), pgdialect.New())
	defer func() { _ = db.Close() }()
	q := build(db.NewUpdate().Model((*occRow)(nil)).Where("o.id = ?", "r1"))
	b, err := q.AppendQuery(db.QueryGen(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestIfVersion(t *testing.T) {
	sqlText := occSQL(t, func(q *bun.UpdateQuery) *bun.UpdateQuery {
		return IfVersion(q, "version", 41)
	})
	for _, want := range []string{`"version" = 41`, `"version" = "version" + 1`} {
		if !strings.Contains(sqlText, want) {
			t.Errorf("missing %q in: %s", want, sqlText)
		}
	}
}

func TestIfState(t *testing.T) {
	sqlText := occSQL(t, func(q *bun.UpdateQuery) *bun.UpdateQuery {
		return IfState(q, "status", occ.Transition{From: []string{"active"}, To: "revoked"})
	})
	if !strings.Contains(sqlText, `"status" IN ('active')`) {
		t.Errorf("guard wrong: %s", sqlText)
	}
	if !strings.Contains(sqlText, `"status" = 'revoked'`) {
		t.Errorf("set wrong: %s", sqlText)
	}
	// From 空：无守卫，仅 SET。
	wild := occSQL(t, func(q *bun.UpdateQuery) *bun.UpdateQuery {
		return IfState(q, "status", occ.Transition{To: "archived"})
	})
	if strings.Contains(wild, "IN (") {
		t.Errorf("empty From must not guard: %s", wild)
	}
}

type fakeResult struct{ n int64 }

func (f fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (f fakeResult) RowsAffected() (int64, error) { return f.n, nil }

func TestMapGuardedUpdate(t *testing.T) {
	if err := MapGuardedUpdate(fakeResult{1}, nil); err != nil {
		t.Errorf("applied update must pass: %v", err)
	}
	if err := MapGuardedUpdate(fakeResult{0}, nil); !storeerr.IsConflict(err) {
		t.Errorf("0 rows must be ErrConflict, got %v", err)
	}
	// 驱动错误经 MapError 归一。
	pg := pgdriver.NewError(map[byte]string{'C': "23505"})
	if err := MapGuardedUpdate(nil, errors.Join(errors.New("upd"), pg)); !storeerr.IsAlreadyExists(err) {
		t.Errorf("driver error must map, got %v", err)
	}
}
