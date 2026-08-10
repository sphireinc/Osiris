package osiris

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLSPWorkspaceRenameTransactionRejectsChangedAffectedDocument(t *testing.T) {
	server, modulePath, mainPath := newLSPWorkspaceRenameTransactionServer(t)
	mainURI := pathToURI(mainPath)
	position := lspPosition{Line: 2, Character: utf16Units("let message = cat(\"😀\", greetings.") + 2}

	ctx := &renameMutationContext{parent: context.Background(), trigger: 2, mutate: func() {
		updated := "package greetings\nfunc Hello(name string) string => cat(\"Updated \", name)\nexport Hello\n"
		if err := server.upsertDocumentContext(context.Background(), pathToURI(modulePath), updated, 11); err != nil {
			t.Errorf("mutate module overlay: %v", err)
		}
	}}
	_, err := server.renameWorkspaceEditContext(ctx, mainURI, position, "Greet", intPointer(7))
	if err == nil {
		t.Fatal("rename accepted a mixed-generation workspace snapshot")
	}
	var coded interface{ LSPCode() int }
	if !errors.As(err, &coded) || coded.LSPCode() != -32602 {
		t.Fatalf("transactional rename error = %v, want -32602", err)
	}
	if got := server.document(pathToURI(modulePath)); got == nil || got.version != 11 || got.text == "" {
		t.Fatalf("mutated module overlay = %#v, want the newer version to remain installed", got)
	}
}

func TestLSPWorkspaceRenameTransactionConcurrentMutation(t *testing.T) {
	server, modulePath, mainPath := newLSPWorkspaceRenameTransactionServer(t)
	mainURI := pathToURI(mainPath)
	position := lspPosition{Line: 2, Character: utf16Units("let message = cat(\"😀\", greetings.") + 2}
	ctx := newRenameSnapshotGateContext()
	result := make(chan error, 1)
	go func() {
		_, err := server.renameWorkspaceEditContext(ctx, mainURI, position, "Greet", intPointer(7))
		result <- err
	}()
	select {
	case <-ctx.reached:
	case <-time.After(5 * time.Second):
		t.Fatal("rename did not reach its snapshot transaction boundary")
	}
	updated := "package greetings\nfunc Hello(name string) string => cat(\"Concurrent \", name)\nexport Hello\n"
	if err := server.upsertDocumentContext(context.Background(), pathToURI(modulePath), updated, 12); err != nil {
		t.Fatal(err)
	}
	close(ctx.release)
	if err := <-result; err == nil {
		t.Fatal("concurrent module replacement was accepted by rename")
	} else {
		var coded interface{ LSPCode() int }
		if !errors.As(err, &coded) || coded.LSPCode() != -32602 {
			t.Fatalf("concurrent transactional rename error = %v, want -32602", err)
		}
	}
}

func TestLSPWorkspaceRenameTransactionProtocolFixtureIsDeterministic(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "lsp", "lsp-workspace-rename-transaction.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	first := runLSPProtocolFixture(t, fixture)
	if first != runLSPProtocolFixture(t, fixture) {
		t.Fatal("rename transaction fixture is nondeterministic")
	}
	frames := decodeLSPProtocolFrames(t, first)
	var sawRename, sawStale bool
	for _, frame := range frames {
		id, ok := lspProtocolFrameID(frame)
		if !ok {
			continue
		}
		switch id {
		case 2:
			data, _ := json.Marshal(frame["result"])
			var edit lspWorkspaceEdit
			if err := json.Unmarshal(data, &edit); err != nil || len(edit.DocumentChanges) != 1 || edit.DocumentChanges[0].TextDocument.Version != 4 {
				t.Fatalf("transaction fixture rename result = %s, err=%v", data, err)
			}
			sawRename = true
		case 3:
			lspProtocolAssertErrorCode(t, frame, -32602)
			sawStale = true
		}
	}
	if !sawRename || !sawStale {
		t.Fatalf("rename transaction fixture frames = %#v", frames)
	}
}

func TestLSPWorkspaceRenameTransactionCancellationIsWireLevel(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "lsp", "lsp-workspace-rename-transaction.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var chunks [][]byte
	for index, line := range splitLSPFixtureLines(fixture) {
		if index > 2 {
			break
		}
		chunks = append(chunks, lspFrame(line))
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelAfterLSPRead{chunks: chunks, cancel: cancel, cancelAfter: 2}
	var output strings.Builder
	server := NewLanguageServer(NewRuntime(), &output, ioDiscardWriter{})
	if err := server.Serve(ctx, reader); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled rename Serve error = %v", err)
	}
	frames := decodeLSPProtocolFrames(t, output.String())
	for _, frame := range frames {
		if id, ok := lspProtocolFrameID(frame); ok && id == 2 {
			lspProtocolAssertErrorCode(t, frame, -32800)
			if _, hasResult := frame["result"]; hasResult {
				t.Fatalf("canceled rename returned a result: %#v", frame)
			}
			return
		}
	}
	t.Fatalf("canceled rename produced no wire response: %#v", frames)
}

func newLSPWorkspaceRenameTransactionServer(t *testing.T) (*languageServer, string, string) {
	t.Helper()
	root := t.TempDir()
	modulePath := filepath.Join(root, "greetings.osr")
	mainPath := filepath.Join(root, "main.osr")
	moduleSource := "package greetings\nfunc Hello(name string) string => cat(\"Hello \", name)\nexport Hello\n"
	mainSource := "package main\nimport \"./greetings.osr\" as greetings\nlet message = cat(\"😀\", greetings.Hello(\"Ada\"))\nemit message\n"
	if err := os.WriteFile(modulePath, []byte(moduleSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, []byte(mainSource), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewLanguageServer(NewRuntime(), nil, nil)
	server.configureWorkspace(initializeParams{RootPath: root})
	server.refreshWorkspace(context.Background())
	if err := server.upsertDocumentContext(context.Background(), pathToURI(mainPath), mainSource, 7); err != nil {
		t.Fatal(err)
	}
	return server, modulePath, mainPath
}

type renameMutationContext struct {
	parent  context.Context
	checks  atomic.Int32
	trigger int32
	mutate  func()
	once    sync.Once
}

func (c *renameMutationContext) Deadline() (time.Time, bool) { return c.parent.Deadline() }
func (c *renameMutationContext) Done() <-chan struct{}       { return c.parent.Done() }
func (c *renameMutationContext) Value(key any) any           { return c.parent.Value(key) }
func (c *renameMutationContext) Err() error {
	if err := c.parent.Err(); err != nil {
		return err
	}
	if c.checks.Add(1) == c.trigger {
		c.once.Do(c.mutate)
	}
	return nil
}

type renameSnapshotGateContext struct {
	parent  context.Context
	checks  atomic.Int32
	reached chan struct{}
	release chan struct{}
	once    sync.Once
}

func newRenameSnapshotGateContext() *renameSnapshotGateContext {
	return &renameSnapshotGateContext{parent: context.Background(), reached: make(chan struct{}), release: make(chan struct{})}
}

func (c *renameSnapshotGateContext) Deadline() (time.Time, bool) { return c.parent.Deadline() }
func (c *renameSnapshotGateContext) Done() <-chan struct{}       { return c.parent.Done() }
func (c *renameSnapshotGateContext) Value(key any) any           { return c.parent.Value(key) }
func (c *renameSnapshotGateContext) Err() error {
	if err := c.parent.Err(); err != nil {
		return err
	}
	if c.checks.Add(1) == 2 {
		c.once.Do(func() { close(c.reached) })
		<-c.release
	}
	return nil
}

func splitLSPFixtureLines(fixture []byte) []string {
	lines := strings.Split(strings.TrimSpace(string(fixture)), "\n")
	for index := range lines {
		lines[index] = strings.TrimSpace(lines[index])
	}
	return lines
}
