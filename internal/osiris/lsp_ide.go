package osiris

// This file contains the editor-facing extensions for the stdio LSP server.
// The protocol layer deliberately consumes the same Parse, CheckProgram,
// dependency, formatter, and language-test inventory APIs used by the CLI.
// It owns protocol conversion and bounded indexing, not a second compiler.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	lspServerVersion        = "0.3.0"
	lspMaxWorkspaceFiles    = 512
	lspMaxWorkspaceFileSize = 4 << 20
	lspMaxWorkspaceSymbols  = 2000
	lspMaxSemanticTokens    = 10000
)

type lspRPCError struct {
	code    int
	message string
}

func (e lspRPCError) Error() string { return e.message }
func (e lspRPCError) LSPCode() int  { return e.code }

func lspInvalidParams(err error) error {
	if err == nil {
		err = fmt.Errorf("invalid parameters")
	}
	return lspRPCError{code: -32602, message: "invalid params: " + err.Error()}
}

func lspMethodNotFound(method string) error {
	return lspRPCError{code: -32601, message: "method not found: " + method}
}

func lspRequestCancelled(err error) error {
	if err == nil {
		err = context.Canceled
	}
	return lspRPCError{code: -32800, message: "request cancelled: " + err.Error()}
}

type initializeParams struct {
	RootURI          string `json:"rootUri"`
	RootPath         string `json:"rootPath"`
	WorkspaceFolders []struct {
		URI  string `json:"uri"`
		Name string `json:"name"`
	} `json:"workspaceFolders"`
	Capabilities map[string]any `json:"capabilities"`
}

func (s *languageServer) configureWorkspace(params initializeParams) {
	roots := make([]string, 0, len(params.WorkspaceFolders)+1)
	if params.RootURI != "" {
		roots = append(roots, params.RootURI)
	} else if params.RootPath != "" {
		roots = append(roots, pathToURI(params.RootPath))
	}
	for _, folder := range params.WorkspaceFolders {
		if folder.URI != "" {
			roots = append(roots, folder.URI)
		}
	}
	paths := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		path, err := uriToPath(root)
		if err != nil || path == "" {
			continue
		}
		path, err = filepath.Abs(path)
		if err != nil {
			continue
		}
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	s.mu.Lock()
	s.workspaceRoots = paths
	if len(paths) > 0 {
		s.workspaceRoot = paths[0]
	} else {
		s.workspaceRoot = ""
	}
	s.mu.Unlock()
}

func (s *languageServer) capabilities() map[string]any {
	return map[string]any{
		// 2 is TextDocumentSyncKind.Incremental.
		"textDocumentSync":           map[string]any{"openClose": true, "change": 2, "save": map[string]any{"includeText": false}},
		"hoverProvider":              true,
		"declarationProvider":        true,
		"definitionProvider":         true,
		"typeDefinitionProvider":     true,
		"implementationProvider":     true,
		"referencesProvider":         true,
		"documentHighlightProvider":  true,
		"completionProvider":         map[string]any{"triggerCharacters": []string{".", "(", "@", " "}, "resolveProvider": false},
		"signatureHelpProvider":      map[string]any{"triggerCharacters": []string{"(", ","}},
		"renameProvider":             map[string]any{"prepareProvider": true},
		"documentSymbolProvider":     true,
		"workspaceSymbolProvider":    true,
		"semanticTokensProvider":     map[string]any{"legend": map[string]any{"tokenTypes": lspSemanticTokenTypes, "tokenModifiers": []string{}}, "full": true},
		"inlayHintProvider":          true,
		"foldingRangeProvider":       true,
		"selectionRangeProvider":     true,
		"callHierarchyProvider":      true,
		"typeHierarchyProvider":      true,
		"codeActionProvider":         map[string]any{"codeActionKinds": []string{"quickfix", "source.organizeImports"}},
		"documentFormattingProvider": true,
		"diagnosticProvider":         map[string]any{"identifier": "osiris", "interFileDependencies": true, "workspaceDiagnostics": false},
		"codeLensProvider":           map[string]any{"resolveProvider": false},
	}
}

func (s *languageServer) handleIDERequest(ctx context.Context, method string, raw json.RawMessage) (any, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, true, lspRequestCancelled(err)
	}
	decode := func(dst any) error {
		if len(raw) == 0 || string(raw) == "null" {
			return fmt.Errorf("missing params")
		}
		return json.Unmarshal(raw, dst)
	}
	checkVersion := func(uri string, expected *int) error {
		if uri == "" {
			return lspInvalidParams(fmt.Errorf("textDocument.uri is required"))
		}
		return s.checkDocumentVersion(s.document(uri), expected)
	}
	switch method {
	case "textDocument/documentSymbol":
		var p lspTextDocumentParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.documentSymbolsContext(ctx, p.TextDocument.URI)
		return v, true, err
	case "workspace/symbol":
		var p struct {
			Query string `json:"query"`
		}
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		v, err := s.workspaceSymbols(ctx, p.Query)
		return v, true, err
	case "textDocument/typeDefinition":
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version *int   `json:"version,omitempty"`
			} `json:"textDocument"`
			Position *lspPosition `json:"position"`
		}
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		if p.Position == nil {
			return nil, true, lspInvalidParams(fmt.Errorf("position is required"))
		}
		v, err := s.typeDefinitionContext(ctx, p.TextDocument.URI, *p.Position)
		return v, true, err
	case "textDocument/declaration":
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version *int   `json:"version,omitempty"`
			} `json:"textDocument"`
			Position *lspPosition `json:"position"`
		}
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		if p.Position == nil {
			return nil, true, lspInvalidParams(fmt.Errorf("position is required"))
		}
		v, err := s.declarationContext(ctx, p.TextDocument.URI, *p.Position)
		return v, true, err
	case "textDocument/implementation":
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version *int   `json:"version,omitempty"`
			} `json:"textDocument"`
			Position *lspPosition `json:"position"`
		}
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		if p.Position == nil {
			return nil, true, lspInvalidParams(fmt.Errorf("position is required"))
		}
		v, err := s.implementationsContext(ctx, p.TextDocument.URI, *p.Position)
		return v, true, err
	case "textDocument/references":
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version *int   `json:"version,omitempty"`
			} `json:"textDocument"`
			Position *lspPosition `json:"position"`
		}
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		if p.Position == nil {
			return nil, true, lspInvalidParams(fmt.Errorf("position is required"))
		}
		v, err := s.referencesContext(ctx, p.TextDocument.URI, *p.Position)
		return v, true, err
	case "textDocument/documentHighlight":
		var p lspDocumentHighlightParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if p.TextDocument == nil || p.TextDocument.URI == "" {
			return nil, true, lspInvalidParams(fmt.Errorf("textDocument.uri is required"))
		}
		if p.Position == nil || p.Position.Line == nil || p.Position.Character == nil {
			return nil, true, lspInvalidParams(fmt.Errorf("position.line and position.character are required"))
		}
		position := lspPosition{Line: *p.Position.Line, Character: *p.Position.Character}
		v, err := s.documentHighlightsContext(ctx, p.TextDocument.URI, position, p.TextDocument.Version)
		return v, true, err
	case "textDocument/semanticTokens/full":
		var p lspTextDocumentParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.semanticTokensContext(ctx, p.TextDocument.URI)
		return v, true, err
	case "textDocument/signatureHelp":
		var p signatureHelpParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if p.TextDocument.URI == "" {
			return nil, true, lspInvalidParams(fmt.Errorf("textDocument.uri is required"))
		}
		if p.Position == nil {
			return nil, true, lspInvalidParams(fmt.Errorf("position is required"))
		}
		v, err := s.signatureHelpContext(ctx, p.TextDocument.URI, *p.Position, p.TextDocument.Version)
		return v, true, err
	case "textDocument/inlayHint":
		var p lspRangeParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		if p.Range == nil {
			return nil, true, lspInvalidParams(fmt.Errorf("range is required"))
		}
		v, err := s.inlayHintsContext(ctx, p.TextDocument.URI, *p.Range)
		return v, true, err
	case "textDocument/foldingRange":
		var p lspTextDocumentParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.foldingRangesContext(ctx, p.TextDocument.URI)
		return v, true, err
	case "textDocument/selectionRange":
		var p lspSelectionRangeParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		if p.Positions == nil {
			return nil, true, lspInvalidParams(fmt.Errorf("positions is required"))
		}
		v, err := s.selectionRangesContext(ctx, p.TextDocument.URI, *p.Positions)
		return v, true, err
	case "textDocument/codeAction":
		var p lspCodeActionParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if p.Range == nil {
			return nil, true, lspInvalidParams(fmt.Errorf("range is required"))
		}
		v, err := s.codeActionsAtVersionContext(ctx, p.TextDocument.URI, *p.Range, p.Context.Diagnostics, p.TextDocument.Version)
		return v, true, err
	case "textDocument/formatting":
		var p lspFormattingParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if p.TextDocument == nil || p.TextDocument.URI == "" {
			return nil, true, lspInvalidParams(fmt.Errorf("textDocument.uri is required"))
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.formattingAtVersionContext(ctx, p.TextDocument.URI, p.TextDocument.Version)
		return v, true, err
	case "textDocument/diagnostic":
		var p lspTextDocumentParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.pullDiagnosticsContext(ctx, p.TextDocument.URI)
		return v, true, err
	case "textDocument/codeLens":
		var p lspTextDocumentParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.codeLensesContext(ctx, p.TextDocument.URI)
		return v, true, err
	case "textDocument/prepareRename":
		var p lspStrictTextDocumentPositionParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		position, err := p.Position.value()
		if err != nil {
			return nil, true, lspInvalidParams(err)
		}
		v, err := s.prepareRenameContext(ctx, p.TextDocument.URI, position)
		if err != nil {
			return nil, true, err
		}
		return v, true, err
	case "callHierarchy/prepare":
		var p lspStrictTextDocumentPositionParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		position, err := p.Position.value()
		if err != nil {
			return nil, true, lspInvalidParams(err)
		}
		v, err := s.prepareCallHierarchyContext(ctx, p.TextDocument.URI, position)
		if err != nil {
			return nil, true, err
		}
		return v, true, err
	case "callHierarchy/incomingCalls":
		var p struct {
			Item lspCallHierarchyItem `json:"item"`
		}
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if p.Item.Name == "" || p.Item.URI == "" {
			return nil, true, lspInvalidParams(fmt.Errorf("call hierarchy item requires name and uri"))
		}
		v, err := s.incomingCallsContext(ctx, p.Item)
		return v, true, err
	case "callHierarchy/outgoingCalls":
		var p struct {
			Item lspCallHierarchyItem `json:"item"`
		}
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if p.Item.Name == "" || p.Item.URI == "" {
			return nil, true, lspInvalidParams(fmt.Errorf("call hierarchy item requires name and uri"))
		}
		v, err := s.outgoingCallsContext(ctx, p.Item)
		return v, true, err
	case "typeHierarchy/prepare":
		var p lspTextDocumentPositionParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		if p.Position == nil {
			return nil, true, lspInvalidParams(fmt.Errorf("position is required"))
		}
		v, err := s.prepareTypeHierarchyContext(ctx, p.TextDocument.URI, *p.Position)
		return v, true, err
	case "typeHierarchy/supertypes":
		var p struct {
			Item lspTypeHierarchyItem `json:"item"`
		}
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if p.Item.Name == "" || p.Item.URI == "" {
			return nil, true, lspInvalidParams(fmt.Errorf("type hierarchy item requires name and uri"))
		}
		v, err := s.typeSupertypesContext(ctx, p.Item)
		return v, true, err
	case "typeHierarchy/subtypes":
		var p struct {
			Item lspTypeHierarchyItem `json:"item"`
		}
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if p.Item.Name == "" || p.Item.URI == "" {
			return nil, true, lspInvalidParams(fmt.Errorf("type hierarchy item requires name and uri"))
		}
		v, err := s.typeSubtypesContext(ctx, p.Item)
		return v, true, err
	case "osiris/testInventory":
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version *int   `json:"version,omitempty"`
			} `json:"textDocument"`
		}
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.testInventoryContext(ctx, p.TextDocument.URI)
		return v, true, err
	case "osiris/testResults":
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version *int   `json:"version,omitempty"`
			} `json:"textDocument"`
			Backend string `json:"backend,omitempty"`
			Filter  string `json:"filter,omitempty"`
		}
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.testResultsContext(ctx, p.TextDocument.URI, p.Backend, p.Filter)
		return v, true, err
	default:
		return nil, false, nil
	}
}

type lspTextDocumentParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int   `json:"version,omitempty"`
	} `json:"textDocument"`
	Position lspPosition `json:"position"`
}

// lspRequiredPosition preserves the distinction between an omitted position
// component and the valid zero value. LSP line and character coordinates are
// zero-based, so decoding directly into lspPosition would silently accept
// `{}` as `{line: 0, character: 0}`.
type lspRequiredPosition struct {
	Line      *int `json:"line"`
	Character *int `json:"character"`
}

func (p *lspRequiredPosition) value() (lspPosition, error) {
	if p == nil || p.Line == nil || p.Character == nil {
		return lspPosition{}, fmt.Errorf("position.line and position.character are required")
	}
	if *p.Line < 0 || *p.Character < 0 {
		return lspPosition{}, fmt.Errorf("position.line and position.character must be non-negative")
	}
	return lspPosition{Line: *p.Line, Character: *p.Character}, nil
}

type lspStrictTextDocumentPositionParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int   `json:"version,omitempty"`
	} `json:"textDocument"`
	Position *lspRequiredPosition `json:"position"`
}

type lspTextDocumentPositionParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int   `json:"version,omitempty"`
	} `json:"textDocument"`
	Position *lspPosition `json:"position"`
}

type lspFormattingParams struct {
	TextDocument *struct {
		URI     string `json:"uri"`
		Version *int   `json:"version,omitempty"`
	} `json:"textDocument"`
}

// lspDocumentHighlightParams keeps required protocol fields distinguishable
// from their zero values. LSP positions are zero-based, so an omitted line or
// character cannot be represented safely by lspPosition alone.
type lspDocumentHighlightParams struct {
	TextDocument *struct {
		URI     string `json:"uri"`
		Version *int   `json:"version,omitempty"`
	} `json:"textDocument"`
	Position *struct {
		Line      *int `json:"line"`
		Character *int `json:"character"`
	} `json:"position"`
}

// lspDocumentHighlight is the standard textDocument/documentHighlight result
// item. Kind is read, write, or text as defined by the LSP protocol.
type lspDocumentHighlight struct {
	Range lspRange `json:"range"`
	Kind  string   `json:"kind,omitempty"`
}

type lspRangeParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int   `json:"version,omitempty"`
	} `json:"textDocument"`
	Range *lspRange `json:"range"`
}

type lspSelectionRangeParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int   `json:"version,omitempty"`
	} `json:"textDocument"`
	Positions *[]lspPosition `json:"positions"`
}

type lspCodeActionParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int   `json:"version,omitempty"`
	} `json:"textDocument"`
	Range   *lspRange `json:"range"`
	Context struct {
		Diagnostics []lspDiagnostic `json:"diagnostics"`
	} `json:"context"`
}

var lspSemanticTokenTypes = []string{"namespace", "type", "class", "enum", "interface", "struct", "typeParameter", "parameter", "variable", "property", "function", "method", "macro", "keyword", "modifier", "comment", "string", "number", "operator"}

func (s *languageServer) document(uri string) *documentState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.docs[uri]
}

func (s *languageServer) checkDocumentVersion(doc *documentState, expected *int) error {
	if expected == nil {
		return nil
	}
	if doc == nil {
		return lspInvalidParams(fmt.Errorf("document is not open"))
	}
	if doc.version != *expected {
		return lspInvalidParams(fmt.Errorf("stale document version %d; current version is %d", *expected, doc.version))
	}
	return nil
}

func (s *languageServer) allDocuments() []*documentState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byURI := make(map[string]*documentState, len(s.docs)+len(s.workspace))
	for uri, doc := range s.workspace {
		byURI[uri] = doc
	}
	for uri, doc := range s.docs {
		byURI[uri] = doc
	}
	uris := make([]string, 0, len(byURI))
	for uri := range byURI {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	out := make([]*documentState, 0, len(uris))
	for _, uri := range uris {
		out = append(out, byURI[uri])
	}
	return out
}

// resolveLSPImportedModules resolves only source files that are both
// reachable from the current document and inside the configured workspace
// roots. Repository references without local source, virtual stdlib modules,
// and paths outside the workspace are intentionally omitted.
func (s *languageServer) resolveLSPImportedModules(ctx context.Context, state *documentState, available map[string]*documentState) map[string]lspResolvedModule {
	if ctx == nil {
		ctx = context.Background()
	}
	if state == nil || state.prog == nil {
		return nil
	}
	base := state.sourceName
	if base == "" {
		base, _ = uriToPath(state.uri)
	}
	documents := make(map[string]*documentState)
	for uri, doc := range available {
		documents[uri] = doc
	}
	for _, doc := range s.allDocuments() {
		if doc != nil {
			documents[doc.uri] = doc
		}
	}
	if state.uri != "" {
		documents[state.uri] = state
	}

	resolved := make(map[string]lspResolvedModule)
	for _, statement := range state.prog.Statements {
		if err := ctx.Err(); err != nil {
			return resolved
		}
		imp, ok := statement.(importStmt)
		if !ok {
			continue
		}
		ref, selector := splitModuleRef(imp.Path)
		if strings.HasPrefix(ref, "std/") {
			continue
		}
		path, kind, err := resolveModulePath(ref, base)
		if kind == moduleKindRepository {
			if module, ok := s.resolveLSPRepositoryModule(ctx, state, ref, selector, base, documents); ok {
				resolved[imp.Path] = module
			}
			continue
		}
		if err != nil || kind != moduleKindFile || !s.workspacePathAllowed(path) {
			continue
		}
		moduleURI := pathToURI(path)
		doc := documents[moduleURI]
		if doc == nil {
			source, readErr := os.ReadFile(path)
			if readErr != nil || len(source) > lspMaxWorkspaceFileSize {
				continue
			}
			parsed, parseErr := Parse(path, string(source))
			if parseErr != nil {
				continue
			}
			doc = &documentState{uri: moduleURI, sourceName: path, text: string(source), prog: parsed}
			doc.analysis = analyzeDocumentSourceContext(ctx, s.rt, moduleURI, doc.text, parsed)
			doc.diagnostics = diagnosticsInText(doc.text, doc.analysis.diagnostics)
			s.mu.Lock()
			if existing := s.docs[moduleURI]; existing != nil {
				doc = existing
			} else if existing := s.workspace[moduleURI]; existing != nil {
				doc = existing
			} else {
				s.workspace[moduleURI] = doc
			}
			s.mu.Unlock()
			documents[moduleURI] = doc
		}
		if exports := lspExportedModuleMembers(doc); len(exports) > 0 {
			resolved[imp.Path] = lspResolvedModule{uri: moduleURI, exports: exports}
		}
	}
	return resolved
}

// resolveLSPRepositoryModule materializes a repository module through the
// runtime's verified registry, then exposes only its explicit exports to the
// semantic index. Provider resolution remains context-aware and cached by the
// registry; the LSP never executes repository source or invents a filesystem
// URI for a virtual module.
func (s *languageServer) resolveLSPRepositoryModule(ctx context.Context, state *documentState, ref, selector, base string, documents map[string]*documentState) (lspResolvedModule, bool) {
	if s == nil || s.rt == nil || state == nil || ref == "" {
		return lspResolvedModule{}, false
	}
	key, source, err := s.rt.registryFor(base).materializeContext(ctx, ref, base, selector)
	if err != nil || len(source) > lspMaxWorkspaceFileSize {
		return lspResolvedModule{}, false
	}
	moduleURI := "osiris://repository/" + url.PathEscape(key)
	doc := documents[moduleURI]
	if doc == nil {
		parsed, parseErr := Parse(key, source)
		if parseErr != nil {
			return lspResolvedModule{}, false
		}
		doc = &documentState{uri: moduleURI, sourceName: key, text: source, prog: parsed}
		doc.analysis = analyzeDocumentSourceContext(ctx, s.rt, moduleURI, doc.text, parsed)
		doc.diagnostics = diagnosticsInText(doc.text, doc.analysis.diagnostics)
		s.mu.Lock()
		if existing := s.docs[moduleURI]; existing != nil {
			doc = existing
		} else if existing := s.workspace[moduleURI]; existing != nil {
			doc = existing
		} else {
			s.workspace[moduleURI] = doc
		}
		s.mu.Unlock()
		documents[moduleURI] = doc
	}
	exports := lspExportedModuleMembers(doc)
	if len(exports) == 0 {
		return lspResolvedModule{}, false
	}
	return lspResolvedModule{uri: moduleURI, exports: exports}, true
}

// relinkLSPDocumentsContext performs a two-pass rebuild so all source-backed
// module definitions are fresh before importers acquire references to them.
// This prevents an importer from retaining pointers to a previous analysis
// generation when an open module overlay changes.
func (s *languageServer) relinkLSPDocumentsContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return
	}

	s.mu.RLock()
	workspaceSnapshot := make(map[string]*documentState, len(s.workspace))
	for uri, doc := range s.workspace {
		workspaceSnapshot[uri] = doc
	}
	documentsSnapshot := make(map[string]*documentState, len(s.docs))
	for uri, doc := range s.docs {
		documentsSnapshot[uri] = doc
	}
	workspaceRoot := s.workspaceRoot
	workspaceRoots := append([]string(nil), s.workspaceRoots...)
	s.mu.RUnlock()

	cloneDocumentState := func(state *documentState) *documentState {
		if state == nil {
			return nil
		}
		clone := *state
		return &clone
	}
	privateWorkspace := make(map[string]*documentState, len(workspaceSnapshot))
	for uri, doc := range workspaceSnapshot {
		privateWorkspace[uri] = cloneDocumentState(doc)
	}
	privateDocuments := make(map[string]*documentState, len(documentsSnapshot))
	for uri, doc := range documentsSnapshot {
		privateDocuments[uri] = cloneDocumentState(doc)
	}
	privateServer := &languageServer{
		rt:             s.rt,
		docs:           privateDocuments,
		workspace:      privateWorkspace,
		workspaceRoot:  workspaceRoot,
		workspaceRoots: workspaceRoots,
	}

	for _, doc := range privateServer.allDocuments() {
		if err := ctx.Err(); err != nil {
			return
		}
		if doc == nil || doc.prog == nil {
			continue
		}
		doc.analysis = analyzeDocumentSourceContext(ctx, privateServer.rt, doc.uri, doc.text, doc.prog)
		if err := ctx.Err(); err != nil {
			return
		}
		doc.diagnostics = diagnosticsInText(doc.text, doc.analysis.diagnostics)
	}

	for _, doc := range privateServer.allDocuments() {
		if err := ctx.Err(); err != nil {
			return
		}
		if doc == nil || doc.prog == nil {
			continue
		}
		modules := privateServer.resolveLSPImportedModules(ctx, doc, nil)
		if err := ctx.Err(); err != nil {
			return
		}
		doc.analysis = analyzeDocumentSourceContextWithModules(ctx, privateServer.rt, doc.uri, doc.text, doc.prog, modules)
		if err := ctx.Err(); err != nil {
			return
		}
		doc.diagnostics = diagnosticsInText(doc.text, doc.analysis.diagnostics)
	}
	if err := ctx.Err(); err != nil {
		return
	}

	documentMapsMatch := func(current, snapshot map[string]*documentState) bool {
		if len(current) != len(snapshot) {
			return false
		}
		for uri, doc := range snapshot {
			if current[uri] != doc {
				return false
			}
		}
		return true
	}
	rootsMatch := func(current []string, snapshot []string) bool {
		if len(current) != len(snapshot) {
			return false
		}
		for index := range snapshot {
			if current[index] != snapshot[index] {
				return false
			}
		}
		return true
	}

	s.mu.Lock()
	if err := ctx.Err(); err != nil ||
		!documentMapsMatch(s.workspace, workspaceSnapshot) ||
		!documentMapsMatch(s.docs, documentsSnapshot) ||
		s.workspaceRoot != workspaceRoot ||
		!rootsMatch(s.workspaceRoots, workspaceRoots) {
		s.mu.Unlock()
		return
	}
	s.workspace = privateServer.workspace
	s.docs = privateServer.docs
	s.mu.Unlock()
}

func (s *languageServer) refreshWorkspace(ctx context.Context) {
	s.mu.RLock()
	roots := append([]string(nil), s.workspaceRoots...)
	if len(roots) == 0 && s.workspaceRoot != "" {
		roots = []string{s.workspaceRoot}
	}
	documentsSnapshot := make(map[string]*documentState, len(s.docs))
	for uri, doc := range s.docs {
		documentsSnapshot[uri] = doc
	}
	workspaceSnapshot := make(map[string]*documentState, len(s.workspace))
	for uri, doc := range s.workspace {
		workspaceSnapshot[uri] = doc
	}
	workspaceRoot := s.workspaceRoot
	workspaceRoots := append([]string(nil), s.workspaceRoots...)
	s.mu.RUnlock()
	if len(roots) == 0 || ctx == nil || ctx.Err() != nil {
		return
	}
	sort.Strings(roots)
	indexed := make(map[string]*documentState)
	files := 0
	for _, root := range roots {
		if files >= lspMaxWorkspaceFiles {
			break
		}
		walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if entry.IsDir() {
				if path != root && (entry.Name() == ".git" || entry.Name() == ".osiris" || strings.HasPrefix(entry.Name(), ".")) {
					return filepath.SkipDir
				}
				return nil
			}
			// WalkDir does not follow symlinked directories, but reading a
			// symlinked file would still escape the workspace root. Keep the
			// index rooted by excluding symlink entries altogether.
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if files >= lspMaxWorkspaceFiles || !strings.HasSuffix(entry.Name(), ".osr") {
				return nil
			}
			info, err := entry.Info()
			if err != nil || info.Size() > lspMaxWorkspaceFileSize {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			uri := pathToURI(path)
			state := &documentState{uri: uri, sourceName: path, text: string(src)}
			state.prog, err = Parse(path, state.text)
			if err != nil {
				state.diagnostics = []lspDiagnostic{{Range: lspRange{}, Severity: 1, Source: "osiris", Message: err.Error()}}
			} else {
				state.analysis = analyzeDocumentContext(ctx, s.rt, uri, state.prog)
				if err := ctx.Err(); err != nil {
					return err
				}
				state.diagnostics = diagnosticsInText(state.text, state.analysis.diagnostics)
			}
			indexed[uri] = state
			files++
			return nil
		})
		if ctx.Err() != nil {
			return
		}
		if walkErr != nil {
			continue
		}
	}
	if ctx.Err() != nil {
		return
	}
	for uri := range documentsSnapshot {
		delete(indexed, uri)
	}

	cloneDocumentState := func(state *documentState) *documentState {
		if state == nil {
			return nil
		}
		clone := *state
		return &clone
	}
	privateDocuments := make(map[string]*documentState, len(documentsSnapshot))
	for uri, doc := range documentsSnapshot {
		privateDocuments[uri] = cloneDocumentState(doc)
	}
	privateServer := &languageServer{
		rt:             s.rt,
		docs:           privateDocuments,
		workspace:      indexed,
		workspaceRoot:  workspaceRoot,
		workspaceRoots: workspaceRoots,
	}
	privateServer.relinkLSPDocumentsContext(ctx)
	if ctx.Err() != nil {
		return
	}

	documentMapsMatch := func(current, snapshot map[string]*documentState) bool {
		if len(current) != len(snapshot) {
			return false
		}
		for uri, doc := range snapshot {
			if current[uri] != doc {
				return false
			}
		}
		return true
	}
	rootsMatch := func(current []string, snapshot []string) bool {
		if len(current) != len(snapshot) {
			return false
		}
		for index := range snapshot {
			if current[index] != snapshot[index] {
				return false
			}
		}
		return true
	}

	s.mu.Lock()
	if err := ctx.Err(); err != nil ||
		!documentMapsMatch(s.workspace, workspaceSnapshot) ||
		!documentMapsMatch(s.docs, documentsSnapshot) ||
		s.workspaceRoot != workspaceRoot ||
		!rootsMatch(s.workspaceRoots, workspaceRoots) {
		s.mu.Unlock()
		return
	}
	s.workspace = privateServer.workspace
	s.docs = privateServer.docs
	s.mu.Unlock()
}

func (s *languageServer) documentSymbols(uri string) ([]lspDocumentSymbol, error) {
	return s.documentSymbolsContext(context.Background(), uri)
}

// isLSPDocumentOrWorkspaceSymbol reports whether a source definition belongs
// in the flat document/workspace symbol indexes. Function parameters remain
// lexical navigation targets, but they are not declaration-level symbols for
// these flat providers; publishing them here would duplicate local scope
// details and make the two symbol indexes disagree with their documented
// contract.
func isLSPDocumentOrWorkspaceSymbol(def *symbolDef) bool {
	return def != nil && def.kind != "param" && validSourceSpan(def.span)
}

func (s *languageServer) documentSymbolsContext(ctx context.Context, uri string) ([]lspDocumentSymbol, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	if uri == "" {
		return nil, lspInvalidParams(fmt.Errorf("textDocument.uri is required"))
	}
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if doc.analysis == nil {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		return []lspDocumentSymbol{}, nil
	}
	out := make([]lspDocumentSymbol, 0, len(doc.analysis.defs))
	for _, def := range doc.analysis.defs {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		if !isLSPDocumentOrWorkspaceSymbol(def) || def.uri != uri {
			continue
		}
		out = append(out, lspDocumentSymbol{Name: def.name, Detail: def.detail, Kind: lspSymbolKind(def.kind), Range: s.rangeForURI(uri, def.span), SelectionRange: s.rangeForURI(uri, def.span)})
	}
	sort.SliceStable(out, func(i, j int) bool { return symbolRangeLess(out[i].Range, out[j].Range, out[i].Name, out[j].Name) })
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	return out, nil
}

// documentHighlights is the background-context compatibility wrapper for
// textDocument/documentHighlight callers that use the in-process API.
func (s *languageServer) documentHighlights(uri string, pos lspPosition, expectedVersion *int) ([]lspDocumentHighlight, error) {
	return s.documentHighlightsContext(context.Background(), uri, pos, expectedVersion)
}

// documentHighlightsContext resolves one symbol identity and returns only its
// occurrences in the requested document. Definitions and assignment targets
// are write highlights; resolved reads, exports, members, and type references
// are read highlights. The private result is checked for cancellation and a
// changed version before publication.
func (s *languageServer) documentHighlightsContext(ctx context.Context, uri string, pos lspPosition, expectedVersion *int) ([]lspDocumentHighlight, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cancelled := func() error {
		if err := ctx.Err(); err != nil {
			return lspRequestCancelled(err)
		}
		return nil
	}
	if err := cancelled(); err != nil {
		return nil, err
	}
	if uri == "" {
		return nil, lspInvalidParams(fmt.Errorf("textDocument.uri is required"))
	}
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if err := s.checkDocumentVersion(doc, expectedVersion); err != nil {
		return nil, err
	}
	if _, ok := lspUTF16Offset(doc.text, pos); !ok {
		return nil, lspInvalidParams(fmt.Errorf("invalid UTF-16 position"))
	}
	if doc.analysis == nil {
		if err := cancelled(); err != nil {
			return nil, err
		}
		if err := s.checkDocumentVersion(s.document(uri), expectedVersion); err != nil {
			return nil, err
		}
		return []lspDocumentHighlight{}, nil
	}
	ref, def := doc.analysis.findAt(toSourcePosInText(doc.text, pos))
	if err := cancelled(); err != nil {
		return nil, err
	}
	if ref != nil {
		def = ref.target
	}
	if def == nil {
		if err := s.checkDocumentVersion(s.document(uri), expectedVersion); err != nil {
			return nil, err
		}
		return []lspDocumentHighlight{}, nil
	}

	out := make([]lspDocumentHighlight, 0, len(doc.analysis.refs)+1)
	seen := make(map[lspRange]struct{}, len(doc.analysis.refs)+1)
	appendHighlight := func(span sourceSpan, kind string) {
		if !validSourceSpan(span) {
			return
		}
		rng := s.rangeForURI(uri, span)
		if _, ok := seen[rng]; ok {
			return
		}
		seen[rng] = struct{}{}
		out = append(out, lspDocumentHighlight{Range: rng, Kind: kind})
	}
	if def.uri == uri {
		appendHighlight(def.span, "write")
	}
	for _, candidate := range doc.analysis.refs {
		if err := cancelled(); err != nil {
			return nil, err
		}
		if candidate == nil || candidate.target != def {
			continue
		}
		kind := "read"
		if candidate.detail == "assignment" {
			kind = "write"
		}
		appendHighlight(candidate.span, kind)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Range.Start.Line != out[j].Range.Start.Line {
			return out[i].Range.Start.Line < out[j].Range.Start.Line
		}
		if out[i].Range.Start.Character != out[j].Range.Start.Character {
			return out[i].Range.Start.Character < out[j].Range.Start.Character
		}
		if out[i].Range.End.Line != out[j].Range.End.Line {
			return out[i].Range.End.Line < out[j].Range.End.Line
		}
		return out[i].Range.End.Character < out[j].Range.End.Character
	})
	if err := cancelled(); err != nil {
		return nil, err
	}
	if err := s.checkDocumentVersion(s.document(uri), expectedVersion); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *languageServer) workspaceSymbols(ctx context.Context, query string) ([]lspWorkspaceSymbol, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	if len(s.allDocuments()) == 0 {
		s.refreshWorkspace(ctx)
	}
	query = strings.ToLower(strings.TrimSpace(query))
	out := make([]lspWorkspaceSymbol, 0)
	documents := s.allDocuments()
	for _, doc := range documents {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		if doc == nil || doc.analysis == nil {
			continue
		}
		for _, def := range doc.analysis.defs {
			if err := ctx.Err(); err != nil {
				return nil, lspRequestCancelled(err)
			}
			if !isLSPDocumentOrWorkspaceSymbol(def) || (query != "" && !strings.Contains(strings.ToLower(def.name), query)) {
				continue
			}
			out = append(out, lspWorkspaceSymbol{Name: def.name, Kind: lspSymbolKind(def.kind), Detail: def.detail, Location: lspLocation{URI: def.uri, Range: s.rangeForURI(def.uri, def.span)}})
			if len(out) >= lspMaxWorkspaceSymbols {
				break
			}
		}
		if len(out) >= lspMaxWorkspaceSymbols {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Location.URI < out[j].Location.URI
	})
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	return out, nil
}

func (s *languageServer) typeDefinition(uri string, pos lspPosition) (any, error) {
	return s.typeDefinitionContext(context.Background(), uri, pos)
}

func (s *languageServer) typeDefinitionContext(ctx context.Context, uri string, pos lspPosition) (any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cancelled := func() error {
		if err := ctx.Err(); err != nil {
			return lspRequestCancelled(err)
		}
		return nil
	}
	if err := cancelled(); err != nil {
		return nil, err
	}
	if uri == "" {
		return nil, lspInvalidParams(fmt.Errorf("textDocument.uri is required"))
	}
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if _, ok := lspUTF16Offset(doc.text, pos); !ok {
		return nil, lspInvalidParams(fmt.Errorf("invalid UTF-16 position"))
	}
	if doc.analysis == nil {
		if err := cancelled(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	ref, def := doc.analysis.findAt(toSourcePosInText(doc.text, pos))
	if err := cancelled(); err != nil {
		return nil, err
	}
	if ref != nil {
		def = ref.target
		if def != nil && validSourceSpan(def.span) && (def.kind == "struct" || def.kind == "enum" || def.kind == "trait" || def.kind == "type alias" || def.kind == "newtype") {
			location := lspLocation{URI: def.uri, Range: s.rangeForURI(def.uri, def.span)}
			if err := cancelled(); err != nil {
				return nil, err
			}
			return location, nil
		}
	}
	if def == nil || def.typeName == "" {
		return nil, nil
	}
	var candidates []*symbolDef
	for _, candidate := range s.allDocuments() {
		if err := cancelled(); err != nil {
			return nil, err
		}
		if candidate == nil || candidate.analysis == nil {
			continue
		}
		for _, item := range candidate.analysis.defs {
			if err := cancelled(); err != nil {
				return nil, err
			}
			if item != nil && item.name == def.typeName && validSourceSpan(item.span) {
				candidates = append(candidates, item)
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].uri != candidates[j].uri {
			return candidates[i].uri < candidates[j].uri
		}
		if candidates[i].span.start.line != candidates[j].span.start.line {
			return candidates[i].span.start.line < candidates[j].span.start.line
		}
		if candidates[i].span.start.col != candidates[j].span.start.col {
			return candidates[i].span.start.col < candidates[j].span.start.col
		}
		return candidates[i].name < candidates[j].name
	})
	if err := cancelled(); err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	location := lspLocation{URI: candidates[0].uri, Range: s.rangeForURI(candidates[0].uri, candidates[0].span)}
	if err := cancelled(); err != nil {
		return nil, err
	}
	return location, nil
}

func (s *languageServer) implementations(uri string, pos lspPosition) ([]lspLocation, error) {
	return s.implementationsContext(context.Background(), uri, pos)
}

func (s *languageServer) implementationsContext(ctx context.Context, uri string, pos lspPosition) ([]lspLocation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cancelled := func() error {
		if err := ctx.Err(); err != nil {
			return lspRequestCancelled(err)
		}
		return nil
	}
	if err := cancelled(); err != nil {
		return nil, err
	}
	if uri == "" {
		return nil, lspInvalidParams(fmt.Errorf("textDocument.uri is required"))
	}
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if _, ok := lspUTF16Offset(doc.text, pos); !ok {
		return nil, lspInvalidParams(fmt.Errorf("invalid UTF-16 position"))
	}
	if doc.analysis == nil {
		if err := cancelled(); err != nil {
			return nil, err
		}
		return []lspLocation{}, nil
	}
	ref, def := doc.analysis.findAt(toSourcePosInText(doc.text, pos))
	if err := cancelled(); err != nil {
		return nil, err
	}
	if ref != nil {
		def = ref.target
	}
	if def == nil {
		return []lspLocation{}, nil
	}
	name := def.name
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		name = name[dot+1:]
	}
	wantedKind := "function"
	if def.kind == "trait method" || def.kind == "method" {
		wantedKind = "method"
	}
	seen := map[string]struct{}{}
	var out []lspLocation
	for _, candidate := range s.allDocuments() {
		if err := cancelled(); err != nil {
			return nil, err
		}
		if candidate == nil || candidate.analysis == nil {
			continue
		}
		for _, item := range candidate.analysis.defs {
			if err := cancelled(); err != nil {
				return nil, err
			}
			if item == nil || !validSourceSpan(item.span) || item.name != name {
				continue
			}
			if item.kind != wantedKind {
				continue
			}
			key := item.uri + fmt.Sprintf(":%d:%d", item.span.start.line, item.span.start.col)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, lspLocation{URI: item.uri, Range: s.rangeForURI(item.uri, item.span)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].URI != out[j].URI {
			return out[i].URI < out[j].URI
		}
		if out[i].Range.Start.Line != out[j].Range.Start.Line {
			return out[i].Range.Start.Line < out[j].Range.Start.Line
		}
		if out[i].Range.Start.Character != out[j].Range.Start.Character {
			return out[i].Range.Start.Character < out[j].Range.Start.Character
		}
		if out[i].Range.End.Line != out[j].Range.End.Line {
			return out[i].Range.End.Line < out[j].Range.End.Line
		}
		return out[i].Range.End.Character < out[j].Range.End.Character
	})
	if err := cancelled(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *languageServer) semanticTokens(uri string) (lspSemanticTokensResult, error) {
	return s.semanticTokensContext(context.Background(), uri)
}

func (s *languageServer) semanticTokensContext(ctx context.Context, uri string) (lspSemanticTokensResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return lspSemanticTokensResult{}, lspRequestCancelled(err)
	}
	if uri == "" {
		return lspSemanticTokensResult{}, lspInvalidParams(fmt.Errorf("textDocument.uri is required"))
	}
	doc := s.document(uri)
	if doc == nil {
		return lspSemanticTokensResult{}, lspInvalidParams(fmt.Errorf("document is not open: %s", uri))
	}
	tokens, err := scanSemanticTokensContext(ctx, doc.text, doc.analysis)
	if err != nil {
		return lspSemanticTokensResult{}, err
	}
	if len(tokens) > lspMaxSemanticTokens {
		tokens = tokens[:lspMaxSemanticTokens]
	}
	return encodeSemanticTokensContext(ctx, doc.text, tokens)
}

func encodeSemanticTokensContext(ctx context.Context, text string, tokens []lspSemanticToken) (lspSemanticTokensResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return lspSemanticTokensResult{}, lspRequestCancelled(err)
	}
	data := make([]int, 0, len(tokens)*5)
	lastLine, lastChar := 0, 0
	for index, token := range tokens {
		if err := ctx.Err(); err != nil {
			return lspSemanticTokensResult{}, lspRequestCancelled(err)
		}
		if !semanticTokenSpanValid(text, token) {
			return lspSemanticTokensResult{}, fmt.Errorf("invalid semantic token span at index %d", index)
		}
		if token.Line < lastLine || (token.Line == lastLine && token.Character < lastChar) {
			return lspSemanticTokensResult{}, fmt.Errorf("semantic tokens are not sorted at index %d", index)
		}
		lineDelta := token.Line - lastLine
		charDelta := token.Character
		if lineDelta == 0 {
			charDelta -= lastChar
		}
		data = append(data, lineDelta, charDelta, token.Length, token.TokenType, 0)
		lastLine, lastChar = token.Line, token.Character
	}
	if err := ctx.Err(); err != nil {
		return lspSemanticTokensResult{}, lspRequestCancelled(err)
	}
	return lspSemanticTokensResult{Data: data}, nil
}

func (s *languageServer) inlayHints(uri string, r lspRange) ([]lspInlayHint, error) {
	return s.inlayHintsContext(context.Background(), uri, r)
}

func (s *languageServer) inlayHintsContext(ctx context.Context, uri string, r lspRange) ([]lspInlayHint, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if !validLSPRange(doc.text, r) {
		return nil, lspInvalidParams(fmt.Errorf("invalid inlay hint range"))
	}
	if doc.analysis == nil || doc.analysis.checked == nil {
		return []lspInlayHint{}, nil
	}
	var out []lspInlayHint
	for _, binding := range doc.analysis.checked.Bindings {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		typeName := doc.analysis.checked.DisplayType(binding, TypeFormatSource)
		if typeName == "" || !lspSpanOverlapsRange(binding.Span, r, doc.text) {
			continue
		}
		pos := s.rangeForURI(uri, binding.Span).End
		out = append(out, lspInlayHint{Position: pos, Label: typeName, Kind: 1, PaddingLeft: true})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Position.Line != out[j].Position.Line {
			return out[i].Position.Line < out[j].Position.Line
		}
		if out[i].Position.Character != out[j].Position.Character {
			return out[i].Position.Character < out[j].Position.Character
		}
		return out[i].Label < out[j].Label
	})
	return out, nil
}

func (s *languageServer) foldingRanges(uri string) ([]lspFoldingRange, error) {
	return s.foldingRangesContext(context.Background(), uri)
}

func (s *languageServer) foldingRangesContext(ctx context.Context, uri string) ([]lspFoldingRange, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	var out []lspFoldingRange
	if doc.analysis == nil {
		return out, nil
	}
	if doc.prog != nil {
		for _, stmt := range doc.prog.Statements {
			if err := ctx.Err(); err != nil {
				return nil, lspRequestCancelled(err)
			}
			r := s.rangeForURI(uri, lspStmtSpan(stmt))
			if r.End.Line > r.Start.Line {
				out = append(out, lspFoldingRange{StartLine: r.Start.Line, StartCharacter: r.Start.Character, EndLine: r.End.Line, EndCharacter: r.End.Character, Kind: "region"})
			}
		}
	}
	lines := strings.Split(doc.text, "\n")
	for i := 0; i < len(lines); {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		if !strings.HasPrefix(strings.TrimSpace(lines[i]), "//") {
			i++
			continue
		}
		start := i
		for i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "//") {
			i++
		}
		if i > start {
			out = append(out, lspFoldingRange{StartLine: start, EndLine: i, Kind: "comment"})
		}
		i++
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].StartLine != out[j].StartLine {
			return out[i].StartLine < out[j].StartLine
		}
		if out[i].StartCharacter != out[j].StartCharacter {
			return out[i].StartCharacter < out[j].StartCharacter
		}
		if out[i].EndLine != out[j].EndLine {
			return out[i].EndLine < out[j].EndLine
		}
		if out[i].EndCharacter != out[j].EndCharacter {
			return out[i].EndCharacter < out[j].EndCharacter
		}
		return out[i].Kind < out[j].Kind
	})
	return out, nil
}

func (s *languageServer) selectionRanges(uri string, positions []lspPosition) ([]lspSelectionRange, error) {
	return s.selectionRangesContext(context.Background(), uri, positions)
}

func (s *languageServer) selectionRangesContext(ctx context.Context, uri string, positions []lspPosition) ([]lspSelectionRange, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	out := make([]lspSelectionRange, 0, len(positions))
	for _, pos := range positions {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		offset, ok := lspUTF16Offset(doc.text, pos)
		if !ok {
			return nil, lspInvalidParams(fmt.Errorf("invalid selection position"))
		}
		start, end := wordBounds(doc.text, offset)
		word := lspRangeFromOffsets(doc.text, start, end)
		lines := strings.Split(doc.text, "\n")
		lineText := ""
		if pos.Line >= 0 && pos.Line < len(lines) {
			lineText = lines[pos.Line]
		}
		line := lspRange{Start: lspPosition{Line: pos.Line, Character: 0}, End: lspPosition{Line: pos.Line, Character: utf16Units(lineText)}}
		root := lspRange{Start: lspPosition{Line: 0, Character: 0}, End: fullDocumentRange(doc.text).End}
		lineSelection := &lspSelectionRange{Range: line, Parent: &lspSelectionRange{Range: root}}
		if start == end {
			out = append(out, lspSelectionRange{Range: lspRange{Start: pos, End: pos}, Parent: lineSelection})
		} else {
			out = append(out, lspSelectionRange{Range: word, Parent: lineSelection})
		}
	}
	return out, nil
}

func (s *languageServer) codeActions(uri string, r lspRange, diagnostics []lspDiagnostic) ([]lspCodeAction, error) {
	return s.codeActionsAtVersion(uri, r, diagnostics, nil)
}

func (s *languageServer) codeActionsAtVersion(uri string, r lspRange, diagnostics []lspDiagnostic, expectedVersion *int) ([]lspCodeAction, error) {
	return s.codeActionsAtVersionContext(context.Background(), uri, r, diagnostics, expectedVersion)
}

func (s *languageServer) codeActionsAtVersionContext(ctx context.Context, uri string, r lspRange, diagnostics []lspDiagnostic, expectedVersion *int) ([]lspCodeAction, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	if uri == "" {
		return nil, lspInvalidParams(fmt.Errorf("textDocument.uri is required"))
	}
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if err := s.checkDocumentVersion(doc, expectedVersion); err != nil {
		return nil, err
	}
	if !validLSPRange(doc.text, r) {
		return nil, lspInvalidParams(fmt.Errorf("invalid code action range"))
	}
	var out []lspCodeAction
	for _, diagnostic := range diagnostics {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		if strings.Contains(strings.ToLower(diagnostic.Message), "deprecated") {
			out = append(out, lspCodeAction{Title: "Migrate deprecated API", Kind: "quickfix", Diagnostics: []lspDiagnostic{diagnostic}})
		}
	}
	if edits, err := organizeImportEditsContext(ctx, doc); err != nil {
		return nil, err
	} else if len(edits) > 0 {
		out = append(out, lspCodeAction{
			Title: "Organize Osiris imports",
			Kind:  "source.organizeImports",
			Edit:  versionedWorkspaceEdit(map[string][]lspTextEdit{uri: edits}, map[string]int{uri: doc.version}),
		})
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	return out, nil
}

// organizeImportEdits returns only the edits needed to sort a leading import
// block. It deliberately does not reformat the whole document: a source
// organize-import action must be safe to apply independently of formatting,
// preserve unrelated source text, and remain valid when the client applies
// edits from the end of the document toward the beginning.
//
// Imports that occur after another non-import statement are left alone. Moving
// those imports could cross executable declarations or comments whose source
// ownership is not represented in the AST. The formatter remains available for
// full-document formatting in textDocument/formatting.
func organizeImportEdits(doc *documentState) ([]lspTextEdit, error) {
	return organizeImportEditsContext(context.Background(), doc)
}

func organizeImportEditsContext(ctx context.Context, doc *documentState) ([]lspTextEdit, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	if doc == nil {
		return nil, fmt.Errorf("document is not open")
	}
	prog := doc.prog
	if prog == nil {
		var err error
		prog, err = Parse(doc.sourceName, doc.text)
		if err != nil {
			return nil, err
		}
	}
	type importSlot struct {
		stmt  importStmt
		start int
		end   int
		text  string
	}
	slots := make([]importSlot, 0)
	blocked := false
	for _, statement := range prog.Statements {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		imp, ok := statement.(importStmt)
		if !ok {
			if len(slots) > 0 {
				blocked = true
			}
			continue
		}
		if blocked {
			return []lspTextEdit{}, nil
		}
		start, ok := sourceSpanOffsets(doc.text, imp.Span)
		if !ok {
			return nil, fmt.Errorf("import source span is invalid")
		}
		end := start + imp.Span.end.col - imp.Span.start.col
		if end < start || end > len(doc.text) {
			return nil, fmt.Errorf("import source span exceeds document")
		}
		slots = append(slots, importSlot{stmt: imp, start: start, end: end, text: doc.text[start:end]})
	}
	if len(slots) < 2 {
		return []lspTextEdit{}, nil
	}
	sorted := append([]importSlot(nil), slots...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].stmt.Path != sorted[j].stmt.Path {
			return sorted[i].stmt.Path < sorted[j].stmt.Path
		}
		return sorted[i].stmt.Alias < sorted[j].stmt.Alias
	})
	edits := make([]lspTextEdit, 0, len(slots))
	for i, slot := range slots {
		replacement := formatStmt(sorted[i].stmt)
		if slot.text == replacement {
			continue
		}
		edits = append(edits, lspTextEdit{Range: lspRangeFromOffsets(doc.text, slot.start, slot.end), NewText: replacement})
	}
	sort.Slice(edits, func(i, j int) bool {
		if edits[i].Range.Start.Line != edits[j].Range.Start.Line {
			return edits[i].Range.Start.Line > edits[j].Range.Start.Line
		}
		return edits[i].Range.Start.Character > edits[j].Range.Start.Character
	})
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	return edits, nil
}

func (s *languageServer) formatting(uri string) ([]lspTextEdit, error) {
	return s.formattingAtVersion(uri, nil)
}

func (s *languageServer) formattingAtVersion(uri string, expectedVersion *int) ([]lspTextEdit, error) {
	return s.formattingAtVersionContext(context.Background(), uri, expectedVersion)
}

// formattingAtVersionContext formats one immutable document snapshot. The
// formatter itself is synchronous, so cancellation is checked before parsing
// and after the complete edit has been built; a canceled request never
// publishes a partial edit.
func (s *languageServer) formattingAtVersionContext(ctx context.Context, uri string, expectedVersion *int) ([]lspTextEdit, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if err := s.checkDocumentVersion(doc, expectedVersion); err != nil {
		return nil, err
	}
	formatted, err := FormatSource(doc.sourceName, doc.text)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	if formatted == doc.text {
		return []lspTextEdit{}, nil
	}
	return []lspTextEdit{{Range: fullDocumentRange(doc.text), NewText: formatted}}, nil
}

func (s *languageServer) pullDiagnostics(uri string) (lspDiagnosticReport, error) {
	return s.pullDiagnosticsContext(context.Background(), uri)
}

// pullDiagnosticsContext returns a complete, deterministic snapshot of the
// current document diagnostics. The snapshot is copied before it is returned
// so a caller cannot mutate the server's open-document state. Diagnostics are
// ordered by source range and then by their stable metadata, which keeps pull
// responses reproducible even when multiple analyzer phases report the same
// source region. The context is checked before and after the copy so a client
// cancellation cannot publish a partially observed result.
func (s *languageServer) pullDiagnosticsContext(ctx context.Context, uri string) (lspDiagnosticReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return lspDiagnosticReport{}, lspRequestCancelled(err)
	}
	if uri == "" {
		return lspDiagnosticReport{}, lspInvalidParams(fmt.Errorf("textDocument.uri is required"))
	}
	doc := s.document(uri)
	if doc == nil {
		return lspDiagnosticReport{}, fmt.Errorf("document not open: %s", uri)
	}
	items := append([]lspDiagnostic(nil), doc.diagnostics...)
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Range.Start.Line != b.Range.Start.Line {
			return a.Range.Start.Line < b.Range.Start.Line
		}
		if a.Range.Start.Character != b.Range.Start.Character {
			return a.Range.Start.Character < b.Range.Start.Character
		}
		if a.Range.End.Line != b.Range.End.Line {
			return a.Range.End.Line < b.Range.End.Line
		}
		if a.Range.End.Character != b.Range.End.Character {
			return a.Range.End.Character < b.Range.End.Character
		}
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return a.Message < b.Message
	})
	if err := ctx.Err(); err != nil {
		return lspDiagnosticReport{}, lspRequestCancelled(err)
	}
	return lspDiagnosticReport{Kind: "full", Items: items}, nil
}

func (s *languageServer) codeLenses(uri string) ([]lspCodeLens, error) {
	return s.codeLensesContext(context.Background(), uri)
}

func (s *languageServer) codeLensesContext(ctx context.Context, uri string) ([]lspCodeLens, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	inventory, err := s.testInventoryContext(ctx, uri)
	if err != nil {
		// A production document without a test sidecar is a valid empty lens
		// result. Syntax errors in a test file are likewise reported by the
		// ordinary diagnostics path rather than as a code-lens failure.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || isLSPRequestCancellation(err) {
			return nil, err
		}
		return []lspCodeLens{}, nil
	}
	out := make([]lspCodeLens, 0, len(inventory.Tests))
	for _, entry := range inventory.Tests {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		command := lspCommand{Title: "Run " + entry.Name, Command: "osiris.test", Arguments: []any{entry.File, entry.Name}}
		if entry.Benchmark {
			command = lspCommand{Title: "Benchmark " + entry.Name, Command: "osiris.benchmark", Arguments: []any{entry.File, entry.Name}}
		}
		rng, err := lspLanguageTestRange(s, entry)
		if err != nil {
			return nil, err
		}
		out = append(out, lspCodeLens{Range: rng, Command: &command})
	}
	return out, nil
}

// lspLanguageTestRange converts the language-test source location contract
// (one-based rune columns) to the LSP contract (zero-based UTF-16 columns).
// Keeping this conversion at the protocol boundary matters for test
// directives that contain astral characters: a rune column and a UTF-16
// column are not interchangeable there.
func lspLanguageTestRange(s *languageServer, entry LanguageTestEntry) (lspRange, error) {
	location := entry.Location
	source := ""
	if s != nil && entry.File != "" {
		if overlay := s.document(pathToURI(entry.File)); overlay != nil {
			source = overlay.text
		}
	}
	if source == "" && entry.File != "" {
		data, err := os.ReadFile(entry.File)
		if err != nil {
			return lspRange{}, err
		}
		source = string(data)
	}
	if source == "" {
		return lspRange{}, fmt.Errorf("test source is unavailable: %s", entry.File)
	}
	start, ok := lspLanguageTestPosition(source, location.Start)
	if !ok {
		return lspRange{}, fmt.Errorf("invalid test start location in %s", entry.File)
	}
	end, ok := lspLanguageTestPosition(source, location.End)
	if !ok {
		return lspRange{}, fmt.Errorf("invalid test end location in %s", entry.File)
	}
	return lspRange{Start: start, End: end}, nil
}

func lspLanguageTestPosition(source string, position SourcePosition) (lspPosition, bool) {
	if position.Line < 1 || position.Column < 1 {
		return lspPosition{}, false
	}
	lines := strings.Split(source, "\n")
	if position.Line > len(lines) {
		return lspPosition{}, false
	}
	line := strings.TrimSuffix(lines[position.Line-1], "\r")
	runes := []rune(line)
	runeOffset := position.Column - 1
	if runeOffset < 0 || runeOffset > len(runes) {
		return lspPosition{}, false
	}
	return lspPosition{Line: position.Line - 1, Character: utf16Units(string(runes[:runeOffset]))}, true
}

func (s *languageServer) testInventory(uri string) (LanguageTestInventory, error) {
	return s.testInventoryContext(context.Background(), uri)
}

func (s *languageServer) testInventoryContext(ctx context.Context, uri string) (LanguageTestInventory, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return LanguageTestInventory{}, lspRequestCancelled(err)
	}
	doc := s.document(uri)
	if doc == nil {
		root, err := uriToPath(uri)
		if err == nil {
			if info, statErr := os.Stat(root); statErr == nil && info.IsDir() {
				return s.workspaceTestInventoryContext(ctx, root)
			}
		}
		return LanguageTestInventory{}, fmt.Errorf("document not open: %s", uri)
	}
	file, source := doc.sourceName, doc.text
	if !isLanguageTestFile(file) {
		if companion := colocatedTestFile(file); companion != "" {
			if !s.workspacePathAllowed(companion) {
				return LanguageTestInventory{}, fmt.Errorf("test companion %q is outside configured workspace roots", companion)
			}
			file = companion
			if overlay := s.document(pathToURI(companion)); overlay != nil {
				source = overlay.text
			} else {
				data, err := os.ReadFile(companion)
				if err != nil {
					return LanguageTestInventory{}, err
				}
				source = string(data)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return LanguageTestInventory{}, lspRequestCancelled(err)
	}
	entries, err := parseLanguageTestFile(file, source)
	if err != nil {
		return LanguageTestInventory{}, err
	}
	if err := ctx.Err(); err != nil {
		return LanguageTestInventory{}, lspRequestCancelled(err)
	}
	out := LanguageTestInventory{SchemaVersion: 1, Files: []string{file}}
	if doc.prog != nil {
		for _, stmt := range doc.prog.Statements {
			if pkg, ok := stmt.(packageStmt); ok {
				out.Package = pkg.Name
				break
			}
		}
	}
	if manifest, err := LoadModuleManifest(ModuleRootFor(file)); err == nil {
		out.Module = manifest.Module
	}
	for _, entry := range entries {
		out.Tests = append(out.Tests, LanguageTestEntry{Name: entry.name, File: entry.file, Location: entry.location, Skip: entry.skipReason != "", ExpectedFailure: entry.expectedFailure, Benchmark: entry.benchmark != nil, Fuzz: entry.fuzz != nil})
	}
	return out, nil
}

// workspaceTestInventoryContext discovers every language-test sidecar below
// one configured workspace directory. It is intentionally metadata-only: the
// returned files are sorted, parsed, and associated with the same overlays
// used by single-document inventory, but no test body is executed.
func (s *languageServer) workspaceTestInventoryContext(ctx context.Context, root string) (LanguageTestInventory, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return LanguageTestInventory{}, lspRequestCancelled(err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return LanguageTestInventory{}, err
	}
	if !info.IsDir() {
		return LanguageTestInventory{}, fmt.Errorf("test inventory root is not a directory: %s", root)
	}
	if !s.workspacePathAllowed(root) || !s.hasConfiguredWorkspaceRoot() {
		return LanguageTestInventory{}, fmt.Errorf("test inventory root is outside configured workspace roots: %s", root)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return LanguageTestInventory{}, err
	}
	root = filepath.Clean(root)
	files := make([]string, 0)
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == ".git" || entry.Name() == ".osiris" || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !isLanguageTestFile(path) {
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil || fileInfo.Size() > lspMaxWorkspaceFileSize {
			return nil
		}
		files = append(files, filepath.Clean(path))
		if len(files) >= lspMaxWorkspaceFiles {
			return filepath.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		if errors.Is(walkErr, context.Canceled) || errors.Is(walkErr, context.DeadlineExceeded) {
			return LanguageTestInventory{}, lspRequestCancelled(walkErr)
		}
		return LanguageTestInventory{}, walkErr
	}
	if err := ctx.Err(); err != nil {
		return LanguageTestInventory{}, lspRequestCancelled(err)
	}
	sort.Strings(files)
	out := LanguageTestInventory{SchemaVersion: 1, Files: files}
	if manifest, err := LoadModuleManifest(ModuleRootFor(root)); err == nil {
		out.Module = manifest.Module
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return LanguageTestInventory{}, lspRequestCancelled(err)
		}
		source, err := os.ReadFile(file)
		if err != nil {
			return LanguageTestInventory{}, err
		}
		if overlay := s.document(pathToURI(file)); overlay != nil {
			source = []byte(overlay.text)
		}
		entries, err := parseLanguageTestFile(file, string(source))
		if err != nil {
			return LanguageTestInventory{}, err
		}
		for _, entry := range entries {
			out.Tests = append(out.Tests, LanguageTestEntry{Name: entry.name, File: entry.file, Location: entry.location, Skip: entry.skipReason != "", ExpectedFailure: entry.expectedFailure, Benchmark: entry.benchmark != nil, Fuzz: entry.fuzz != nil})
		}
	}
	return out, nil
}

func isLSPRequestCancellation(err error) bool {
	var coded interface{ LSPCode() int }
	return errors.As(err, &coded) && coded.LSPCode() == -32800
}

func (s *languageServer) hasConfiguredWorkspaceRoot() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.workspaceRoots) > 0 || s.workspaceRoot != ""
}

// workspacePathAllowed is intentionally stricter than filepath.Rel alone:
// lexical containment prevents ../ escapes, while EvalSymlinks prevents a
// sidecar symlink from redirecting an inventory read outside a configured
// workspace root. With no configured root, an explicitly opened document may
// still use its neighboring sidecar, matching standalone LSP behavior.
func (s *languageServer) workspacePathAllowed(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	abs = filepath.Clean(abs)
	s.mu.RLock()
	roots := append([]string(nil), s.workspaceRoots...)
	if len(roots) == 0 && s.workspaceRoot != "" {
		roots = []string{s.workspaceRoot}
	}
	s.mu.RUnlock()
	if len(roots) == 0 {
		return true
	}
	resolvedPath, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return false
	}
	resolvedPath, err = filepath.Abs(resolvedPath)
	if err != nil {
		return false
	}
	for _, root := range roots {
		rootAbs, err := filepath.Abs(filepath.Clean(root))
		if err != nil || !pathWithin(rootAbs, abs) {
			continue
		}
		resolvedRoot, err := filepath.EvalSymlinks(rootAbs)
		if err != nil {
			continue
		}
		resolvedRoot, err = filepath.Abs(resolvedRoot)
		if err == nil && pathWithin(resolvedRoot, resolvedPath) {
			return true
		}
	}
	return false
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func (s *languageServer) prepareRename(uri string, pos lspPosition) (any, error) {
	return s.prepareRenameContext(context.Background(), uri, pos)
}

// prepareRenameContext returns the exact source range under the cursor when
// the analyzer has a source-backed identity for that occurrence. A definition
// range from another document or a virtual standard-library URI is never
// treated as editable source.
func (s *languageServer) prepareRenameContext(ctx context.Context, uri string, pos lspPosition) (any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	doc := s.documentForAnalysis(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if doc.analysis == nil {
		return nil, nil
	}
	if _, ok := lspUTF16Offset(doc.text, pos); !ok {
		return nil, lspInvalidParams(fmt.Errorf("invalid rename position"))
	}
	ref, def := doc.analysis.findAt(toSourcePosInText(doc.text, pos))
	span := sourceSpan{}
	if ref != nil {
		def = ref.target
		span = ref.span
	} else if def != nil {
		span = def.span
	}
	if !s.sourceBackedRenameOccurrence(uri, doc.text, span, def) {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	return toLSPRangeInText(doc.text, span), nil
}

func (s *languageServer) renameWorkspaceEdit(uri string, pos lspPosition, replacement string) (lspWorkspaceEdit, error) {
	return s.renameWorkspaceEditContext(context.Background(), uri, pos, replacement, nil)
}

func (s *languageServer) renameWorkspaceEditAtVersion(uri string, pos lspPosition, replacement string, expectedVersion *int) (lspWorkspaceEdit, error) {
	return s.renameWorkspaceEditContext(context.Background(), uri, pos, replacement, expectedVersion)
}

// renameWorkspaceEditContext builds a source-backed, versioned workspace edit.
// Analyzer target pointers are the identity boundary: shadowed names and
// same-spelled members are not renamed unless the current analyzer resolved
// them to the same symbol. Every source range is validated against the live
// document text before being emitted.
func (s *languageServer) renameWorkspaceEditContext(ctx context.Context, uri string, pos lspPosition, replacement string, expectedVersion *int) (lspWorkspaceEdit, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return lspWorkspaceEdit{}, lspRequestCancelled(err)
	}
	if !validIdentifier(replacement) {
		return lspWorkspaceEdit{}, lspInvalidParams(fmt.Errorf("invalid replacement identifier %q", replacement))
	}
	doc := s.documentForAnalysis(uri)
	if doc == nil || doc.analysis == nil {
		return lspWorkspaceEdit{}, fmt.Errorf("document not open: %s", uri)
	}
	if err := s.checkDocumentVersion(doc, expectedVersion); err != nil {
		return lspWorkspaceEdit{}, err
	}
	if _, ok := lspUTF16Offset(doc.text, pos); !ok {
		return lspWorkspaceEdit{}, lspInvalidParams(fmt.Errorf("invalid rename position"))
	}
	ref, target := doc.analysis.findAt(toSourcePosInText(doc.text, pos))
	if ref != nil {
		target = ref.target
	}
	if ref != nil {
		if !s.sourceBackedRenameOccurrence(uri, doc.text, ref.span, target) {
			return lspWorkspaceEdit{}, fmt.Errorf("no renameable symbol at position")
		}
	} else if !sourceBackedRenameTarget(uri, doc.text, target) {
		return lspWorkspaceEdit{}, fmt.Errorf("no renameable symbol at position")
	}
	documentSnapshot := make(map[string]*documentState)
	for _, candidateDoc := range s.allDocuments() {
		if candidateDoc != nil {
			documentSnapshot[candidateDoc.uri] = candidateDoc
		}
	}
	if documentSnapshot[uri] != doc {
		return lspWorkspaceEdit{}, lspInvalidParams(fmt.Errorf("document changed while preparing rename for %s", uri))
	}
	targetDoc := documentSnapshot[target.uri]
	var snapshotTarget *symbolDef
	if targetDoc != nil && targetDoc.analysis != nil {
		for _, candidate := range targetDoc.analysis.defs {
			if sameRenameDefinition(candidate, target) {
				snapshotTarget = candidate
				break
			}
		}
	}
	if snapshotTarget == nil || !renameSpanMatches(targetDoc.text, snapshotTarget.span, snapshotTarget.name) {
		return lspWorkspaceEdit{}, lspInvalidParams(fmt.Errorf("document changed while preparing rename for %s", target.uri))
	}
	target = snapshotTarget
	// The target and every candidate below must come from one indexed
	// generation. This check gives tests and request implementations a
	// transaction boundary after the initial document/version validation.
	if err := ctx.Err(); err != nil {
		return lspWorkspaceEdit{}, lspRequestCancelled(err)
	}
	byURI := make(map[string][]lspRenameSpanEdit)
	documents := make([]*documentState, 0, len(documentSnapshot))
	for _, candidateDoc := range documentSnapshot {
		documents = append(documents, candidateDoc)
	}
	sort.Slice(documents, func(i, j int) bool { return documents[i].uri < documents[j].uri })
	for _, candidateDoc := range documents {
		if err := ctx.Err(); err != nil {
			return lspWorkspaceEdit{}, lspRequestCancelled(err)
		}
		if candidateDoc == nil || candidateDoc.analysis == nil {
			continue
		}
		for _, candidate := range candidateDoc.analysis.defs {
			if err := ctx.Err(); err != nil {
				return lspWorkspaceEdit{}, lspRequestCancelled(err)
			}
			if candidate == nil || candidate.uri != candidateDoc.uri {
				continue
			}
			if sameRenameDefinition(candidate, target) {
				if item, ok := renameSpanEdit(candidateDoc.text, candidate.span, target.name); ok {
					byURI[candidateDoc.uri] = append(byURI[candidateDoc.uri], item)
				}
			}
		}
		for _, candidate := range candidateDoc.analysis.refs {
			if err := ctx.Err(); err != nil {
				return lspWorkspaceEdit{}, lspRequestCancelled(err)
			}
			if candidate == nil || candidate.target == nil {
				continue
			}
			if sameRenameDefinition(candidate.target, target) {
				if item, ok := renameSpanEdit(candidateDoc.text, candidate.span, target.name); ok {
					byURI[candidateDoc.uri] = append(byURI[candidateDoc.uri], item)
				}
			}
		}
	}
	if len(byURI) == 0 {
		return lspWorkspaceEdit{}, fmt.Errorf("no renameable references found")
	}
	edit := lspWorkspaceEdit{Changes: make(map[string][]lspTextEdit, len(byURI))}
	for targetURI, spans := range byURI {
		candidateDoc := documentSnapshot[targetURI]
		if candidateDoc == nil {
			return lspWorkspaceEdit{}, lspInvalidParams(fmt.Errorf("document disappeared while preparing rename for %s", targetURI))
		}
		items := spans
		sort.Slice(items, func(i, j int) bool {
			if items[i].start != items[j].start {
				return items[i].start < items[j].start
			}
			return items[i].end < items[j].end
		})
		validated := make([]lspRenameSpanEdit, 0, len(items))
		for _, item := range items {
			if len(validated) > 0 && item.start == validated[len(validated)-1].start && item.end == validated[len(validated)-1].end {
				continue
			}
			if len(validated) > 0 && item.start < validated[len(validated)-1].end {
				return lspWorkspaceEdit{}, fmt.Errorf("overlapping rename source spans")
			}
			validated = append(validated, item)
		}
		for i := len(validated) - 1; i >= 0; i-- {
			item := validated[i]
			rng := toLSPRangeInText(candidateDoc.text, item.span)
			edit.Changes[targetURI] = append(edit.Changes[targetURI], lspTextEdit{Range: rng, NewText: replacement})
		}
		sort.Slice(edit.Changes[targetURI], func(i, j int) bool {
			a, b := edit.Changes[targetURI][i].Range, edit.Changes[targetURI][j].Range
			if a.Start.Line != b.Start.Line {
				return a.Start.Line > b.Start.Line
			}
			return a.Start.Character > b.Start.Character
		})
	}
	if len(edit.Changes) == 0 {
		return lspWorkspaceEdit{}, fmt.Errorf("no renameable references found")
	}
	versions := make(map[string]int, len(edit.Changes))
	for targetURI := range edit.Changes {
		if candidate := documentSnapshot[targetURI]; candidate != nil {
			versions[targetURI] = candidate.version
		}
	}
	edit.DocumentChanges = versionedDocumentChanges(edit.Changes, versions)
	if err := ctx.Err(); err != nil {
		return lspWorkspaceEdit{}, lspRequestCancelled(err)
	}
	if err := s.validateRenameDocumentSnapshot(edit.Changes, documentSnapshot); err != nil {
		return lspWorkspaceEdit{}, err
	}
	return edit, nil
}

// validateRenameDocumentSnapshot is the commit check for a multi-document
// rename. The server returns an edit rather than applying it, but the edit is
// still a transaction over the source generations that were analyzed. If an
// affected open overlay or workspace-indexed file was replaced while the
// request was scanning, the caller must retry instead of receiving a mixed
// generation edit.
func (s *languageServer) validateRenameDocumentSnapshot(changes map[string][]lspTextEdit, snapshot map[string]*documentState) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for uri := range changes {
		expected := snapshot[uri]
		current := s.docs[uri]
		if current == nil {
			current = s.workspace[uri]
		}
		if expected == nil || current != expected || current.version != expected.version {
			return lspInvalidParams(fmt.Errorf("document changed while preparing rename for %s", uri))
		}
	}
	return nil
}

func sourceBackedRenameTarget(uri, text string, def *symbolDef) bool {
	return def != nil && def.uri == uri && validIdentifier(def.name) && renameSpanMatches(text, def.span, def.name)
}

// sourceBackedRenameOccurrence accepts a reference in the current document
// when its definition is either local or an indexed, source-backed user
// module. Virtual standard-library symbols and unresolved definitions have no
// editable source span and therefore remain outside rename.
func (s *languageServer) sourceBackedRenameOccurrence(uri, text string, occurrence sourceSpan, def *symbolDef) bool {
	if def == nil || !validIdentifier(def.name) || !renameSpanMatches(text, occurrence, def.name) {
		return false
	}
	if def.uri == uri {
		return true
	}
	definition := s.documentForAnalysis(def.uri)
	if definition == nil || definition.analysis == nil || definition.prog == nil || definition.uri != def.uri {
		return false
	}
	if _, err := uriToPath(def.uri); err != nil {
		return false
	}
	return renameSpanMatches(definition.text, def.span, def.name)
}

func renameSpanMatches(text string, span sourceSpan, name string) bool {
	if name == "" {
		return false
	}
	return spanText(text, span) == name
}

func sameRenameDefinition(candidate, target *symbolDef) bool {
	if candidate == nil || target == nil || candidate.uri != target.uri {
		return false
	}
	return candidate == target || (candidate.name == target.name && candidate.kind == target.kind && candidate.span == target.span)
}

func renameSpanEdit(text string, span sourceSpan, name string) (lspRenameSpanEdit, bool) {
	start, ok := sourceSpanOffsets(text, span)
	if !ok || !renameSpanMatches(text, span, name) {
		return lspRenameSpanEdit{}, false
	}
	return lspRenameSpanEdit{span: span, start: start, end: start + len(name)}, true
}

type lspCallHierarchyItem struct {
	Name           string   `json:"name"`
	Kind           int      `json:"kind"`
	Detail         string   `json:"detail,omitempty"`
	URI            string   `json:"uri"`
	Range          lspRange `json:"range"`
	SelectionRange lspRange `json:"selectionRange"`
	Data           string   `json:"data,omitempty"`
}

type lspCallHierarchyIncoming struct {
	From       lspCallHierarchyItem `json:"from"`
	FromRanges []lspRange           `json:"fromRanges"`
}

type lspCallHierarchyOutgoing struct {
	To         lspCallHierarchyItem `json:"to"`
	FromRanges []lspRange           `json:"fromRanges"`
}

func callHierarchyItem(def *symbolDef, s *languageServer) lspCallHierarchyItem {
	rng := s.rangeForURI(def.uri, def.span)
	return lspCallHierarchyItem{Name: def.name, Kind: lspSymbolKind(def.kind), Detail: def.detail, URI: def.uri, Range: rng, SelectionRange: rng, Data: fmt.Sprintf("%s:%d:%d", def.uri, def.span.start.line, def.span.start.col)}
}

func (s *languageServer) prepareCallHierarchy(uri string, pos lspPosition) ([]lspCallHierarchyItem, error) {
	return s.prepareCallHierarchyContext(context.Background(), uri, pos)
}

func (s *languageServer) prepareCallHierarchyContext(ctx context.Context, uri string, pos lspPosition) ([]lspCallHierarchyItem, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if _, ok := lspUTF16Offset(doc.text, pos); !ok {
		return nil, lspInvalidParams(fmt.Errorf("invalid UTF-16 position"))
	}
	if doc.analysis == nil {
		return []lspCallHierarchyItem{}, nil
	}
	ref, def := doc.analysis.findAt(toSourcePosInText(doc.text, pos))
	if ref != nil {
		def = ref.target
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	if def == nil || (def.kind != "function" && def.kind != "macro" && def.kind != "trait method") {
		return []lspCallHierarchyItem{}, nil
	}
	return []lspCallHierarchyItem{callHierarchyItem(def, s)}, nil
}

func (s *languageServer) incomingCalls(item lspCallHierarchyItem) ([]lspCallHierarchyIncoming, error) {
	return s.incomingCallsContext(context.Background(), item)
}

func (s *languageServer) incomingCallsContext(ctx context.Context, item lspCallHierarchyItem) ([]lspCallHierarchyIncoming, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	target := s.callHierarchyDefinition(item)
	if target == nil {
		return []lspCallHierarchyIncoming{}, nil
	}
	groups := make(map[*symbolDef]int)
	var out []lspCallHierarchyIncoming
	for _, doc := range s.allDocuments() {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		if doc == nil || doc.analysis == nil {
			continue
		}
		for _, ref := range doc.analysis.refs {
			if err := ctx.Err(); err != nil {
				return nil, lspRequestCancelled(err)
			}
			if ref == nil || ref.target != target || !callHierarchyCallable(ref.target) {
				continue
			}
			from := callHierarchyContainingDefinition(doc, ref.span.start)
			if from == nil {
				continue
			}
			if n, ok := groups[from]; ok {
				out[n].FromRanges = append(out[n].FromRanges, s.rangeForURI(doc.uri, ref.span))
			} else {
				groups[from] = len(out)
				out = append(out, lspCallHierarchyIncoming{From: callHierarchyItem(from, s), FromRanges: []lspRange{s.rangeForURI(doc.uri, ref.span)}})
			}
		}
	}
	for i := range out {
		sort.Slice(out[i].FromRanges, func(a, b int) bool { return callHierarchyRangeLess(out[i].FromRanges[a], out[i].FromRanges[b]) })
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From.URI != out[j].From.URI {
			return out[i].From.URI < out[j].From.URI
		}
		if out[i].From.Name != out[j].From.Name {
			return out[i].From.Name < out[j].From.Name
		}
		return callHierarchyRangeLess(out[i].FromRanges[0], out[j].FromRanges[0])
	})
	return out, nil
}

func (s *languageServer) outgoingCalls(item lspCallHierarchyItem) ([]lspCallHierarchyOutgoing, error) {
	return s.outgoingCallsContext(context.Background(), item)
}

func (s *languageServer) outgoingCallsContext(ctx context.Context, item lspCallHierarchyItem) ([]lspCallHierarchyOutgoing, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	caller := s.callHierarchyDefinition(item)
	if caller == nil {
		return []lspCallHierarchyOutgoing{}, nil
	}
	doc := s.document(item.URI)
	if doc == nil || doc.analysis == nil {
		return []lspCallHierarchyOutgoing{}, nil
	}
	groups := make(map[*symbolDef]int)
	var out []lspCallHierarchyOutgoing
	for _, ref := range doc.analysis.refs {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		if ref == nil || ref.target == nil || !callHierarchyCallable(ref.target) || callHierarchyContainingDefinition(doc, ref.span.start) != caller {
			continue
		}
		if n, ok := groups[ref.target]; ok {
			out[n].FromRanges = append(out[n].FromRanges, s.rangeForURI(doc.uri, ref.span))
			continue
		}
		groups[ref.target] = len(out)
		out = append(out, lspCallHierarchyOutgoing{To: callHierarchyItem(ref.target, s), FromRanges: []lspRange{s.rangeForURI(doc.uri, ref.span)}})
	}
	for i := range out {
		sort.Slice(out[i].FromRanges, func(a, b int) bool { return callHierarchyRangeLess(out[i].FromRanges[a], out[i].FromRanges[b]) })
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].To.URI != out[j].To.URI {
			return out[i].To.URI < out[j].To.URI
		}
		if out[i].To.Name != out[j].To.Name {
			return out[i].To.Name < out[j].To.Name
		}
		return callHierarchyRangeLess(out[i].FromRanges[0], out[j].FromRanges[0])
	})
	return out, nil
}

func callHierarchyCallable(def *symbolDef) bool {
	return def != nil && (def.kind == "function" || def.kind == "macro" || def.kind == "trait method")
}

func (s *languageServer) callHierarchyDefinition(item lspCallHierarchyItem) *symbolDef {
	for _, doc := range s.allDocuments() {
		if doc == nil || doc.analysis == nil {
			continue
		}
		for _, def := range doc.analysis.defs {
			if !callHierarchyCallable(def) || def.name != item.Name || def.uri != item.URI {
				continue
			}
			candidate := callHierarchyItem(def, s)
			if item.Data != "" {
				if candidate.Data == item.Data {
					return def
				}
				continue
			}
			if candidate.Range == item.Range {
				return def
			}
		}
	}
	return nil
}

func callHierarchyContainingDefinition(doc *documentState, pos sourcePos) *symbolDef {
	if doc == nil || doc.prog == nil || doc.analysis == nil {
		return nil
	}
	var best *symbolDef
	var bestSpan sourceSpan
	consider := func(fn funcStmt) {
		if !fn.Span.contains(pos) {
			return
		}
		for _, def := range doc.analysis.defs {
			if !callHierarchyCallable(def) || def.name != fn.Name || def.span.start != fn.NameSpan.start {
				continue
			}
			if best == nil || spanWidth(fn.Span) < spanWidth(bestSpan) {
				best = def
				bestSpan = fn.Span
			}
			return
		}
	}
	for _, stmt := range doc.prog.Statements {
		switch st := stmt.(type) {
		case funcStmt:
			consider(st)
		case macroStmt:
			if st.Span.contains(pos) {
				for _, def := range doc.analysis.defs {
					if callHierarchyCallable(def) && def.kind == "macro" && def.name == st.Name && def.span.start == st.NameSpan.start {
						if best == nil || spanWidth(st.Span) < spanWidth(bestSpan) {
							best = def
							bestSpan = st.Span
						}
						break
					}
				}
			}
		case traitStmt:
			for _, method := range st.Methods {
				consider(method)
			}
		case implStmt:
			for _, method := range st.Methods {
				consider(method)
			}
		}
	}
	return best
}

func callHierarchyRangeLess(a, b lspRange) bool {
	if a.Start.Line != b.Start.Line {
		return a.Start.Line < b.Start.Line
	}
	if a.Start.Character != b.Start.Character {
		return a.Start.Character < b.Start.Character
	}
	if a.End.Line != b.End.Line {
		return a.End.Line < b.End.Line
	}
	return a.End.Character < b.End.Character
}

type lspTypeHierarchyItem struct {
	Name           string   `json:"name"`
	Kind           int      `json:"kind"`
	URI            string   `json:"uri"`
	Range          lspRange `json:"range"`
	SelectionRange lspRange `json:"selectionRange"`
	Data           string   `json:"data,omitempty"`
}

func (s *languageServer) prepareTypeHierarchy(uri string, pos lspPosition) ([]lspTypeHierarchyItem, error) {
	return s.prepareTypeHierarchyContext(context.Background(), uri, pos)
}

func (s *languageServer) prepareTypeHierarchyContext(ctx context.Context, uri string, pos lspPosition) ([]lspTypeHierarchyItem, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if _, ok := lspUTF16Offset(doc.text, pos); !ok {
		return nil, lspInvalidParams(fmt.Errorf("invalid UTF-16 position"))
	}
	if doc.analysis == nil {
		return []lspTypeHierarchyItem{}, nil
	}
	ref, def := doc.analysis.findAt(toSourcePosInText(doc.text, pos))
	if ref != nil {
		def = ref.target
	}
	if def == nil || (def.kind != "struct" && def.kind != "enum" && def.kind != "trait" && def.kind != "type alias" && def.kind != "newtype") {
		return []lspTypeHierarchyItem{}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	rng := s.rangeForURI(def.uri, def.span)
	return []lspTypeHierarchyItem{{Name: def.name, Kind: lspSymbolKind(def.kind), URI: def.uri, Range: rng, SelectionRange: rng, Data: def.name}}, nil
}

func (s *languageServer) typeSupertypes(item lspTypeHierarchyItem) ([]lspTypeHierarchyItem, error) {
	return s.typeSupertypesContext(context.Background(), item)
}

func (s *languageServer) typeSupertypesContext(ctx context.Context, item lspTypeHierarchyItem) ([]lspTypeHierarchyItem, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	if item.Kind != lspSymbolKind("struct") && item.Kind != lspSymbolKind("enum") && item.Kind != lspSymbolKind("newtype") {
		return []lspTypeHierarchyItem{}, nil
	}
	target := item.Data
	if target == "" {
		target = item.Name
	}
	seen := map[string]struct{}{}
	var out []lspTypeHierarchyItem
	for _, doc := range s.allDocuments() {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		if doc == nil || doc.prog == nil {
			continue
		}
		for _, stmt := range doc.prog.Statements {
			if err := ctx.Err(); err != nil {
				return nil, lspRequestCancelled(err)
			}
			impl, ok := stmt.(implStmt)
			if !ok || impl.Target != target {
				continue
			}
			def := s.typeDefinitionByNameContext(ctx, impl.Trait)
			if err := ctx.Err(); err != nil {
				return nil, lspRequestCancelled(err)
			}
			if def == nil || def.kind != "trait" || !validSourceSpan(def.span) {
				continue
			}
			key := def.uri + ":" + def.name
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			rng := s.rangeForURI(def.uri, def.span)
			out = append(out, lspTypeHierarchyItem{Name: def.name, Kind: lspSymbolKind(def.kind), URI: def.uri, Range: rng, SelectionRange: rng, Data: def.name})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	return out, nil
}

func (s *languageServer) typeSubtypes(item lspTypeHierarchyItem) ([]lspTypeHierarchyItem, error) {
	return s.typeSubtypesContext(context.Background(), item)
}

func (s *languageServer) typeSubtypesContext(ctx context.Context, item lspTypeHierarchyItem) ([]lspTypeHierarchyItem, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	if item.Kind != lspSymbolKind("trait") {
		return []lspTypeHierarchyItem{}, nil
	}
	target := item.Data
	if target == "" {
		target = item.Name
	}
	seen := map[string]struct{}{}
	var out []lspTypeHierarchyItem
	for _, doc := range s.allDocuments() {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		if doc == nil || doc.prog == nil {
			continue
		}
		for _, stmt := range doc.prog.Statements {
			if err := ctx.Err(); err != nil {
				return nil, lspRequestCancelled(err)
			}
			impl, ok := stmt.(implStmt)
			if !ok || impl.Trait != target {
				continue
			}
			def := s.typeDefinitionByNameContext(ctx, impl.Target)
			if err := ctx.Err(); err != nil {
				return nil, lspRequestCancelled(err)
			}
			if def == nil || !validSourceSpan(def.span) {
				continue
			}
			key := def.uri + ":" + def.name
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			rng := s.rangeForURI(def.uri, def.span)
			out = append(out, lspTypeHierarchyItem{Name: def.name, Kind: lspSymbolKind(def.kind), URI: def.uri, Range: rng, SelectionRange: rng, Data: def.name})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].URI < out[j].URI
	})
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	return out, nil
}

func (s *languageServer) typeDefinitionByName(name string) *symbolDef {
	return s.typeDefinitionByNameContext(context.Background(), name)
}

func (s *languageServer) typeDefinitionByNameContext(ctx context.Context, name string) *symbolDef {
	for _, doc := range s.allDocuments() {
		if ctx != nil && ctx.Err() != nil {
			return nil
		}
		if doc == nil || doc.analysis == nil {
			continue
		}
		for _, def := range doc.analysis.defs {
			if def != nil && def.name == name && (def.kind == "struct" || def.kind == "enum" || def.kind == "trait" || def.kind == "type alias" || def.kind == "newtype") {
				return def
			}
		}
	}
	return nil
}

// lspDocumentSymbol and related wire structs intentionally stay local to the
// server. Their fields mirror the LSP JSON shape without exposing protocol
// concerns through the compiler APIs.
type lspDocumentSymbol struct {
	Name           string   `json:"name"`
	Detail         string   `json:"detail,omitempty"`
	Kind           int      `json:"kind"`
	Range          lspRange `json:"range"`
	SelectionRange lspRange `json:"selectionRange"`
}
type lspWorkspaceSymbol struct {
	Name     string      `json:"name"`
	Kind     int         `json:"kind"`
	Detail   string      `json:"detail,omitempty"`
	Location lspLocation `json:"location"`
}
type lspSemanticTokensResult struct {
	Data []int `json:"data"`
}
type lspSemanticToken struct{ Line, Character, Length, TokenType int }
type lspInlayHint struct {
	Position    lspPosition `json:"position"`
	Label       string      `json:"label"`
	Kind        int         `json:"kind,omitempty"`
	PaddingLeft bool        `json:"paddingLeft,omitempty"`
}
type lspFoldingRange struct {
	StartLine      int    `json:"startLine"`
	StartCharacter int    `json:"startCharacter,omitempty"`
	EndLine        int    `json:"endLine"`
	EndCharacter   int    `json:"endCharacter,omitempty"`
	Kind           string `json:"kind,omitempty"`
}
type lspSelectionRange struct {
	Range  lspRange           `json:"range"`
	Parent *lspSelectionRange `json:"parent,omitempty"`
}
type lspTextEdit struct {
	Range   lspRange `json:"range"`
	NewText string   `json:"newText"`
}

type lspRenameSpanEdit struct {
	span  sourceSpan
	start int
	end   int
}
type lspWorkspaceEdit struct {
	Changes         map[string][]lspTextEdit   `json:"changes,omitempty"`
	DocumentChanges []lspVersionedDocumentEdit `json:"documentChanges,omitempty"`
}

type lspVersionedDocumentEdit struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version int    `json:"version"`
	} `json:"textDocument"`
	Edits []lspTextEdit `json:"edits"`
}

func (e lspWorkspaceEdit) MarshalJSON() ([]byte, error) {
	if len(e.DocumentChanges) > 0 {
		return json.Marshal(struct {
			DocumentChanges []lspVersionedDocumentEdit `json:"documentChanges"`
		}{DocumentChanges: e.DocumentChanges})
	}
	type plain lspWorkspaceEdit
	return json.Marshal(plain(e))
}

func versionedWorkspaceEdit(changes map[string][]lspTextEdit, versions map[string]int) lspWorkspaceEdit {
	return lspWorkspaceEdit{Changes: changes, DocumentChanges: versionedDocumentChanges(changes, versions)}
}

func versionedDocumentChanges(changes map[string][]lspTextEdit, versions map[string]int) []lspVersionedDocumentEdit {
	uris := make([]string, 0, len(changes))
	for uri := range changes {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	out := make([]lspVersionedDocumentEdit, 0, len(uris))
	for _, uri := range uris {
		item := lspVersionedDocumentEdit{Edits: append([]lspTextEdit(nil), changes[uri]...)}
		item.TextDocument.URI = uri
		item.TextDocument.Version = versions[uri]
		out = append(out, item)
	}
	return out
}

type lspCodeAction struct {
	Title       string           `json:"title"`
	Kind        string           `json:"kind,omitempty"`
	Diagnostics []lspDiagnostic  `json:"diagnostics,omitempty"`
	Edit        lspWorkspaceEdit `json:"edit,omitempty"`
}
type lspDiagnosticReport struct {
	Kind  string          `json:"kind"`
	Items []lspDiagnostic `json:"items"`
}
type lspCommand struct {
	Title     string `json:"title"`
	Command   string `json:"command"`
	Arguments []any  `json:"arguments,omitempty"`
}
type lspCodeLens struct {
	Range   lspRange    `json:"range"`
	Command *lspCommand `json:"command,omitempty"`
}

func (s *languageServer) documentForAnalysis(uri string) *documentState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if doc := s.docs[uri]; doc != nil {
		return doc
	}
	return s.workspace[uri]
}

func validSourceSpan(sp sourceSpan) bool {
	return sp.start.line > 0 && sp.start.col > 0 && sp.end.line >= sp.start.line
}

func symbolRangeLess(a, b lspRange, an, bn string) bool {
	if a.Start.Line != b.Start.Line {
		return a.Start.Line < b.Start.Line
	}
	if a.Start.Character != b.Start.Character {
		return a.Start.Character < b.Start.Character
	}
	return an < bn
}

func lspSymbolKind(kind string) int {
	switch kind {
	case "module":
		return 2
	case "function", "macro":
		return 12
	case "variable", "param", "iter":
		return 13
	case "struct":
		return 23
	case "enum":
		return 10
	case "enum case":
		return 22
	case "trait":
		return 11
	case "trait method":
		return 6
	case "type alias", "newtype":
		return 26
	case "package":
		return 4
	default:
		return 13
	}
}

func spanInRange(sp sourceSpan, r lspRange, text string) bool {
	if r == (lspRange{}) {
		return true
	}
	got := toLSPRangeInText(text, sp)
	return got.Start.Line >= r.Start.Line && got.Start.Line <= r.End.Line
}

func validLSPRange(text string, r lspRange) bool {
	start, ok := lspUTF16Offset(text, r.Start)
	if !ok {
		return false
	}
	end, ok := lspUTF16Offset(text, r.End)
	return ok && end >= start
}

func lspPositionLess(a, b lspPosition) bool {
	return a.Line < b.Line || (a.Line == b.Line && a.Character < b.Character)
}

func lspPositionLessOrEqual(a, b lspPosition) bool {
	return a == b || lspPositionLess(a, b)
}

func lspRangesOverlap(a, b lspRange) bool {
	return lspPositionLessOrEqual(a.Start, b.End) && lspPositionLessOrEqual(b.Start, a.End)
}

func lspSpanOverlapsRange(sp sourceSpan, r lspRange, text string) bool {
	return lspRangesOverlap(toLSPRangeInText(text, sp), r)
}

func lspRangeFromOffsets(text string, start, end int) lspRange {
	return lspRange{Start: offsetToLSPPosition(text, start), End: offsetToLSPPosition(text, end)}
}

func offsetToLSPPosition(text string, offset int) lspPosition {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	line, at := 0, 0
	for at < offset {
		if text[at] == '\n' {
			line++
		}
		at++
	}
	lineStart := strings.LastIndexByte(text[:offset], '\n') + 1
	units := 0
	for _, r := range text[lineStart:offset] {
		if r > 0xffff {
			units += 2
		} else {
			units++
		}
	}
	return lspPosition{Line: line, Character: units}
}

func wordBounds(text string, offset int) (int, int) {
	if offset > len(text) {
		offset = len(text)
	}
	start, end := offset, offset
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:start])
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
			break
		}
		start -= size
	}
	for end < len(text) {
		r, size := utf8.DecodeRuneInString(text[end:])
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
			break
		}
		end += size
	}
	return start, end
}

func scanSemanticTokens(text string, analysis *documentAnalysis) []lspSemanticToken {
	tokens, _ := scanSemanticTokensContext(context.Background(), text, analysis)
	return tokens
}

func scanSemanticTokensContext(ctx context.Context, text string, analysis *documentAnalysis) ([]lspSemanticToken, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if analysis == nil {
		return nil, nil
	}
	var out []lspSemanticToken
	lines := strings.Split(text, "\n")
	for lineNo, line := range lines {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		if start, ok := semanticCommentStart(line); ok {
			out = append(out, lspSemanticToken{Line: lineNo, Character: utf16Units(line[:start]), Length: utf16Units(line[start:]), TokenType: 15})
		}
	}

	lex := newLexer(text)
	for {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		tok, err := lex.nextToken()
		if err != nil {
			// Preserve useful tokens from a partially typed document. The
			// parser owns syntax diagnostics; semantic tokens remain a
			// best-effort editor feature while the source is incomplete.
			break
		}
		if tok.kind == tokenEOF {
			break
		}
		if tok.line < 1 || tok.line > len(lines) || tok.col < 1 {
			continue
		}
		line := lines[tok.line-1]
		start := tok.col - 1
		if start > len(line) {
			continue
		}
		raw := tok.raw
		if raw == "" {
			raw = tok.text
		}
		if raw == "" || start+len(raw) > len(line) {
			continue
		}
		tokenType, ok := semanticTokenTypeForToken(tok, analysis, tok.line, start+1)
		if !ok {
			continue
		}
		out = append(out, lspSemanticToken{Line: tok.line - 1, Character: utf16Units(line[:start]), Length: utf16Units(line[start : start+len(raw)]), TokenType: tokenType})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		if out[i].Character != out[j].Character {
			return out[i].Character < out[j].Character
		}
		if out[i].Length != out[j].Length {
			return out[i].Length < out[j].Length
		}
		return out[i].TokenType < out[j].TokenType
	})
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	return out, nil
}

func semanticTokenSpanValid(text string, token lspSemanticToken) bool {
	if token.Line < 0 || token.Character < 0 || token.Length <= 0 || token.TokenType < 0 || token.TokenType >= len(lspSemanticTokenTypes) {
		return false
	}
	lines := strings.Split(text, "\n")
	if token.Line >= len(lines) {
		return false
	}
	line := lines[token.Line]
	start, ok := utf16ByteOffset(line, token.Character)
	if !ok {
		return false
	}
	end, ok := utf16ByteOffset(line, token.Character+token.Length)
	return ok && end > start
}

func utf16ByteOffset(line string, units int) (int, bool) {
	if units < 0 || !utf8.ValidString(line) {
		return 0, false
	}
	used, at := 0, 0
	for at < len(line) {
		if used == units {
			return at, true
		}
		r, size := utf8.DecodeRuneInString(line[at:])
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units < used+width {
			return 0, false
		}
		used += width
		at += size
	}
	return at, used == units
}

func semanticCommentStart(line string) (int, bool) {
	inString, escaped := false, false
	for i, r := range line {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				inString = false
			}
			continue
		}
		if r == '"' {
			inString = true
			continue
		}
		if r == '#' {
			return i, true
		}
		if r == '/' && i+1 < len(line) && line[i+1] == '/' {
			return i, true
		}
	}
	return 0, false
}

func semanticTokenTypeForToken(tok token, analysis *documentAnalysis, line, col int) (int, bool) {
	switch tok.kind {
	case tokenIdent:
		for _, def := range analysis.defs {
			if def != nil && def.span.start.line == line && def.span.start.col == col && def.name == tok.text {
				return semanticTokenType(def.kind), true
			}
		}
		for _, ref := range analysis.refs {
			if ref == nil || ref.span.start.line != line || ref.span.start.col != col || ref.target == nil {
				continue
			}
			return semanticTokenType(ref.target.kind), true
		}
		return 8, true
	case tokenString, tokenTag:
		return 16, true
	case tokenNumber:
		return 17, true
	case tokenLet, tokenVar, tokenConst, tokenEmit, tokenMacro, tokenFunc,
		tokenReturn, tokenTry, tokenCatch, tokenWhile, tokenFor, tokenIn,
		tokenBreak, tokenContinue, tokenDefer, tokenPackage, tokenExport,
		tokenEnum, tokenStruct, tokenType, tokenNewtype, tokenImport, tokenFFI,
		tokenMatch, tokenSwitch, tokenAs, tokenWith, tokenTrue, tokenFalse, tokenNull:
		return 13, true
	case tokenDot, tokenComma, tokenColon, tokenSemicolon, tokenLParen, tokenRParen,
		tokenLBrace, tokenRBrace, tokenLBracket, tokenRBracket:
		return 18, false
	default:
		return 18, true
	}
}

func semanticTokenType(kind string) int {
	switch kind {
	case "module", "package":
		return 0
	case "type alias", "newtype":
		return 1
	case "struct":
		return 5
	case "enum":
		return 3
	case "trait":
		return 4
	case "function":
		return 10
	case "trait method":
		return 11
	case "macro":
		return 12
	case "param":
		return 7
	case "builtin":
		return 10
	case "variable", "iter":
		return 8
	case "enum case":
		return 2
	default:
		return 8
	}
}
func utf16Units(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xffff {
			n += 2
		} else {
			n++
		}
	}
	return n
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
