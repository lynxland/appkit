package bunx

import (
	"database/sql"
	"errors"

	"github.com/lynxland/appkit/storeerr"
	"github.com/uptrace/bun/driver/pgdriver"
)

// MapError 把 pgdriver 存储错误映射为 storeerr 领域哨兵（保留原错误链，
// 多 %w——errors.Is 同时命中哨兵与驱动原因）；SQL ErrNoRows 映射
// ErrNotFound；未知错误原样返回。仓储层写路径统一过此函数，用例层只
// 面对 storeerr 哨兵 → 领域错误的最后一跳。
//
// 注意 pgdriver.Error 是值类型（无 SQLState 方法，Field('C') 取
// SQLSTATE）——历史上指针形态的 errors.As 曾静默漏判，此坑由本函数收口。
func MapError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return storeerr.Wrap(storeerr.ErrNotFound, err, "")
	}
	var pg pgdriver.Error
	if errors.As(err, &pg) { // 值类型：As 目标必须是 pgdriver.Error 而非指针
		switch pg.Field('C') {
		case "23505":
			return storeerr.Wrap(storeerr.ErrAlreadyExists, err, pg.Field('t')) // t=表名
		case "23503":
			return storeerr.Wrap(storeerr.ErrForeignKey, err, pg.Field('t'))
		case "40001":
			return storeerr.Wrap(storeerr.ErrConflict, err, pg.Field('t'))
		case "40P01", "53300", "55P03":
			return storeerr.Wrap(storeerr.ErrBusy, err, pg.Field('t'))
		}
	}
	return err
}

// Classify 是 storeerr.Classifier 的 pgdriver 实现（SQLSTATE 白名单，
// 来源 torchwood testutil 的瞬时错误重试经验）：连接类/资源类/序列化
// 失败可重试；约束类永久；未识别交还调用方。
func Classify(err error) (storeerr.Class, bool) {
	var pg pgdriver.Error
	if !errors.As(err, &pg) {
		return storeerr.ClassUnknown, false
	}
	switch pg.Field('C') {
	case "40001", "40P01", "53300", "55P03", "57P03",
		"08000", "08001", "08003", "08004", "08006":
		return storeerr.ClassTransient, true
	case "23505", "23503", "23502", "23000", "22P02":
		return storeerr.ClassPermanent, true
	}
	return storeerr.ClassUnknown, false
}
