package occ

import (
	"errors"
	"strings"
	"testing"

	"github.com/lynxland/appkit/storeerr"
)

func TestNext(t *testing.T) {
	cases := []struct{ current, want int64 }{{0, 1}, {-3, 1}, {1, 2}, {41, 42}}
	for _, tc := range cases {
		if got := Next(tc.current); got != tc.want {
			t.Errorf("Next(%d) = %d, want %d", tc.current, got, tc.want)
		}
	}
}

func TestCheck(t *testing.T) {
	if err := Check(7, 7); err != nil {
		t.Errorf("matching versions must pass: %v", err)
	}
	err := Check(3, 7)
	if !errors.Is(err, storeerr.ErrConflict) {
		t.Errorf("mismatch must be ErrConflict, got %v", err)
	}
	// 消息含两侧值（投影/排障需要）。
	for _, want := range []string{"3", "7"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message %q missing %s", err.Error(), want)
		}
	}
	// 包裹链穿透。
	if !storeerr.IsConflict(errors.Join(errors.New("outer"), err)) {
		t.Error("IsConflict must pierce wrapping")
	}
}

func TestExpect(t *testing.T) {
	e := Expect{Version: 5}
	if !e.Matches(5) || e.Matches(4) {
		t.Error("Expect.Matches broken")
	}
}

func TestTransition(t *testing.T) {
	open := Transition{From: []string{"open"}, To: "done"}

	next, err := open.Apply("open")
	if err != nil || next != "done" {
		t.Errorf("Apply(open) = (%q, %v)", next, err)
	}
	_, err = open.Apply("done")
	if !errors.Is(err, storeerr.ErrConflict) {
		t.Errorf("illegal transition must be ErrConflict, got %v", err)
	}
	if !strings.Contains(err.Error(), "done") || !strings.Contains(err.Error(), "open") {
		t.Errorf("message must carry both states: %q", err.Error())
	}

	// From 空 = 任意起点。
	wild := Transition{To: "archived"}
	if !wild.Allows("anything") {
		t.Error("empty From must allow any source")
	}
	if next, err := wild.Apply("x"); err != nil || next != "archived" {
		t.Errorf("wildcard Apply: (%q, %v)", next, err)
	}
	// 多起点集。
	multi := Transition{From: []string{"draft", "paused"}, To: "published"}
	if !multi.Allows("draft") || !multi.Allows("paused") || multi.Allows("published") {
		t.Error("multi-source Allows broken")
	}
}
