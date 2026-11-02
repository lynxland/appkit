package testutil_test

import (
	"context"
	"testing"
	"time"

	"github.com/lynxland/appkit/contrib/bunx/testutil"
	"github.com/lynxland/appkit/events"
	"github.com/lynxland/appkit/idgen"
)

func TestCapturePublisher(t *testing.T) {
	p := testutil.NewCapturePublisher()
	env := events.Envelope{ID: idgen.New(), Name: "t.a.b", Module: "t", OccurredAt: time.Now()}
	if err := p.Publish(context.Background(), env); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if names := p.Names(); len(names) != 1 || names[0] != "t.a.b" {
		t.Fatalf("names = %v", names)
	}
	// 与 BusPublisher 同语义:非法事件名拒绝(不静默记录)。
	if err := p.Publish(context.Background(), events.Envelope{Name: "t.a_b", Module: "t"}); err == nil {
		t.Fatal("invalid event name should be rejected")
	}
	p.Fail = true
	if err := p.Publish(context.Background(), env); err == nil {
		t.Fatal("fail flag should error")
	}
	if len(p.Envelopes()) != 1 {
		t.Fatalf("failed publish must not record, got %d", len(p.Envelopes()))
	}
}

func TestInlineRunner(t *testing.T) {
	called := false
	if err := (testutil.InlineRunner{}).Run(context.Background(), func(context.Context) error {
		called = true
		return nil
	}); err != nil || !called {
		t.Fatalf("run: err=%v called=%v", err, called)
	}
}
