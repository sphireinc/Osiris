package osiris

// binding is the lexical definition carried by a scope. Keeping scopes in a
// small component separates shared name-resolution state from evaluator,
// session, macro, match, and bytecode execution machinery.
type binding struct {
	expr Expr
	env  *scope
}

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
