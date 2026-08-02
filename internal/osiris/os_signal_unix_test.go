//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package osiris

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestOSSignalSubscriptionReceivesSignal(t *testing.T) {
	rt := NewRuntime()
	rt.GrantCapability("os.signal")
	value, err := osSignalContextValue(rt, context.Background(), []Value{stringValue("SIGUSR1")})
	if err != nil {
		t.Skipf("SIGUSR1 is unavailable on this platform: %v", err)
	}
	stream, err := value.AsStream()
	if err != nil {
		t.Fatal(err)
	}
	defer stream.close()
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Signal(syscall.SIGUSR1); err != nil {
		t.Skipf("process signal delivery is unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	line, err := stream.readLineContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if line.String() != `"SIGUSR1"` {
		t.Fatalf("signal event = %s, want SIGUSR1", line.String())
	}
}
