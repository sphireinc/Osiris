package osiris

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxLSPMessageBytes bounds one framed JSON-RPC payload before allocation.
// Editors should send incremental changes; a larger document update is a
// protocol/resource error rather than an opportunity to exhaust the server.
const maxLSPMessageBytes = 16 << 20

type languageServer struct {
	rt     *Runtime
	out    io.Writer
	log    io.Writer
	docs   map[string]*documentState
	closed bool
}

type documentState struct {
	uri         string
	sourceName  string
	text        string
	prog        *Program
	diagnostics []lspDiagnostic
	analysis    *documentAnalysis
}

type documentAnalysis struct {
	diagnostics []lspDiagnostic
	defs        []*symbolDef
	refs        []*symbolRef
	enumCases   map[string]*symbolDef
}

// findAt operates on documentAnalysis values and keeps the
// documentanalysis-specific behavior attached to the receiver so callers can
// use the method as the natural entry point for that data structure.
//
// It returns *symbolRef, *symbolDef so callers can continue with the next
// stage of the pipeline without re-deriving the same information.
func (a *documentAnalysis) findAt(pos sourcePos) (*symbolRef, *symbolDef) {
	var bestRef *symbolRef
	for _, ref := range a.refs {
		if ref.span.contains(pos) {
			if bestRef == nil || spanWidth(ref.span) < spanWidth(bestRef.span) {
				bestRef = ref
			}
		}
	}
	var bestDef *symbolDef
	for _, def := range a.defs {
		if def.span.contains(pos) {
			if bestDef == nil || spanWidth(def.span) < spanWidth(bestDef.span) {
				bestDef = def
			}
		}
	}
	if bestRef != nil {
		return bestRef, bestRef.target
	}
	return nil, bestDef
}

// spanWidth handles the span width step in the surrounding package and returns
// int.
//
// It returns int so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func spanWidth(sp sourceSpan) int {
	if sp.start.line == 0 && sp.start.col == 0 && sp.end.line == 0 && sp.end.col == 0 {
		return 1 << 30
	}
	if sp.start.line == sp.end.line {
		return sp.end.col - sp.start.col
	}
	return (sp.end.line-sp.start.line)*1_000_000 + sp.end.col - sp.start.col
}

type symbolDef struct {
	name   string
	kind   string
	detail string
	uri    string
	span   sourceSpan
}

type symbolRef struct {
	span   sourceSpan
	target *symbolDef
	detail string
}

type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

type lspLocation struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

type lspDiagnostic struct {
	Range    lspRange `json:"range"`
	Severity int      `json:"severity,omitempty"`
	Source   string   `json:"source,omitempty"`
	Message  string   `json:"message"`
}

type lspHover struct {
	Contents lspMarkupContent `json:"contents"`
	Range    *lspRange        `json:"range,omitempty"`
}

type lspMarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// NewLanguageServer constructs a new language server and wires together the
// internal caches, registries, and default tables that the caller would
// otherwise have to assemble by hand.
//
// It returns *languageServer so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func NewLanguageServer(rt *Runtime, out, log io.Writer) *languageServer {
	if rt == nil {
		rt = NewRuntime()
	}
	if out == nil {
		out = io.Discard
	}
	if log == nil {
		log = io.Discard
	}
	return &languageServer{rt: rt, out: out, log: log, docs: make(map[string]*documentState)}
}

// Serve operates on languageServer values and keeps the
// languageserver-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns an error when the operation cannot complete cleanly.
func (s *languageServer) Serve(ctx context.Context, in io.Reader) error {
	r := bufio.NewReader(in)

messageLoop:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		msg, err := readLSPMessage(r)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if msg.Method == "" {
			continue
		}
		switch msg.Method {
		case "exit":
			return nil
		case "initialized":
			continue
		case "shutdown":
			s.closed = true
			if err := s.writeResponse(msg.ID, nil, nil); err != nil {
				return err
			}
			continue
		case "initialize":
			if err := s.writeResponse(msg.ID, map[string]any{
				"capabilities": map[string]any{
					// 2 is TextDocumentSyncKind.Incremental. Every edit is
					// applied against the current document with UTF-16 positions.
					"textDocumentSync":      map[string]any{"openClose": true, "change": 2},
					"hoverProvider":         true,
					"definitionProvider":    true,
					"referencesProvider":    true,
					"completionProvider":    map[string]any{"triggerCharacters": []string{".", "("}},
					"signatureHelpProvider": map[string]any{"triggerCharacters": []string{"(", ","}},
					"renameProvider":        true,
				},
			}, nil); err != nil {
				return err
			}
			continue
		}

		switch msg.Method {
		case "textDocument/didOpen":
			var params didOpenParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				return err
			}
			s.upsertDocument(params.TextDocument.URI, params.TextDocument.Text)
			continue
		case "textDocument/didChange":
			var params didChangeParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				return err
			}
			state := s.docs[params.TextDocument.URI]
			text := ""
			if state != nil {
				text = state.text
			}
			for _, change := range params.ContentChanges {
				var applyErr error
				text, applyErr = applyLSPChange(text, change.Range, change.Text)
				if applyErr != nil {
					// didChange is a notification, so there is no response in
					// which to report a bad client range. Preserve the prior
					// document rather than corrupting it or taking down the server.
					_, _ = fmt.Fprintf(s.log, "osiris lsp: ignored invalid didChange for %s: %v\n", params.TextDocument.URI, applyErr)
					continue messageLoop
				}
			}
			s.upsertDocument(params.TextDocument.URI, text)
			continue
		case "textDocument/didClose":
			var params didCloseParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				return err
			}
			delete(s.docs, params.TextDocument.URI)
			if err := s.publishDiagnostics(params.TextDocument.URI, nil); err != nil {
				return err
			}
			continue
		}
		if !msg.IsRequest() {
			continue
		}
		switch msg.Method {
		case "textDocument/hover":
			var params hoverParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				return err
			}
			resp, err := s.hover(params.TextDocument.URI, params.Position)
			if err != nil {
				return s.writeResponse(msg.ID, nil, err)
			}
			if err := s.writeResponse(msg.ID, resp, nil); err != nil {
				return err
			}
		case "textDocument/definition":
			var params definitionParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				return err
			}
			resp, err := s.definition(params.TextDocument.URI, params.Position)
			if err != nil {
				return s.writeResponse(msg.ID, nil, err)
			}
			if err := s.writeResponse(msg.ID, resp, nil); err != nil {
				return err
			}
		case "textDocument/references":
			var params referencesParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				return err
			}
			resp, err := s.references(params.TextDocument.URI, params.Position)
			if err != nil {
				return s.writeResponse(msg.ID, nil, err)
			}
			if err := s.writeResponse(msg.ID, resp, nil); err != nil {
				return err
			}
		case "textDocument/completion":
			var params completionParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				return err
			}
			resp, err := s.completion(params.TextDocument.URI, params.Position)
			if err != nil {
				return s.writeResponse(msg.ID, nil, err)
			}
			if err := s.writeResponse(msg.ID, resp, nil); err != nil {
				return err
			}
		case "textDocument/signatureHelp":
			var params signatureHelpParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				return err
			}
			resp, err := s.signatureHelp(params.TextDocument.URI, params.Position)
			if err != nil {
				return s.writeResponse(msg.ID, nil, err)
			}
			if err := s.writeResponse(msg.ID, resp, nil); err != nil {
				return err
			}
		case "textDocument/rename":
			var params renameParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				return err
			}
			rewritten, err := s.RenameDocument(params.TextDocument.URI, params.Position, params.NewName)
			if err != nil {
				return s.writeResponse(msg.ID, nil, err)
			}
			state := s.docs[params.TextDocument.URI]
			edit := map[string]any{"changes": map[string]any{params.TextDocument.URI: []any{map[string]any{"range": fullDocumentRange(state.text), "newText": rewritten}}}}
			if err := s.writeResponse(msg.ID, edit, nil); err != nil {
				return err
			}
		}
	}
}

// upsertDocument operates on languageServer values and keeps the
// languageserver-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (s *languageServer) upsertDocument(uri, text string) {
	state := &documentState{uri: uri, sourceName: uri, text: text}
	if path, err := uriToPath(uri); err == nil {
		state.sourceName = path
	}
	prog, err := Parse(state.sourceName, text)
	if err != nil {
		state.diagnostics = []lspDiagnostic{{Range: lspRange{}, Severity: 1, Source: "osiris", Message: err.Error()}}
	} else {
		state.prog = prog
		state.analysis = analyzeDocument(s.rt, uri, prog)
		state.diagnostics = diagnosticsInText(text, state.analysis.diagnostics)
	}
	s.docs[uri] = state
	_ = s.publishDiagnostics(uri, state.diagnostics)
}

// rangeForURI converts parser rune columns to the UTF-16 positions required by
// LSP. Definitions can point at a different open document, whose text must be
// used for the conversion.
func (s *languageServer) rangeForURI(uri string, sp sourceSpan) lspRange {
	if target := s.docs[uri]; target != nil {
		return toLSPRangeInText(target.text, sp)
	}
	return toLSPRange(sp)
}

// hover operates on languageServer values and keeps the
// languageserver-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns *lspHover and an error. Callers should only trust the value when
// the error is nil.
func (s *languageServer) hover(uri string, pos lspPosition) (*lspHover, error) {
	state := s.docs[uri]
	if state == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if state.analysis == nil {
		return nil, nil
	}
	ref, def := state.analysis.findAt(toSourcePosInText(state.text, pos))
	if ref == nil && def == nil {
		return nil, nil
	}
	var content strings.Builder
	if ref != nil && ref.detail != "" {
		content.WriteString(ref.detail)
		content.WriteByte('\n')
	}
	if def != nil {
		content.WriteString("**")
		content.WriteString(def.name)
		content.WriteString("**")
		if def.kind != "" {
			content.WriteString("  ")
			content.WriteString(def.kind)
		}
		if def.detail != "" {
			content.WriteString("\n")
			content.WriteString(def.detail)
		}
	}
	h := &lspHover{Contents: lspMarkupContent{Kind: "markdown", Value: content.String()}}
	if ref != nil {
		r := toLSPRangeInText(state.text, ref.span)
		h.Range = &r
	}
	return h, nil
}

// definition operates on languageServer values and keeps the
// languageserver-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns any and an error. Callers should only trust the value when the
// error is nil.
func (s *languageServer) definition(uri string, pos lspPosition) (any, error) {
	state := s.docs[uri]
	if state == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if state.analysis == nil {
		return nil, nil
	}
	ref, def := state.analysis.findAt(toSourcePosInText(state.text, pos))
	if ref != nil && ref.target != nil {
		if ref.target.uri == "" || (ref.target.span.start.line == 0 && ref.target.span.start.col == 0 && ref.target.span.end.line == 0 && ref.target.span.end.col == 0) {
			return nil, nil
		}
		return lspLocation{URI: ref.target.uri, Range: s.rangeForURI(ref.target.uri, ref.target.span)}, nil
	}
	if def != nil {
		if def.uri == "" || (def.span.start.line == 0 && def.span.start.col == 0 && def.span.end.line == 0 && def.span.end.col == 0) {
			return nil, nil
		}
		return lspLocation{URI: def.uri, Range: s.rangeForURI(def.uri, def.span)}, nil
	}
	return nil, nil
}

type lspCompletionItem struct {
	Label  string `json:"label"`
	Kind   int    `json:"kind,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type lspSignatureParameter struct {
	Label string `json:"label"`
}

type lspSignatureInformation struct {
	Label      string                  `json:"label"`
	Parameters []lspSignatureParameter `json:"parameters,omitempty"`
}

type lspSignatureHelp struct {
	Signatures      []lspSignatureInformation `json:"signatures"`
	ActiveSignature int                       `json:"activeSignature,omitempty"`
	ActiveParameter int                       `json:"activeParameter,omitempty"`
}

func (s *languageServer) completion(uri string, pos lspPosition) ([]lspCompletionItem, error) {
	state := s.docs[uri]
	if state == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if state.analysis == nil {
		return []lspCompletionItem{}, nil
	}
	seen := make(map[string]lspCompletionItem)
	for _, def := range state.analysis.defs {
		if def == nil || def.name == "" {
			continue
		}
		label := def.name
		if dot := strings.LastIndexByte(label, '.'); dot >= 0 {
			label = label[dot+1:]
		}
		kind := 6
		switch def.kind {
		case "function", "builtin", "trait method":
			kind = 3
		case "struct", "type alias", "newtype", "trait", "enum":
			kind = 7
		case "variable", "param", "iter":
			kind = 6
		}
		seen[label] = lspCompletionItem{Label: label, Kind: kind, Detail: def.detail}
	}
	items := make([]lspCompletionItem, 0, len(seen))
	for _, item := range seen {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	return items, nil
}

func (s *languageServer) references(uri string, pos lspPosition) ([]lspLocation, error) {
	state := s.docs[uri]
	if state == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if state.analysis == nil {
		return []lspLocation{}, nil
	}
	ref, target := state.analysis.findAt(toSourcePosInText(state.text, pos))
	if ref != nil {
		target = ref.target
	}
	if target == nil {
		return []lspLocation{}, nil
	}
	locations := make([]lspLocation, 0, len(state.analysis.refs)+1)
	if target.uri == uri && target.span.start.line != 0 {
		locations = append(locations, lspLocation{URI: target.uri, Range: s.rangeForURI(target.uri, target.span)})
	}
	for _, candidate := range state.analysis.refs {
		if candidate.target == target {
			locations = append(locations, lspLocation{URI: uri, Range: s.rangeForURI(uri, candidate.span)})
		}
	}
	return locations, nil
}

func (s *languageServer) signatureHelp(uri string, pos lspPosition) (*lspSignatureHelp, error) {
	state := s.docs[uri]
	if state == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if state.analysis == nil {
		return nil, nil
	}
	offset, ok := lspUTF16Offset(state.text, pos)
	if !ok {
		return nil, fmt.Errorf("invalid signature-help position")
	}
	prefix := state.text[:offset]
	open := strings.LastIndexByte(prefix, '(')
	if open < 0 {
		return nil, nil
	}
	nameEnd := open
	for nameEnd > 0 && (prefix[nameEnd-1] == ' ' || prefix[nameEnd-1] == '\t') {
		nameEnd--
	}
	nameStart := nameEnd
	for nameStart > 0 {
		r, size := utf8.DecodeLastRuneInString(prefix[:nameStart])
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.') {
			break
		}
		nameStart -= size
	}
	name := prefix[nameStart:nameEnd]
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		name = name[dot+1:]
	}
	var detail string
	for _, def := range state.analysis.defs {
		if def != nil && def.name == name && def.detail != "" {
			detail = def.detail
			break
		}
	}
	if detail == "" {
		return nil, nil
	}
	return &lspSignatureHelp{Signatures: []lspSignatureInformation{{Label: detail}}}, nil
}

// RenameDocument returns a formatted, dry-run rewrite of the symbol at pos.
// It never mutates the open document. Reference selection uses the semantic
// analysis target pointer, so same-spelled shadowed bindings are excluded.
func (s *languageServer) RenameDocument(uri string, pos lspPosition, replacement string) (string, error) {
	if !validIdentifier(replacement) {
		return "", fmt.Errorf("invalid replacement identifier %q", replacement)
	}
	state := s.docs[uri]
	if state == nil || state.analysis == nil {
		return "", fmt.Errorf("document not open: %s", uri)
	}
	ref, target := state.analysis.findAt(toSourcePosInText(state.text, pos))
	if ref != nil {
		target = ref.target
	}
	if target == nil || target.uri == "" {
		return "", fmt.Errorf("no renameable symbol at position")
	}
	if target.uri != uri {
		return "", fmt.Errorf("cross-document rename is not supported")
	}
	spans := []sourceSpan{target.span}
	for _, candidate := range state.analysis.refs {
		if candidate.target == target && spanText(state.text, candidate.span) == target.name {
			spans = append(spans, candidate.span)
		}
	}
	type edit struct{ start, end int }
	edits := make([]edit, 0, len(spans))
	seen := map[edit]bool{}
	for _, span := range spans {
		start, ok := sourceSpanOffsets(state.text, span)
		if !ok {
			return "", fmt.Errorf("rename source span is invalid")
		}
		item := edit{start: start, end: start + (span.end.col - span.start.col)}
		if !seen[item] {
			edits, seen[item] = append(edits, item), true
		}
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	rewritten := state.text
	for _, item := range edits {
		rewritten = rewritten[:item.start] + replacement + rewritten[item.end:]
	}
	return FormatSource(state.sourceName, rewritten)
}

func spanText(text string, span sourceSpan) string {
	start, ok := sourceSpanOffsets(text, span)
	if !ok {
		return ""
	}
	end := start + span.end.col - span.start.col
	if end < start || end > len(text) {
		return ""
	}
	return text[start:end]
}

func validIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

func sourceSpanOffsets(text string, span sourceSpan) (int, bool) {
	if span.start.line < 1 || span.start.col < 1 || span.end.line != span.start.line || span.end.col < span.start.col {
		return 0, false
	}
	lines := strings.SplitAfter(text, "\n")
	if span.start.line > len(lines) {
		return 0, false
	}
	offset := 0
	for i := 0; i < span.start.line-1; i++ {
		offset += len(lines[i])
	}
	start := offset + span.start.col - 1
	if start > len(text) || start+(span.end.col-span.start.col) > len(text) {
		return 0, false
	}
	return start, true
}

func fullDocumentRange(text string) lspRange {
	lines := strings.Split(text, "\n")
	last := lines[len(lines)-1]
	return toLSPRangeInText(text, sourceSpan{start: sourcePos{line: 1, col: 1}, end: sourcePos{line: len(lines), col: len(last) + 1}})
}

// publishDiagnostics operates on languageServer values and keeps the
// languageserver-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns an error when the operation cannot complete cleanly.
func (s *languageServer) publishDiagnostics(uri string, diags []lspDiagnostic) error {
	return s.writeNotification("textDocument/publishDiagnostics", map[string]any{
		"uri":         uri,
		"diagnostics": diags,
	})
}

// writeResponse operates on languageServer values and keeps the
// languageserver-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns an error when the operation cannot complete cleanly.
func (s *languageServer) writeResponse(id json.RawMessage, result any, rpcErr error) error {
	resp := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id)}
	if rpcErr != nil {
		resp["error"] = map[string]any{"code": -32603, "message": rpcErr.Error()}
	} else {
		resp["result"] = result
	}
	return s.writeMessage(resp)
}

// writeNotification operates on languageServer values and keeps the
// languageserver-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns an error when the operation cannot complete cleanly.
func (s *languageServer) writeNotification(method string, params any) error {
	return s.writeMessage(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// writeMessage operates on languageServer values and keeps the
// languageserver-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns an error when the operation cannot complete cleanly.
func (s *languageServer) writeMessage(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n%s", len(data), data)
	return err
}

type lspMessage struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	ID     json.RawMessage `json:"id"`
}

// IsRequest operates on lspMessage values and keeps the lspmessage-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func (m lspMessage) IsRequest() bool { return len(m.ID) > 0 }

// readLSPMessage handles the read lspmessage step in the surrounding package
// and returns lspMessage, error.
//
// It returns lspMessage and an error. Callers should only trust the value when
// the error is nil.
func readLSPMessage(r *bufio.Reader) (lspMessage, error) {
	var length int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return lspMessage{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			if _, err := fmt.Sscanf(line, "Content-Length: %d", &length); err != nil {
				return lspMessage{}, err
			}
		}
	}
	if length <= 0 {
		return lspMessage{}, io.EOF
	}
	if length > maxLSPMessageBytes {
		return lspMessage{}, fmt.Errorf("LSP message exceeds %d-byte limit", maxLSPMessageBytes)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return lspMessage{}, err
	}
	var msg lspMessage
	if err := json.Unmarshal(buf, &msg); err != nil {
		return lspMessage{}, err
	}
	return msg, nil
}

type didOpenParams struct {
	TextDocument struct {
		URI  string `json:"uri"`
		Text string `json:"text"`
	} `json:"textDocument"`
}

type didChangeParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	ContentChanges []struct {
		Range *lspRange `json:"range,omitempty"`
		Text  string    `json:"text"`
	} `json:"contentChanges"`
}

type renameParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position lspPosition `json:"position"`
	NewName  string      `json:"newName"`
}

func applyLSPChange(text string, change *lspRange, replacement string) (string, error) {
	if change == nil {
		return replacement, nil
	}
	start, ok := lspUTF16Offset(text, change.Start)
	if !ok {
		return "", fmt.Errorf("invalid incremental change start")
	}
	end, ok := lspUTF16Offset(text, change.End)
	if !ok || end < start {
		return "", fmt.Errorf("invalid incremental change end")
	}
	return text[:start] + replacement + text[end:], nil
}

func lspUTF16Offset(text string, pos lspPosition) (int, bool) {
	if pos.Line < 0 || pos.Character < 0 {
		return 0, false
	}
	lines := strings.SplitAfter(text, "\n")
	if pos.Line >= len(lines) {
		return 0, false
	}
	offset := 0
	for i := 0; i < pos.Line; i++ {
		offset += len(lines[i])
	}
	line := strings.TrimSuffix(lines[pos.Line], "\n")
	units, at := 0, 0
	for at < len(line) && units < pos.Character {
		r, size := utf8.DecodeRuneInString(line[at:])
		at += size
		if r > 0xffff {
			units += 2
		} else {
			units++
		}
	}
	if units != pos.Character {
		return 0, false
	}
	return offset + at, true
}

type didCloseParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
}

type hoverParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position lspPosition `json:"position"`
}

type definitionParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position lspPosition `json:"position"`
}

type referencesParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position lspPosition `json:"position"`
}

type completionParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position lspPosition `json:"position"`
}

type signatureHelpParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position lspPosition `json:"position"`
}

// analyzeDocument handles the analyze document step in the surrounding package
// and returns *documentAnalysis.
//
// It returns *documentAnalysis so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func analyzeDocument(rt *Runtime, uri string, prog *Program) *documentAnalysis {
	a := &documentAnalysis{enumCases: make(map[string]*symbolDef)}
	root := &lspScope{defs: map[string]*symbolDef{}}
	for name := range rt.Builtins {
		root.defs[name] = &symbolDef{name: name, kind: "builtin", detail: "builtin function", uri: "", span: sourceSpan{}}
	}
	_, _ = analyzeStmtSeq(a, root, uri, prog.Statements, false)
	if _, err := rt.CheckProgram(context.Background(), prog); err != nil {
		span := sourceSpan{}
		message := err.Error()
		if typed, ok := err.(*OsirisError); ok {
			message = typed.message
			if typed.location != nil {
				span = typed.location.Span
			}
		}
		a.diagnostics = append(a.diagnostics, diagnosticAt(span, message))
	}
	return a
}

type lspScope struct {
	parent *lspScope
	defs   map[string]*symbolDef
}

// lookup operates on lspScope values and keeps the lspscope-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns *symbolDef so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (s *lspScope) lookup(name string) *symbolDef {
	for cur := s; cur != nil; cur = cur.parent {
		if def, ok := cur.defs[name]; ok {
			return def
		}
	}
	return nil
}

// push operates on lspScope values and keeps the lspscope-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns *lspScope so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (s *lspScope) push(def *symbolDef) *lspScope {
	return &lspScope{
		parent: s,
		defs: map[string]*symbolDef{
			def.name: def,
		},
	}
}

// analyzeStmtSeq handles the analyze stmt seq step in the surrounding package
// and returns *lspScope, error.
//
// It returns *lspScope and an error. Callers should only trust the value when
// the error is nil.
func analyzeStmtSeq(a *documentAnalysis, scope *lspScope, uri string, stmts []Stmt, inBlock bool) (*lspScope, error) {
	cur := scope
	functions := &lspScope{parent: cur, defs: make(map[string]*symbolDef)}
	for _, stmt := range stmts {
		st, ok := stmt.(funcStmt)
		if !ok {
			continue
		}
		def := &symbolDef{name: st.Name, kind: "function", detail: functionDetail(st), uri: uri, span: st.NameSpan}
		functions.defs[st.Name] = def
	}
	if len(functions.defs) > 0 {
		cur = functions
	}
	for _, stmt := range stmts {
		next, err := analyzeStmt(a, cur, uri, stmt, inBlock)
		if err != nil {
			return cur, err
		}
		if next != nil {
			cur = next
		}
	}
	return cur, nil
}

// analyzeStmt handles the analyze stmt step in the surrounding package and
// returns *lspScope, error.
//
// It returns *lspScope and an error. Callers should only trust the value when
// the error is nil.
func analyzeStmt(a *documentAnalysis, scope *lspScope, uri string, stmt Stmt, inBlock bool) (*lspScope, error) {
	switch st := stmt.(type) {
	case packageStmt:
		def := &symbolDef{name: st.Name, kind: "package", detail: packageDetail(st), uri: uri, span: st.NameSpan}
		a.defs = append(a.defs, def)
		return scope.push(def), nil
	case importStmt:
		def := &symbolDef{name: st.Alias, kind: "module", detail: importDetail(st), uri: uri, span: st.AliasSpan}
		a.defs = append(a.defs, def)
		return scope.push(def), nil
	case enumStmt:
		def := &symbolDef{name: st.Name, kind: "enum", detail: fmt.Sprintf("enum with %d cases", len(st.Cases)), uri: uri, span: st.NameSpan}
		a.defs = append(a.defs, def)
		for _, item := range st.Cases {
			caseDef := &symbolDef{name: st.Name + "." + item.Name, kind: "enum case", detail: "case of " + st.Name, uri: uri, span: item.Span}
			a.defs = append(a.defs, caseDef)
			a.enumCases[caseDef.name] = caseDef
		}
		return scope.push(def), nil
	case structStmt:
		def := &symbolDef{name: st.Name, kind: "struct", detail: fmt.Sprintf("struct with %d fields", len(st.Fields)), uri: uri, span: st.NameSpan}
		a.defs = append(a.defs, def)
		return scope.push(def), nil
	case typeDeclStmt:
		def := &symbolDef{name: st.Name, kind: "type alias", detail: "alias of " + st.Target, uri: uri, span: st.NameSpan}
		a.defs = append(a.defs, def)
		return scope.push(def), nil
	case newtypeStmt:
		def := &symbolDef{name: st.Name, kind: "newtype", detail: "newtype of " + st.Target, uri: uri, span: st.NameSpan}
		a.defs = append(a.defs, def)
		return scope.push(def), nil
	case letStmt:
		if err := analyzeExpr(a, scope, uri, st.Expr); err != nil {
			return nil, err
		}
		if len(st.Names) > 0 {
			for _, name := range st.Names {
				item := &symbolDef{name: name, kind: "variable", detail: "tuple binding", uri: uri, span: st.Span}
				a.defs = append(a.defs, item)
				scope = scope.push(item)
			}
			return scope, nil
		}
		def := &symbolDef{name: st.Name, kind: "variable", detail: "binding", uri: uri, span: st.NameSpan}
		a.defs = append(a.defs, def)
		return scope.push(def), nil
	case assignStmt:
		return scope, analyzeExpr(a, scope, uri, st.Expr)
	case emitStmt:
		return scope, analyzeExpr(a, scope, uri, st.Expr)
	case macroStmt:
		def := &symbolDef{name: st.Name, kind: "macro", detail: "compile-time macro", uri: uri, span: st.NameSpan}
		a.defs = append(a.defs, def)
		child := &lspScope{parent: scope, defs: map[string]*symbolDef{}}
		for _, param := range st.Params {
			child.defs[param] = &symbolDef{name: param, kind: "param", detail: "macro parameter", uri: uri, span: st.Span}
		}
		if err := analyzeExpr(a, child, uri, st.Body); err != nil {
			return nil, err
		}
		return scope.push(def), nil
	case traitStmt:
		def := &symbolDef{name: st.Name, kind: "trait", detail: fmt.Sprintf("trait with %d methods", len(st.Methods)), uri: uri, span: st.NameSpan}
		a.defs = append(a.defs, def)
		for _, method := range st.Methods {
			a.defs = append(a.defs, &symbolDef{name: st.Name + "." + method.Name, kind: "trait method", detail: functionDetail(method), uri: uri, span: method.NameSpan})
		}
		return scope.push(def), nil
	case implStmt:
		for _, method := range st.Methods {
			if _, err := analyzeStmt(a, scope, uri, method, inBlock); err != nil {
				return nil, err
			}
		}
		return scope, nil
	case funcStmt:
		def := scope.lookup(st.Name)
		predeclared := def != nil && def.kind == "function" && def.span == st.NameSpan
		if !predeclared {
			def = &symbolDef{name: st.Name, kind: "function", detail: functionDetail(st), uri: uri, span: st.NameSpan}
		}
		a.defs = append(a.defs, def)
		child := &lspScope{parent: scope, defs: map[string]*symbolDef{st.Name: def}}
		for i, param := range st.Params {
			detail := "function parameter"
			if i < len(st.ParamTypes) && st.ParamTypes[i] != "" {
				detail += " " + st.ParamTypes[i]
			}
			span := st.Span
			if i < len(st.ParamSpans) {
				span = st.ParamSpans[i]
			}
			child.defs[param] = &symbolDef{name: param, kind: "param", detail: detail, uri: uri, span: span}
		}
		if err := analyzeExpr(a, child, uri, st.Body); err != nil {
			return nil, err
		}
		if predeclared {
			return scope, nil
		}
		return scope.push(def), nil
	case whileStmt:
		if err := analyzeExpr(a, scope, uri, st.Cond); err != nil {
			return nil, err
		}
		return scope, analyzeExpr(a, scope, uri, st.Body)
	case forStmt:
		if err := analyzeExpr(a, scope, uri, st.Iter); err != nil {
			return nil, err
		}
		def := &symbolDef{name: st.Name, kind: "iter", detail: "loop item", uri: uri, span: st.NameSpan}
		a.defs = append(a.defs, def)
		child := &lspScope{parent: scope, defs: map[string]*symbolDef{st.Name: def}}
		return scope, analyzeExpr(a, child, uri, st.Body)
	case returnStmt:
		if st.Expr != nil {
			return scope, analyzeExpr(a, scope, uri, st.Expr)
		}
		return scope, nil
	case breakStmt, continueStmt:
		return scope, nil
	case exportStmt:
		for i, name := range st.Names {
			def := scope.lookup(name)
			if def == nil {
				a.diagnostics = append(a.diagnostics, diagnosticAt(st.NameSpans[i], fmt.Sprintf("exported name %q is undefined", name)))
				continue
			}
			a.refs = append(a.refs, &symbolRef{span: st.NameSpans[i], target: def, detail: "export"})
		}
		return scope, nil
	default:
		return scope, nil
	}
}

// analyzeExpr handles the analyze expr step in the surrounding package and
// returns error.
//
// It returns an error when the operation cannot complete cleanly.
func analyzeExpr(a *documentAnalysis, scope *lspScope, uri string, expr Expr) error {
	switch n := expr.(type) {
	case *identExpr:
		def := scope.lookup(n.name)
		if def == nil {
			a.diagnostics = append(a.diagnostics, diagnosticAt(n.Span, fmt.Sprintf("undefined name %q", n.name)))
			return nil
		}
		a.refs = append(a.refs, &symbolRef{span: n.Span, target: def, detail: def.kind})
	case *memberExpr:
		if err := analyzeExpr(a, scope, uri, n.object); err != nil {
			return err
		}
		def := resolveMemberTarget(a, scope, n.object, n.name)
		if def != nil {
			a.refs = append(a.refs, &symbolRef{span: n.NameSpan, target: def, detail: "member " + n.name})
		}
	case *callExpr:
		if err := analyzeExpr(a, scope, uri, n.callee); err != nil {
			return err
		}
		for _, arg := range n.args {
			if err := analyzeExpr(a, scope, uri, arg); err != nil {
				return err
			}
		}
	case *unaryExpr:
		return analyzeExpr(a, scope, uri, n.expr)
	case *binaryExpr:
		if err := analyzeExpr(a, scope, uri, n.left); err != nil {
			return err
		}
		return analyzeExpr(a, scope, uri, n.right)
	case *listExpr:
		for _, item := range n.items {
			if err := analyzeExpr(a, scope, uri, item); err != nil {
				return err
			}
		}
	case *tupleExpr:
		for _, item := range n.items {
			if err := analyzeExpr(a, scope, uri, item); err != nil {
				return err
			}
		}
	case *structExpr:
		for _, field := range n.Fields {
			if err := analyzeExpr(a, scope, uri, field.Expr); err != nil {
				return err
			}
		}
	case *structUpdateExpr:
		if err := analyzeExpr(a, scope, uri, n.Base); err != nil {
			return err
		}
		for _, field := range n.Fields {
			if err := analyzeExpr(a, scope, uri, field.Expr); err != nil {
				return err
			}
		}
	case *indexExpr:
		if err := analyzeExpr(a, scope, uri, n.Object); err != nil {
			return err
		}
		return analyzeExpr(a, scope, uri, n.Index)
	case *tryExpr:
		if err := analyzeExpr(a, scope, uri, n.body); err != nil {
			return err
		}
		child := &lspScope{parent: scope, defs: map[string]*symbolDef{}}
		catchDef := &symbolDef{name: n.catchName, kind: "error", detail: "caught error", uri: uri, span: n.Span}
		child.defs[n.catchName] = catchDef
		a.defs = append(a.defs, catchDef)
		return analyzeExpr(a, child, uri, n.handler)
	case *propagateExpr:
		return analyzeExpr(a, scope, uri, n.expr)
	case *blockExpr:
		child := &lspScope{parent: scope, defs: map[string]*symbolDef{}}
		final, err := analyzeStmtSeq(a, child, uri, n.stmts, true)
		if err != nil {
			return err
		}
		if n.tail != nil {
			return analyzeExpr(a, final, uri, n.tail)
		}
	case *matchExpr:
		if err := analyzeExpr(a, scope, uri, n.subject); err != nil {
			return err
		}
		for _, arm := range n.arms {
			if err := validatePatternBindings(arm.pattern, ""); err != nil {
				a.diagnostics = append(a.diagnostics, diagnosticAt(patternSpan(arm.pattern), err.Error()))
			}
			child := &lspScope{parent: scope, defs: map[string]*symbolDef{}}
			for _, def := range patternDefs(uri, arm.pattern) {
				child.defs[def.name] = def
				a.defs = append(a.defs, def)
			}
			if arm.guard != nil {
				if err := analyzeExpr(a, child, uri, arm.guard); err != nil {
					return err
				}
			}
			if err := analyzeExpr(a, child, uri, arm.body); err != nil {
				return err
			}
		}
	case *literalExpr:
		return nil
	default:
		return nil
	}
	return nil
}

// resolveMemberTarget handles the resolve member target step in the
// surrounding package and returns *symbolDef.
//
// It returns *symbolDef so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func resolveMemberTarget(a *documentAnalysis, scope *lspScope, expr Expr, member string) *symbolDef {
	if ident, ok := expr.(*identExpr); ok {
		if a != nil && a.enumCases != nil {
			if def := a.enumCases[ident.name+"."+member]; def != nil {
				return def
			}
		}
		return scope.lookup(ident.name)
	}
	return nil
}

// patternDefs handles the pattern defs step in the surrounding package and
// returns []*symbolDef.
//
// It returns []*symbolDef so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func patternDefs(uri string, p Pattern) []*symbolDef {
	switch n := p.(type) {
	case bindPattern:
		return []*symbolDef{{name: n.name, kind: "pattern", detail: "pattern binding", uri: uri, span: n.Span}}
	case listPattern:
		var out []*symbolDef
		for _, item := range n.items {
			out = append(out, patternDefs(uri, item)...)
		}
		return out
	case enumPattern:
		var out []*symbolDef
		for _, item := range n.items {
			out = append(out, patternDefs(uri, item)...)
		}
		return out
	case orPattern:
		if len(n.alternatives) == 0 {
			return nil
		}
		return patternDefs(uri, n.alternatives[0])
	case andPattern:
		return append(patternDefs(uri, n.left), patternDefs(uri, n.right)...)
	case typePattern:
		if n.binding == "" || n.binding == "_" {
			return nil
		}
		return []*symbolDef{{name: n.binding, kind: "pattern", detail: n.typeName + " pattern binding", uri: uri, span: n.Span}}
	default:
		return nil
	}
}

// defsFromParams handles the defs from params step in the surrounding package
// and returns []*symbolDef.
//
// It returns []*symbolDef so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func defsFromParams(uri string, params []string, sp sourceSpan) []*symbolDef {
	out := make([]*symbolDef, 0, len(params))
	for _, name := range params {
		out = append(out, &symbolDef{name: name, kind: "param", detail: "macro parameter", uri: uri, span: sp})
	}
	return out
}

// functionDetail handles the function detail step in the surrounding package
// and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func functionDetail(st funcStmt) string {
	signature, err := staticFunctionType(st)
	if err != nil {
		return "function declaration"
	}
	prefix := "func "
	if st.ReceiverType != "" {
		prefix = "func (" + st.Receiver + " " + st.ReceiverType + ") "
	}
	return prefix + st.Name + strings.TrimPrefix(signature.String(), "func")
}

// packageDetail handles the package detail step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func packageDetail(st packageStmt) string {
	if st.Version == "" {
		return "package"
	}
	return "package version " + st.Version
}

// importDetail handles the import detail step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func importDetail(st importStmt) string {
	ref, selector := splitModuleRef(st.Path)
	if selector != "" {
		return "import " + ref + "@" + selector
	}
	return "import " + ref
}

// diagnosticAt handles the diagnostic at step in the surrounding package and
// returns lspDiagnostic.
//
// It returns lspDiagnostic so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func diagnosticAt(sp sourceSpan, msg string) lspDiagnostic {
	return lspDiagnostic{Range: toLSPRange(sp), Severity: 1, Source: "osiris", Message: msg}
}

// diagnosticsInText upgrades analysis diagnostics from parser rune columns to
// the UTF-16 columns required on the wire by the Language Server Protocol.
func diagnosticsInText(text string, diagnostics []lspDiagnostic) []lspDiagnostic {
	converted := make([]lspDiagnostic, len(diagnostics))
	copy(converted, diagnostics)
	for i := range converted {
		r := converted[i].Range
		if r == (lspRange{}) {
			continue
		}
		converted[i].Range = toLSPRangeInText(text, sourceSpan{
			start: sourcePos{line: r.Start.Line + 1, col: r.Start.Character + 1},
			end:   sourcePos{line: r.End.Line + 1, col: r.End.Character + 1},
		})
	}
	return converted
}

// toSourcePos handles the to source pos step in the surrounding package and
// returns sourcePos.
//
// It returns sourcePos so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func toSourcePos(pos lspPosition) sourcePos {
	return sourcePos{line: pos.Line + 1, col: pos.Character + 1}
}

func toSourcePosInText(text string, pos lspPosition) sourcePos {
	lines := strings.Split(text, "\n")
	if pos.Line < 0 || pos.Line >= len(lines) {
		return sourcePos{line: pos.Line + 1, col: pos.Character + 1}
	}
	line := lines[pos.Line]
	units, bytes := 0, 0
	for bytes < len(line) && units < pos.Character {
		r, size := utf8.DecodeRuneInString(line[bytes:])
		bytes += size
		if r > 0xffff {
			units += 2
		} else {
			units++
		}
	}
	// Parser locations currently use UTF-8 byte columns (the lexer advances its
	// source column by encoded byte width). Convert the editor's UTF-16
	// position back to that source-map representation for lookup.
	return sourcePos{line: pos.Line + 1, col: bytes + 1}
}

func toLSPRangeInText(text string, sp sourceSpan) lspRange {
	r := toLSPRange(sp)
	lines := strings.Split(text, "\n")
	convert := func(p lspPosition) lspPosition {
		if p.Line < 0 || p.Line >= len(lines) {
			return p
		}
		line := lines[p.Line]
		byteAt := p.Character
		if byteAt > len(line) {
			byteAt = len(line)
		}
		// Parser spans should lie on a code-point boundary. Be defensive for
		// malformed/generated spans: never slice through a UTF-8 sequence.
		for byteAt > 0 && byteAt < len(line) && (line[byteAt]&0xc0) == 0x80 {
			byteAt--
		}
		units := 0
		for _, ch := range line[:byteAt] {
			if ch > 0xffff {
				units += 2
			} else {
				units++
			}
		}
		p.Character = units
		return p
	}
	r.Start, r.End = convert(r.Start), convert(r.End)
	return r
}

// toLSPRange handles the to lsprange step in the surrounding package and
// returns lspRange.
//
// It returns lspRange so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func toLSPRange(sp sourceSpan) lspRange {
	if sp.start.line == 0 && sp.start.col == 0 && sp.end.line == 0 && sp.end.col == 0 {
		return lspRange{}
	}
	return lspRange{
		Start: lspPosition{Line: sp.start.line - 1, Character: sp.start.col - 1},
		End:   lspPosition{Line: sp.end.line - 1, Character: sp.end.col - 1},
	}
}

// spanFromName handles the span from name step in the surrounding package and
// returns sourceSpan.
//
// It returns sourceSpan so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func spanFromName(name string, fallback sourceSpan) sourceSpan {
	if fallback.start.line != 0 || fallback.start.col != 0 {
		return fallback
	}
	return sourceSpan{}
}

// uriToPath handles the uri to path step in the surrounding package and
// returns string, error.
//
// It returns string and an error. Callers should only trust the value when the
// error is nil.
func uriToPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" {
		return uri, nil
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("unsupported uri scheme %q", u.Scheme)
	}
	path, err := url.PathUnescape(u.Path)
	if err != nil {
		return "", err
	}
	return filepath.FromSlash(path), nil
}

// pathToURI handles the path to uri step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func pathToURI(path string) string {
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}
