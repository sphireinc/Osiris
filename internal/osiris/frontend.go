package osiris

import (
	"context"
	"fmt"
)

// ParsedProgram is surface syntax produced by ParseSurface. Its AST is not a
// semantic input to a backend; callers must compile it through Frontend.
type ParsedProgram struct{ program *Program }

func ParseSurface(sourceName, source string) (ParsedProgram, error) {
	program, err := Parse(sourceName, source)
	if err != nil {
		return ParsedProgram{}, err
	}
	return ParsedProgram{program: program}, nil
}

func parsedProgramOf(program *Program) (ParsedProgram, error) {
	if program == nil {
		return ParsedProgram{}, fmt.Errorf("cannot compile nil program")
	}
	return ParsedProgram{program: program}, nil
}

func (p ParsedProgram) SourceName() string {
	if p.program == nil {
		return ""
	}
	return p.program.SourceName
}

// ExpandedProgram is the sole macro-expansion result. Macro expansion occurs
// once in Frontend. The wrapped AST is package-private so it cannot be passed
// directly to an artifact backend.
type ExpandedProgram struct {
	parsed  ParsedProgram
	program *Program
	macros  map[string]*macroDef
}

// SymbolID is a stable identity allocated by resolution, independent of a
// later type/HIR representation.
type SymbolID uint32

type SymbolKind string

const (
	SymbolBinding  SymbolKind = "binding"
	SymbolFunction SymbolKind = "function"
	SymbolImport   SymbolKind = "import"
	SymbolType     SymbolKind = "type"
	SymbolMacro    SymbolKind = "macro"
)

type Symbol struct {
	ID   SymbolID
	Name string
	Kind SymbolKind
	Span sourceSpan
}

// ResolvedProgram records declaration identities for the expanded tree. The
// resolver deliberately keeps source spelling/spans in the surface AST until
// the typed-HIR task owns a structural rewrite.
type ResolvedProgram struct {
	expanded ExpandedProgram
	Symbols  []Symbol
	// References maps parsed identifier nodes to their declaration identity.
	// Later typed-HIR lowering consumes this table instead of reconstructing
	// lexical names from source spelling.
	References map[*identExpr]SymbolID
}

// TypedProgram is the only frontend result consumed by VM and lowering APIs.
// It carries the post-expansion checked program; it is not typed HIR.
type TypedProgram struct {
	resolved ResolvedProgram
	checked  *CheckedProgram
}

func (p *TypedProgram) SourceName() string {
	if p == nil {
		return ""
	}
	return p.resolved.expanded.program.SourceName
}

func (p *TypedProgram) Checked() *CheckedProgram {
	if p == nil {
		return nil
	}
	return p.checked
}

// Frontend is the single ordered front-end driver. Session state supplies
// macro/module context for incremental execution without reintroducing a raw
// pre-expansion type-check pass.
func (rt *Runtime) Frontend(ctx context.Context, parsed ParsedProgram) (*TypedProgram, error) {
	if rt == nil {
		rt = NewRuntime()
	}
	if parsed.program == nil {
		return nil, fmt.Errorf("frontend requires a parsed program")
	}
	session := NewSession(rt, ctx, parsed.program.SourceName)
	return session.frontend(parsed, nil)
}

func (s *Session) frontend(parsed ParsedProgram, stack []string) (*TypedProgram, error) {
	if parsed.program == nil {
		return nil, fmt.Errorf("frontend requires a parsed program")
	}
	expanded, macros, err := s.expandProgramWithStack(parsed.program, stack)
	if err != nil {
		return nil, normalizeError(err, parsed.program.SourceName)
	}
	if err := validateTensorShapes(expanded); err != nil {
		return nil, normalizeError(err, expanded.SourceName)
	}
	if err := validateConcurrentEmits(expanded); err != nil {
		return nil, normalizeError(err, expanded.SourceName)
	}
	checked, err := s.checkExpandedProgram(expanded)
	if err != nil {
		return nil, normalizeError(err, expanded.SourceName)
	}
	symbols, references := resolveFrontendSymbols(parsed.program, expanded)
	resolved := ResolvedProgram{expanded: ExpandedProgram{parsed: parsed, program: expanded, macros: macros}, Symbols: symbols, References: references}
	attachFrontendSymbolIDs(checked, resolved.Symbols)
	return &TypedProgram{resolved: resolved, checked: checked}, nil
}

func attachFrontendSymbolIDs(checked *CheckedProgram, symbols []Symbol) {
	if checked == nil {
		return
	}
	for i := range checked.Bindings {
		for _, symbol := range symbols {
			if symbol.Kind == SymbolBinding && symbol.Name == checked.Bindings[i].Name && symbol.Span == checked.Bindings[i].Span {
				checked.Bindings[i].Symbol = symbol.ID
				break
			}
		}
	}
}

func (s *Session) checkExpandedProgram(prog *Program) (*CheckedProgram, error) {
	if len(s.typecheckStmts) == 0 {
		return s.rt.CheckProgram(s.ctx, prog)
	}
	statements := make([]Stmt, 0, len(s.typecheckStmts)+len(prog.Statements))
	statements = append(statements, s.typecheckStmts...)
	statements = append(statements, prog.Statements...)
	return s.rt.CheckProgram(s.ctx, &Program{SourceName: prog.SourceName, Statements: statements})
}

func resolveFrontendSymbols(surface, expanded *Program) ([]Symbol, map[*identExpr]SymbolID) {
	if surface == nil || expanded == nil {
		return nil, nil
	}
	var symbols []Symbol
	references := make(map[*identExpr]SymbolID)
	top := make(map[string]SymbolID)
	next := SymbolID(1)
	add := func(name string, kind SymbolKind, span sourceSpan) {
		if name == "" {
			return
		}
		symbols = append(symbols, Symbol{ID: next, Name: name, Kind: kind, Span: span})
		top[name] = next
		next++
	}
	for _, statement := range surface.Statements {
		if node, ok := statement.(macroStmt); ok {
			add(node.Name, SymbolMacro, node.NameSpan)
		}
	}
	for _, statement := range expanded.Statements {
		switch node := statement.(type) {
		case letStmt:
			if len(node.Names) == 0 {
				add(node.Name, SymbolBinding, node.NameSpan)
			} else {
				for _, name := range node.Names {
					add(name, SymbolBinding, node.NameSpan)
				}
			}
		case funcStmt:
			add(node.Name, SymbolFunction, node.NameSpan)
		case importStmt:
			add(node.Alias, SymbolImport, node.AliasSpan)
		case macroStmt:
			add(node.Name, SymbolMacro, node.NameSpan)
		case structStmt:
			add(node.Name, SymbolType, node.NameSpan)
		case typeDeclStmt:
			add(node.Name, SymbolType, node.NameSpan)
		case newtypeStmt:
			add(node.Name, SymbolType, node.NameSpan)
		case enumStmt:
			add(node.Name, SymbolType, node.NameSpan)
		}
	}
	var walkExpr func(Expr, map[string]SymbolID)
	walkExpr = func(expr Expr, env map[string]SymbolID) {
		switch n := expr.(type) {
		case *identExpr:
			if id, ok := env[n.name]; ok {
				references[n] = id
			}
		case *unaryExpr:
			walkExpr(n.expr, env)
		case *binaryExpr:
			walkExpr(n.left, env)
			walkExpr(n.right, env)
		case *callExpr:
			walkExpr(n.callee, env)
			for _, arg := range n.args {
				walkExpr(arg, env)
			}
		case *memberExpr:
			walkExpr(n.object, env)
		case *indexExpr:
			walkExpr(n.Object, env)
			walkExpr(n.Index, env)
		case *listExpr:
			for _, item := range n.items {
				walkExpr(item, env)
			}
		case *tupleExpr:
			for _, item := range n.items {
				walkExpr(item, env)
			}
		case *lazyExpr:
			walkExpr(n.Body, env)
		case *tryExpr:
			walkExpr(n.body, env)
			walkExpr(n.handler, env)
		case *propagateExpr:
			walkExpr(n.expr, env)
		case *blockExpr:
			child := make(map[string]SymbolID, len(env))
			for name, id := range env {
				child[name] = id
			}
			for _, stmt := range n.stmts {
				switch st := stmt.(type) {
				case letStmt:
					walkExpr(st.Expr, child)
				case assignStmt:
					walkExpr(st.Expr, child)
				case emitStmt:
					walkExpr(st.Expr, child)
				case returnStmt:
					walkExpr(st.Expr, child)
				case funcStmt:
					walkExpr(st.Body, child)
				}
			}
			walkExpr(n.tail, child)
		}
	}
	for _, statement := range expanded.Statements {
		switch n := statement.(type) {
		case letStmt:
			walkExpr(n.Expr, top)
		case assignStmt:
			walkExpr(n.Expr, top)
		case emitStmt:
			walkExpr(n.Expr, top)
		case funcStmt:
			walkExpr(n.Body, top)
		case returnStmt:
			walkExpr(n.Expr, top)
		case whileStmt:
			walkExpr(n.Cond, top)
			walkExpr(n.Body, top)
		case forStmt:
			walkExpr(n.Iter, top)
			walkExpr(n.Body, top)
		}
	}
	return symbols, references
}

func (rt *Runtime) frontendProgram(ctx context.Context, program *Program) (*TypedProgram, error) {
	parsed, err := parsedProgramOf(program)
	if err != nil {
		return nil, err
	}
	return rt.Frontend(ctx, parsed)
}
