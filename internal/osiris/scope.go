package osiris

// binding is the lexical definition carried by a scope. Keeping scopes in a
// small component separates shared name-resolution state from evaluator,
// session, macro, match, and bytecode execution machinery.
type binding struct {
	expr Expr
	env  *scope
	// value is populated for eager immutable declarations. expr/env remain for
	// functions, imports, and transitional compiler-owned bindings that are not
	// ordinary lets.
	value   *Value
	cell    *bindingCell
	mutable bool
}

func eagerBinding(value Value) binding {
	// Keep the literal expression as metadata for legacy analyzers/fingerprint
	// passes while lookup itself returns value directly. It is not a deferred
	// initializer AST or captured lexical environment.
	return binding{expr: &literalExpr{value: value}, value: &value}
}

// storedBindingValue returns the value already owned by an eager binding or
// mutable cell. Runtime inspection and export paths must use it before any
// legacy compiler-owned expression metadata so ordinary lets can never be
// re-evaluated through tooling.
func storedBindingValue(b binding) (Value, bool) {
	if b.cell != nil {
		return b.cell.value, true
	}
	if b.value != nil {
		return *b.value, true
	}
	return Value{}, false
}

type bindingCell struct{ value Value }

// scope is an immutable-parent lexical environment. Callers allocate a child
// only when introducing bindings; lookup never mutates a parent scope.
type scope struct {
	parent *scope
	items  map[string]binding
}

func (s *scope) lookup(name string) (binding, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if b, ok := cur.items[name]; ok {
			return b, true
		}
	}
	return binding{}, false
}

func cloneScope(s *scope) *scope {
	if s == nil {
		return nil
	}
	parent := cloneScope(s.parent)
	clone := &scope{parent: parent, items: make(map[string]binding, len(s.items))}
	for name, b := range s.items {
		b.env = parent
		if b.cell != nil {
			b.cell = &bindingCell{value: b.cell.value}
		}
		if b.value != nil {
			value := *b.value
			b.value = &value
		}
		clone.items[name] = b
	}
	return clone
}
