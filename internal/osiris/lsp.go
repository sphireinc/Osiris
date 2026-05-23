package osiris

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
)

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
	_ = ctx
	r := bufio.NewReader(in)
	for {
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
					"textDocumentSync":   map[string]any{"openClose": true, "change": 1},
					"hoverProvider":      true,
					"definitionProvider": true,
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
			text := ""
			if len(params.ContentChanges) > 0 {
				text = params.ContentChanges[len(params.ContentChanges)-1].Text
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
		state.diagnostics = state.analysis.diagnostics
	}
	s.docs[uri] = state
	_ = s.publishDiagnostics(uri, state.diagnostics)
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
	ref, def := state.analysis.findAt(toSourcePos(pos))
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
		r := toLSPRange(ref.span)
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
	ref, def := state.analysis.findAt(toSourcePos(pos))
	if ref != nil && ref.target != nil {
		if ref.target.uri == "" || (ref.target.span.start.line == 0 && ref.target.span.start.col == 0 && ref.target.span.end.line == 0 && ref.target.span.end.col == 0) {
			return nil, nil
		}
		return lspLocation{URI: ref.target.uri, Range: toLSPRange(ref.target.span)}, nil
	}
	if def != nil {
		if def.uri == "" || (def.span.start.line == 0 && def.span.start.col == 0 && def.span.end.line == 0 && def.span.end.col == 0) {
			return nil, nil
		}
		return lspLocation{URI: def.uri, Range: toLSPRange(def.span)}, nil
	}
	return nil, nil
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
		Text string `json:"text"`
	} `json:"contentChanges"`
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

// analyzeDocument handles the analyze document step in the surrounding package
// and returns *documentAnalysis.
//
// It returns *documentAnalysis so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func analyzeDocument(rt *Runtime, uri string, prog *Program) *documentAnalysis {
	a := &documentAnalysis{}
	root := &lspScope{defs: map[string]*symbolDef{}}
	for name := range rt.Builtins {
		root.defs[name] = &symbolDef{name: name, kind: "builtin", detail: "builtin function", uri: "", span: sourceSpan{}}
	}
	_, _ = analyzeStmtSeq(a, root, uri, prog.Statements, false)
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
	case letStmt:
		if err := analyzeExpr(a, scope, uri, st.Expr); err != nil {
			return nil, err
		}
		def := &symbolDef{name: st.Name, kind: "variable", detail: "binding", uri: uri, span: st.NameSpan}
		a.defs = append(a.defs, def)
		return scope.push(def), nil
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
	case funcStmt:
		def := &symbolDef{name: st.Name, kind: "function", detail: functionDetail(st), uri: uri, span: st.NameSpan}
		a.defs = append(a.defs, def)
		child := &lspScope{parent: scope, defs: map[string]*symbolDef{st.Name: def}}
		for _, param := range st.Params {
			child.defs[param] = &symbolDef{name: param, kind: "param", detail: "function parameter", uri: uri, span: st.Span}
		}
		if err := analyzeExpr(a, child, uri, st.Body); err != nil {
			return nil, err
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
		def := resolveMemberTarget(a, scope, n.object)
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
	case *tryExpr:
		if err := analyzeExpr(a, scope, uri, n.body); err != nil {
			return err
		}
		child := &lspScope{parent: scope, defs: map[string]*symbolDef{}}
		catchDef := &symbolDef{name: n.catchName, kind: "error", detail: "caught error", uri: uri, span: n.Span}
		child.defs[n.catchName] = catchDef
		a.defs = append(a.defs, catchDef)
		return analyzeExpr(a, child, uri, n.handler)
	case *blockExpr:
		child := &lspScope{parent: scope, defs: map[string]*symbolDef{}}
		_, err := analyzeStmtSeq(a, child, uri, n.stmts, true)
		if err != nil {
			return err
		}
		if n.tail != nil {
			return analyzeExpr(a, child, uri, n.tail)
		}
	case *matchExpr:
		if err := analyzeExpr(a, scope, uri, n.subject); err != nil {
			return err
		}
		for _, arm := range n.arms {
			child := &lspScope{parent: scope, defs: map[string]*symbolDef{}}
			for _, def := range patternDefs(uri, arm.pattern) {
				child.defs[def.name] = def
				a.defs = append(a.defs, def)
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
func resolveMemberTarget(a *documentAnalysis, scope *lspScope, expr Expr) *symbolDef {
	if ident, ok := expr.(*identExpr); ok {
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
	return "function declaration"
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

// toSourcePos handles the to source pos step in the surrounding package and
// returns sourcePos.
//
// It returns sourcePos so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func toSourcePos(pos lspPosition) sourcePos {
	return sourcePos{line: pos.Line + 1, col: pos.Character + 1}
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
