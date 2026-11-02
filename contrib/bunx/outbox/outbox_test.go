package outbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/lynxland/appkit/contrib/bunx/buntx"
	outboximpl "github.com/lynxland/appkit/contrib/bunx/outbox"
	"github.com/lynxland/appkit/contrib/bunx/outbox/migrations"
	"github.com/lynxland/appkit/contrib/bunx/testutil"
	"github.com/lynxland/appkit/events"
	outboxshape "github.com/lynxland/appkit/outbox"
	"github.com/uptrace/bun"
)

func newStore(t *testing.T) (outboxshape.Store, *outboximpl.Store, *bun.DB) {
	t.Helper()
	db := testutil.NewDB(t, testutil.Chain{Module: "outbox", FS: migrations.FS()})
	return outboximpl.NewStore(db), outboximpl.NewStore(db), db
}

// captureSink 捕获投递记录。
type captureSink struct {
	delivered [][]outboxshape.Record
	fail      bool
}

func (s *captureSink) Deliver(_ context.Context, records []outboxshape.Record) error {
	if s.fail {
		return context.DeadlineExceeded
	}
	s.delivered = append(s.delivered, records)
	return nil
}

func TestOutboxAppendInTransaction(t *testing.T) {
	store, impl, db := newStore(t)
	ctx := context.Background()

	// 在事务内追加（四件一拍——业务写与事件同事务）。
	err := buntx.New(db).Run(ctx, func(ctx context.Context) error {
		return store.Append(ctx, outboxshape.Record{
			ID: "evt-1", Name: "demo.thing.created", Aggregate: "demo/x",
			Payload: []byte(`{"id":"evt-1"}`), CreatedAt: time.Now().UTC(),
		})
	})
	if err != nil {
		t.Fatalf("append in tx: %v", err)
	}
	// 事务已提交：泵可直接拉取。
	batch, err := impl.ClaimBatch(ctx, 10, 10, time.Now().UTC())
	if err != nil || len(batch) != 1 || batch[0].ID != "evt-1" {
		t.Fatalf("after tx commit: batch=%+v err=%v", batch, err)
	}
}

func TestOutboxClaimDeliverMark(t *testing.T) {
	store, _, _ := newStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// 追加三条。
	for i, name := range []string{"a.x.created", "b.y.updated", "c.z.deleted"} {
		if err := store.Append(ctx, outboxshape.Record{
			ID: string(rune('A' + i)), Name: name,
			Payload: []byte(`{}`), CreatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// 拉取（一批 2 条）。
	batch, err := store.ClaimBatch(ctx, 2, 10, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 2 || batch[0].ID != "A" {
		t.Fatalf("batch = %+v", batch)
	}
	// 二批（SKIP LOCKED：不重复）。
	batch2, err := store.ClaimBatch(ctx, 10, 10, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch2) != 1 || batch2[0].ID != "C" {
		t.Fatalf("batch2 = %+v", batch2)
	}

	// 标记 A/B 已发。
	if err := store.MarkPublished(ctx, []string{"A", "B"}, now); err != nil {
		t.Fatal(err)
	}
	// C 失败释放→再拉取。
	if err := store.MarkFailed(ctx, []string{"C"}, now); err != nil {
		t.Fatal(err)
	}
	retry, err := store.ClaimBatch(ctx, 10, 10, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(retry) != 1 || retry[0].ID != "C" {
		t.Fatalf("retry = %+v", retry)
	}
}

func TestOutboxDeadLetter(t *testing.T) {
	store, _, _ := newStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := store.Append(ctx, outboxshape.Record{ID: "D", Name: "x.y.z", Payload: []byte(`{}`), CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	// attempts 达上限（maxAttempts=2）→ 死信（ClaimBatch 首先标记 dead）。
	for i := 0; i < 2; i++ {
		if _, err := store.ClaimBatch(ctx, 10, 2, now); err != nil {
			t.Fatal(err)
		}
		if err := store.MarkFailed(ctx, []string{"D"}, now); err != nil {
			t.Fatal(err)
		}
	}
	batch, err := store.ClaimBatch(ctx, 10, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 0 {
		t.Fatalf("dead record still claimable: %+v", batch)
	}
}

// TestPumpOnce 泵的一轮（真实 DB + captureSink）。
func TestPumpOnce(t *testing.T) {
	store, _, _ := newStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		if err := store.Append(ctx, outboxshape.Record{
			ID: string(rune('E' + i)), Name: "demo.thing.created",
			Payload: []byte(`{}`), CreatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	sink := &captureSink{}
	if err := outboximpl.PumpOnce(ctx, store, sink, 10, 10); err != nil {
		t.Fatal(err)
	}
	if len(sink.delivered) != 1 || len(sink.delivered[0]) != 3 {
		t.Fatalf("delivered = %+v", sink.delivered)
	}
	// 再拉取：无待发。
	batch, _ := store.ClaimBatch(ctx, 10, 10, now)
	if len(batch) != 0 {
		t.Fatalf("after pump: %+v", batch)
	}
	_ = events.Envelope{} // events 依赖保留（信封格式测试在 root 层）
}
