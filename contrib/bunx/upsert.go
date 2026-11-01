package bunx

import (
	"github.com/uptrace/bun"
)

// Upsert 给 INSERT 追加 ON CONFLICT (keys) DO UPDATE SET col = EXCLUDED.col
// 形态的 upsert 语义。updateColumns 为空时等价 DO NOTHING（存在即跳过）。
//
// 约定：keys 是冲突检测列（唯一索引/主键），updateColumns 是覆写列
// 白名单——显式列名，不做裸全模型覆盖（torchwood UPDATE 零值列烧毁
// identity 列的事故教训：宁缺勿滥）。
func Upsert(q *bun.InsertQuery, conflictKeys, updateColumns []string) *bun.InsertQuery {
	if len(conflictKeys) == 0 {
		return q // 无冲突检测键：退化为普通 INSERT
	}
	if len(updateColumns) == 0 {
		return q.On("CONFLICT (" + quoteIdent(conflictKeys) + ") DO NOTHING")
	}
	sets := make([]string, len(updateColumns))
	for i, col := range updateColumns {
		sets[i] = col + " = EXCLUDED." + col
	}
	return q.On("CONFLICT (" + quoteIdent(conflictKeys) + ") DO UPDATE SET " + joinStrings(sets, ", "))
}

// quoteIdent 列名列表转 SQL 标识符元组（"a", "b"——显式引号防保留字）。
func quoteIdent(cols []string) string {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = `"` + c + `"`
	}
	return joinStrings(quoted, ", ")
}

func joinStrings(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}
