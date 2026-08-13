package osiris

import (
	"fmt"
	"strconv"
	"strings"
)

// TypeID is a compact, context-local canonical type identity. ID zero is
// invalid; a TypeContext owns all other IDs and can be discarded as one unit.
type TypeID uint32

type TypeKind uint8

const (
	TypeInvalid TypeKind = iota
	TypeDynamic
	TypePrimitive
	TypeNominal
	TypeGenericParameter
	TypeList
	TypeTuple
	TypeMap
	TypeSet
	TypeOption
	TypeResult
	TypeLazy
	TypeTask
	TypeFunction
)

// TypeFormatMode controls presentation only. It is deliberately excluded from
// type interning and semantic comparison.
type TypeFormatMode uint8

const (
	TypeFormatSource TypeFormatMode = iota
	TypeFormatDiagnostic
)

type TypeNode struct {
	Kind        TypeKind
	Name        string // canonical constructor/primitive name, never formatted then reparsed.
	Declaration SymbolID
	Args        []TypeID
	Result      TypeID
	Display     string // optional presentation metadata; excluded from identity.
}

// TypeContext interns nodes for one compilation. It has no process-global
// state, which keeps parallel frontend/LSP compilations isolated and lets a
// completed compilation release the entire arena.
type TypeContext struct {
	nodes []TypeNode
	index map[string]TypeID
}

func NewTypeContext() *TypeContext {
	return &TypeContext{nodes: []TypeNode{{Kind: TypeInvalid}}, index: make(map[string]TypeID)}
}

func (tc *TypeContext) Intern(node TypeNode) TypeID {
	if tc == nil {
		panic("nil TypeContext")
	}
	key := typeNodeKey(node)
	if id, ok := tc.index[key]; ok {
		return id
	}
	node.Args = append([]TypeID(nil), node.Args...)
	id := TypeID(len(tc.nodes))
	tc.nodes = append(tc.nodes, node)
	tc.index[key] = id
	return id
}

func typeNodeKey(node TypeNode) string {
	var b strings.Builder
	b.WriteString(strconv.Itoa(int(node.Kind)))
	b.WriteByte('|')
	b.WriteString(node.Name)
	b.WriteByte('|')
	b.WriteString(strconv.FormatUint(uint64(node.Declaration), 10))
	b.WriteByte('|')
	for _, arg := range node.Args {
		b.WriteString(strconv.FormatUint(uint64(arg), 10))
		b.WriteByte(',')
	}
	b.WriteByte('|')
	b.WriteString(strconv.FormatUint(uint64(node.Result), 10))
	return b.String()
}

func (tc *TypeContext) Node(id TypeID) (TypeNode, bool) {
	if tc == nil || id == 0 || int(id) >= len(tc.nodes) {
		return TypeNode{}, false
	}
	return tc.nodes[id], true
}

func (tc *TypeContext) Equal(left, right TypeID) bool { return left != 0 && left == right }

// InternStatic is the sole adapter from legacy source-parsed staticType to a
// canonical compiler type. It is intentionally one-way: formatting never
// participates in construction or equality.
func (tc *TypeContext) InternStatic(typ staticType) TypeID {
	return tc.internStatic(typ, nil, 0)
}

// InternStaticWithGenericParameters adapts a source-parsed type in the
// declaration that owns its generic parameters. The owner is part of a type
// parameter's identity, so `T` from two declarations can never alias merely
// because their source spelling matches.
func (tc *TypeContext) InternStaticWithGenericParameters(typ staticType, parameters []typeParam, owner SymbolID) TypeID {
	parameterIndexes := make(map[string]int, len(parameters))
	for index, parameter := range parameters {
		parameterIndexes[parameter.Name] = index
	}
	return tc.internStatic(typ, parameterIndexes, owner)
}

func (tc *TypeContext) internStatic(typ staticType, parameters map[string]int, owner SymbolID) TypeID {
	if index, ok := parameters[typ.name]; ok && len(typ.params) == 0 && typ.result == nil {
		return tc.Intern(TypeNode{Kind: TypeGenericParameter, Name: typ.name, Declaration: owner, Args: []TypeID{TypeID(index + 1)}})
	}
	args := make([]TypeID, len(typ.params))
	for i := range typ.params {
		args[i] = tc.internStatic(typ.params[i], parameters, owner)
	}
	result := TypeID(0)
	if typ.result != nil {
		result = tc.internStatic(*typ.result, parameters, owner)
	}
	kind := typeKindForStatic(typ)
	return tc.Intern(TypeNode{Kind: kind, Name: typ.name, Args: args, Result: result})
}

func typeKindForStatic(typ staticType) TypeKind {
	if typ.isDynamic() {
		return TypeDynamic
	}
	if typ.isFunction() {
		return TypeFunction
	}
	switch typ.name {
	case "list":
		return TypeList
	case "tuple":
		return TypeTuple
	case "map":
		return TypeMap
	case "set":
		return TypeSet
	case "option":
		return TypeOption
	case "result":
		return TypeResult
	case staticTypeLazy:
		return TypeLazy
	case "task":
		return TypeTask
	case "T", "U", "V":
		return TypeGenericParameter
	}
	if isBuiltinTypeName(typ.name) || typ.name == "null" || typ.name == "error" || typ.name == "module" || typ.name == "function" {
		return TypePrimitive
	}
	return TypeNominal
}

func isBuiltinTypeName(name string) bool {
	if name == "bool" || name == "string" || name == "number" || name == "any" || name == "dynamic" {
		return true
	}
	if len(name) >= 2 && (name[0] == 'u' || name[0] == 'i' || name[0] == 'f') {
		_, err := strconv.Atoi(name[1:])
		return err == nil
	}
	return false
}

func (tc *TypeContext) Format(id TypeID, _ ...TypeFormatMode) string {
	node, ok := tc.Node(id)
	if !ok {
		return "<invalid>"
	}
	if node.Kind == TypeFunction {
		params := make([]string, len(node.Args))
		for i, arg := range node.Args {
			params[i] = tc.Format(arg)
		}
		return "func(" + strings.Join(params, ", ") + ") " + tc.Format(node.Result)
	}
	name := node.Name
	if node.Display != "" {
		name = node.Display
	}
	if len(node.Args) == 0 {
		return name
	}
	args := make([]string, len(node.Args))
	for i, arg := range node.Args {
		args[i] = tc.Format(arg)
	}
	return name + "<" + strings.Join(args, ", ") + ">"
}

// Substitute creates canonical replacement nodes without format/parse round
// trips. Generic parameter identity is represented by its canonical node ID.
func (tc *TypeContext) Substitute(id TypeID, replacements map[TypeID]TypeID) TypeID {
	seen := make(map[TypeID]TypeID)
	var visit func(TypeID) TypeID
	visit = func(current TypeID) TypeID {
		if replacement, ok := replacements[current]; ok {
			return replacement
		}
		if cached, ok := seen[current]; ok {
			return cached
		}
		node, ok := tc.Node(current)
		if !ok {
			return 0
		}
		changed := false
		args := append([]TypeID(nil), node.Args...)
		for i := range args {
			next := visit(args[i])
			changed = changed || next != args[i]
			args[i] = next
		}
		result := node.Result
		if result != 0 {
			next := visit(result)
			changed = changed || next != result
			result = next
		}
		if !changed {
			seen[current] = current
			return current
		}
		node.Args, node.Result = args, result
		out := tc.Intern(node)
		seen[current] = out
		return out
	}
	return visit(id)
}

func (tc *TypeContext) MustFormat(id TypeID) string {
	if id == 0 {
		return "<invalid>"
	}
	return tc.Format(id)
}

func (id TypeID) String() string { return fmt.Sprintf("TypeID(%d)", id) }
