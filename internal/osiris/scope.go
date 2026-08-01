package osiris

// binding is the lexical definition carried by a scope. Keeping scopes in a
// small component separates shared name-resolution state from evaluator,
// session, macro, match, and bytecode execution machinery.
type binding struct {
	expr    Expr
	env     *scope
	cell    *bindingCell
	mutable bool
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
		clone.items[name] = b
	}
	return clone
}
