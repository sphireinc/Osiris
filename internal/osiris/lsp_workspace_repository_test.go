package osiris

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLSPRepositoryModuleNavigationUsesVerifiedProviderSource(t *testing.T) {
	dir := t.TempDir()
	if _, err := ModuleInit(dir, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	version := RepositoryVersion{
		ModulePath: "github.com/acme/lib",
		Selector:   "1.2.3",
		Version:    "v1.2.3",
		Tag:        "v1.2.3",
		Commit:     strings.Repeat("b", 40),
		Provider:   "fixture-provider-v1",
	}
	provider := &fixtureRepositoryProvider{path: "github.com/acme/lib", snapshot: snapshotFromFiles(map[string][]byte{
		"main.osr": []byte("package lib 1.2.3\nexport answer\nlet answer = 42\n"),
	}, version)}
	rt := NewRuntime()
	rt.SetRepositoryProviders(provider)
	server := NewLanguageServer(rt, ioDiscardWriter{}, ioDiscardWriter{})
	server.configureWorkspace(initializeParams{RootPath: dir})
	mainPath := filepath.Join(dir, "main.osr")
	uri := pathToURI(mainPath)
	source := "import \"github.com/acme/lib@1.2.3\" as lib\nlet result = lib.answer\nemit result\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 3); err != nil {
		t.Fatal(err)
	}
	if len(provider.resolves) != 1 || provider.resolves[0] != "1.2.3" {
		doc := server.document(uri)
		t.Fatalf("provider selectors = %#v, want one 1.2.3 resolution; doc=%#v diagnostics=%#v", provider.resolves, doc, doc.analysis.diagnostics)
	}
	position := lspPosition{Line: 1, Character: utf16Units("let result = lib.")}
	items, err := server.completion(uri, position)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.Label == "answer" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("repository module completion = %#v, want answer", items)
	}
	locations, err := server.definition(uri, lspPosition{Line: 1, Character: utf16Units("let result = lib.") + 1})
	if err != nil {
		t.Fatal(err)
	}
	location, ok := locations.(lspLocation)
	if !ok || !strings.HasPrefix(location.URI, "osiris://repository/") {
		t.Fatalf("repository module definition = %#v, want verified virtual URI", locations)
	}
	if _, err := os.Stat(filepath.Join(dir, ".osiris.lock")); err != nil {
		t.Fatalf("verified repository resolution did not record lock state: %v", err)
	}
}
