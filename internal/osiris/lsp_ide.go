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
		"definitionProvider":         true,
		"typeDefinitionProvider":     true,
		"implementationProvider":     true,
		"referencesProvider":         true,
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
		v, err := s.documentSymbols(p.TextDocument.URI)
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
		var p lspTextDocumentParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.typeDefinition(p.TextDocument.URI, p.Position)
		return v, true, err
	case "textDocument/implementation":
		var p lspTextDocumentParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.implementations(p.TextDocument.URI, p.Position)
		return v, true, err
	case "textDocument/semanticTokens/full":
		var p lspTextDocumentParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.semanticTokens(p.TextDocument.URI)
		return v, true, err
	case "textDocument/inlayHint":
		var p lspRangeParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.inlayHints(p.TextDocument.URI, p.Range)
		return v, true, err
	case "textDocument/foldingRange":
		var p lspTextDocumentParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.foldingRanges(p.TextDocument.URI)
		return v, true, err
	case "textDocument/selectionRange":
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version *int   `json:"version,omitempty"`
			} `json:"textDocument"`
			Positions []lspPosition `json:"positions"`
		}
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.selectionRanges(p.TextDocument.URI, p.Positions)
		return v, true, err
	case "textDocument/codeAction":
		var p lspCodeActionParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		v, err := s.codeActionsAtVersion(p.TextDocument.URI, p.Range, p.Context.Diagnostics, p.TextDocument.Version)
		return v, true, err
	case "textDocument/formatting":
		var p lspTextDocumentParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		v, err := s.formattingAtVersion(p.TextDocument.URI, p.TextDocument.Version)
		return v, true, err
	case "textDocument/diagnostic":
		var p lspTextDocumentParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.pullDiagnostics(p.TextDocument.URI)
		return v, true, err
	case "textDocument/codeLens":
		var p lspTextDocumentParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.codeLenses(p.TextDocument.URI)
		return v, true, err
	case "textDocument/prepareRename":
		var p lspTextDocumentParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.prepareRename(p.TextDocument.URI, p.Position)
		return v, true, err
	case "callHierarchy/prepare":
		var p lspTextDocumentParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.prepareCallHierarchy(p.TextDocument.URI, p.Position)
		return v, true, err
	case "callHierarchy/incomingCalls":
		var p struct {
			Item lspCallHierarchyItem `json:"item"`
		}
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		v, err := s.incomingCalls(p.Item)
		return v, true, err
	case "callHierarchy/outgoingCalls":
		var p struct {
			Item lspCallHierarchyItem `json:"item"`
		}
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		v, err := s.outgoingCalls(p.Item)
		return v, true, err
	case "typeHierarchy/prepare":
		var p lspTextDocumentParams
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		if err := checkVersion(p.TextDocument.URI, p.TextDocument.Version); err != nil {
			return nil, true, err
		}
		v, err := s.prepareTypeHierarchy(p.TextDocument.URI, p.Position)
		return v, true, err
	case "typeHierarchy/supertypes":
		var p struct {
			Item lspTypeHierarchyItem `json:"item"`
		}
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		v, err := s.typeSupertypes(p.Item)
		return v, true, err
	case "typeHierarchy/subtypes":
		var p struct {
			Item lspTypeHierarchyItem `json:"item"`
		}
		if err := decode(&p); err != nil {
			return nil, true, lspInvalidParams(err)
		}
		v, err := s.typeSubtypes(p.Item)
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
		v, err := s.testInventory(p.TextDocument.URI)
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

type lspRangeParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int   `json:"version,omitempty"`
	} `json:"textDocument"`
	Range lspRange `json:"range"`
}

type lspCodeActionParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int   `json:"version,omitempty"`
	} `json:"textDocument"`
	Range   lspRange `json:"range"`
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

func (s *languageServer) refreshWorkspace(ctx context.Context) {
	s.mu.RLock()
	roots := append([]string(nil), s.workspaceRoots...)
	if len(roots) == 0 && s.workspaceRoot != "" {
		roots = []string{s.workspaceRoot}
	}
	s.mu.RUnlock()
	if len(roots) == 0 || ctx == nil {
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
				state.diagnostics = diagnosticsInText(state.text, state.analysis.diagnostics)
			}
			indexed[uri] = state
			files++
			return nil
		})
		if walkErr != nil && !errors.Is(walkErr, context.Canceled) && !errors.Is(walkErr, context.DeadlineExceeded) {
			continue
		}
		if ctx.Err() != nil {
			return
		}
	}
	if ctx.Err() != nil {
		return
	}
	s.mu.Lock()
	for uri := range s.docs {
		delete(indexed, uri)
	}
	s.workspace = indexed
	s.mu.Unlock()
}

func (s *languageServer) documentSymbols(uri string) ([]lspDocumentSymbol, error) {
	if uri == "" {
		return nil, lspInvalidParams(fmt.Errorf("textDocument.uri is required"))
	}
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if doc.analysis == nil {
		return []lspDocumentSymbol{}, nil
	}
	out := make([]lspDocumentSymbol, 0, len(doc.analysis.defs))
	for _, def := range doc.analysis.defs {
		if def == nil || def.uri != uri || !validSourceSpan(def.span) {
			continue
		}
		out = append(out, lspDocumentSymbol{Name: def.name, Detail: def.detail, Kind: lspSymbolKind(def.kind), Range: s.rangeForURI(uri, def.span), SelectionRange: s.rangeForURI(uri, def.span)})
	}
	sort.SliceStable(out, func(i, j int) bool { return symbolRangeLess(out[i].Range, out[j].Range, out[i].Name, out[j].Name) })
	return out, nil
}

func (s *languageServer) workspaceSymbols(ctx context.Context, query string) ([]lspWorkspaceSymbol, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(s.allDocuments()) == 0 {
		s.refreshWorkspace(ctx)
	}
	query = strings.ToLower(query)
	out := make([]lspWorkspaceSymbol, 0)
	for _, doc := range s.allDocuments() {
		if doc == nil || doc.analysis == nil {
			continue
		}
		for _, def := range doc.analysis.defs {
			if def == nil || !validSourceSpan(def.span) || (query != "" && !strings.Contains(strings.ToLower(def.name), query)) {
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
	return out, nil
}

func (s *languageServer) typeDefinition(uri string, pos lspPosition) (any, error) {
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if doc.analysis == nil {
		return nil, nil
	}
	ref, def := doc.analysis.findAt(toSourcePosInText(doc.text, pos))
	if ref != nil {
		def = ref.target
	}
	if def == nil || def.typeName == "" {
		return nil, nil
	}
	for _, candidate := range s.allDocuments() {
		if candidate == nil || candidate.analysis == nil {
			continue
		}
		for _, item := range candidate.analysis.defs {
			if item.name == def.typeName && validSourceSpan(item.span) {
				return lspLocation{URI: item.uri, Range: s.rangeForURI(item.uri, item.span)}, nil
			}
		}
	}
	return nil, nil
}

func (s *languageServer) implementations(uri string, pos lspPosition) ([]lspLocation, error) {
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if doc.analysis == nil {
		return []lspLocation{}, nil
	}
	ref, def := doc.analysis.findAt(toSourcePosInText(doc.text, pos))
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
	seen := map[string]struct{}{}
	var out []lspLocation
	for _, candidate := range s.allDocuments() {
		if candidate == nil || candidate.analysis == nil {
			continue
		}
		for _, item := range candidate.analysis.defs {
			if item == nil || !validSourceSpan(item.span) || item.name != name {
				continue
			}
			if item.kind != "function" && item.kind != "trait method" {
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
	sort.Slice(out, func(i, j int) bool {
		if out[i].URI != out[j].URI {
			return out[i].URI < out[j].URI
		}
		return out[i].Range.Start.Line < out[j].Range.Start.Line
	})
	return out, nil
}

func (s *languageServer) semanticTokens(uri string) (lspSemanticTokensResult, error) {
	doc := s.document(uri)
	if doc == nil {
		return lspSemanticTokensResult{}, fmt.Errorf("document not open: %s", uri)
	}
	tokens := scanSemanticTokens(doc.text, doc.analysis)
	if len(tokens) > lspMaxSemanticTokens {
		tokens = tokens[:lspMaxSemanticTokens]
	}
	data := make([]int, 0, len(tokens)*5)
	lastLine, lastChar := 0, 0
	for _, token := range tokens {
		lineDelta := token.Line - lastLine
		charDelta := token.Character
		if lineDelta == 0 {
			charDelta -= lastChar
		}
		data = append(data, lineDelta, charDelta, token.Length, token.TokenType, 0)
		lastLine, lastChar = token.Line, token.Character
	}
	return lspSemanticTokensResult{Data: data}, nil
}

func (s *languageServer) inlayHints(uri string, r lspRange) ([]lspInlayHint, error) {
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if doc.analysis == nil || doc.analysis.checked == nil {
		return []lspInlayHint{}, nil
	}
	var out []lspInlayHint
	for _, binding := range doc.analysis.checked.Bindings {
		if binding.Type == "" || !spanInRange(binding.Span, r, doc.text) {
			continue
		}
		pos := s.rangeForURI(uri, binding.Span).End
		out = append(out, lspInlayHint{Position: pos, Label: []string{" : " + binding.Type}, Kind: 1, PaddingLeft: true})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Position.Line != out[j].Position.Line {
			return out[i].Position.Line < out[j].Position.Line
		}
		return out[i].Position.Character < out[j].Position.Character
	})
	return out, nil
}

func (s *languageServer) foldingRanges(uri string) ([]lspFoldingRange, error) {
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	var out []lspFoldingRange
	if doc.analysis == nil {
		return out, nil
	}
	for _, def := range doc.analysis.defs {
		r := s.rangeForURI(uri, def.span)
		if r.End.Line > r.Start.Line {
			out = append(out, lspFoldingRange{StartLine: r.Start.Line, StartCharacter: r.Start.Character, EndLine: r.End.Line, EndCharacter: r.End.Character, Kind: "region"})
		}
	}
	lines := strings.Split(doc.text, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "//") && i+1 < len(lines) {
			out = append(out, lspFoldingRange{StartLine: i, EndLine: i, Kind: "comment"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartLine < out[j].StartLine })
	return out, nil
}

func (s *languageServer) selectionRanges(uri string, positions []lspPosition) ([]lspSelectionRange, error) {
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	out := make([]lspSelectionRange, 0, len(positions))
	for _, pos := range positions {
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
		out = append(out, lspSelectionRange{Range: word, Parent: &lspSelectionRange{Range: line, Parent: &lspSelectionRange{Range: root}}})
	}
	return out, nil
}

func (s *languageServer) codeActions(uri string, r lspRange, diagnostics []lspDiagnostic) ([]lspCodeAction, error) {
	return s.codeActionsAtVersion(uri, r, diagnostics, nil)
}

func (s *languageServer) codeActionsAtVersion(uri string, r lspRange, diagnostics []lspDiagnostic, expectedVersion *int) ([]lspCodeAction, error) {
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if err := s.checkDocumentVersion(doc, expectedVersion); err != nil {
		return nil, err
	}
	var out []lspCodeAction
	for _, diagnostic := range diagnostics {
		if strings.Contains(strings.ToLower(diagnostic.Message), "deprecated") {
			out = append(out, lspCodeAction{Title: "Migrate deprecated API", Kind: "quickfix", Diagnostics: []lspDiagnostic{diagnostic}})
		}
	}
	if edits, err := organizeImportEdits(doc); err != nil {
		return nil, err
	} else if len(edits) > 0 {
		out = append(out, lspCodeAction{
			Title: "Organize Osiris imports",
			Kind:  "source.organizeImports",
			Edit:  versionedWorkspaceEdit(map[string][]lspTextEdit{uri: edits}, map[string]int{uri: doc.version}),
		})
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
	return edits, nil
}

func (s *languageServer) formatting(uri string) ([]lspTextEdit, error) {
	return s.formattingAtVersion(uri, nil)
}

func (s *languageServer) formattingAtVersion(uri string, expectedVersion *int) ([]lspTextEdit, error) {
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
	if formatted == doc.text {
		return []lspTextEdit{}, nil
	}
	return []lspTextEdit{{Range: fullDocumentRange(doc.text), NewText: formatted}}, nil
}

func (s *languageServer) pullDiagnostics(uri string) (lspDiagnosticReport, error) {
	doc := s.document(uri)
	if doc == nil {
		return lspDiagnosticReport{}, fmt.Errorf("document not open: %s", uri)
	}
	return lspDiagnosticReport{Kind: "full", Items: doc.diagnostics}, nil
}

func (s *languageServer) codeLenses(uri string) ([]lspCodeLens, error) {
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	entries, err := parseLanguageTestFile(doc.sourceName, doc.text)
	if err != nil {
		return []lspCodeLens{}, nil
	}
	var out []lspCodeLens
	for _, entry := range entries {
		command := lspCommand{Title: "Run " + entry.name, Command: "osiris.test", Arguments: []any{entry.file, entry.name}}
		if entry.benchmark != nil {
			command = lspCommand{Title: "Benchmark " + entry.name, Command: "osiris.benchmark", Arguments: []any{entry.file, entry.name}}
		}
		location := entry.location
		rng := lspRange{Start: lspPosition{Line: maxInt(0, location.Start.Line-1), Character: maxInt(0, location.Start.Column-1)}, End: lspPosition{Line: maxInt(0, location.End.Line-1), Character: maxInt(0, location.End.Column-1)}}
		out = append(out, lspCodeLens{Range: rng, Command: &command})
	}
	return out, nil
}

func (s *languageServer) testInventory(uri string) (LanguageTestInventory, error) {
	doc := s.document(uri)
	if doc == nil {
		return LanguageTestInventory{}, fmt.Errorf("document not open: %s", uri)
	}
	file, source := doc.sourceName, doc.text
	if !isLanguageTestFile(file) {
		if companion := colocatedTestFile(file); companion != "" {
			data, err := os.ReadFile(companion)
			if err != nil {
				return LanguageTestInventory{}, err
			}
			file, source = companion, string(data)
		}
	}
	entries, err := parseLanguageTestFile(file, source)
	if err != nil {
		return LanguageTestInventory{}, err
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
	if manifest, err := LoadModuleManifest(ModuleRootFor(doc.sourceName)); err == nil {
		out.Module = manifest.Module
	}
	for _, entry := range entries {
		out.Tests = append(out.Tests, LanguageTestEntry{Name: entry.name, File: entry.file, Location: entry.location, Skip: entry.skipReason != "", ExpectedFailure: entry.expectedFailure, Benchmark: entry.benchmark != nil, Fuzz: entry.fuzz != nil})
	}
	return out, nil
}

func (s *languageServer) prepareRename(uri string, pos lspPosition) (any, error) {
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if doc.analysis == nil {
		return nil, nil
	}
	ref, def := doc.analysis.findAt(toSourcePosInText(doc.text, pos))
	if ref != nil {
		def = ref.target
	}
	if def == nil || def.uri == "" || !validIdentifier(def.name) {
		return nil, nil
	}
	return s.rangeForURI(uri, def.span), nil
}

func (s *languageServer) renameWorkspaceEdit(uri string, pos lspPosition, replacement string) (lspWorkspaceEdit, error) {
	return s.renameWorkspaceEditAtVersion(uri, pos, replacement, nil)
}

func (s *languageServer) renameWorkspaceEditAtVersion(uri string, pos lspPosition, replacement string, expectedVersion *int) (lspWorkspaceEdit, error) {
	if !validIdentifier(replacement) {
		return lspWorkspaceEdit{}, lspInvalidParams(fmt.Errorf("invalid replacement identifier %q", replacement))
	}
	doc := s.document(uri)
	if doc == nil || doc.analysis == nil {
		return lspWorkspaceEdit{}, fmt.Errorf("document not open: %s", uri)
	}
	if err := s.checkDocumentVersion(doc, expectedVersion); err != nil {
		return lspWorkspaceEdit{}, err
	}
	ref, target := doc.analysis.findAt(toSourcePosInText(doc.text, pos))
	if ref != nil {
		target = ref.target
	}
	if target == nil || target.uri == "" {
		return lspWorkspaceEdit{}, fmt.Errorf("no renameable symbol at position")
	}
	type spanEdit struct {
		span sourceSpan
		text string
	}
	byURI := make(map[string][]spanEdit)
	for _, candidateDoc := range s.allDocuments() {
		if candidateDoc == nil || candidateDoc.analysis == nil {
			continue
		}
		for _, candidate := range candidateDoc.analysis.defs {
			if candidate == nil || candidate.uri != candidateDoc.uri {
				continue
			}
			if candidate.uri == target.uri && candidate.span == target.span {
				byURI[candidateDoc.uri] = append(byURI[candidateDoc.uri], spanEdit{span: candidate.span, text: candidateDoc.text})
			}
		}
		for _, candidate := range candidateDoc.analysis.refs {
			if candidate == nil || candidate.target == nil {
				continue
			}
			if candidate.target == target {
				byURI[candidateDoc.uri] = append(byURI[candidateDoc.uri], spanEdit{span: candidate.span, text: candidateDoc.text})
			}
		}
	}
	if len(byURI) == 0 {
		return lspWorkspaceEdit{}, fmt.Errorf("no renameable references found")
	}
	edit := lspWorkspaceEdit{Changes: make(map[string][]lspTextEdit, len(byURI))}
	for targetURI, spans := range byURI {
		candidateDoc := s.document(targetURI)
		if candidateDoc == nil {
			for _, indexed := range s.allDocuments() {
				if indexed != nil && indexed.uri == targetURI {
					candidateDoc = indexed
					break
				}
			}
		}
		if candidateDoc == nil {
			continue
		}
		seen := map[string]struct{}{}
		for _, item := range spans {
			rng := s.rangeForURI(targetURI, item.span)
			key := fmt.Sprintf("%d:%d:%d:%d", rng.Start.Line, rng.Start.Character, rng.End.Line, rng.End.Character)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
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
		if candidate := s.document(targetURI); candidate != nil {
			versions[targetURI] = candidate.version
			continue
		}
		for _, indexed := range s.allDocuments() {
			if indexed != nil && indexed.uri == targetURI {
				versions[targetURI] = indexed.version
				break
			}
		}
	}
	edit.DocumentChanges = versionedDocumentChanges(edit.Changes, versions)
	return edit, nil
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
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if doc.analysis == nil {
		return []lspCallHierarchyItem{}, nil
	}
	ref, def := doc.analysis.findAt(toSourcePosInText(doc.text, pos))
	if ref != nil {
		def = ref.target
	}
	if def == nil || (def.kind != "function" && def.kind != "macro" && def.kind != "trait method") {
		return []lspCallHierarchyItem{}, nil
	}
	return []lspCallHierarchyItem{callHierarchyItem(def, s)}, nil
}

func (s *languageServer) incomingCalls(item lspCallHierarchyItem) ([]lspCallHierarchyIncoming, error) {
	var out []lspCallHierarchyIncoming
	for _, doc := range s.allDocuments() {
		if doc == nil || doc.analysis == nil {
			continue
		}
		for _, ref := range doc.analysis.refs {
			if ref == nil || ref.target == nil || ref.target.name != item.Name || (ref.target.kind != "function" && ref.target.kind != "trait method") {
				continue
			}
			from := item
			for _, def := range doc.analysis.defs {
				if def.span.contains(ref.span.start) && def.kind == "function" {
					from = callHierarchyItem(def, s)
					break
				}
			}
			out = append(out, lspCallHierarchyIncoming{From: from, FromRanges: []lspRange{s.rangeForURI(doc.uri, ref.span)}})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From.URI < out[j].From.URI })
	return out, nil
}

func (s *languageServer) outgoingCalls(item lspCallHierarchyItem) ([]lspCallHierarchyOutgoing, error) {
	doc := s.document(item.URI)
	if doc == nil {
		return []lspCallHierarchyOutgoing{}, nil
	}
	var out []lspCallHierarchyOutgoing
	seen := map[string]int{}
	for _, ref := range doc.analysis.refs {
		if ref == nil || ref.target == nil || (ref.target.kind != "function" && ref.target.kind != "trait method") {
			continue
		}
		if n, ok := seen[ref.target.name]; ok {
			out[n].FromRanges = append(out[n].FromRanges, s.rangeForURI(doc.uri, ref.span))
			continue
		}
		seen[ref.target.name] = len(out)
		out = append(out, lspCallHierarchyOutgoing{To: callHierarchyItem(ref.target, s), FromRanges: []lspRange{s.rangeForURI(doc.uri, ref.span)}})
	}
	return out, nil
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
	doc := s.document(uri)
	if doc == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
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
	rng := s.rangeForURI(def.uri, def.span)
	return []lspTypeHierarchyItem{{Name: def.name, Kind: lspSymbolKind(def.kind), URI: def.uri, Range: rng, SelectionRange: rng, Data: def.name}}, nil
}

func (s *languageServer) typeSupertypes(item lspTypeHierarchyItem) ([]lspTypeHierarchyItem, error) {
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
		if doc == nil || doc.prog == nil {
			continue
		}
		for _, stmt := range doc.prog.Statements {
			impl, ok := stmt.(implStmt)
			if !ok || impl.Target != target {
				continue
			}
			def := s.typeDefinitionByName(impl.Trait)
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
	return out, nil
}

func (s *languageServer) typeSubtypes(item lspTypeHierarchyItem) ([]lspTypeHierarchyItem, error) {
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
		if doc == nil || doc.prog == nil {
			continue
		}
		for _, stmt := range doc.prog.Statements {
			impl, ok := stmt.(implStmt)
			if !ok || impl.Trait != target {
				continue
			}
			def := s.typeDefinitionByName(impl.Target)
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
	return out, nil
}

func (s *languageServer) typeDefinitionByName(name string) *symbolDef {
	for _, doc := range s.allDocuments() {
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
	Label       []string    `json:"label"`
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

func (s *languageServer) documentForAnalysis(uri string) *documentState { return s.document(uri) }

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
	if analysis == nil {
		return nil
	}
	var out []lspSemanticToken
	lines := strings.Split(text, "\n")
	for lineNo, line := range lines {
		for i := 0; i < len(line); {
			if line[i] == '#' || (line[i] == '/' && i+1 < len(line) && line[i+1] == '/') {
				out = append(out, lspSemanticToken{Line: lineNo, Character: utf16Units(line[:i]), Length: utf16Units(line[i:]), TokenType: 15})
				break
			}
			r, size := utf8.DecodeRuneInString(line[i:])
			if r == '"' {
				start := i
				i += size
				for i < len(line) {
					rr, ss := utf8.DecodeRuneInString(line[i:])
					i += ss
					if rr == '"' {
						break
					}
				}
				out = append(out, lspSemanticToken{Line: lineNo, Character: utf16Units(line[:start]), Length: utf16Units(line[start:i]), TokenType: 16})
				continue
			}
			if unicode.IsLetter(r) || r == '_' {
				start := i
				i += size
				for i < len(line) {
					rr, ss := utf8.DecodeRuneInString(line[i:])
					if !(unicode.IsLetter(rr) || unicode.IsDigit(rr) || rr == '_') {
						break
					}
					i += ss
				}
				name := line[start:i]
				typ := 8
				for _, def := range analysis.defs {
					if def.name == name {
						typ = semanticTokenType(def.kind)
						break
					}
				}
				for _, ref := range analysis.refs {
					if ref.span.start.line == lineNo+1 && ref.span.start.col > start && ref.target != nil && ref.target.name == name {
						typ = semanticTokenType(ref.target.kind)
						break
					}
				}
				out = append(out, lspSemanticToken{Line: lineNo, Character: utf16Units(line[:start]), Length: utf16Units(line[start:i]), TokenType: typ})
				continue
			}
			if unicode.IsDigit(r) {
				start := i
				i += size
				for i < len(line) {
					rr, ss := utf8.DecodeRuneInString(line[i:])
					if !(unicode.IsDigit(rr) || rr == '.' || rr == '_') {
						break
					}
					i += ss
				}
				out = append(out, lspSemanticToken{Line: lineNo, Character: utf16Units(line[:start]), Length: utf16Units(line[start:i]), TokenType: 17})
				continue
			}
			i += size
		}
	}
	return out
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
