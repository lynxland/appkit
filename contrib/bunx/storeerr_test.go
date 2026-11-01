package bunx

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/lynx-go/lynx-start/storeerr"
	"github.com/uptrace/bun/driver/pgdriver"
)

func pgErr(code string) error {
	return pgdriver.NewError(map[byte]string{'C': code, 't': "things"})
}

// 值类型坑回归：pgdriver.Error 经包裹链后 As 值目标仍须命中。
func TestMapErrorValueAsPitfall(t *testing.T) {
	wrapped := fmt.Errorf("insert failed: %w", pgErr("23505"))
	mapped := MapError(wrapped)
	if !storeerr.IsAlreadyExists(mapped) {
		t.Fatalf("23505 must map to ErrAlreadyExists: %v", mapped)
	}
	if !errors.Is(mapped, wrapped) {
		t.Error("driver cause lost in mapping")
	}
	var pg pgdriver.Error
	if !errors.As(mapped, &pg) || pg.Field('C') != "23505" {
		t.Error("pgdriver.Error must remain reachable (multi-%w)")
	}
}

func TestMapErrorCodes(t *testing.T) {
	cases := []struct {
		code string
		want func(error) bool
	}{
		{"23505", storeerr.IsAlreadyExists},
		{"23503", storeerr.IsForeignKey},
		{"40001", storeerr.IsConflict},
		{"40P01", storeerr.IsBusy},
		{"53300", storeerr.IsBusy},
	}
	for _, tc := range cases {
		mapped := MapError(pgErr(tc.code))
		if !tc.want(mapped) {
			t.Errorf("SQLSTATE %s mapped wrong: %v", tc.code, mapped)
		}
	}
	// no-rows → ErrNotFound。
	if mapped := MapError(sql.ErrNoRows); !storeerr.IsNotFound(mapped) {
		t.Errorf("ErrNoRows must map to ErrNotFound: %v", mapped)
	}
	// 未知错误原样透传。
	mystery := errors.New("mystery")
	if got := MapError(mystery); got != mystery {
		t.Errorf("unknown error must pass through, got %v", got)
	}
}

func TestClassifyPG(t *testing.T) {
	transient := []string{"40001", "40P01", "53300", "57P03", "08000", "08006"}
	for _, code := range transient {
		if class, ok := Classify(pgErr(code)); !ok || class != storeerr.ClassTransient {
			t.Errorf("SQLSTATE %s: class=%s ok=%v, want transient/true", code, class, ok)
		}
	}
	permanent := []string{"23505", "23503", "23502", "22P02"}
	for _, code := range permanent {
		if class, ok := Classify(pgErr(code)); !ok || class != storeerr.ClassPermanent {
			t.Errorf("SQLSTATE %s: class=%s ok=%v, want permanent/true", code, class, ok)
		}
	}
	// 未识别/非驱动：交还调用方。
	if _, ok := Classify(errors.New("nope")); ok {
		t.Error("non-pgdriver error must not be claimed")
	}
	// 经 storeerr.Classify 组合：Busy 哨兵瞬时、23505 映射后永久。
	if got := storeerr.Classify(MapError(pgErr("40P01"))); got != storeerr.ClassTransient {
		t.Errorf("combined classify busy: %s", got)
	}
	if got := storeerr.Classify(MapError(pgErr("23505")), Classify); got != storeerr.ClassPermanent {
		t.Errorf("combined classify unique: %s", got)
	}
}
