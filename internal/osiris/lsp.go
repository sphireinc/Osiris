package osiris

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// maxLSPMessageBytes bounds one framed JSON-RPC payload before allocation.
// Editors should send incremental changes; a larger document update is a
// protocol/resource error rather than an opportunity to exhaust the server.
const maxLSPMessageBytes = 16 << 20

type languageServer struct {
	rt             *Runtime
	out            io.Writer
	log            io.Writer
	mu             sync.RWMutex
	docs           map[string]*documentState
	workspace      map[string]*documentState
	versions       map[string]int
	workspaceRoot  string
	workspaceRoots []string
	initialized    bool
	closed         bool
	writeMu        sync.Mutex
	requestMu      sync.Mutex
	activeRequests map[string]*activeLSPRequest
	requestWG      sync.WaitGroup
}

type activeLSPRequest struct {
	cancel context.CancelFunc
}

type lspRequestContext struct {
	context.Context
	parent context.Context
}

func (c lspRequestContext) Err() error {
	if err := c.Context.Err(); err != nil {
		return err
	}
	return c.parent.Err()
}

type documentState struct {
	uri         string
	sourceName  string
	text        string
	prog        *Program
	diagnostics []lspDiagnostic
	analysis    *documentAnalysis
	version     int
	generation  uint64
}

type documentAnalysis struct {
	diagnostics []lspDiagnostic
	defs        []*symbolDef
	refs        []*symbolRef
	enumCases   map[string]*symbolDef
	members     map[string]map[string]*symbolDef
	checked     *CheckedProgram
	source      string
}

// lspResolvedModule is the narrow source-backed module view used by the LSP.
// Its exported definitions are pointers into the module document's analysis,
// so references from importing documents share identity with the definition
// and with references in the defining document.
type lspResolvedModule struct {
	uri     string
	exports map[string]*symbolDef
}

// lspAnalysisSpanned is an internal escape hatch for AST extensions. Every
// built-in AST node is covered by lspStmtSpan/lspExprSpan/lspPatternSpan
// below; an extension node can implement this interface to keep an
// unsupported-node diagnostic anchored to its source rather than silently
// disappearing or being reported at the document origin.
type lspAnalysisSpanned interface {
	lspAnalysisSpan() sourceSpan
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
	symbolID SymbolID
	name     string
	kind     string
	detail   string
	uri      string
	span     sourceSpan
	typeName string
	mutable  bool
	constant bool
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
	return &languageServer{
		rt:             rt,
		out:            out,
		log:            log,
		docs:           make(map[string]*documentState),
		workspace:      make(map[string]*documentState),
		versions:       make(map[string]int),
		activeRequests: make(map[string]*activeLSPRequest),
	}
}

func lspRequestIDKey(id json.RawMessage) string {
	return string(bytes.TrimSpace(id))
}

func (s *languageServer) beginLSPRequest(parent context.Context, id json.RawMessage) (context.Context, func()) {
	if parent == nil {
		parent = context.Background()
	}
	child, cancel := context.WithCancel(parent)
	if err := parent.Err(); err != nil {
		cancel()
	}
	ctx := lspRequestContext{Context: child, parent: parent}
	entry := &activeLSPRequest{cancel: cancel}
	key := lspRequestIDKey(id)
	s.requestMu.Lock()
	if s.activeRequests == nil {
		s.activeRequests = make(map[string]*activeLSPRequest)
	}
	previous := s.activeRequests[key]
	s.activeRequests[key] = entry
	s.requestMu.Unlock()
	if previous != nil {
		previous.cancel()
	}
	return ctx, func() {
		s.requestMu.Lock()
		if s.activeRequests[key] == entry {
			delete(s.activeRequests, key)
		}
		s.requestMu.Unlock()
		cancel()
	}
}

func (s *languageServer) cancelLSPRequest(id json.RawMessage) bool {
	key := lspRequestIDKey(id)
	s.requestMu.Lock()
	entry := s.activeRequests[key]
	s.requestMu.Unlock()
	if entry == nil {
		return false
	}
	entry.cancel()
	return true
}

func (s *languageServer) cancelAllLSPRequests() {
	s.requestMu.Lock()
	entries := make([]*activeLSPRequest, 0, len(s.activeRequests))
	for _, entry := range s.activeRequests {
		entries = append(entries, entry)
	}
	s.requestMu.Unlock()
	for _, entry := range entries {
		entry.cancel()
	}
}

func (s *languageServer) waitForLSPRequests() {
	s.requestWG.Wait()
}

func (s *languageServer) handleLSPCancelRequest(msg lspMessage) {
	var params struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil || len(params.ID) == 0 || bytes.Equal(bytes.TrimSpace(params.ID), []byte("null")) {
		_, _ = fmt.Fprintf(s.log, "osiris lsp: ignored malformed $/cancelRequest notification\n")
		return
	}
	s.cancelLSPRequest(params.ID)
}

// Serve operates on languageServer values and keeps the
// languageserver-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns an error when the operation cannot complete cleanly.
func (s *languageServer) Serve(ctx context.Context, in io.Reader) error {
	r := bufio.NewReader(in)
	responseReady := make(chan struct{})
	close(responseReady)

	for {
		select {
		case <-ctx.Done():
			s.cancelAllLSPRequests()
			s.waitForLSPRequests()
			return ctx.Err()
		default:
		}
		msg, err := readLSPMessage(r)
		if err != nil {
			if err == io.EOF {
				s.waitForLSPRequests()
				return nil
			}
			s.cancelAllLSPRequests()
			s.waitForLSPRequests()
			return err
		}
		if msg.Method == "" {
			if msg.IsRequest() {
				if err := s.writeResponse(msg.ID, nil, lspRPCError{code: -32600, message: "invalid request: method is required"}); err != nil {
					return err
				}
			}
			continue
		}
		switch msg.Method {
		case "$/cancelRequest":
			s.handleLSPCancelRequest(msg)
			continue
		case "exit":
			s.mu.Lock()
			s.closed = true
			s.mu.Unlock()
			s.waitForLSPRequests()
			return nil
		case "initialized":
			continue
		case "shutdown":
			s.waitForLSPRequests()
			s.closed = true
			if err := s.writeResponse(msg.ID, nil, nil); err != nil {
				return err
			}
			continue
		case "initialize":
			var params initializeParams
			if len(msg.Params) > 0 && string(msg.Params) != "null" {
				if err := json.Unmarshal(msg.Params, &params); err != nil {
					if msg.IsRequest() {
						if writeErr := s.writeResponse(msg.ID, nil, lspInvalidParams(err)); writeErr != nil {
							return writeErr
						}
					}
					continue
				}
			}
			s.configureWorkspace(params)
			if err := s.writeResponse(msg.ID, map[string]any{
				"capabilities": s.capabilities(),
				"serverInfo":   map[string]any{"name": "osiris", "version": lspServerVersion},
			}, nil); err != nil {
				return err
			}
			s.mu.Lock()
			s.initialized = true
			s.mu.Unlock()
			if params.RootURI != "" || params.RootPath != "" || len(params.WorkspaceFolders) > 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
				s.refreshWorkspace(ctx)
			}
			continue
		}

		switch msg.Method {
		case "textDocument/didOpen":
			var params didOpenParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				if writeErr := s.reportInvalidParams(msg, err); writeErr != nil {
					return writeErr
				}
				continue
			}
			if params.TextDocument.URI == "" {
				if writeErr := s.reportInvalidParams(msg, fmt.Errorf("textDocument.uri is required")); writeErr != nil {
					return writeErr
				}
				continue
			}
			if err := s.upsertDocumentContext(ctx, params.TextDocument.URI, params.TextDocument.Text, params.TextDocument.Version); err != nil {
				if writeErr := s.reportInvalidParams(msg, err); writeErr != nil {
					return writeErr
				}
			}
			continue
		case "textDocument/didChange":
			var params didChangeParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				if writeErr := s.reportInvalidParams(msg, err); writeErr != nil {
					return writeErr
				}
				continue
			}
			if params.TextDocument.URI == "" {
				if writeErr := s.reportInvalidParams(msg, fmt.Errorf("textDocument.uri is required")); writeErr != nil {
					return writeErr
				}
				continue
			}
			if len(params.ContentChanges) == 0 {
				if writeErr := s.reportInvalidParams(msg, fmt.Errorf("contentChanges must not be empty")); writeErr != nil {
					return writeErr
				}
				continue
			}
			if err := s.applyDocumentChangesContext(ctx, params.TextDocument.URI, params.TextDocument.Version, params.ContentChanges); err != nil {
				if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					// didChange is a notification, so there is no response in
					// which to report a stale version or bad client range. The
					// transaction helper has not published anything on failure.
					_, _ = fmt.Fprintf(s.log, "osiris lsp: ignored invalid didChange for %s: %v\n", params.TextDocument.URI, err)
				}
			}
			continue
		case "textDocument/didSave":
			var params didSaveParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				if writeErr := s.reportInvalidParams(msg, err); writeErr != nil {
					return writeErr
				}
				continue
			}
			if params.TextDocument.URI == "" {
				if writeErr := s.reportInvalidParams(msg, fmt.Errorf("textDocument.uri is required")); writeErr != nil {
					return writeErr
				}
				continue
			}
			if s.document(params.TextDocument.URI) == nil {
				if writeErr := s.reportInvalidParams(msg, fmt.Errorf("document is not open: %s", params.TextDocument.URI)); writeErr != nil {
					return writeErr
				}
				continue
			}
			if err := s.revalidateDocumentContext(ctx, params.TextDocument.URI, params.Text); err != nil {
				if writeErr := s.reportInvalidParams(msg, err); writeErr != nil {
					return writeErr
				}
			}
			continue
		case "textDocument/didClose":
			var params didCloseParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				if writeErr := s.reportInvalidParams(msg, err); writeErr != nil {
					return writeErr
				}
				continue
			}
			if params.TextDocument.URI == "" {
				if writeErr := s.reportInvalidParams(msg, fmt.Errorf("textDocument.uri is required")); writeErr != nil {
					return writeErr
				}
				continue
			}
			s.mu.Lock()
			delete(s.docs, params.TextDocument.URI)
			delete(s.versions, params.TextDocument.URI)
			s.mu.Unlock()
			// Closing an overlay must restore the current disk-backed document
			// (or remove it from the workspace index if it no longer exists).
			// Otherwise navigation after didClose would continue reading the
			// stale snapshot that was indexed before the editor opened it.
			if err := s.restoreClosedWorkspaceDocument(ctx, params.TextDocument.URI); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				_, _ = fmt.Fprintf(s.log, "osiris lsp: could not restore closed document %s: %v\n", params.TextDocument.URI, err)
			}
			if err := s.publishDiagnostics(params.TextDocument.URI, nil); err != nil {
				return err
			}
			continue
		case "workspace/didChangeWatchedFiles":
			var params didChangeWatchedFilesParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				if writeErr := s.reportInvalidParams(msg, err); writeErr != nil {
					return writeErr
				}
				continue
			}
			if err := validateWatchedFileChanges(params.Changes); err != nil {
				if writeErr := s.reportInvalidParams(msg, err); writeErr != nil {
					return writeErr
				}
				continue
			}
			// Refresh the disk-backed index as one transaction after the
			// notification batch. Open documents remain authoritative overlays,
			// while deleted files disappear from workspace navigation.
			s.refreshWorkspace(ctx)
			continue
		}
		if !msg.IsRequest() {
			continue
		}
		requestCtx, finish := s.beginLSPRequest(ctx, msg.ID)
		ready := responseReady
		responseReady = make(chan struct{})
		completed := responseReady
		s.requestWG.Add(1)
		go func(msg lspMessage, ctx context.Context, finish func(), ready <-chan struct{}, completed chan struct{}) {
			defer s.requestWG.Done()
			defer finish()
			defer close(completed)
			<-ready
			if err := s.dispatchLSPRequest(ctx, msg); err != nil {
				_, _ = fmt.Fprintf(s.log, "osiris lsp: request %s failed: %v\n", lspRequestIDKey(msg.ID), err)
			}
		}(msg, requestCtx, finish, ready, completed)
	}
}

func (s *languageServer) dispatchLSPRequest(ctx context.Context, msg lspMessage) error {
	if err := ctx.Err(); err != nil {
		return s.writeResponse(msg.ID, nil, lspRequestCancelled(err))
	}
	switch msg.Method {
	case "textDocument/hover":
		var params hoverParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return s.reportInvalidParams(msg, err)
		}
		if params.TextDocument.URI == "" {
			return s.reportInvalidParams(msg, fmt.Errorf("textDocument.uri is required"))
		}
		if err := s.checkDocumentVersion(s.document(params.TextDocument.URI), params.TextDocument.Version); err != nil {
			return s.writeResponse(msg.ID, nil, err)
		}
		resp, err := s.hoverContext(ctx, params.TextDocument.URI, params.Position)
		if err != nil {
			return s.writeResponse(msg.ID, nil, err)
		}
		return s.writeResponse(msg.ID, resp, nil)
	case "textDocument/definition":
		var params definitionParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return s.reportInvalidParams(msg, err)
		}
		if params.TextDocument.URI == "" {
			return s.reportInvalidParams(msg, fmt.Errorf("textDocument.uri is required"))
		}
		if err := s.checkDocumentVersion(s.document(params.TextDocument.URI), params.TextDocument.Version); err != nil {
			return s.writeResponse(msg.ID, nil, err)
		}
		resp, err := s.definitionContext(ctx, params.TextDocument.URI, params.Position)
		if err != nil {
			return s.writeResponse(msg.ID, nil, err)
		}
		return s.writeResponse(msg.ID, resp, nil)
	case "textDocument/completion":
		var params completionParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return s.reportInvalidParams(msg, err)
		}
		if params.TextDocument.URI == "" {
			return s.reportInvalidParams(msg, fmt.Errorf("textDocument.uri is required"))
		}
		if err := s.checkDocumentVersion(s.document(params.TextDocument.URI), params.TextDocument.Version); err != nil {
			return s.writeResponse(msg.ID, nil, err)
		}
		resp, err := s.completionContext(ctx, params.TextDocument.URI, params.Position)
		if err != nil {
			return s.writeResponse(msg.ID, nil, err)
		}
		return s.writeResponse(msg.ID, resp, nil)
	case "textDocument/rename":
		var params renameParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return s.reportInvalidParams(msg, err)
		}
		if params.TextDocument.URI == "" || params.NewName == "" {
			return s.reportInvalidParams(msg, fmt.Errorf("textDocument.uri and newName are required"))
		}
		edit, err := s.renameWorkspaceEditContext(ctx, params.TextDocument.URI, params.Position, params.NewName, params.TextDocument.Version)
		if err != nil {
			return s.writeResponse(msg.ID, nil, err)
		}
		return s.writeResponse(msg.ID, edit, nil)
	default:
		result, handled, err := s.handleIDERequest(ctx, msg.Method, msg.Params)
		if !handled {
			return s.writeResponse(msg.ID, nil, lspMethodNotFound(msg.Method))
		}
		if err != nil {
			return s.writeResponse(msg.ID, nil, err)
		}
		return s.writeResponse(msg.ID, result, nil)
	}
}

// restoreClosedWorkspaceDocument replaces the disk-backed index entry after
// an open-document overlay is removed. Analysis is built privately and
// installed only after parsing and context checks complete, so a canceled
// didClose cannot publish a partially restored document.
func (s *languageServer) restoreClosedWorkspaceDocument(ctx context.Context, uri string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := uriToPath(uri)
	if err != nil || !s.workspacePathAllowed(path) || !strings.HasSuffix(path, ".osr") {
		s.mu.Lock()
		delete(s.workspace, uri)
		s.mu.Unlock()
		s.relinkLSPDocumentsContext(ctx)
		return nil
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > lspMaxWorkspaceFileSize {
		s.mu.Lock()
		delete(s.workspace, uri)
		s.mu.Unlock()
		s.relinkLSPDocumentsContext(ctx)
		return nil
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	state := &documentState{uri: uri, sourceName: path, text: string(source)}
	state.prog, err = Parse(path, state.text)
	if err != nil {
		state.diagnostics = []lspDiagnostic{{Range: lspRange{}, Severity: 1, Source: "osiris", Message: err.Error()}}
	} else {
		state.analysis = analyzeDocumentContext(ctx, s.rt, uri, state.prog)
		state.diagnostics = diagnosticsInText(state.text, state.analysis.diagnostics)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if _, open := s.docs[uri]; !open {
		s.workspace[uri] = state
	}
	s.mu.Unlock()
	s.relinkLSPDocumentsContext(ctx)
	return ctx.Err()
}

// upsertDocument operates on languageServer values and keeps the
// languageserver-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (s *languageServer) upsertDocument(uri, text string) {
	_ = s.upsertDocumentContext(context.Background(), uri, text, 0)
}

// upsertDocumentContext parses and checks one document off the server lock,
// then publishes it only if it is newer than the state that was analyzed.
// This is the stale-result boundary for both ordinary incremental edits and
// concurrent workspace refreshes.
func (s *languageServer) upsertDocumentContext(ctx context.Context, uri, text string, version int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	state, err := s.analyzeDocumentState(ctx, uri, text, version)
	if err != nil {
		return err
	}
	return s.publishDocumentStateContext(ctx, uri, state, nil)
}

// publishDocumentStateContext installs one fully analyzed document state. If
// expected is non-nil, the state is published only when that exact document
// snapshot is still current. This expected-snapshot boundary prevents a
// multi-change didChange transaction from applying its private edits to a
// document that another transaction has already replaced.
func (s *languageServer) publishDocumentStateContext(ctx context.Context, uri string, state *documentState, expected *documentState) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return err
	}
	current, exists := s.versions[uri]
	if expected != nil && s.docs[uri] != expected {
		s.mu.Unlock()
		return fmt.Errorf("document changed while applying didChange for %s", uri)
	}
	if exists && state.version > 0 && current > 0 && state.version <= current {
		s.mu.Unlock()
		return fmt.Errorf("stale document version %d for %s; current version is %d", state.version, uri, current)
	}
	s.versions[uri] = state.version
	state.generation = uint64(state.version)
	s.docs[uri] = state
	s.mu.Unlock()
	s.relinkLSPDocumentsContext(ctx)
	_ = s.publishDiagnostics(uri, state.diagnostics)
	return nil
}

// applyDocumentChangesContext validates and applies a complete didChange
// batch against a private text snapshot. Each range is resolved against the
// text produced by the preceding change, but no document state is published
// until every change, analysis step, cancellation check, and stale-snapshot
// check has passed.
func (s *languageServer) applyDocumentChangesContext(ctx context.Context, uri string, version int, changes []lspContentChange) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(changes) == 0 {
		return fmt.Errorf("contentChanges must not be empty")
	}
	s.mu.RLock()
	base := s.docs[uri]
	if base == nil {
		s.mu.RUnlock()
		return fmt.Errorf("document is not open: %s", uri)
	}
	baseText := base.text
	baseVersion := base.version
	s.mu.RUnlock()
	if version > 0 && baseVersion > 0 && version <= baseVersion {
		return fmt.Errorf("stale document version %d for %s; current version is %d", version, uri, baseVersion)
	}
	text, err := applyLSPChangesContext(ctx, baseText, changes)
	if err != nil {
		return err
	}
	state, err := s.analyzeDocumentState(ctx, uri, text, version)
	if err != nil {
		return err
	}
	return s.publishDocumentStateContext(ctx, uri, state, base)
}

// analyzeDocumentState parses and analyzes a document without publishing or
// installing it. Keeping this phase separate lets didSave revalidate the
// editor's saved overlay without inventing a new document version.
func (s *languageServer) analyzeDocumentState(ctx context.Context, uri, text string, version int) (*documentState, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	state := &documentState{uri: uri, sourceName: uri, text: text, version: version}
	if path, err := uriToPath(uri); err == nil {
		state.sourceName = path
	}
	prog, err := Parse(state.sourceName, text)
	if err != nil {
		state.diagnostics = []lspDiagnostic{{Range: lspRange{}, Severity: 1, Source: "osiris", Message: err.Error()}}
	} else {
		state.prog = prog
		state.analysis = analyzeDocumentSourceContextWithModules(ctx, s.rt, uri, text, prog, s.resolveLSPImportedModules(ctx, state, nil))
		state.diagnostics = diagnosticsInText(text, state.analysis.diagnostics)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return state, nil
}

// revalidateDocumentContext handles textDocument/didSave. The optional save
// text is analyzed at the currently open document version; it never advances
// the version because LSP didSave carries no version field. If a didChange or
// didClose wins while analysis is in flight, the saved result is discarded.
// When text is nil, the current overlay is captured under the same read lock
// as the version and generation, so a concurrent change cannot pair stale
// text with a newer version.
func (s *languageServer) revalidateDocumentContext(ctx context.Context, uri string, text *string) error {
	s.mu.RLock()
	current := s.docs[uri]
	if current == nil {
		s.mu.RUnlock()
		return fmt.Errorf("document is not open: %s", uri)
	}
	version, generation := current.version, current.generation
	if text == nil {
		value := current.text
		text = &value
	}
	s.mu.RUnlock()

	state, err := s.analyzeDocumentState(ctx, uri, *text, version)
	if err != nil {
		return err
	}
	s.mu.Lock()
	live := s.docs[uri]
	if live == nil || live.version != version || live.generation != generation {
		s.mu.Unlock()
		return nil
	}
	state.generation = live.generation
	s.docs[uri] = state
	s.mu.Unlock()
	_ = s.publishDiagnostics(uri, state.diagnostics)
	return nil
}

// rangeForURI converts parser rune columns to the UTF-16 positions required by
// LSP. Definitions can point at a different open document, whose text must be
// used for the conversion.
func (s *languageServer) rangeForURI(uri string, sp sourceSpan) lspRange {
	if target := s.documentForAnalysis(uri); target != nil {
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
	return s.hoverContext(context.Background(), uri, pos)
}

// hoverContext resolves one hover response transactionally. The response is
// assembled privately and is published only after the request context has
// remained live through source lookup, markdown construction, and range
// conversion.
func (s *languageServer) hoverContext(ctx context.Context, uri string, pos lspPosition) (*lspHover, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	state := s.document(uri)
	if state == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if state.analysis == nil {
		return nil, nil
	}
	ref, def := state.analysis.findAt(toSourcePosInText(state.text, pos))
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
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
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
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
	return s.definitionContext(context.Background(), uri, pos)
}

func (s *languageServer) declaration(uri string, pos lspPosition) (any, error) {
	return s.declarationContext(context.Background(), uri, pos)
}

func (s *languageServer) declarationContext(ctx context.Context, uri string, pos lspPosition) (any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	state := s.document(uri)
	if state == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if _, ok := lspUTF16Offset(state.text, pos); !ok {
		return nil, lspInvalidParams(fmt.Errorf("position is outside the document or splits a UTF-16 surrogate pair"))
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	return s.definitionContext(ctx, uri, pos)
}

// definitionContext resolves a definition into a private location value and
// performs a final cancellation check before publication. Cross-document
// range conversion therefore cannot leak a partially computed response.
func (s *languageServer) definitionContext(ctx context.Context, uri string, pos lspPosition) (any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	state := s.document(uri)
	if state == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if state.analysis == nil {
		return nil, nil
	}
	ref, def := state.analysis.findAt(toSourcePosInText(state.text, pos))
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	if ref != nil && ref.target != nil {
		if ref.target.uri == "" || (ref.target.span.start.line == 0 && ref.target.span.start.col == 0 && ref.target.span.end.line == 0 && ref.target.span.end.col == 0) {
			return nil, nil
		}
		location := lspLocation{URI: ref.target.uri, Range: s.rangeForURI(ref.target.uri, ref.target.span)}
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		return location, nil
	}
	if def != nil {
		if def.uri == "" || (def.span.start.line == 0 && def.span.start.col == 0 && def.span.end.line == 0 && def.span.end.col == 0) {
			return nil, nil
		}
		location := lspLocation{URI: def.uri, Range: s.rangeForURI(def.uri, def.span)}
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		return location, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	return nil, nil
}

type lspCompletionItem struct {
	Label  string             `json:"label"`
	Kind   int                `json:"kind,omitempty"`
	Detail string             `json:"detail,omitempty"`
	Data   *lspCompletionData `json:"data,omitempty"`
}

type lspCompletionData struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// typedMemberCompletions returns members for a checked local binding when the
// cursor is immediately after its dot. It intentionally requires a proven
// nominal type; dynamic receivers continue through the existing lexical and
// module completion paths rather than presenting speculative members.
func typedMemberCompletions(a *documentAnalysis, prefix string) []lspCompletionItem {
	if a == nil || len(prefix) == 0 || prefix[len(prefix)-1] != '.' {
		return nil
	}
	dot := len(prefix) - 1
	start := dot
	for start > 0 {
		ch := prefix[start-1]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
			start--
			continue
		}
		break
	}
	base := prefix[start:dot]
	if base == "" {
		return nil
	}
	var typeName string
	for i := len(a.defs) - 1; i >= 0; i-- {
		def := a.defs[i]
		if def != nil && def.name == base {
			typeName = def.typeName
			break
		}
	}
	if typeName == "" {
		// A nominal type expression such as User.render is useful even when
		// the receiver is not a local binding.
		typeName = base
	}
	members := a.members[typeName]
	if len(members) == 0 {
		return nil
	}
	items := make([]lspCompletionItem, 0, len(members))
	for name, def := range members {
		kind := 5 // field
		if def.kind == "function" {
			kind = 3 // function
		} else if def.kind == "method" || def.kind == "trait method" {
			kind = 2 // method
		} else if def.kind == "constant" {
			kind = 21 // constant
		}
		item := lspCompletionItem{Label: name, Kind: kind, Detail: def.detail}
		if def.uri != "" && !strings.HasPrefix(def.uri, "osiris://") {
			item.Data = &lspCompletionData{URI: def.uri, Name: def.name}
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	return items
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
	return s.completionContext(context.Background(), uri, pos)
}

// completionContext builds a deterministic completion array in private
// storage. Cancellation is checked around typed/module lookups and while
// collecting lexical definitions; a canceled request always returns a nil
// result rather than an incomplete array.
func (s *languageServer) completionContext(ctx context.Context, uri string, pos lspPosition) ([]lspCompletionItem, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	state := s.document(uri)
	if state == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if state.analysis == nil {
		return []lspCompletionItem{}, nil
	}
	if offset, ok := lspUTF16Offset(state.text, pos); ok && offset > 0 && state.prog != nil {
		prefix := state.text[:offset]
		if strings.HasSuffix(prefix, ".") {
			if items := typedMemberCompletions(state.analysis, prefix); items != nil {
				if err := ctx.Err(); err != nil {
					return nil, lspRequestCancelled(err)
				}
				return items, nil
			}
			aliasEnd := len(prefix) - 1
			aliasStart := aliasEnd
			for aliasStart > 0 {
				r, size := utf8.DecodeLastRuneInString(prefix[:aliasStart])
				if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
					break
				}
				aliasStart -= size
			}
			alias := prefix[aliasStart:aliasEnd]
			for _, stmt := range state.prog.Statements {
				if err := ctx.Err(); err != nil {
					return nil, lspRequestCancelled(err)
				}
				imp, ok := stmt.(importStmt)
				if !ok || imp.Alias != alias || !strings.HasPrefix(imp.Path, "std/") {
					continue
				}
				ref, selector := splitModuleRef(imp.Path)
				if modValue, err := stdlibModule(s.rt, ref, selector); err == nil {
					if mod, err := modValue.AsModule(); err == nil {
						items := make([]lspCompletionItem, 0, len(mod.members))
						for member := range mod.members {
							if err := ctx.Err(); err != nil {
								return nil, lspRequestCancelled(err)
							}
							items = append(items, lspCompletionItem{Label: member, Kind: 3, Detail: "module member"})
						}
						sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
						if err := ctx.Err(); err != nil {
							return nil, lspRequestCancelled(err)
						}
						return items, nil
					}
				}
			}
		}
	}
	seen := make(map[string]lspCompletionItem)
	for _, def := range state.analysis.defs {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
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
		case "type parameter":
			kind = 25 // LSP CompletionItemKind.TypeParameter
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
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	return items, nil
}

func (s *languageServer) references(uri string, pos lspPosition) ([]lspLocation, error) {
	return s.referencesContext(context.Background(), uri, pos)
}

func (s *languageServer) referencesContext(ctx context.Context, uri string, pos lspPosition) ([]lspLocation, error) {
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
	state := s.document(uri)
	if state == nil {
		return nil, fmt.Errorf("document not open: %s", uri)
	}
	if _, ok := lspUTF16Offset(state.text, pos); !ok {
		return nil, lspInvalidParams(fmt.Errorf("invalid UTF-16 position"))
	}
	if state.analysis == nil {
		if err := cancelled(); err != nil {
			return nil, err
		}
		return []lspLocation{}, nil
	}
	ref, target := state.analysis.findAt(toSourcePosInText(state.text, pos))
	if err := cancelled(); err != nil {
		return nil, err
	}
	if ref != nil {
		target = ref.target
	}
	if target == nil {
		return []lspLocation{}, nil
	}
	documents := s.allDocuments()
	locations := make([]lspLocation, 0, len(documents))
	if target.uri != "" && !strings.HasPrefix(target.uri, "osiris://stdlib/") && target.span.start.line != 0 {
		locations = append(locations, lspLocation{URI: target.uri, Range: s.rangeForURI(target.uri, target.span)})
	}
	for _, doc := range documents {
		if err := cancelled(); err != nil {
			return nil, err
		}
		if doc == nil || doc.analysis == nil {
			continue
		}
		for _, candidate := range doc.analysis.refs {
			if err := cancelled(); err != nil {
				return nil, err
			}
			if candidate == nil || candidate.target != target || candidate.span.start.line == 0 {
				continue
			}
			locations = append(locations, lspLocation{URI: doc.uri, Range: s.rangeForURI(doc.uri, candidate.span)})
		}
	}
	sort.SliceStable(locations, func(i, j int) bool {
		if locations[i].URI != locations[j].URI {
			return locations[i].URI < locations[j].URI
		}
		if locations[i].Range.Start.Line != locations[j].Range.Start.Line {
			return locations[i].Range.Start.Line < locations[j].Range.Start.Line
		}
		if locations[i].Range.Start.Character != locations[j].Range.Start.Character {
			return locations[i].Range.Start.Character < locations[j].Range.Start.Character
		}
		if locations[i].Range.End.Line != locations[j].Range.End.Line {
			return locations[i].Range.End.Line < locations[j].Range.End.Line
		}
		return locations[i].Range.End.Character < locations[j].Range.End.Character
	})
	if err := cancelled(); err != nil {
		return nil, err
	}
	return locations, nil
}

func (s *languageServer) signatureHelp(uri string, pos lspPosition) (*lspSignatureHelp, error) {
	return s.signatureHelpContext(context.Background(), uri, pos, nil)
}

func (s *languageServer) signatureHelpContext(ctx context.Context, uri string, pos lspPosition, expectedVersion *int) (*lspSignatureHelp, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	if uri == "" {
		return nil, lspInvalidParams(fmt.Errorf("textDocument.uri is required"))
	}
	state := s.document(uri)
	if state == nil {
		return nil, lspInvalidParams(fmt.Errorf("document is not open: %s", uri))
	}
	if err := s.checkDocumentVersion(state, expectedVersion); err != nil {
		return nil, err
	}
	if state.analysis == nil {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		return nil, nil
	}
	offset, ok := lspUTF16Offset(state.text, pos)
	if !ok {
		return nil, lspInvalidParams(fmt.Errorf("invalid signature-help position"))
	}
	prefix := state.text[:offset]
	open, activeParameter, ok, err := lspActiveCallContext(ctx, prefix)
	if err != nil {
		return nil, lspRequestCancelled(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	// Some clients request help at the opening-parenthesis position rather
	// than immediately after it. Treat that boundary as the start of the call
	// while keeping the normal nested-call scan for positions inside it.
	if !ok && offset < len(state.text) && state.text[offset] == '(' {
		open, activeParameter, ok = offset, 0, true
	}
	if !ok {
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
	var parameters []lspSignatureParameter
	if state.prog != nil {
		if fn, found := findLSPFunction(state.prog, name); found {
			detail = functionDetail(fn)
			parameters = lspSignatureParameters(fn)
		}
	}
	for _, def := range state.analysis.defs {
		if detail == "" && def != nil && def.name == name && def.detail != "" {
			detail = def.detail
			break
		}
	}
	if detail == "" {
		return nil, nil
	}
	if activeParameter >= len(parameters) && len(parameters) > 0 {
		activeParameter = len(parameters) - 1
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	return &lspSignatureHelp{
		Signatures:      []lspSignatureInformation{{Label: detail, Parameters: parameters}},
		ActiveSignature: 0,
		ActiveParameter: activeParameter,
	}, nil
}

// lspActiveCall finds the innermost unterminated call at the cursor and
// returns its opening parenthesis and zero-based argument index. Scanning the
// complete prefix, instead of taking the last '(' byte, keeps signature help
// correct for nested calls, commas in nested calls, and quoted strings.
func lspActiveCall(prefix string) (open int, activeParameter int, ok bool) {
	open, activeParameter, ok, _ = lspActiveCallContext(context.Background(), prefix)
	return open, activeParameter, ok
}

func lspActiveCallContext(ctx context.Context, prefix string) (open int, activeParameter int, ok bool, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	stack := make([]int, 0, 4)
	args := make([]int, 0, 4)
	inString := false
	escaped := false
	for i, r := range prefix {
		if err := ctx.Err(); err != nil {
			return 0, 0, false, err
		}
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
			} else if r == '"' {
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '(':
			stack = append(stack, i)
			args = append(args, 0)
		case ')':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
				args = args[:len(args)-1]
			}
		case ',':
			if len(args) > 0 {
				args[len(args)-1]++
			}
		}
	}
	if len(stack) == 0 {
		return 0, 0, false, nil
	}
	return stack[len(stack)-1], args[len(args)-1], true, nil
}

func findLSPFunction(program *Program, name string) (funcStmt, bool) {
	if program == nil {
		return funcStmt{}, false
	}
	for _, stmt := range program.Statements {
		switch st := stmt.(type) {
		case funcStmt:
			if st.Name == name {
				return st, true
			}
		case implStmt:
			for _, method := range st.Methods {
				if method.Name == name {
					return method, true
				}
			}
		case traitStmt:
			for _, method := range st.Methods {
				if method.Name == name {
					return method, true
				}
			}
		}
	}
	return funcStmt{}, false
}

func lspSignatureParameters(fn funcStmt) []lspSignatureParameter {
	parameters := make([]lspSignatureParameter, 0, len(fn.Params))
	for i, name := range fn.Params {
		label := name
		if i < len(fn.ParamTypes) && fn.ParamTypes[i] != "" {
			label += " " + fn.ParamTypes[i]
		}
		parameters = append(parameters, lspSignatureParameter{Label: label})
	}
	return parameters
}

// RenameDocument returns a formatted, dry-run rewrite of the symbol at pos.
// It never mutates the open document. Reference selection uses the semantic
// analysis target pointer, so same-spelled shadowed bindings are excluded.
func (s *languageServer) RenameDocument(uri string, pos lspPosition, replacement string) (string, error) {
	if !validIdentifier(replacement) {
		return "", fmt.Errorf("invalid replacement identifier %q", replacement)
	}
	state := s.document(uri)
	if state == nil || state.analysis == nil {
		return "", fmt.Errorf("document not open: %s", uri)
	}
	ref, target := state.analysis.findAt(toSourcePosInText(state.text, pos))
	if ref != nil {
		target = ref.target
	}
	if !sourceBackedRenameTarget(uri, state.text, target) {
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
	for i := 1; i < len(edits); i++ {
		if edits[i].end > edits[i-1].start {
			return "", fmt.Errorf("overlapping rename source spans")
		}
	}
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

// reportInvalidParams keeps malformed requests inside the JSON-RPC session.
// Notifications have no response channel, so they are logged and ignored;
// requests receive the standard -32602 error and the server continues with
// the next framed message.
func (s *languageServer) reportInvalidParams(msg lspMessage, err error) error {
	if msg.IsRequest() {
		return s.writeResponse(msg.ID, nil, lspInvalidParams(err))
	}
	_, logErr := fmt.Fprintf(s.log, "osiris lsp: ignored malformed %s notification: %v\n", msg.Method, err)
	return logErr
}

// writeResponse operates on languageServer values and keeps the
// languageserver-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns an error when the operation cannot complete cleanly.
func (s *languageServer) writeResponse(id json.RawMessage, result any, rpcErr error) error {
	resp := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id)}
	if rpcErr != nil {
		code := -32603
		if coded, ok := rpcErr.(interface{ LSPCode() int }); ok {
			code = coded.LSPCode()
		}
		resp["error"] = map[string]any{"code": code, "message": rpcErr.Error()}
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
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n%s", len(data), data)
	return err
}

type lspMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      json.RawMessage `json:"id"`
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
	foundLength := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return lspMessage{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "content-length") {
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || parsed < 0 {
				return lspMessage{}, fmt.Errorf("invalid Content-Length header")
			}
			length, foundLength = parsed, true
		}
	}
	if !foundLength {
		return lspMessage{}, fmt.Errorf("missing Content-Length header")
	}
	if length == 0 {
		return lspMessage{}, fmt.Errorf("empty LSP message")
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
		URI     string `json:"uri"`
		Text    string `json:"text"`
		Version int    `json:"version"`
	} `json:"textDocument"`
}

type didChangeParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version int    `json:"version"`
	} `json:"textDocument"`
	ContentChanges []lspContentChange `json:"contentChanges"`
}

type lspContentChange struct {
	Range *lspRange `json:"range,omitempty"`
	Text  string    `json:"text"`
}

type didSaveParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Text *string `json:"text,omitempty"`
}

type renameParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int   `json:"version,omitempty"`
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

// applyLSPChangesContext applies a complete change batch to private text.
// Ranges in later changes address the text produced by earlier changes, as
// required by the LSP incremental synchronization contract.
func applyLSPChangesContext(ctx context.Context, text string, changes []lspContentChange) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for index, change := range changes {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		updated, err := applyLSPChange(text, change.Range, change.Text)
		if err != nil {
			return "", fmt.Errorf("content change %d: %w", index, err)
		}
		text = updated
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return text, nil
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

type didChangeWatchedFilesParams struct {
	Changes []watchedFileChange `json:"changes"`
}

type watchedFileChange struct {
	URI  string `json:"uri"`
	Type int    `json:"type"`
}

func validateWatchedFileChanges(changes []watchedFileChange) error {
	for i, change := range changes {
		if change.URI == "" {
			return fmt.Errorf("changes[%d].uri is required", i)
		}
		if change.Type < 1 || change.Type > 3 {
			return fmt.Errorf("changes[%d].type must be 1, 2, or 3", i)
		}
		parsed, err := url.Parse(change.URI)
		if err != nil || parsed.Scheme != "file" || parsed.Path == "" {
			if err == nil {
				err = fmt.Errorf("file URI is required")
			}
			return fmt.Errorf("changes[%d].uri is invalid: %w", i, err)
		}
	}
	return nil
}

type hoverParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int   `json:"version,omitempty"`
	} `json:"textDocument"`
	Position lspPosition `json:"position"`
}

type definitionParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int   `json:"version,omitempty"`
	} `json:"textDocument"`
	Position lspPosition `json:"position"`
}

type referencesParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int   `json:"version,omitempty"`
	} `json:"textDocument"`
	Position lspPosition `json:"position"`
}

type completionParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int   `json:"version,omitempty"`
	} `json:"textDocument"`
	Position lspPosition `json:"position"`
}

type signatureHelpParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int   `json:"version,omitempty"`
	} `json:"textDocument"`
	Position *lspPosition `json:"position"`
}

// analyzeDocument handles the analyze document step in the surrounding package
// and returns *documentAnalysis.
//
// It returns *documentAnalysis so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func analyzeDocument(rt *Runtime, uri string, prog *Program) *documentAnalysis {
	return analyzeDocumentContext(context.Background(), rt, uri, prog)
}

func analyzeDocumentContext(ctx context.Context, rt *Runtime, uri string, prog *Program) *documentAnalysis {
	return analyzeDocumentSourceContext(ctx, rt, uri, "", prog)
}

// analyzeDocumentSourceContext is the source-aware semantic entry point. The
// parser stores normalized type names while the LSP needs source-backed
// ranges, so retaining the original text lets generic type references resolve
// to the exact identifier instead of the enclosing parameter declaration.
func analyzeDocumentSourceContext(ctx context.Context, rt *Runtime, uri, source string, prog *Program) *documentAnalysis {
	return analyzeDocumentSourceContextWithModules(ctx, rt, uri, source, prog, nil)
}

func analyzeDocumentSourceContextWithModules(ctx context.Context, rt *Runtime, uri, source string, prog *Program, modules map[string]lspResolvedModule) *documentAnalysis {
	a := &documentAnalysis{enumCases: make(map[string]*symbolDef), members: make(map[string]map[string]*symbolDef), source: source}
	if ctx == nil {
		ctx = context.Background()
	}
	if rt == nil {
		rt = NewRuntime()
	}
	// Type diagnostics consume the same post-expansion checked program as
	// execution backends. Navigation keeps the surface Program until expansion
	// origin chains become node-addressable, preserving macro call locations.
	semantic := prog
	frontendTyped, frontendErr := rt.frontendProgram(ctx, prog)
	if frontendErr != nil {
		span := sourceSpan{}
		message := frontendErr.Error()
		if typedErr, ok := frontendErr.(*OsirisError); ok {
			message = typedErr.message
			if typedErr.location != nil {
				span = typedErr.location.Span
			}
		}
		a.diagnostics = append(a.diagnostics, diagnosticAt(span, message))
	} else {
		a.checked = frontendTyped.checked
	}
	root := &lspScope{defs: map[string]*symbolDef{}}
	for name := range rt.Builtins {
		root.defs[name] = &symbolDef{name: name, kind: "builtin", detail: "builtin function", uri: "", span: sourceSpan{}}
	}
	// Imports are predeclared before semantic expressions are analyzed. This
	// lets the module index install source-backed members early enough for
	// member expressions to create references to the shared exported symbols.
	for _, stmt := range semantic.Statements {
		st, ok := stmt.(importStmt)
		if !ok || st.Alias == "" {
			continue
		}
		def := &symbolDef{name: st.Alias, kind: "module", detail: importDetail(st), uri: uri, span: st.AliasSpan}
		root.defs[st.Alias] = def
		addAnalysisDefinition(a, def)
	}
	indexLSPImportedModules(a, root, rt, semantic.Statements)
	indexLSPImportedUserModules(a, root, semantic.Statements, modules)
	analysisScope, _ := analyzeStmtSeq(a, root, uri, semantic.Statements, false)
	// analyzeStmtSeq may create a function scope above root. The import aliases
	// remain visible through that parent scope, while this second pass keeps the
	// final scope's aliases consistent for callers that inspect it directly.
	indexLSPImportedModules(a, analysisScope, rt, semantic.Statements)
	indexLSPImportedUserModules(a, analysisScope, semantic.Statements, modules)
	resolveCheckedMemberReferences(a)
	if a.checked == nil {
		checked, checkErr := rt.CheckProgram(ctx, semantic)
		a.checked = checked
		if checkErr != nil {
			span := sourceSpan{}
			message := checkErr.Error()
			if typed, ok := checkErr.(*OsirisError); ok {
				message = typed.message
				if typed.location != nil {
					span = typed.location.Span
				}
			}
			a.diagnostics = append(a.diagnostics, diagnosticAt(span, message))
		}
	}
	if a.checked != nil {
		for _, binding := range a.checked.Bindings {
			typeName := a.checked.DisplayType(binding, TypeFormatDiagnostic)
			for _, def := range a.defs {
				if def.name == binding.Name && def.span == binding.Span {
					def.typeName = typeName
					if typeName != "" && !strings.Contains(def.detail, typeName) {
						def.detail += " : " + typeName
					}
				}
			}
		}
		resolveCheckedMemberReferences(a)
	}
	if frontendTyped != nil {
		for _, def := range a.defs {
			for _, symbol := range frontendTyped.resolved.Symbols {
				if def.name == symbol.Name && def.span == symbol.Span {
					def.symbolID = symbol.ID
					break
				}
			}
		}
	}
	return a
}

// resolveCheckedMemberReferences repairs member targets after the shared
// checker has attached nominal types to local bindings. LSP analysis runs
// before checking so it can still produce diagnostics for incomplete source;
// this second pass upgrades only the member references that are now provably
// tied to a struct or implementation method.
func resolveCheckedMemberReferences(a *documentAnalysis) {
	if a == nil || len(a.members) == 0 {
		return
	}
	for _, ref := range a.refs {
		if ref == nil || ref.target == nil || !strings.HasPrefix(ref.detail, "member ") || ref.target.typeName == "" {
			continue
		}
		member := strings.TrimPrefix(ref.detail, "member ")
		if def := a.members[ref.target.typeName][member]; def != nil {
			ref.target = def
		}
	}
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
		predeclareAnalysisStmt(a, functions, uri, stmt)
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
		def := analysisNamedDefinition(scope, st.Name, "package", packageDetail(st), uri, st.NameSpan)
		addAnalysisDefinition(a, def)
		return scope.push(def), nil
	case importStmt:
		def := analysisNamedDefinition(scope, st.Alias, "module", importDetail(st), uri, st.AliasSpan)
		addAnalysisDefinition(a, def)
		return scope.push(def), nil
	case enumStmt:
		def := analysisNamedDefinition(scope, st.Name, "enum", fmt.Sprintf("enum with %d cases", len(st.Cases)), uri, st.NameSpan)
		addAnalysisDefinition(a, def)
		for _, item := range st.Cases {
			caseDef := &symbolDef{name: st.Name + "." + item.Name, kind: "enum case", detail: "case of " + st.Name, uri: uri, span: item.Span}
			if existing := a.enumCases[caseDef.name]; existing != nil {
				caseDef = existing
			}
			addAnalysisDefinition(a, caseDef)
			a.enumCases[caseDef.name] = caseDef
		}
		return scope.push(def), nil
	case structStmt:
		def := analysisNamedDefinition(scope, st.Name, "struct", fmt.Sprintf("struct with %d fields", len(st.Fields)), uri, st.NameSpan)
		addAnalysisDefinition(a, def)
		for _, field := range st.Fields {
			analyzeTypeReference(a, scope, uri, field.Type, field.TypeSpan, "struct field type")
			addLSPMember(a, st.Name, &symbolDef{
				name: field.Name, kind: "field", detail: "field " + field.Type,
				uri: uri, span: field.NameSpan, typeName: field.Type,
			})
		}
		return scope.push(def), nil
	case typeDeclStmt:
		def := analysisNamedDefinition(scope, st.Name, "type alias", "alias of "+st.Target, uri, st.NameSpan)
		addAnalysisDefinition(a, def)
		return scope.push(def), nil
	case newtypeStmt:
		def := analysisNamedDefinition(scope, st.Name, "newtype", "newtype of "+st.Target, uri, st.NameSpan)
		addAnalysisDefinition(a, def)
		return scope.push(def), nil
	case letStmt:
		if err := analyzeExpr(a, scope, uri, st.Expr); err != nil {
			return nil, err
		}
		if len(st.Names) > 0 {
			for _, name := range st.Names {
				item := scope.lookup(name)
				if item == nil || item.kind != "variable" {
					item = &symbolDef{name: name, kind: "variable", detail: lspBindingDetail(st, true), uri: uri, span: st.Span, mutable: st.Mutable, constant: st.Const}
				} else {
					item.detail = lspBindingDetail(st, true)
					item.mutable = st.Mutable
					item.constant = st.Const
				}
				addAnalysisDefinition(a, item)
				scope = scope.push(item)
			}
			return scope, nil
		}
		def := scope.lookup(st.Name)
		if def == nil || def.kind != "variable" {
			def = &symbolDef{name: st.Name, kind: "variable", detail: lspBindingDetail(st, false), uri: uri, span: st.NameSpan, mutable: st.Mutable, constant: st.Const}
		} else {
			def.detail = lspBindingDetail(st, false)
			def.mutable = st.Mutable
			def.constant = st.Const
		}
		addAnalysisDefinition(a, def)
		return scope.push(def), nil
	case assignStmt:
		if def := scope.lookup(st.Name); def != nil {
			a.refs = append(a.refs, &symbolRef{span: st.NameSpan, target: def, detail: "assignment"})
			if !def.mutable {
				a.diagnostics = append(a.diagnostics, diagnosticAt(st.NameSpan, fmt.Sprintf("cannot assign to immutable binding %q", st.Name)))
			}
		} else {
			a.diagnostics = append(a.diagnostics, diagnosticAt(st.NameSpan, fmt.Sprintf("undefined assignment target %q", st.Name)))
		}
		return scope, analyzeExpr(a, scope, uri, st.Expr)
	case emitStmt:
		return scope, analyzeExpr(a, scope, uri, st.Expr)
	case macroStmt:
		def := scope.lookup(st.Name)
		if def == nil || def.kind != "macro" {
			def = &symbolDef{name: st.Name, kind: "macro", detail: "compile-time macro", uri: uri, span: st.NameSpan}
		}
		addAnalysisDefinition(a, def)
		child := &lspScope{parent: scope, defs: map[string]*symbolDef{}}
		paramSpans := lspMacroParameterSpans(a.source, st)
		for i, param := range st.Params {
			span := st.Span
			if i < len(paramSpans) {
				span = paramSpans[i]
			}
			paramDef := &symbolDef{name: param, kind: "param", detail: "macro parameter", uri: uri, span: span}
			child.defs[param] = paramDef
			addAnalysisDefinition(a, paramDef)
		}
		if err := analyzeExpr(a, child, uri, st.Body); err != nil {
			return nil, err
		}
		return scope.push(def), nil
	case traitStmt:
		def := analysisNamedDefinition(scope, st.Name, "trait", fmt.Sprintf("trait with %d methods", len(st.Methods)), uri, st.NameSpan)
		addAnalysisDefinition(a, def)
		traitScope := &lspScope{parent: scope, defs: map[string]*symbolDef{}}
		for _, param := range st.TypeParams {
			paramDef := lspTypeParameterDefinition(uri, param)
			traitScope.defs[param.Name] = paramDef
			addAnalysisDefinition(a, paramDef)
		}
		for _, method := range st.Methods {
			addLSPMember(a, st.Name, &symbolDef{name: method.Name, kind: "trait method", detail: functionDetail(method), uri: uri, span: method.NameSpan})
			analyzeGenericFunctionSignature(a, traitScope, uri, method, st.TypeParams)
		}
		return scope.push(def), nil
	case implStmt:
		analyzeTypeReference(a, scope, uri, st.Trait, st.TraitSpan, "trait")
		analyzeTypeReference(a, scope, uri, st.Target, st.TargetSpan, "implementation target")
		for _, method := range st.Methods {
			addLSPMember(a, st.Target, &symbolDef{name: method.Name, kind: "method", detail: functionDetail(method), uri: uri, span: method.NameSpan, typeName: st.Target})
			if _, err := analyzeStmt(a, scope, uri, method, inBlock); err != nil {
				return nil, err
			}
		}
		return scope, nil
	case funcStmt:
		def := scope.lookup(st.Name)
		predeclared := def != nil && def.kind == "function" && def.span == st.NameSpan
		if st.ReceiverType != "" {
			if member := a.members[st.ReceiverType][st.Name]; member != nil {
				def = member
				predeclared = true
			}
		}
		if !predeclared {
			def = &symbolDef{name: st.Name, kind: "function", detail: functionDetail(st), uri: uri, span: st.NameSpan}
		}
		addAnalysisDefinition(a, def)
		child := &lspScope{parent: scope, defs: map[string]*symbolDef{st.Name: def}}
		for _, param := range st.TypeParams {
			paramDef := lspTypeParameterDefinition(uri, param)
			child.defs[param.Name] = paramDef
			addAnalysisDefinition(a, paramDef)
		}
		analyzeGenericFunctionSignature(a, child, uri, st, st.TypeParams)
		for i, param := range st.Params {
			detail := "function parameter"
			if i < len(st.ParamTypes) && st.ParamTypes[i] != "" {
				detail += " " + st.ParamTypes[i]
			}
			span := st.Span
			if i < len(st.ParamSpans) {
				span = st.ParamSpans[i]
			}
			paramDef := &symbolDef{name: param, kind: "param", detail: detail, uri: uri, span: span}
			child.defs[param] = paramDef
			addAnalysisDefinition(a, paramDef)
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
	case deferStmt:
		if st.Body == nil {
			a.diagnostics = append(a.diagnostics, diagnosticAt(st.Span, "defer statement has no body"))
			return scope, nil
		}
		return scope, analyzeExpr(a, scope, uri, st.Body)
	default:
		a.diagnostics = append(a.diagnostics, diagnosticAt(lspStmtSpan(stmt), fmt.Sprintf("LSP analysis does not support statement %T", stmt)))
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
		analyzeGenericCallTypeArguments(a, scope, n)
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
		analyzeTypeReference(a, scope, uri, n.TypeName, n.Span, "struct")
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
			analyzePattern(a, scope, uri, arm.pattern)
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
		a.diagnostics = append(a.diagnostics, diagnosticAt(lspExprSpan(expr), fmt.Sprintf("LSP analysis does not support expression %T", expr)))
		return nil
	}
	return nil
}

// analyzeGenericCallTypeArguments connects explicit generic call arguments to
// an in-scope type parameter. This is intentionally limited to type parameters
// already proven by the lexical scope; ordinary concrete types remain checker
// data and do not acquire speculative LSP definitions.
func analyzeGenericCallTypeArguments(a *documentAnalysis, scope *lspScope, call *callExpr) {
	if a == nil || scope == nil || call == nil || len(call.typeArgs) == 0 {
		return
	}
	for _, typeArg := range call.typeArgs {
		for _, param := range strings.FieldsFunc(typeArg, func(r rune) bool {
			return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')
		}) {
			def := scope.lookup(param)
			if def == nil || def.kind != "type parameter" {
				continue
			}
			for _, span := range lspSourceIdentifierSpans(a.source, call.Span, param, 0) {
				a.refs = append(a.refs, &symbolRef{span: span, target: def, detail: "generic type argument"})
			}
		}
	}
}

func predeclareAnalysisStmt(a *documentAnalysis, scope *lspScope, uri string, stmt Stmt) {
	switch st := stmt.(type) {
	case funcStmt:
		scope.defs[st.Name] = &symbolDef{name: st.Name, kind: "function", detail: functionDetail(st), uri: uri, span: st.NameSpan}
	case enumStmt:
		scope.defs[st.Name] = &symbolDef{name: st.Name, kind: "enum", detail: fmt.Sprintf("enum with %d cases", len(st.Cases)), uri: uri, span: st.NameSpan}
		for _, item := range st.Cases {
			def := &symbolDef{name: st.Name + "." + item.Name, kind: "enum case", detail: "case of " + st.Name, uri: uri, span: item.Span}
			a.enumCases[def.name] = def
			addAnalysisDefinition(a, def)
		}
	case structStmt:
		scope.defs[st.Name] = &symbolDef{name: st.Name, kind: "struct", detail: fmt.Sprintf("struct with %d fields", len(st.Fields)), uri: uri, span: st.NameSpan}
	case typeDeclStmt:
		scope.defs[st.Name] = &symbolDef{name: st.Name, kind: "type alias", detail: "alias of " + st.Target, uri: uri, span: st.NameSpan}
	case newtypeStmt:
		scope.defs[st.Name] = &symbolDef{name: st.Name, kind: "newtype", detail: "newtype of " + st.Target, uri: uri, span: st.NameSpan}
	case traitStmt:
		scope.defs[st.Name] = &symbolDef{name: st.Name, kind: "trait", detail: fmt.Sprintf("trait with %d methods", len(st.Methods)), uri: uri, span: st.NameSpan}
	case macroStmt:
		// Macros participate in lexical source navigation before their body is
		// analyzed. Predeclaring them also makes forward and nested macro calls
		// resolve through the same scope used by the shared parser/checker.
		scope.defs[st.Name] = &symbolDef{name: st.Name, kind: "macro", detail: "compile-time macro", uri: uri, span: st.NameSpan}
	case letStmt:
		if st.Name != "" {
			scope.defs[st.Name] = &symbolDef{name: st.Name, kind: "variable", detail: lspBindingDetail(st, false), uri: uri, span: st.NameSpan, mutable: st.Mutable, constant: st.Const}
		}
		for _, name := range st.Names {
			scope.defs[name] = &symbolDef{name: name, kind: "variable", detail: lspBindingDetail(st, true), uri: uri, span: st.Span, mutable: st.Mutable, constant: st.Const}
		}
	}
}

func lspBindingDetail(st letStmt, tuple bool) string {
	detail := "immutable binding"
	if st.Const {
		detail = "constant binding"
	} else if st.Mutable {
		detail = "mutable binding"
	}
	if tuple {
		detail = strings.Replace(detail, " binding", " tuple binding", 1)
	}
	return detail
}

func analysisNamedDefinition(scope *lspScope, name, kind, detail, uri string, span sourceSpan) *symbolDef {
	if existing := scope.lookup(name); existing != nil && existing.kind == kind && existing.span == span {
		return existing
	}
	return &symbolDef{name: name, kind: kind, detail: detail, uri: uri, span: span}
}

func addAnalysisDefinition(a *documentAnalysis, def *symbolDef) {
	if def == nil {
		return
	}
	for _, existing := range a.defs {
		if existing == def || (existing.name == def.name && existing.kind == def.kind && existing.uri == def.uri && existing.span == def.span) {
			return
		}
	}
	a.defs = append(a.defs, def)
}

// addLSPMember indexes one source-backed member under its nominal owner. The
// same symbol is also added to the document definition set so hover, go-to-
// definition, and references all share one identity.
func addLSPMember(a *documentAnalysis, owner string, def *symbolDef) {
	if a == nil || owner == "" || def == nil || def.name == "" {
		return
	}
	if a.members == nil {
		a.members = make(map[string]map[string]*symbolDef)
	}
	if a.members[owner] == nil {
		a.members[owner] = make(map[string]*symbolDef)
	}
	if existing := a.members[owner][def.name]; existing != nil {
		return
	}
	a.members[owner][def.name] = def
	addAnalysisDefinition(a, def)
}

// lspExportedModuleMembers returns the definitions named by export statements
// in a parsed module. The returned pointers are deliberately borrowed from
// the module analysis so importing documents can share identity with the
// defining document.
func lspExportedModuleMembers(doc *documentState) map[string]*symbolDef {
	if doc == nil || doc.analysis == nil {
		return nil
	}
	exports := make(map[string]*symbolDef)
	for _, ref := range doc.analysis.refs {
		if ref == nil || ref.detail != "export" || ref.target == nil || ref.target.name == "" {
			continue
		}
		exports[ref.target.name] = ref.target
	}
	return exports
}

// lspTypeParameterDefinition creates a source-backed generic parameter
// symbol. Parser typeParam spans include the optional bound; the declaration
// symbol itself is narrowed to the parameter identifier.
func lspTypeParameterDefinition(uri string, param typeParam) *symbolDef {
	span := param.Span
	if span.start.line != 0 && span.start.col != 0 {
		span.end = sourcePos{line: span.start.line, col: span.start.col + len(param.Name)}
	}
	detail := "type parameter"
	if param.Bound != "" {
		detail += " bounded by " + param.Bound
	}
	return &symbolDef{name: param.Name, kind: "type parameter", detail: detail, uri: uri, span: span, typeName: param.Name}
}

// analyzeGenericFunctionSignature indexes generic type-parameter references
// in bounds, parameter types, and return types. The checker remains the source
// of type validity; this pass only connects the already parsed names to LSP
// definitions and references.
func analyzeGenericFunctionSignature(a *documentAnalysis, scope *lspScope, uri string, fn funcStmt, params []typeParam) {
	if a == nil {
		return
	}
	for _, param := range params {
		if param.Bound != "" {
			addGenericTypeReferences(a, scope, uri, param.Bound, param.Span, len(param.Name), params, "generic bound")
		}
	}
	for i, typeName := range fn.ParamTypes {
		if typeName == "" || i >= len(fn.ParamSpans) {
			continue
		}
		skip := 0
		if i < len(fn.Params) {
			skip = len(fn.Params[i])
		}
		if len(params) == 0 || !containsAnyGenericTypeParameter(typeName, params) {
			analyzeTypeReference(a, scope, uri, typeName, lspParameterTypeSpan(fn.ParamSpans[i], fn.Params, i, typeName), "parameter type")
		} else {
			addGenericTypeReferences(a, scope, uri, typeName, fn.ParamSpans[i], skip, params, "generic parameter type")
		}
	}
	if fn.ReturnType != "" {
		if len(params) == 0 || !containsAnyGenericTypeParameter(fn.ReturnType, params) {
			analyzeTypeReference(a, scope, uri, fn.ReturnType, fn.ReturnTypeSpan, "return type")
		} else {
			addGenericTypeReferences(a, scope, uri, fn.ReturnType, fn.ReturnTypeSpan, 0, params, "generic return type")
		}
	}
}

func lspParameterTypeSpan(region sourceSpan, params []string, index int, typeName string) sourceSpan {
	if index >= 0 && index < len(params) && region.start.line == region.end.line {
		start := region.start
		start.col += len(params[index]) + 1
		return sourceSpan{start: start, end: sourcePos{line: start.line, col: start.col + len(typeName)}}
	}
	return region
}

func containsAnyGenericTypeParameter(typeName string, params []typeParam) bool {
	for _, param := range params {
		if lspTypeContainsIdentifier(typeName, param.Name) {
			return true
		}
	}
	return false
}

func addGenericTypeReferences(a *documentAnalysis, scope *lspScope, uri, typeName string, region sourceSpan, skip int, params []typeParam, detail string) {
	for _, param := range params {
		def := scope.lookup(param.Name)
		if def == nil || !lspTypeContainsIdentifier(typeName, param.Name) {
			continue
		}
		spans := lspSourceIdentifierSpans(a.source, region, param.Name, skip)
		if len(spans) == 0 && a.source == "" && strings.TrimSpace(typeName) == param.Name {
			spans = []sourceSpan{region}
		}
		for _, span := range spans {
			a.refs = append(a.refs, &symbolRef{span: span, target: def, detail: detail})
		}
	}
}

func lspTypeContainsIdentifier(typeName, name string) bool {
	for offset := 0; offset < len(typeName); {
		at := strings.Index(typeName[offset:], name)
		if at < 0 {
			return false
		}
		at += offset
		if lspIdentifierBoundary(typeName, at, at+len(name)) {
			return true
		}
		offset = at + len(name)
	}
	return false
}

func lspIdentifierBoundary(text string, start, end int) bool {
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(text[:start])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			return false
		}
	}
	if end < len(text) {
		r, _ := utf8.DecodeRuneInString(text[end:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			return false
		}
	}
	return true
}

// lspSourceIdentifierSpans finds all exact identifier occurrences inside one
// parser span. Source spans are byte-column based, so this remains correct for
// Unicode text and the final conversion to UTF-16 stays at the protocol edge.
func lspSourceIdentifierSpans(source string, region sourceSpan, name string, skip int) []sourceSpan {
	if source == "" || name == "" || region.start.line < 1 || region.start.line > len(strings.Split(source, "\n")) {
		return nil
	}
	line := strings.Split(source, "\n")[region.start.line-1]
	start := region.start.col - 1
	end := region.end.col - 1
	if start < 0 {
		start = 0
	}
	if end <= start || end > len(line) {
		end = len(line)
	}
	start += skip
	if start >= end {
		return nil
	}
	var out []sourceSpan
	for cursor := start; cursor < end; {
		at := strings.Index(line[cursor:end], name)
		if at < 0 {
			break
		}
		at += cursor
		next := at + len(name)
		if next <= end && lspIdentifierBoundary(line, at, next) {
			out = append(out, sourceSpan{
				start: sourcePos{line: region.start.line, col: at + 1},
				end:   sourcePos{line: region.start.line, col: next + 1},
			})
		}
		cursor = next
	}
	return out
}

// lspMacroParameterSpans recovers the exact source-backed spans for macro
// parameters from the original source. The parser intentionally stores macro
// parameters as names only, so the LSP performs this narrow lexical recovery
// at the semantic boundary rather than duplicating macro parsing rules.
// Source columns remain UTF-8 byte columns until the final LSP conversion.
func lspMacroParameterSpans(source string, st macroStmt) []sourceSpan {
	if source == "" || len(st.Params) == 0 || st.NameSpan.start.line < 1 {
		return nil
	}
	lines := strings.Split(source, "\n")
	startLine := st.NameSpan.start.line - 1
	endLine := st.Span.end.line - 1
	if startLine < 0 || startLine >= len(lines) {
		return nil
	}
	if endLine < startLine || endLine >= len(lines) {
		endLine = startLine
	}

	foundOpen := false
	paramIndex := 0
	var spans []sourceSpan
	for lineIndex := startLine; lineIndex <= endLine && paramIndex < len(st.Params); lineIndex++ {
		line := lines[lineIndex]
		at := 0
		if lineIndex == startLine {
			at = st.NameSpan.end.col - 1
			if at < 0 || at > len(line) {
				at = 0
			}
		}
		for at < len(line) && paramIndex < len(st.Params) {
			if !foundOpen {
				open := strings.IndexByte(line[at:], '(')
				if open < 0 {
					break
				}
				at += open + 1
				foundOpen = true
				continue
			}
			if line[at] == ')' {
				return spans
			}
			r, size := utf8.DecodeRuneInString(line[at:])
			if unicode.IsLetter(r) || r == '_' {
				start := at
				at += size
				for at < len(line) {
					next, nextSize := utf8.DecodeRuneInString(line[at:])
					if !(unicode.IsLetter(next) || unicode.IsDigit(next) || next == '_') {
						break
					}
					at += nextSize
				}
				if lspIdentifierBoundary(line, start, at) && line[start:at] == st.Params[paramIndex] {
					spans = append(spans, sourceSpan{
						start: sourcePos{line: lineIndex + 1, col: start + 1},
						end:   sourcePos{line: lineIndex + 1, col: at + 1},
					})
					paramIndex++
				}
				continue
			}
			at += size
		}
	}
	return spans
}

// indexLSPImportedModules indexes exports from standard-library modules that
// the runtime can resolve under its current capability profile. The generated
// definition URI is intentionally virtual: stdlib exports do not have an
// editable source file, but a stable URI and deterministic line for each
// export let clients navigate to the exact registered symbol instead of
// falling back to an alias or returning no definition.
func indexLSPImportedModules(a *documentAnalysis, scope *lspScope, rt *Runtime, stmts []Stmt) {
	if a == nil || scope == nil || rt == nil {
		return
	}
	for _, stmt := range stmts {
		st, ok := stmt.(importStmt)
		if !ok {
			continue
		}
		ref, selector := splitModuleRef(st.Path)
		if !strings.HasPrefix(ref, "std/") {
			continue
		}
		modValue, err := stdlibModule(rt, ref, selector)
		if err != nil {
			// Capability-gated or otherwise unavailable modules remain
			// conservative: their exports must not be advertised as proven.
			continue
		}
		mod, err := modValue.AsModule()
		if err != nil {
			continue
		}
		alias := scope.lookup(st.Alias)
		if alias == nil || alias.kind != "module" {
			continue
		}
		owner := "module:" + ref
		alias.typeName = owner
		names := make([]string, 0, len(mod.members))
		for name := range mod.members {
			names = append(names, name)
		}
		sort.Strings(names)
		for index, name := range names {
			value := mod.members[name]
			kind := "constant"
			if value.Kind() == "function" {
				kind = "function"
			}
			detail := ref + "." + name
			if kind == "function" {
				detail += " (standard-library function)"
			} else {
				detail += " : " + value.Kind()
			}
			addLSPMember(a, owner, &symbolDef{
				name:   name,
				kind:   kind,
				detail: detail,
				uri:    "osiris://stdlib/" + strings.TrimPrefix(ref, "std/"),
				span: sourceSpan{
					start: sourcePos{line: index + 1, col: 1},
					end:   sourcePos{line: index + 1, col: utf8.RuneCountInString(name) + 1},
				},
			})
		}
	}
}

// indexLSPImportedUserModules installs only explicitly exported definitions
// from locally resolved user modules. Unlike virtual stdlib members, these
// definitions already belong to a real document and are therefore reused
// directly rather than copied into the importing document's definition list.
func indexLSPImportedUserModules(a *documentAnalysis, scope *lspScope, stmts []Stmt, modules map[string]lspResolvedModule) {
	if a == nil || scope == nil || len(modules) == 0 {
		return
	}
	for _, stmt := range stmts {
		st, ok := stmt.(importStmt)
		if !ok {
			continue
		}
		resolved, ok := modules[st.Path]
		if !ok || resolved.uri == "" || len(resolved.exports) == 0 {
			continue
		}
		alias := scope.lookup(st.Alias)
		if alias == nil || alias.kind != "module" {
			continue
		}
		owner := "module:" + st.Path
		alias.typeName = owner
		if a.members == nil {
			a.members = make(map[string]map[string]*symbolDef)
		}
		if a.members[owner] == nil {
			a.members[owner] = make(map[string]*symbolDef, len(resolved.exports))
		}
		names := make([]string, 0, len(resolved.exports))
		for name := range resolved.exports {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if def := resolved.exports[name]; def != nil {
				a.members[owner][name] = def
			}
		}
	}
}

func analyzeTypeReference(a *documentAnalysis, scope *lspScope, uri, name string, span sourceSpan, detail string) {
	if name == "" {
		return
	}
	if def := scope.lookup(name); def != nil {
		a.refs = append(a.refs, &symbolRef{span: span, target: def, detail: detail})
		return
	}
	if _, builtin := builtinTypeSet()[name]; builtin {
		return
	}
	a.diagnostics = append(a.diagnostics, diagnosticAt(span, fmt.Sprintf("undefined %s type %q", detail, name)))
}

func analyzePattern(a *documentAnalysis, scope *lspScope, uri string, pattern Pattern) {
	switch p := pattern.(type) {
	case wildcardPattern, bindPattern, literalPattern, relationalPattern:
		return
	case enumPattern:
		var enumDef *symbolDef
		if def := scope.lookup(p.typeName); def != nil && def.kind == "enum" {
			enumDef = def
		} else {
			for _, def := range a.defs {
				if def != nil && def.kind == "enum" && def.name == p.typeName {
					enumDef = def
					break
				}
			}
		}
		if enumDef != nil {
			for _, span := range lspSourceIdentifierSpans(a.source, p.Span, p.typeName, 0) {
				a.refs = append(a.refs, &symbolRef{span: span, target: enumDef, detail: "enum pattern type"})
			}
		}
		if def := a.enumCases[p.typeName+"."+p.caseName]; def != nil {
			a.refs = append(a.refs, &symbolRef{span: p.Span, target: def, detail: "enum pattern"})
		} else {
			a.diagnostics = append(a.diagnostics, diagnosticAt(p.Span, fmt.Sprintf("undefined enum case %s.%s", p.typeName, p.caseName)))
		}
		for _, item := range p.items {
			analyzePattern(a, scope, uri, item)
		}
	case structPattern:
		analyzeTypeReference(a, scope, uri, p.typeName, p.Span, "struct pattern")
		for _, field := range p.fields {
			if members := a.members[p.typeName]; members != nil {
				if def := members[field.name]; def != nil {
					a.refs = append(a.refs, &symbolRef{span: field.Span, target: def, detail: "struct pattern field"})
				} else {
					a.diagnostics = append(a.diagnostics, diagnosticAt(field.Span, fmt.Sprintf("undefined field %q in struct pattern %s", field.name, p.typeName)))
				}
			} else {
				a.diagnostics = append(a.diagnostics, diagnosticAt(field.Span, fmt.Sprintf("undefined struct %q in pattern", p.typeName)))
			}
			analyzePattern(a, scope, uri, field.pattern)
		}
	case listPattern:
		for _, item := range p.items {
			analyzePattern(a, scope, uri, item)
		}
	case orPattern:
		for _, alternative := range p.alternatives {
			analyzePattern(a, scope, uri, alternative)
		}
	case andPattern:
		analyzePattern(a, scope, uri, p.left)
		analyzePattern(a, scope, uri, p.right)
	case notPattern:
		analyzePattern(a, scope, uri, p.inner)
	case typePattern:
		analyzeTypeReference(a, scope, uri, p.typeName, p.Span, "pattern")
	default:
		a.diagnostics = append(a.diagnostics, diagnosticAt(lspPatternSpan(pattern), fmt.Sprintf("LSP analysis does not support pattern %T", pattern)))
	}
}

func lspStmtSpan(stmt Stmt) sourceSpan {
	if node, ok := stmt.(lspAnalysisSpanned); ok {
		return node.lspAnalysisSpan()
	}
	switch st := stmt.(type) {
	case letStmt:
		return st.Span
	case assignStmt:
		return st.Span
	case emitStmt:
		return st.Span
	case deferStmt:
		return st.Span
	case macroStmt:
		return st.Span
	case funcStmt:
		return st.Span
	case traitStmt:
		return st.Span
	case implStmt:
		return st.Span
	case returnStmt:
		return st.Span
	case whileStmt:
		return st.Span
	case forStmt:
		return st.Span
	case breakStmt:
		return st.Span
	case continueStmt:
		return st.Span
	case importStmt:
		return st.Span
	case packageStmt:
		return st.Span
	case exportStmt:
		return st.Span
	case enumStmt:
		return st.Span
	case structStmt:
		return st.Span
	case typeDeclStmt:
		return st.Span
	case newtypeStmt:
		return st.Span
	default:
		return sourceSpan{}
	}
}

func lspExprSpan(expr Expr) sourceSpan {
	if node, ok := expr.(lspAnalysisSpanned); ok {
		return node.lspAnalysisSpan()
	}
	switch n := expr.(type) {
	case *literalExpr:
		return n.Span
	case *identExpr:
		return n.Span
	case *unaryExpr:
		return n.Span
	case *binaryExpr:
		return n.Span
	case *callExpr:
		return n.Span
	case *blockExpr:
		return n.Span
	case *tryExpr:
		return n.Span
	case *propagateExpr:
		return n.Span
	case *memberExpr:
		return n.Span
	case *listExpr:
		return n.Span
	case *tupleExpr:
		return n.Span
	case *structExpr:
		return n.Span
	case *structUpdateExpr:
		return n.Span
	case *indexExpr:
		return n.Span
	case *matchExpr:
		return n.Span
	default:
		return sourceSpan{}
	}
}

func lspPatternSpan(pattern Pattern) sourceSpan {
	if node, ok := pattern.(lspAnalysisSpanned); ok {
		return node.lspAnalysisSpan()
	}
	return patternSpan(pattern)
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
		if a != nil && a.members != nil {
			if receiver := scope.lookup(ident.name); receiver != nil && receiver.typeName != "" {
				if def := a.members[receiver.typeName][member]; def != nil {
					return def
				}
			}
			if def := a.members[ident.name][member]; def != nil {
				return def
			}
		}
		return scope.lookup(ident.name)
	}
	if nested, ok := expr.(*memberExpr); ok {
		if receiver := resolveMemberTarget(a, scope, nested.object, nested.name); receiver != nil && receiver.typeName != "" {
			if def := a.members[receiver.typeName][member]; def != nil {
				return def
			}
		}
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
	case structPattern:
		var out []*symbolDef
		for _, field := range n.fields {
			out = append(out, patternDefs(uri, field.pattern)...)
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
	name := st.Name
	if len(st.TypeParams) > 0 {
		parts := make([]string, len(st.TypeParams))
		for i, param := range st.TypeParams {
			parts[i] = param.Name
			if param.Bound != "" {
				parts[i] += ": " + param.Bound
			}
		}
		name += "<" + strings.Join(parts, ", ") + ">"
	}
	return prefix + name + strings.TrimPrefix(signature.String(), "func")
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
