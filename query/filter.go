// Package query 是动态查询的 typed AST 组合器内核（ADR-2024 D13 第二
// 梯队，torchwood pkg/query 裁剪版——纯 Filter/Order 构造器，DSL 字符串
// 解析与 pgvector/SQL 编译留应用侧/适配层）。严格中立：零外部依赖。
//
// 应用在自己的 repo 层把 Filter 树编译成 SQL/内存过滤——本包只定义
// 树的形态与构造器，保证「条件可组合、可序列化、可校验」。
//
// 裁剪边界（相对 torchwood pkg/query）：保留 12 个核心算子+AND/OR
// 组合器；去掉 DSL 解析器（equal("a","b") 字符串语法）、向量检索
// （VectorSearch/pgvector）、系统列（$createdAt）——这些是 torchwood
// 业务特化，不是通用件。
package query

import (
	"fmt"
	"sort"
	"strings"
)

// Op 是过滤算子。
type Op uint8

const (
	OpEqual Op = iota + 1
	OpNotEqual
	OpGreaterThan
	OpGreaterOrEqual
	OpLessThan
	OpLessOrEqual
	OpIn
	OpNotIn
	OpBetween  // Values 恰好两个 [lo, hi]
	OpContains // 字符串/数组包含
	OpStartsWith
	OpEndsWith
	OpIsNull // Values 空
	OpNotNull
)

func (o Op) String() string {
	switch o {
	case OpEqual:
		return "="
	case OpNotEqual:
		return "!="
	case OpGreaterThan:
		return ">"
	case OpGreaterOrEqual:
		return ">="
	case OpLessThan:
		return "<"
	case OpLessOrEqual:
		return "<="
	case OpIn:
		return "in"
	case OpNotIn:
		return "not-in"
	case OpBetween:
		return "between"
	case OpContains:
		return "contains"
	case OpStartsWith:
		return "starts-with"
	case OpEndsWith:
		return "ends-with"
	case OpIsNull:
		return "is-null"
	case OpNotNull:
		return "not-null"
	default:
		return "op(?)"
	}
}

// Filter 是过滤树节点：叶子（Attribute+Op+Values）或组合（Children
// 非空时 Op 必须是 And/Or）。
type Filter struct {
	Attribute string
	Op        Op
	Values    []any
	Children  []Filter // And/Or 组合时使用
}

// And 组合多个条件（全满足）。
func And(children ...Filter) Filter {
	return Filter{Op: OpLogicalAnd, Children: flatten(OpLogicalAnd, children)}
}

// Or 组合多个条件（任一满足）。
func Or(children ...Filter) Filter {
	return Filter{Op: OpLogicalOr, Children: flatten(OpLogicalOr, children)}
}

// 逻辑算子（仅用于组合节点，不参与叶子 Op 枚举——刻意用独立常量域
// 避免误用为叶子）。
const (
	OpLogicalAnd Op = 200
	OpLogicalOr  Op = 201
)

// flatten 同层同类节点展平（And(And(a,b),c) → And(a,b,c)）。
func flatten(op Op, children []Filter) []Filter {
	var out []Filter
	for _, c := range children {
		if c.Op == op && len(c.Children) > 0 {
			out = append(out, c.Children...)
		} else {
			out = append(out, c)
		}
	}
	return out
}

// -- 叶子构造器（链式调用面） --

func Eq(attr string, v any) Filter { return Filter{Attribute: attr, Op: OpEqual, Values: []any{v}} }
func Ne(attr string, v any) Filter { return Filter{Attribute: attr, Op: OpNotEqual, Values: []any{v}} }
func Gt(attr string, v any) Filter {
	return Filter{Attribute: attr, Op: OpGreaterThan, Values: []any{v}}
}
func Ge(attr string, v any) Filter {
	return Filter{Attribute: attr, Op: OpGreaterOrEqual, Values: []any{v}}
}
func Lt(attr string, v any) Filter { return Filter{Attribute: attr, Op: OpLessThan, Values: []any{v}} }
func Le(attr string, v any) Filter {
	return Filter{Attribute: attr, Op: OpLessOrEqual, Values: []any{v}}
}
func In(attr string, vs ...any) Filter    { return Filter{Attribute: attr, Op: OpIn, Values: vs} }
func NotIn(attr string, vs ...any) Filter { return Filter{Attribute: attr, Op: OpNotIn, Values: vs} }
func Between(attr string, lo, hi any) Filter {
	return Filter{Attribute: attr, Op: OpBetween, Values: []any{lo, hi}}
}
func Contains(attr, substr string) Filter {
	return Filter{Attribute: attr, Op: OpContains, Values: []any{substr}}
}
func StartsWith(attr, prefix string) Filter {
	return Filter{Attribute: attr, Op: OpStartsWith, Values: []any{prefix}}
}
func EndsWith(attr, suffix string) Filter {
	return Filter{Attribute: attr, Op: OpEndsWith, Values: []any{suffix}}
}
func IsNull(attr string) Filter  { return Filter{Attribute: attr, Op: OpIsNull} }
func NotNull(attr string) Filter { return Filter{Attribute: attr, Op: OpNotNull} }

// Order 是排序方向。
type Order uint8

const (
	Ascending Order = iota
	Descending
)

// OrderClause 是一个排序列。
type OrderClause struct {
	Attribute string
	Order     Order
}

// OrderBy 构造排序列集（DDL：OrderBy("created", Descending, "name")）。
func OrderBy(attrs ...any) []OrderClause {
	var out []OrderClause
	for _, a := range attrs {
		switch v := a.(type) {
		case string:
			out = append(out, OrderClause{Attribute: v, Order: Ascending})
		case OrderClause:
			out = append(out, v)
		}
	}
	return out
}

// Desc 标记降序（OrderBy("a", Desc("b"))）。
func Desc(attr string) OrderClause { return OrderClause{Attribute: attr, Order: Descending} }

// -- 校验与摘要 --

// Validate 校验树形合法性：组合节点 Attribute/Values 必空、叶子 Values
// 数量与算子匹配（Between=2/In>=1/IsNull=0 等）。
func (f Filter) Validate() error {
	return validate(f, 0)
}

func validate(f Filter, depth int) error {
	if depth > 16 {
		return &InvalidError{Msg: "filter tree too deep (>16)"}
	}
	if f.Op == OpLogicalAnd || f.Op == OpLogicalOr {
		if f.Attribute != "" || len(f.Values) > 0 {
			return &InvalidError{Msg: "logical node must not carry attribute/values"}
		}
		if len(f.Children) == 0 {
			return &InvalidError{Msg: "logical node needs children"}
		}
		for _, c := range f.Children {
			if err := validate(c, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	// 叶子节点。
	if f.Attribute == "" {
		return &InvalidError{Msg: "leaf node needs attribute"}
	}
	if len(f.Children) > 0 {
		return &InvalidError{Msg: "leaf node must not carry children"}
	}
	switch f.Op {
	case OpBetween:
		if len(f.Values) != 2 {
			return &InvalidError{Msg: "between needs exactly 2 values"}
		}
	case OpIn, OpNotIn:
		if len(f.Values) == 0 {
			return &InvalidError{Msg: "in needs >=1 values"}
		}
	case OpIsNull, OpNotNull:
		if len(f.Values) > 0 {
			return &InvalidError{Msg: "null checks carry no values"}
		}
	default:
		if len(f.Values) != 1 {
			return &InvalidError{Msg: "scalar op needs exactly 1 value"}
		}
	}
	return nil
}

// InvalidError 是校验失败。
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return "query: " + e.Msg }

// Digest 返回确定性摘要（树→规范串；crud v2 的 FilterDigest 适配形态）。
func (f Filter) Digest() string {
	var b strings.Builder
	writeDigest(&b, f)
	return b.String()
}

func writeDigest(b *strings.Builder, f Filter) {
	if f.Op == OpLogicalAnd || f.Op == OpLogicalOr {
		b.WriteByte('(')
		for i, c := range f.Children {
			if i > 0 {
				b.WriteByte(';')
			}
			writeDigest(b, c)
		}
		b.WriteByte(')')
		return
	}
	b.WriteString(f.Attribute)
	b.WriteByte(' ')
	b.WriteString(f.Op.String())
	b.WriteByte(' ')
	b.WriteByte('[')
	for i, v := range f.Values {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(sprint(v))
	}
	b.WriteByte(']')
}

func sprint(v any) string {
	switch x := v.(type) {
	case string:
		return `"` + x + `"`
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// SortOrderClauses 稳定排序辅助（属性名升序、方向降序优先）——供
// adapter 端生成确定性 SQL。
func SortOrderClauses(orders []OrderClause) {
	sort.SliceStable(orders, func(i, j int) bool {
		if orders[i].Attribute != orders[j].Attribute {
			return orders[i].Attribute < orders[j].Attribute
		}
		return orders[i].Order < orders[j].Order
	})
}
