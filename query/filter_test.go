package query

import "testing"

func TestConstructors(t *testing.T) {
	f := And(
		Eq("status", "active"),
		Or(
			Gt("created", "2026-01-01"),
			In("org", "a", "b"),
		),
		Between("seq", 1, 100),
	)
	if err := f.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestValidateNegatives(t *testing.T) {
	cases := []Filter{
		{Op: OpLogicalAnd, Children: []Filter{}},                                        // 空子
		{Op: OpLogicalAnd, Attribute: "x", Children: []Filter{Eq("a", 1)}},              // 组合带属性
		{Op: OpEqual, Values: []any{}},                                                  // 缺属性+值
		{Op: OpBetween, Attribute: "x", Values: []any{1}},                               // 需要 2
		{Op: OpIn, Attribute: "x", Values: []any{}},                                     // 需要 >=1
		{Op: OpIsNull, Attribute: "x", Values: []any{1}},                                // 不带值
		{Op: OpEqual, Attribute: "x", Values: []any{1}, Children: []Filter{Eq("y", 2)}}, // 叶带子
	}
	for i, f := range cases {
		if err := f.Validate(); err == nil {
			t.Errorf("[%d] want error: %+v", i, f)
		}
	}
}

func TestFlatten(t *testing.T) {
	f := And(Eq("a", 1), And(Eq("b", 2), Eq("c", 3)))
	if len(f.Children) != 3 { // 同层展平
		t.Fatalf("children = %d, want 3 (flattened)", len(f.Children))
	}
}

func TestDigestDeterministic(t *testing.T) {
	a := And(Eq("x", 1), Or(Gt("y", 2), Between("z", 1, 10)))
	b := And(Eq("x", 1), Or(Gt("y", 2), Between("z", 1, 10)))
	if a.Digest() != b.Digest() {
		t.Fatal("digest not deterministic")
	}
	// 不同值不同摘要。
	c := And(Eq("x", 2), Or(Gt("y", 2), Between("z", 1, 10)))
	if a.Digest() == c.Digest() {
		t.Fatal("different filters share digest")
	}
}

func TestOrderBy(t *testing.T) {
	orders := OrderBy("created", Desc("name"), "id")
	if len(orders) != 3 || orders[0].Order != Ascending || orders[1].Order != Descending || orders[2].Attribute != "id" {
		t.Fatalf("orders = %+v", orders)
	}
}
