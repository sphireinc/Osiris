package osiris

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PollInterest is a portable readiness bitset. It intentionally describes
// events rather than operating-system descriptors so platform adapters can sit
// behind this boundary without leaking epoll/kqueue/IOCP details upward.
type PollInterest uint8

const (
	PollReadable PollInterest = 1 << iota
	PollWritable
	PollError
	PollHangup
)

// PollEvent names one registered resource and the readiness bits reported for
// it. Readiness is advisory: callers must still perform the operation and
// handle an ordinary would-block/closed error.
type PollEvent struct {
	Token  uint64
	Ready  PollInterest
	Closed bool
}

// PollCapabilities describes the selected internal backend without making it
// a public language feature. The portable backend supports synthetic
// registrations and explicit notifications only; OS-handle registration is a
// future platform-adapter capability.
type PollCapabilities struct {
	Backend          string
	HandleReadiness  bool
	SyntheticSignals bool
}

type pollRegistration struct{ interest PollInterest }

// pollContext is a reusable, runtime-local polling boundary. It is safe for
// concurrent registration, removal, notification, and waiting. Close wakes all
// waiters and makes later operations return a structured ordinary Go error for
// normalization at the language boundary.
type pollContext struct {
	mu     sync.Mutex
	regs   map[uint64]pollRegistration
	queued []PollEvent
	wake   chan struct{}
	closed bool
	cap    PollCapabilities
}

func newPollContext() *pollContext {
	poll, _ := newPollContextWithBackend("portable")
	return poll
}

// newPollContextWithBackend keeps explicit adapter selection internal while
// making unavailable platform backends observable to tests and diagnostics.
// Only the portable backend exists in this increment.
func newPollContextWithBackend(backend string) (*pollContext, error) {
	if backend == "" {
		backend = "portable"
	}
	if backend != "portable" {
		return nil, fmt.Errorf("poll backend %q is unsupported", backend)
	}
	return &pollContext{
		regs: make(map[uint64]pollRegistration),
		wake: make(chan struct{}, 1),
		cap:  PollCapabilities{Backend: "portable", SyntheticSignals: true},
	}, nil
}

func (p *pollContext) capabilities() PollCapabilities {
	if p == nil {
		return PollCapabilities{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cap
}

func (p *pollContext) register(token uint64, interest PollInterest) error {
	if token == 0 || interest == 0 {
		return fmt.Errorf("poll registration requires a non-zero token and interest")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return fmt.Errorf("poll context is closed")
	}
	if _, exists := p.regs[token]; exists {
		return fmt.Errorf("poll token %d is already registered", token)
	}
	p.regs[token] = pollRegistration{interest: interest}
	return nil
}

func (p *pollContext) modify(token uint64, interest PollInterest) error {
	if interest == 0 {
		return fmt.Errorf("poll interest must be non-zero")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return fmt.Errorf("poll context is closed")
	}
	if _, exists := p.regs[token]; !exists {
		return fmt.Errorf("poll token %d is not registered", token)
	}
	p.regs[token] = pollRegistration{interest: interest}
	return nil
}

func (p *pollContext) remove(token uint64) bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	_, exists := p.regs[token]
	delete(p.regs, token)
	p.mu.Unlock()
	return exists
}

// notify is the portable backend's bridge for timer/task/resource adapters.
// It filters events against the current registration, preventing stale queued
// readiness after removal or interest modification.
func (p *pollContext) notify(token uint64, ready PollInterest) bool {
	if p == nil || ready == 0 {
		return false
	}
	p.mu.Lock()
	reg, exists := p.regs[token]
	if !p.closed && exists {
		ready &= reg.interest | PollError | PollHangup
		if ready != 0 {
			p.queued = append(p.queued, PollEvent{Token: token, Ready: ready})
		}
	}
	p.mu.Unlock()
	if ready == 0 || !exists {
		return false
	}
	p.signal()
	return true
}

func (p *pollContext) wait(ctx context.Context, timeout time.Duration, limit int) ([]PollEvent, error) {
	if p == nil {
		return nil, fmt.Errorf("poll context is nil")
	}
	if timeout < 0 || limit < 0 {
		return nil, fmt.Errorf("poll timeout and limit must be non-negative")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if events, closed := p.take(limit); len(events) > 0 || closed {
		return events, pollClosedError(closed)
	}
	var timer <-chan time.Time
	var stop func()
	if timeout > 0 {
		t := time.NewTimer(timeout)
		timer = t.C
		stop = func() {
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
		}
		defer stop()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer:
		return nil, nil
	case <-p.wake:
		events, closed := p.take(limit)
		return events, pollClosedError(closed)
	}
}

func (p *pollContext) take(limit int) ([]PollEvent, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.queued) == 0 {
		return nil, p.closed
	}
	n := len(p.queued)
	if limit > 0 && n > limit {
		n = limit
	}
	events := append([]PollEvent(nil), p.queued[:n]...)
	p.queued = append([]PollEvent(nil), p.queued[n:]...)
	return events, p.closed
}

func (p *pollContext) close() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return false
	}
	p.closed = true
	p.regs = nil
	p.mu.Unlock()
	p.signal()
	return true
}

func (p *pollContext) signal() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func pollClosedError(closed bool) error {
	if closed {
		return fmt.Errorf("poll context is closed")
	}
	return nil
}
