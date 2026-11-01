// Package outbox 是事务性发件箱的 PG/bun 实现（root outbox 形状的可插
// dialect 落地）：Append 经 buntx.Conn 参与调用方事务（四件一拍），
// ClaimBatch 以 FOR UPDATE SKIP LOCKED 并发安全拉取，MarkPublished/
// MarkFailed 走 at-least-once 语义。Pump 用 periodic 驱动轮询循环。
//
// DDL 见 migrations/00001_outbox.sql（应用经 goosekit 挂链或手工执行）。
package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lynx-go/lynx-start/contrib/bunx/buntx"
	"github.com/lynx-go/lynx-start/outbox"
	"github.com/uptrace/bun"
)

// Store 实现 outbox.Store（PG）。
type Store struct {
	db *bun.DB
}

// NewStore 构造。
func NewStore(db *bun.DB) *Store { return &Store{db: db} }

// outboxRow 是 PG 表的 PO。
type outboxRow struct {
	bun.BaseModel `bun:"table:outbox_events,alias:o"`

	ID          string       `bun:"id,pk"`
	Name        string       `bun:"name"`
	Aggregate   string       `bun:"aggregate,nullzero"`
	Payload     []byte       `bun:"payload"`
	CreatedAt   time.Time    `bun:"created_at"`
	PublishedAt sql.NullTime `bun:"published_at,nullzero"` // 非零 = 已投递
	Attempts    int          `bun:"attempts,notnull,default:0"`
	Dead        bool         `bun:"dead,notnull,default:false"` // 超限死信
	ClaimedAt   sql.NullTime `bun:"claimed_at,nullzero"`        // 泵在途标记
}

// Append 在调用方事务内写入一条（buntx.Conn：ctx 携带事务则加入）。
func (s *Store) Append(ctx context.Context, r outbox.Record) error {
	if r.ID == "" || r.Name == "" || len(r.Payload) == 0 {
		return fmt.Errorf("outbox: record must have ID/Name/Payload")
	}
	_, err := buntx.Conn(ctx, s.db).NewInsert().
		Model(&outboxRow{
			ID: r.ID, Name: r.Name, Aggregate: r.Aggregate,
			Payload: r.Payload, CreatedAt: r.CreatedAt,
		}).Exec(ctx)
	if err != nil {
		return fmt.Errorf("outbox: append: %w", err)
	}
	return nil
}

// ClaimBatch 以 FOR UPDATE SKIP LOCKED 拉取一批待发（多泵并发安全）。
// 同时递增 attempts 并标记 claimed_at；超限（attempts >= maxAttempts）
// 的记录标记 dead（不再返回）。返回空切片 = 无待发。
func (s *Store) ClaimBatch(ctx context.Context, batchSize, maxAttempts int, now time.Time) ([]outbox.Record, error) {
	// 死信标记先行（超限未死的记录）。
	_, err := s.db.NewUpdate().
		Model((*outboxRow)(nil)).
		Set("dead = true").
		Set("claimed_at = NULL", now).
		Where("published_at IS NULL AND dead = false AND attempts >= ?", maxAttempts).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("outbox: mark dead: %w", err)
	}

	// FOR UPDATE SKIP LOCKED 拉取（在途/已发/死信均跳过）。
	var rows []outboxRow
	err = s.db.NewSelect().
		Model(&rows).
		Where("published_at IS NULL AND dead = false AND claimed_at IS NULL").
		Order("created_at ASC").
		Limit(int64(batchSize)).
		For("UPDATE SKIP LOCKED").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("outbox: claim: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	// 标记在途 + 递增计数。
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	_, err = s.db.NewUpdate().
		Model((*outboxRow)(nil)).
		Set("attempts = attempts + 1").
		Set("claimed_at = ?", now).
		Where("id IN (?)", bun.List(ids)).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("outbox: mark claimed: %w", err)
	}

	records := make([]outbox.Record, len(rows))
	for i, r := range rows {
		records[i] = outbox.Record{ID: r.ID, Name: r.Name, Aggregate: r.Aggregate, Payload: r.Payload, CreatedAt: r.CreatedAt}
	}
	return records, nil
}

// MarkPublished 标记一批为已投递。
func (s *Store) MarkPublished(ctx context.Context, ids []string, now time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.db.NewUpdate().
		Model((*outboxRow)(nil)).
		Set("published_at = ?", now).
		Set("claimed_at = NULL", now).
		Where("id IN (?)", bun.List(ids)).
		Exec(ctx)
	return err
}

// MarkFailed 释放回待发（claimed_at 清空；下轮重投）。
func (s *Store) MarkFailed(ctx context.Context, ids []string, now time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.db.NewUpdate().
		Model((*outboxRow)(nil)).
		Set("claimed_at = NULL", now).
		Where("id IN (?)", bun.List(ids)).
		Exec(ctx)
	return err
}
