package row

import (
	"testing"
	"time"
)

type stampedRow struct {
	Stamps
	ID string
}

func TestStampsEmbedSatisfiesStamper(t *testing.T) {
	var m Stamper = &stampedRow{}
	if m.RowStamps() == nil {
		t.Fatal("embedded Stamps must satisfy Stamper")
	}
}

func TestTombstone(t *testing.T) {
	var tb Tombstone
	if tb.IsDeleted() {
		t.Error("zero tombstone must be alive")
	}
	t1 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	tb.MarkDeleted(t1)
	if !tb.IsDeleted() || !tb.DeletedAt.Equal(t1) {
		t.Fatalf("mark: %+v", tb.DeletedAt)
	}
	// 幂等：再次打戳不覆写（首次删除时刻不可变）。
	t2 := t1.Add(time.Hour)
	tb.MarkDeleted(t2)
	if !tb.DeletedAt.Equal(t1) {
		t.Fatalf("re-mark must not overwrite: %v", tb.DeletedAt)
	}
}

func TestTimeCanonicalForm(t *testing.T) {
	// 带偏移时区输入 → 归一 UTC 输出。
	loc := time.FixedZone("CST", 8*3600)
	got := FormatTime(time.Date(2026, 11, 1, 18, 30, 5, 123, loc))
	want := "2026-11-01T10:30:05Z"
	if got != want {
		t.Fatalf("FormatTime = %q, want %q（亚秒丢弃：秒精度规范形态）", got, want)
	}
	// 往返：解析归一 UTC，亚秒信息按规范丢弃。
	back, err := ParseTime(want)
	if err != nil || !back.Equal(time.Date(2026, 11, 1, 10, 30, 5, 0, time.UTC)) {
		t.Fatalf("ParseTime roundtrip: %v %v", back, err)
	}
	if _, err := ParseTime("not-a-time"); err == nil {
		t.Error("malformed input must error")
	}
	// 字典序 = 时间序（keyset 游标前提）。
	a, b := FormatTime(t1), FormatTime(t1.Add(time.Second))
	if a >= b {
		t.Errorf("lexicographic order broken: %q vs %q", a, b)
	}
}

var t1 = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
