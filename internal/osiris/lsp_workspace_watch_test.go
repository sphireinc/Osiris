package osiris

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLSPWorkspaceWatchedFilesRefreshesBoundedIndex(t *testing.T) {
	dir := t.TempDir()
	uri := pathToURI(filepath.Join(dir, "created.osr"))
	initialize := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":%q}}`, pathToURI(dir))
	watch := fmt.Sprintf(`{"jsonrpc":"2.0","method":"workspace/didChangeWatchedFiles","params":{"changes":[{"uri":%q,"type":1}]}}`, uri)
	query := `{"jsonrpc":"2.0","id":2,"method":"workspace/symbol","params":{"query":"created"}}`
	exit := `{"jsonrpc":"2.0","method":"exit"}`
	chunks := [][]byte{lspFrame(initialize), lspFrame(watch), lspFrame(query), lspFrame(exit)}
	reader := &workspaceWatchReader{chunks: chunks, before: func(index int) error {
		if index != 1 {
			return nil
		}
		return os.WriteFile(filepath.Join(dir, "created.osr"), []byte("func created() string => \"ok\"\n"), 0o644)
	}}
	var output bytes.Buffer
	server := NewLanguageServer(NewRuntime(), &output, io.Discard)
	if err := server.Serve(context.Background(), reader); err != nil {
		t.Fatal(err)
	}
	frames := decodeLSPProtocolFrames(t, output.String())
	var symbols []lspWorkspaceSymbol
	for _, frame := range frames {
		if id, ok := lspProtocolFrameID(frame); !ok || id != 2 {
			continue
		}
		result, ok := frame["result"]
		if !ok {
			t.Fatalf("workspace symbol response = %#v", frame)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(encoded, &symbols); err != nil {
			t.Fatal(err)
		}
	}
	if len(symbols) != 1 || symbols[0].Name != "created" || symbols[0].Location.URI != uri {
		t.Fatalf("workspace symbols after watched-file refresh = %#v, want created.osr symbol", symbols)
	}
}

func TestLSPWatchedFileChangesRejectMalformedEntries(t *testing.T) {
	if err := validateWatchedFileChanges([]watchedFileChange{{URI: "", Type: 1}}); err == nil {
		t.Fatal("empty watched-file URI was accepted")
	}
	if err := validateWatchedFileChanges([]watchedFileChange{{URI: "file:///tmp/example.osr", Type: 4}}); err == nil {
		t.Fatal("invalid watched-file kind was accepted")
	}
	if err := validateWatchedFileChanges([]watchedFileChange{{URI: "not a uri", Type: 1}}); err == nil {
		t.Fatal("malformed watched-file URI was accepted")
	}
}

func TestLSPDidCloseRestoresCurrentDiskDocument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.osr")
	uri := pathToURI(path)
	if err := os.WriteFile(path, []byte("func disk_value() string => \"disk\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewLanguageServer(NewRuntime(), io.Discard, io.Discard)
	server.configureWorkspace(initializeParams{RootPath: dir})
	server.refreshWorkspace(context.Background())
	if doc := server.workspace[uri]; doc == nil || !strings.Contains(doc.text, "disk_value") {
		t.Fatalf("initial workspace document = %#v, want disk snapshot", doc)
	}
	if err := server.upsertDocumentContext(context.Background(), uri, "func overlay_value() string => \"overlay\"\n", 4); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("func saved_value() string => \"saved\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server.mu.Lock()
	delete(server.docs, uri)
	delete(server.versions, uri)
	server.mu.Unlock()
	if err := server.restoreClosedWorkspaceDocument(context.Background(), uri); err != nil {
		t.Fatal(err)
	}
	doc := server.documentForAnalysis(uri)
	if doc == nil || !strings.Contains(doc.text, "saved_value") || strings.Contains(doc.text, "overlay_value") {
		t.Fatalf("restored workspace document = %#v, want current disk content", doc)
	}
}

type workspaceWatchReader struct {
	chunks [][]byte
	before func(int) error
	index  int
}

func (r *workspaceWatchReader) Read(p []byte) (int, error) {
	if r.index >= len(r.chunks) {
		return 0, io.EOF
	}
	if r.before != nil {
		if err := r.before(r.index); err != nil {
			return 0, err
		}
	}
	chunk := r.chunks[r.index]
	r.index++
	return copy(p, chunk), nil
}

func lspFrame(payload string) []byte {
	return []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload))
}
