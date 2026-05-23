package osiris

import "fmt"

type matchState struct {
	bindings map[string]Value
}

// evalMatch handles the eval match step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func evalMatch(ev *evaluator, expr *matchExpr, scope *scope) (Value, error) {
	subject, err := ev.eval(expr.subject, scope)
	if err != nil {
		return nullValue(), err
	}
	for _, arm := range expr.arms {
		state := matchState{bindings: make(map[string]Value, 4)}
		ok, err := matchPattern(arm.pattern, subject, &state)
		if err != nil {
			return nullValue(), err
		}
		if !ok {
			continue
		}
		return ev.eval(arm.body, extendScope(scope, state.bindings))
	}
	return nullValue(), fmt.Errorf("no match for %s", subject.Kind())
}

// matchPattern handles the match pattern step in the surrounding package and
// returns bool, error.
//
// It returns bool and an error. Callers should only trust the value when the
// error is nil.
func matchPattern(pattern Pattern, value Value, state *matchState) (bool, error) {
	switch p := pattern.(type) {
	case wildcardPattern:
		return true, nil
	case bindPattern:
		if prev, ok := state.bindings[p.name]; ok {
			return prev.Equal(value), nil
		}
		state.bindings[p.name] = value
		return true, nil
	case literalPattern:
		return p.value.Equal(value), nil
	case listPattern:
		items, err := value.AsList()
		if err != nil {
			return false, nil
		}
		if len(items) != len(p.items) {
			return false, nil
		}
		for i := range p.items {
			ok, err := matchPattern(p.items[i], items[i], state)
			if err != nil || !ok {
				return false, err
			}
		}
		return true, nil
	default:
		return false, fmt.Errorf("unknown pattern %T", pattern)
	}
}

// extendScope handles the extend scope step in the surrounding package and
// returns *scope.
//
// It returns *scope so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func extendScope(parent *scope, bindings map[string]Value) *scope {
	if len(bindings) == 0 {
		return parent
	}
	items := make(map[string]binding, len(bindings))
	for name, value := range bindings {
		items[name] = binding{
			expr: &literalExpr{value: value},
			env:  nil,
		}
	}
	return &scope{parent: parent, items: items}
}
