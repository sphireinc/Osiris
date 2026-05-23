package osiris

import (
	"fmt"
	"path/filepath"
	"strings"
)

// VizProgramDOT performs the viz program dot step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func VizProgramDOT(prog *Program) string {
	g := newDotGraph("osiris-ast")
	root := g.node(programLabel(prog), "box", "filled", "lightgoldenrodyellow")
	for _, stmt := range prog.Statements {
		g.edge(root, g.walkStmt(stmt))
	}
	return g.String()
}

// VizImportsDOT performs the viz imports dot step in the surrounding package and returns string, error.. It returns string and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func VizImportsDOT(sourceName, src string) (string, error) {
	g := newDotGraph("osiris-imports")
	visited := make(map[string]struct{})
	rootKey := sourceName
	if rootKey == "" {
		rootKey = "<stdin>"
	}
	root := g.node(moduleLabel(rootKey, src), "box", "filled", "lightgoldenrodyellow")
	if err := g.walkImports(root, sourceName, src, visited); err != nil {
		return "", err
	}
	return g.String(), nil
}

type dotGraph struct {
	name string
	b    *strings.Builder
	next int
}

// newDotGraph performs the new dot graph step in the surrounding package and returns *dotGraph.. It returns *dotGraph so callers can continue with the next stage of the pipeline without re-deriving the same information.
func newDotGraph(name string) *dotGraph {
	b := &strings.Builder{}
	b.WriteString("digraph ")
	b.WriteString(name)
	b.WriteString(" {\n")
	b.WriteString("  rankdir=LR;\n")
	b.WriteString("  node [shape=box, style=\"rounded,filled\", fontname=\"Menlo\"];\n")
	return &dotGraph{name: name, b: b}
}

// String handles on dotGraph the string. It keeps the dotGraph subsystem self-contained by handling the string-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (g *dotGraph) String() string {
	g.b.WriteString("}\n")
	return g.b.String()
}

// node handles on dotGraph the node. It keeps the dotGraph subsystem self-contained by handling the node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (g *dotGraph) node(label string, attrs ...string) string {
	id := fmt.Sprintf("n%d", g.next)
	g.next++
	g.b.WriteString("  ")
	g.b.WriteString(id)
	g.b.WriteString(" [label=")
	g.b.WriteString(dotQuote(label))
	if len(attrs) >= 2 {
		for i := 0; i+1 < len(attrs); i += 2 {
			g.b.WriteString(", ")
			g.b.WriteString(attrs[i])
			g.b.WriteString("=")
			g.b.WriteString(dotQuote(attrs[i+1]))
		}
	}
	g.b.WriteString("];\n")
	return id
}

// edge handles on dotGraph the edge. It keeps the dotGraph subsystem self-contained by handling the edge-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (g *dotGraph) edge(from, to string) {
	g.b.WriteString("  ")
	g.b.WriteString(from)
	g.b.WriteString(" -> ")
	g.b.WriteString(to)
	g.b.WriteString(";\n")
}

// walkStmt handles on dotGraph the walk stmt. It keeps the dotGraph subsystem self-contained by handling the walk stmt-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (g *dotGraph) walkStmt(stmt Stmt) string {
	switch s := stmt.(type) {
	case packageStmt:
		label := "package " + s.Name
		if s.Version != "" {
			label += " " + s.Version
		}
		return g.node(label, "fillcolor", "honeydew")
	case exportStmt:
		return g.node("export "+strings.Join(s.Names, ", "), "fillcolor", "honeydew")
	case letStmt:
		id := g.node("let "+s.Name, "fillcolor", "lavender")
		g.edge(id, g.walkExpr(s.Expr))
		return id
	case emitStmt:
		id := g.node("emit", "fillcolor", "mistyrose")
		g.edge(id, g.walkExpr(s.Expr))
		return id
	case macroStmt:
		id := g.node("macro "+s.Name, "fillcolor", "thistle")
		for _, param := range s.Params {
			g.edge(id, g.node("param "+param, "fillcolor", "plum"))
		}
		g.edge(id, g.walkExpr(s.Body))
		return id
	case importStmt:
		return g.node("import "+s.Path+" as "+s.Alias, "fillcolor", "aliceblue")
	default:
		return g.node("<stmt>")
	}
}

// walkExpr handles on dotGraph the walk expr. It keeps the dotGraph subsystem self-contained by handling the walk expr-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (g *dotGraph) walkExpr(expr Expr) string {
	switch n := expr.(type) {
	case *literalExpr:
		return g.node(n.value.Kind()+" "+n.value.String(), "fillcolor", "white")
	case *identExpr:
		return g.node("ident "+n.name, "fillcolor", "white")
	case *unaryExpr:
		id := g.node("unary "+n.op.String(), "fillcolor", "papayawhip")
		g.edge(id, g.walkExpr(n.expr))
		return id
	case *binaryExpr:
		id := g.node("binary "+n.op.String(), "fillcolor", "papayawhip")
		g.edge(id, g.walkExpr(n.left))
		g.edge(id, g.walkExpr(n.right))
		return id
	case *callExpr:
		id := g.node("call", "fillcolor", "paleturquoise")
		g.edge(id, g.walkExpr(n.callee))
		for _, arg := range n.args {
			g.edge(id, g.walkExpr(arg))
		}
		return id
	case *memberExpr:
		id := g.node("member "+n.name, "fillcolor", "paleturquoise")
		g.edge(id, g.walkExpr(n.object))
		return id
	case *listExpr:
		id := g.node("list", "fillcolor", "lavenderblush")
		for _, item := range n.items {
			g.edge(id, g.walkExpr(item))
		}
		return id
	case *tryExpr:
		id := g.node("try", "fillcolor", "mistyrose")
		g.edge(id, g.walkExpr(n.body))
		catchID := g.node("catch "+n.catchName, "fillcolor", "lavender")
		g.edge(id, catchID)
		g.edge(catchID, g.walkExpr(n.handler))
		return id
	case *blockExpr:
		id := g.node("block", "fillcolor", "mintcream")
		for _, stmt := range n.stmts {
			g.edge(id, g.walkStmt(stmt))
		}
		if n.tail != nil {
			g.edge(id, g.walkExpr(n.tail))
		}
		return id
	case *matchExpr:
		label := "match"
		if n.keyword == tokenSwitch {
			label = "switch"
		}
		id := g.node(label, "fillcolor", "khaki")
		g.edge(id, g.walkExpr(n.subject))
		for _, arm := range n.arms {
			armID := g.node(formatPattern(arm.pattern), "fillcolor", "cornsilk")
			g.edge(id, armID)
			g.edge(armID, g.walkExpr(arm.body))
		}
		return id
	default:
		return g.node("<expr>")
	}
}

// walkImports handles on dotGraph the walk imports. It keeps the dotGraph subsystem self-contained by handling the walk imports-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (g *dotGraph) walkImports(root string, sourceName, src string, visited map[string]struct{}) error {
	prog, err := Parse(sourceName, src)
	if err != nil {
		return err
	}
	key := sourceName
	if key == "" {
		key = "<stdin>"
	}
	if _, ok := visited[key]; ok {
		return nil
	}
	visited[key] = struct{}{}

	base := sourceName
	for _, stmt := range prog.Statements {
		imported, ok := stmt.(importStmt)
		if !ok {
			continue
		}
		refKey, kind, importedSrc, err := resolveModuleRef(imported.Path, base)
		if err != nil {
			return err
		}
		if kind == moduleKindStdlib {
			child := g.node(refKey, "ellipse", "filled", "aliceblue")
			g.edge(root, child)
			continue
		}
		if _, seen := visited[refKey]; seen {
			child := g.node(filepath.Base(refKey), "ellipse", "filled", "lightgray")
			g.edge(root, child)
			continue
		}
		child := g.node(moduleLabel(refKey, importedSrc), "box", "filled", "honeydew")
		g.edge(root, child)
		if err := g.walkImports(child, refKey, importedSrc, visited); err != nil {
			return err
		}
	}
	return nil
}

// programLabel performs the program label step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func programLabel(prog *Program) string {
	if prog == nil {
		return "<program>"
	}
	for _, stmt := range prog.Statements {
		if p, ok := stmt.(packageStmt); ok {
			if p.Version != "" {
				return "package " + p.Name + " " + p.Version
			}
			return "package " + p.Name
		}
	}
	return prog.SourceName
}

// programLabelFromSource performs the program label from source step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func programLabelFromSource(sourceName, src string) string {
	prog, err := Parse(sourceName, src)
	if err != nil {
		return sourceName
	}
	return programLabel(prog)
}

// moduleLabel performs the module label step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func moduleLabel(sourceName, src string) string {
	label := sourceName
	if prog, err := Parse(sourceName, src); err == nil {
		if p := programLabel(prog); p != "" {
			label = p
		}
	}
	if base := filepath.Base(sourceName); base != "" && base != "." && base != sourceName {
		return label + "\n" + base
	}
	return label
}

// dotQuote performs the dot quote step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func dotQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteByte(s[i])
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteByte(s[i])
		}
	}
	b.WriteByte('"')
	return b.String()
}
