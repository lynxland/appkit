package events

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lynxland/lynx/eventbus"
	"github.com/lynxland/lynx/lynxtest"
)

func TestEnvelopeValidate(t *testing.T) {
	ev := Envelope{Name: "users.user.created"}
	if err := ev.Validate(); err != nil {
		t.Fatalf("valid name: %v", err)
	}
	if ev.Module != "users" {
		t.Errorf("Module = %q, want users", ev.Module)
	}
	for _, bad := range []Envelope{
		{Name: ""},
		{Name: "users"},
		{Name: "users.user"},
		{Name: "Users.User.Created"},
		{Name: "users.user.created", Module: "other"},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want error", bad)
		}
	}
}

func TestTopicFor(t *testing.T) {
	if got := TopicFor("users.user.created"); got != "module.users" {
		t.Errorf("TopicFor = %q, want module.users", got)
	}
}

func TestBusPublisherRejectsInvalidName(t *testing.T) {
	ctx := lynxtest.NewContext(t)
	p := NewBusPublisher(ctx.Bus())
	if err := p.Publish(context.Background(), Envelope{Name: "bad"}); err == nil {
		t.Fatal("invalid name: want error")
	}
}

func TestBusPublisherRoundtrip(t *testing.T) {
	ctx := lynxtest.NewContext(t)
	p := NewBusPublisher(ctx.Bus())

	got := make(chan Envelope, 1)
	err := ctx.Bus().Subscribe(context.Background(), TopicFor("demo.item.created"),
		func(_ context.Context, e *eventbus.RawEvent) error {
			var ev Envelope
			if err := json.Unmarshal(e.Payload, &ev); err != nil {
				t.Errorf("unmarshal envelope: %v", err)
			}
			got <- ev
			return nil
		})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	want := Envelope{
		ID:      "evt-1",
		Name:    "demo.item.created",
		Payload: json.RawMessage(`{"id":"i1"}`),
	}
	if err := p.Publish(context.Background(), want); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case ev := <-got:
		if ev.Name != want.Name || ev.ID != want.ID || string(ev.Payload) != string(want.Payload) {
			t.Errorf("roundtrip = %+v, want %+v", ev, want)
		}
		if ev.Module != "demo" {
			t.Errorf("Module = %q, want demo", ev.Module)
		}
		if ev.OccurredAt.IsZero() {
			t.Error("OccurredAt should default to now")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler not invoked within 2s")
	}
}
