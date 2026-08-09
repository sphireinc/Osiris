package osiris

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLSPWorkspaceSourceBackedUserModuleRenameFromImporter(t *testing.T) {
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
	mainURI := pathToURI(mainPath)
	if err := server.upsertDocumentContext(context.Background(), mainURI, mainSource, 7); err != nil {
		t.Fatal(err)
	}

	useLine := "let message = cat(\"😀\", greetings.Hello(\"Ada\"))"
	nameStart := len("let message = cat(\"😀\", greetings.")
	position := lspPosition{Line: 2, Character: utf16Units(useLine[:nameStart+2])}
	prepared, err := server.prepareRename(mainURI, position)
	if err != nil {
		t.Fatalf("prepareRename from importer: %v", err)
	}
	preparedRange, ok := prepared.(lspRange)
	if !ok || preparedRange.Start.Line != 2 || preparedRange.Start.Character != utf16Units(useLine[:nameStart]) || preparedRange.End.Character != utf16Units(useLine[:nameStart])+len("Hello") {
		t.Fatalf("prepareRename from importer = %#v, want imported member range", prepared)
	}

	edit, err := server.renameWorkspaceEditAtVersion(mainURI, position, "Greet", intPointer(7))
	if err != nil {
		t.Fatalf("rename from importer: %v", err)
	}
	moduleURI := pathToURI(modulePath)
	if len(edit.Changes[mainURI]) != 1 || len(edit.Changes[moduleURI]) != 2 {
		t.Fatalf("cross-document rename changes = %#v, want one importer and two module edits", edit.Changes)
	}
	if edit.Changes[mainURI][0].Range.Start.Line != 2 || edit.Changes[mainURI][0].NewText != "Greet" {
		t.Fatalf("importer rename edit = %#v", edit.Changes[mainURI])
	}
	if edit.Changes[moduleURI][0].Range.Start.Line != 2 || edit.Changes[moduleURI][0].Range.Start.Character != 7 || edit.Changes[moduleURI][0].NewText != "Greet" || edit.Changes[moduleURI][1].Range.Start.Line != 1 || edit.Changes[moduleURI][1].Range.Start.Character != 5 || edit.Changes[moduleURI][1].NewText != "Greet" {
		t.Fatalf("definition rename edit = %#v", edit.Changes[moduleURI])
	}
	if len(edit.DocumentChanges) != 2 || edit.DocumentChanges[0].TextDocument.URI != moduleURI || edit.DocumentChanges[0].TextDocument.Version != 0 || edit.DocumentChanges[1].TextDocument.URI != mainURI || edit.DocumentChanges[1].TextDocument.Version != 7 {
		t.Fatalf("cross-document versioned changes = %#v", edit.DocumentChanges)
	}

	moduleURIPosition := lspPosition{Line: 1, Character: 7}
	modulePrepared, err := server.prepareRename(moduleURI, moduleURIPosition)
	if err != nil {
		t.Fatalf("prepareRename from definition: %v", err)
	}
	if got, ok := modulePrepared.(lspRange); !ok || got.Start.Line != 1 || got.Start.Character != 5 {
		t.Fatalf("prepareRename from definition = %#v", modulePrepared)
	}
	definitionEdit, err := server.renameWorkspaceEditAtVersion(moduleURI, moduleURIPosition, "Greet", intPointer(0))
	if err != nil {
		t.Fatalf("rename from indexed definition: %v", err)
	}
	if len(definitionEdit.Changes[moduleURI]) != 2 || len(definitionEdit.Changes[mainURI]) != 1 {
		t.Fatalf("rename from indexed definition changes = %#v, want the same two-document edit", definitionEdit.Changes)
	}
}
