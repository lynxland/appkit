// Package storeerr 是存储错误归一（ADR-0009 D12 数据访问电池族之一，
// 证据 3×：appkit 模块 pgdriver 23505 值类型坑、torchwood
// MapDocumentDBError、fleetly IsUniqueViolation/IsForeignKeyViolation 族）：
// 核心只定义领域哨兵与可重试分类——驱动映射归适配器（contrib/bunx 的
// pgdriver/SQLSTATE 实现；非 bun 应用自带，fleetly 的 SQLite 映射同理），
// 消灭每项目各自 errors.As 驱动类型的样板与陷阱。
//
// 用法（仓储层）：
//
//	if storeerr.IsAlreadyExists(bunx.MapError(err)) {
//	    return domain.ErrEmailTaken // 哨兵 → 领域语义的最后一跳归模块
//	}
//
// 用法（可重试判断，显式注入驱动分类器——无全局注册表）：
//
//	class := storeerr.Classify(err, bunx.Classify)
package storeerr

import (
	"errors"
	"fmt"
)

// 领域哨兵：仓储层把驱动错误映射到此（保留原错误链），用例层只面对
// 语义；传输层投影（gRPC status/HTTP 码）见 roadmap ⑨ 的 errstatus。
var (
	// ErrAlreadyExists 唯一约束冲突（PG 23505；SQLite UNIQUE）。
	ErrAlreadyExists = errors.New("already exists")
	// ErrNotFound 行不存在（SQL ErrNoRows 由适配器一并映射）。
	ErrNotFound = errors.New("not found")
	// ErrConflict 并发冲突失配（PG 40001 serialization failure；
	// 乐观并发 version 不匹配——occ 电池复用本哨兵）。
	ErrConflict = errors.New("conflict")
	// ErrForeignKey 外键约束冲突（PG 23503；引用对象不存在/被引用）。
	ErrForeignKey = errors.New("foreign key violation")
	// ErrBusy 资源忙（PG 40P01 deadlock / 53300 too many connections）。
	ErrBusy = errors.New("busy")
)

// Wrap 以哨兵包裹原错误（多 %w：errors.Is 同时命中哨兵与驱动原因）。
// detail 可空——附加上下文（如表名/约束名）。
func Wrap(sentinel, cause error, detail string) error {
	if detail == "" {
		return fmt.Errorf("%w: %w", sentinel, cause)
	}
	return fmt.Errorf("%w: %s: %w", sentinel, detail, cause)
}

// 谓词族：对「已映射」或原生即哨兵的错误判定。
func IsAlreadyExists(err error) bool { return errors.Is(err, ErrAlreadyExists) }
func IsNotFound(err error) bool      { return errors.Is(err, ErrNotFound) }
func IsConflict(err error) bool      { return errors.Is(err, ErrConflict) }
func IsForeignKey(err error) bool    { return errors.Is(err, ErrForeignKey) }
func IsBusy(err error) bool          { return errors.Is(err, ErrBusy) }

// Class 是错误的可重试性分类。
type Class uint8

const (
	// ClassUnknown 未识别——保守起见不重试，调用方自行判断。
	ClassUnknown Class = iota
	// ClassPermanent 语义性失败：重试必然同样结果（唯一冲突/外键/未找到）。
	ClassPermanent
	// ClassTransient 瞬时失败：退避后重试可能成功（连接中断/死锁/
	// 序列化失败/资源忙）。
	ClassTransient
)

func (c Class) String() string {
	switch c {
	case ClassPermanent:
		return "permanent"
	case ClassTransient:
		return "transient"
	default:
		return "unknown"
	}
}

// Classifier 是驱动侧分类器（显式注入，无全局注册表）：识别归自己管的
// 错误返回 (Class, true)，否则 (ClassUnknown, false)。contrib/bunx 提供
// pgdriver/SQLSTATE 实现；非 bun 应用自带。
type Classifier func(err error) (Class, bool)

// Classify 分类错误：先按哨兵语义（永久/瞬时固定映射），再依次询问
// 驱动分类器——第一个给出确定结论者胜。包裹链（Wrap 的多 %w）自然穿透。
func Classify(err error, classifiers ...Classifier) Class {
	if err == nil {
		return ClassUnknown
	}
	if IsAlreadyExists(err) || IsNotFound(err) || IsConflict(err) || IsForeignKey(err) {
		return ClassPermanent
	}
	if IsBusy(err) {
		return ClassTransient
	}
	for _, classify := range classifiers {
		if class, ok := classify(err); ok {
			return class
		}
	}
	return ClassUnknown
}
