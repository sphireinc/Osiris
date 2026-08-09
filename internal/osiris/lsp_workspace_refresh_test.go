package osiris

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLSPWorkspaceRefreshCancellationPreservesPriorIndex(t *testing.T) {
	root := t.TempDir()
	stablePath := filepath.Join(root, "stable.osr")
	stableURI := pathToURI(stablePath)
	if err := os.WriteFile(stablePath, []byte("func stable() string => \"old\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := NewLanguageServer(NewRuntime(), nil, nil)
	server.configureWorkspace(initializeParams{RootPath: root})
	server.refreshWorkspace(context.Background())
	prior := server.workspace[stableURI]
	if prior == nil || prior.analysis == nil {
		t.Fatalf("prior workspace state = %#v, want analyzed stable document", prior)
	}

	for index := 0; index < 32; index++ {
		path := filepath.Join(root, "candidate-"+string(rune('a'+index/26))+string(rune('a'+index%26))+".osr")
		if err := os.WriteFile(path, []byte("func candidate() string => \"new\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cancelled := newCancellationAfterChecksContext(5)
	server.refreshWorkspace(cancelled)

	if server.workspace[stableURI] != prior {
		t.Fatalf("canceled refresh replaced prior stable state: got %#v, want %#v", server.workspace[stableURI], prior)
	}
	if len(server.workspace) != 1 {
		t.Fatalf("canceled refresh workspace size = %d, want prior size 1", len(server.workspace))
	}
}

func TestLSPWorkspaceRefreshConcurrentOverlayDoesNotPublishStaleIndex(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.osr")
	uri := pathToURI(path)
	if err := os.WriteFile(path, []byte("func disk_value() string => \"disk\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := NewLanguageServer(NewRuntime(), nil, nil)
	server.configureWorkspace(initializeParams{RootPath: root})
	server.refreshWorkspace(context.Background())

	gate := newRefreshGateContext()
	done := make(chan struct{})
	go func() {
		server.refreshWorkspace(gate)
		close(done)
	}()
	select {
	case <-gate.reached:
	case <-time.After(5 * time.Second):
		t.Fatal("workspace refresh did not reach its traversal gate")
	}

	if err := server.upsertDocumentContext(context.Background(), uri, "func overlay_value() string => \"overlay\"\n", 4); err != nil {
		t.Fatal(err)
	}
	close(gate.release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("workspace refresh did not finish after concurrent overlay")
	}

	doc := server.documentForAnalysis(uri)
	if doc == nil || doc.text != "func overlay_value() string => \"overlay\"\n" || doc.version != 4 {
		t.Fatalf("concurrent overlay state = %#v, want versioned overlay", doc)
	}
	if workspaceDoc := server.workspace[uri]; workspaceDoc == nil || workspaceDoc.text != "func disk_value() string => \"disk\"\n" {
		t.Fatalf("workspace index after concurrent refresh = %#v, want disk snapshot", workspaceDoc)
	}
}

type cancellationAfterChecksContext struct {
	parent      context.Context
	checks      atomic.Int32
	cancelAfter int32
	done        chan struct{}
	once        sync.Once
}

func newCancellationAfterChecksContext(cancelAfter int32) *cancellationAfterChecksContext {
	return &cancellationAfterChecksContext{parent: context.Background(), cancelAfter: cancelAfter, done: make(chan struct{})}
}

func (c *cancellationAfterChecksContext) Deadline() (time.Time, bool) {
	return c.parent.Deadline()
}

func (c *cancellationAfterChecksContext) Done() <-chan struct{} { return c.done }

func (c *cancellationAfterChecksContext) Err() error {
	if err := c.parent.Err(); err != nil {
		return err
	}
	if c.checks.Add(1) >= c.cancelAfter {
		c.once.Do(func() { close(c.done) })
		return context.Canceled
	}
	return nil
}

func (c *cancellationAfterChecksContext) Value(key any) any { return c.parent.Value(key) }

type refreshGateContext struct {
	parent  context.Context
	checks  atomic.Int32
	reached chan struct{}
	release chan struct{}
	once    sync.Once
}

func newRefreshGateContext() *refreshGateContext {
	return &refreshGateContext{parent: context.Background(), reached: make(chan struct{}), release: make(chan struct{})}
}

func (c *refreshGateContext) Deadline() (time.Time, bool) {
	return c.parent.Deadline()
}

func (c *refreshGateContext) Done() <-chan struct{} { return nil }

func (c *refreshGateContext) Err() error {
	if c.checks.Add(1) == 2 {
		c.once.Do(func() { close(c.reached) })
		<-c.release
	}
	return c.parent.Err()
}

func (c *refreshGateContext) Value(key any) any { return c.parent.Value(key) }
