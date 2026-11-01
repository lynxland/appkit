package storeerr

import (
	"errors"
	"fmt"
	"testing"
)

func TestWrapKeepsBothTargets(t *testing.T) {
	cause := errors.New("driver says no")
	wrapped := Wrap(ErrAlreadyExists, cause, "users")
	if !errors.Is(wrapped, ErrAlreadyExists) {
		t.Error("sentinel lost in wrap")
	}
	if !errors.Is(wrapped, cause) {
		t.Error("cause lost in wrap (multi-%w)")
	}
	// detail 无值形态同样双命中。
	plain := Wrap(ErrNotFound, cause, "")
	if !errors.Is(plain, ErrNotFound) || !errors.Is(plain, cause) {
		t.Error("plain wrap lost targets")
	}
	if got := plain.Error(); got != "not found: driver says no" {
		t.Errorf("plain wrap text = %q", got)
	}
}

func TestPredicates(t *testing.T) {
	outer := fmt.Errorf("call failed: %w", Wrap(ErrConflict, errors.New("40001"), ""))
	if !IsConflict(outer) || !IsConflict(ErrConflict) {
		t.Error("IsConflict must pierce wrapping")
	}
	if IsAlreadyExists(outer) {
		t.Error("predicates must not cross-match")
	}
}

func TestClassifySentinels(t *testing.T) {
	cases := []struct {
		err  error
		want Class
	}{
		{ErrAlreadyExists, ClassPermanent},
		{Wrap(ErrNotFound, errors.New("x"), ""), ClassPermanent},
		{Wrap(ErrForeignKey, errors.New("x"), "t"), ClassPermanent},
		{ErrConflict, ClassPermanent},
		{ErrBusy, ClassTransient},
		{Wrap(ErrBusy, errors.New("40P01"), ""), ClassTransient},
		{errors.New("mystery"), ClassUnknown},
		{nil, ClassUnknown},
	}
	for _, tc := range cases {
		if got := Classify(tc.err); got != tc.want {
			t.Errorf("Classify(%v) = %s, want %s", tc.err, got, tc.want)
		}
	}
}

// TestClassifyCheckerOrder：驱动分类器按序首个确定者胜；哨兵优先于分类器。
func TestClassifyCheckerOrder(t *testing.T) {
	driverErr := errors.New("driver-error")
	first := func(err error) (Class, bool) {
		if errors.Is(err, driverErr) {
			return ClassTransient, true
		}
		return ClassUnknown, false
	}
	second := func(error) (Class, bool) { return ClassPermanent, true } // 永远抢答
	if got := Classify(driverErr, first, second); got != ClassTransient {
		t.Errorf("first decisive classifier must win, got %s", got)
	}
	if got := Classify(errors.New("other"), first, second); got != ClassPermanent {
		t.Errorf("second must answer when first declines, got %s", got)
	}
	// 哨兵优先：即便分类器抢答永久，ErrBusy 仍判瞬时。
	if got := Classify(ErrBusy, second); got != ClassTransient {
		t.Errorf("sentinel semantics must outrank classifiers, got %s", got)
	}
	// nil 分类器切片：纯哨兵路径可用。
	if got := Classify(ErrAlreadyExists); got != ClassPermanent {
		t.Errorf("no classifiers: got %s", got)
	}
}
