package queue

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemQueueBasic(t *testing.T) {
	q := NewMemQueue()
	ctx := context.Background()

	id, err := q.Enqueue(ctx, "jobs", []byte("task1"))
	if err != nil {
		t.Fatal(err)
	}
	msg, err := q.Dequeue(ctx, "jobs", time.Minute)
	if err != nil || msg.ID != id || string(msg.Payload) != "task1" {
		t.Fatalf("dequeue: %+v %v", msg, err)
	}
	// 在途：同 topic 不可再拉取。
	if _, err := q.Dequeue(ctx, "jobs", time.Minute); !errors.Is(err, ErrEmpty) {
		t.Fatalf("in-flight dequeue: %v", err)
	}
	if err := q.Ack(ctx, id); err != nil {
		t.Fatal(err)
	}
	if q.Len() != 0 {
		t.Fatal("after ack")
	}
}

func TestMemQueueVisibilityTimeout(t *testing.T) {
	q := NewMemQueue()
	ctx := context.Background()
	id, _ := q.Enqueue(ctx, "jobs", []byte("x"))
	msg, _ := q.Dequeue(ctx, "jobs", 50*time.Millisecond)
	_ = msg
	time.Sleep(60 * time.Millisecond)
	// 超时回退：可再次拉取（attempts +1）。
	retry, err := q.Dequeue(ctx, "jobs", time.Minute)
	if err != nil || retry.ID != id || retry.Attempts != 1 {
		t.Fatalf("redelivery: %+v %v", retry, err)
	}
}

func TestMemQueueFail(t *testing.T) {
	q := NewMemQueue()
	ctx := context.Background()
	id, _ := q.Enqueue(ctx, "jobs", []byte("x"))
	msg, _ := q.Dequeue(ctx, "jobs", time.Minute)
	if err := q.Fail(ctx, msg.ID); err != nil {
		t.Fatal(err)
	}
	retry, err := q.Dequeue(ctx, "jobs", time.Minute)
	if err != nil || retry.ID != id || retry.Attempts != 1 {
		t.Fatalf("fail→requeue: %+v %v", retry, err)
	}
}

func TestMemQueueTopicIsolation(t *testing.T) {
	q := NewMemQueue()
	ctx := context.Background()
	_, _ = q.Enqueue(ctx, "a", []byte("1"))
	_, _ = q.Enqueue(ctx, "b", []byte("2"))
	msg, _ := q.Dequeue(ctx, "b", time.Minute)
	if string(msg.Payload) != "2" {
		t.Fatalf("topic isolation: %+v", msg)
	}
	if _, err := q.Dequeue(ctx, "a", time.Minute); err != nil {
		t.Fatalf("topic a: %v", err)
	}
}
