package osiris

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPortablePollContextLifecycleAndReadiness(t *testing.T) {
	poll := newPollContext()
	if cap := poll.capabilities(); cap.Backend != "portable" || !cap.SyntheticSignals || cap.HandleReadiness {
		t.Fatalf("capabilities = %#v", cap)
	}
	if err := poll.register(1, PollReadable); err != nil {
		t.Fatal(err)
	}
	if !poll.notify(1, PollReadable|PollWritable) {
		t.Fatal("notify did not queue registered readable event")
	}
	events, err := poll.wait(context.Background(), time.Second, 1)
	if err != nil || len(events) != 1 || events[0].Token != 1 || events[0].Ready != PollReadable {
		t.Fatalf("wait = %#v, %v", events, err)
	}
	if err := poll.modify(1, PollWritable); err != nil {
		t.Fatal(err)
	}
	if poll.notify(1, PollReadable) {
		t.Fatal("stale interest notification succeeded")
	}
	if !poll.notify(1, PollWritable) {
		t.Fatal("modified writable notification failed")
	}
	if !poll.remove(1) || poll.notify(1, PollWritable) {
		t.Fatal("removed registration remained observable")
	}
	if events, err := poll.wait(context.Background(), 10*time.Millisecond, 0); err != nil || len(events) != 1 || events[0].Ready != PollWritable {
		t.Fatalf("queued event after removal = %#v, %v", events, err)
	}
	if events, err := poll.wait(context.Background(), 10*time.Millisecond, 0); err != nil || len(events) != 0 {
		t.Fatalf("timeout wait = %#v, %v", events, err)
	}
	if !poll.close() || poll.close() {
		t.Fatal("close must be idempotent")
	}
	if _, err := poll.wait(context.Background(), 0, 0); err == nil {
		t.Fatal("closed context wait succeeded")
	}
}

func TestPortablePollContextCancellationAndValidation(t *testing.T) {
	poll := newPollContext()
	if err := poll.register(0, PollReadable); err == nil {
		t.Fatal("zero token registration succeeded")
	}
	if err := poll.register(1, 0); err == nil {
		t.Fatal("zero interest registration succeeded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := poll.wait(ctx, 0, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait = %v", err)
	}
	if _, err := poll.wait(context.Background(), -time.Millisecond, 0); err == nil {
		t.Fatal("negative timeout succeeded")
	}
	if _, err := newPollContextWithBackend("epoll"); err == nil {
		t.Fatal("unimplemented platform backend succeeded")
	}
}
