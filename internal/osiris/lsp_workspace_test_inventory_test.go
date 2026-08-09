package osiris

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLSPWorkspaceRootInventoryUsesDeterministicDefaultDiscovery(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "osiris.mod"), []byte("module example.com/acme/app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hidden", "ignored.test.osr"), []byte("// osiris:test name=hidden\nemit 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainTest := filepath.Join(root, "main.test.osr")
	nestedDir := filepath.Join(root, "nested")
	nestedTest := filepath.Join(nestedDir, "nested_test.osr")
	if err := os.Mkdir(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainTest, []byte("// osiris:test name=from-disk\nemit 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nestedTest, []byte("// osiris:test name=nested\nemit 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.test.osr")
	if err := os.WriteFile(outside, []byte("// osiris:test name=escaped\nemit 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(root, "escaped.test.osr")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Logf("symlink unavailable; continuing without symlink exclusion assertion: %v", err)
	}

	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	server.configureWorkspace(initializeParams{RootPath: root})
	server.upsertDocument(pathToURI(mainTest), "// osiris:test name=overlay\n// osiris:benchmark warmup=0 samples=1\nemit 1\n")

	inventory, err := server.testInventoryContext(context.Background(), pathToURI(root))
	if err != nil {
		t.Fatal(err)
	}
	if inventory.SchemaVersion != 1 || inventory.Module != "example.com/acme/app" {
		t.Fatalf("workspace inventory metadata = %#v", inventory)
	}
	wantFiles := []string{mainTest, nestedTest}
	if len(inventory.Files) != len(wantFiles) || inventory.Files[0] != wantFiles[0] || inventory.Files[1] != wantFiles[1] {
		t.Fatalf("workspace inventory files = %#v, want %#v", inventory.Files, wantFiles)
	}
	if len(inventory.Tests) != 2 || inventory.Tests[0].Name != "overlay" || !inventory.Tests[0].Benchmark || inventory.Tests[1].Name != "nested" {
		t.Fatalf("workspace inventory tests = %#v", inventory.Tests)
	}
	for _, file := range inventory.Files {
		if file == symlink {
			t.Fatalf("symlinked test file entered workspace inventory: %#v", inventory.Files)
		}
	}

	params, err := json.Marshal(map[string]any{"textDocument": map[string]any{"uri": pathToURI(root)}})
	if err != nil {
		t.Fatal(err)
	}
	value, handled, err := server.handleIDERequest(context.Background(), "osiris/testInventory", params)
	if err != nil || !handled {
		t.Fatalf("workspace inventory request = %#v, handled=%v, err=%v", value, handled, err)
	}
	wireInventory, ok := value.(LanguageTestInventory)
	if !ok || len(wireInventory.Tests) != len(inventory.Tests) || wireInventory.Files[0] != mainTest {
		t.Fatalf("workspace inventory wire result = %#v", value)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = server.testInventoryContext(cancelled, pathToURI(root))
	var coded interface{ LSPCode() int }
	if !errors.As(err, &coded) || coded.LSPCode() != -32800 {
		t.Fatalf("cancelled workspace inventory error = %v", err)
	}
}
