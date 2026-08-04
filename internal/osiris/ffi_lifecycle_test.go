package osiris

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFFIHandleBorrowPreventsConcurrentRelease(t *testing.T) {
	handle, err := NewFFIHandle("handle:fixture", "native-value")
	if err != nil {
		t.Fatal(err)
	}
	value, err := handle.acquire()
	if err != nil || value != "native-value" {
		t.Fatalf("acquire = %#v, %v", value, err)
	}

	closeResult := make(chan error, 1)
	go func() {
		closeResult <- handle.Close(func(any) error { t.Error("release ran while handle was borrowed"); return nil })
	}()
	select {
	case err := <-closeResult:
		if err == nil || !strings.Contains(err.Error(), "in use") {
			t.Fatalf("close while borrowed = %v, want in-use error", err)
		}
	case <-time.After(time.Second):
		t.Fatal("close did not return while handle was borrowed")
	}

	handle.releaseBorrow()
	var releases int
	if err := handle.Close(func(any) error { releases++; return nil }); err != nil {
		t.Fatal(err)
	}
	if releases != 1 {
		t.Fatalf("releases = %d, want 1", releases)
	}
	if _, err := handle.acquire(); err == nil {
		t.Fatal("closed handle was borrowable")
	}
}

func TestFFIHandleConcurrentCloseWaitsAndReportsOneReleaseError(t *testing.T) {
	handle, err := NewFFIHandle("handle:fixture", struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	releaseStarted := make(chan struct{})
	releaseGate := make(chan struct{})
	wantErr := errors.New("release failed")
	var mu sync.Mutex
	releases := 0
	release := func(any) error {
		mu.Lock()
		releases++
		mu.Unlock()
		close(releaseStarted)
		<-releaseGate
		return wantErr
	}

	first := make(chan error, 1)
	go func() { first <- handle.Close(release) }()
	<-releaseStarted
	second := make(chan error, 1)
	go func() { second <- handle.Close(func(any) error { t.Error("second close released twice"); return nil }) }()
	select {
	case err := <-second:
		t.Fatalf("concurrent close returned before release completed: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseGate)
	if err := <-first; !errors.Is(err, wantErr) {
		t.Fatalf("first close = %v, want %v", err, wantErr)
	}
	if err := <-second; !errors.Is(err, wantErr) {
		t.Fatalf("second close = %v, want %v", err, wantErr)
	}
	mu.Lock()
	gotReleases := releases
	mu.Unlock()
	if gotReleases != 1 {
		t.Fatalf("release calls = %d, want 1", gotReleases)
	}
}

func TestFFILibraryHandleReleaseCannotBeOverridden(t *testing.T) {
	handle, err := NewFFIHandle("handle:fixture", struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	var owned, override int
	handle.release = func(any) error { owned++; return nil }
	if err := handle.Close(func(any) error { override++; return nil }); err != nil {
		t.Fatal(err)
	}
	if owned != 1 || override != 0 {
		t.Fatalf("owned releases = %d, override releases = %d", owned, override)
	}
}

func TestFFICallContextRejectsCancellationBeforeNativeBorrow(t *testing.T) {
	handle, err := NewFFIHandle("handle:fixture", "native-value")
	if err != nil {
		t.Fatal(err)
	}
	library := &FFILibrary{
		manifest: FFIManifest{Functions: []FFIFunction{{
			Name: "use", Symbol: "fixture_use", Args: []string{"handle:fixture"},
			Result: "handle:fixture", Ownership: "owned", CallingConvention: "c", ThreadAffinity: "any",
		}}},
		native: ffiNativeLibrary{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := library.CallContext(ctx, "use", []Value{ffiHandleValueOf(handle)}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled call = %v, want context canceled", err)
	}
	if _, err := handle.acquire(); err != nil {
		t.Fatalf("canceled call borrowed or closed handle: %v", err)
	}
	handle.releaseBorrow()
}
