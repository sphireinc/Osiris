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
		armScope := extendScope(scope, state.bindings)
		if arm.guard != nil {
			guard, err := ev.eval(arm.guard, armScope)
			if err != nil {
				return nullValue(), err
			}
			if !guard.Truthy() {
				continue
			}
		}
		return ev.eval(arm.body, armScope)
	}
	return nullValue(), noMatchError(subject, expr.arms)
}

func noMatchError(subject Value, arms []matchArm) error {
	if subject.kind == kindEnum && subject.enum != nil {
		for _, arm := range arms {
			switch arm.pattern.(type) {
			case wildcardPattern, bindPattern:
				return fmt.Errorf("no matching arm for enum %s", subject.enum.typeName)
			}
		}
		return fmt.Errorf("non-exhaustive match for enum %s (no arm matched %s)", subject.enum.typeName, subject.String())
	}
	return fmt.Errorf("no match for %s", subject.Kind())
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
	case enumPattern:
		if value.kind != kindEnum || value.enum == nil || value.enum.typeName != p.typeName || value.enum.caseName != p.caseName || len(value.enum.payload) != len(p.items) {
			return false, nil
		}
		for i, child := range p.items {
			ok, err := matchPattern(child, value.enum.payload[i], state)
			if err != nil || !ok {
				return false, err
			}
		}
		return true, nil
	case orPattern:
		for _, alternative := range p.alternatives {
			candidate := matchState{bindings: cloneMatchBindings(state.bindings)}
			ok, err := matchPattern(alternative, value, &candidate)
			if err != nil {
				return false, err
			}
			if ok {
				state.bindings = candidate.bindings
				return true, nil
			}
		}
		return false, nil
	case andPattern:
		if ok, err := matchPattern(p.left, value, state); err != nil || !ok {
			return ok, err
		}
		return matchPattern(p.right, value, state)
	case notPattern:
		candidate := matchState{bindings: cloneMatchBindings(state.bindings)}
		ok, err := matchPattern(p.inner, value, &candidate)
		return !ok, err
	case typePattern:
		if !matchesTypePattern(p.typeName, value) {
			return false, nil
		}
		if p.binding == "_" || p.binding == "" {
			return true, nil
		}
		if previous, exists := state.bindings[p.binding]; exists {
			return previous.Equal(value), nil
		}
		state.bindings[p.binding] = value
		return true, nil
	case relationalPattern:
		cmp, err := value.Compare(p.value)
		if err != nil {
			return false, nil
		}
		switch p.op {
		case tokenLess:
			return cmp < 0, nil
		case tokenLessEqual:
			return cmp <= 0, nil
		case tokenGreater:
			return cmp > 0, nil
		case tokenGreaterEqual:
			return cmp >= 0, nil
		}
		return false, fmt.Errorf("unknown relational pattern operator %s", p.op)
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
	case structPattern:
		if value.kind != kindStruct || value.structType == nil || value.structValue == nil || value.structType.name != p.typeName {
			return false, nil
		}
		for _, field := range p.fields {
			index, exists := value.structType.byName[field.name]
			if !exists || index < 0 || index >= len(value.structValue.fields) {
				return false, nil
			}
			ok, err := matchPattern(field.pattern, value.structValue.fields[index], state)
			if err != nil || !ok {
				return false, err
			}
		}
		return true, nil
	default:
		return false, fmt.Errorf("unknown pattern %T", pattern)
	}
}

func matchesTypePattern(name string, value Value) bool {
	switch name {
	case "number":
		switch value.kind {
		case kindNumber, kindDecimal, kindFixed, kindInterval, kindModInt, kindBigInt, kindBigFloat, kindInt, kindUint, kindQuantity:
			return true
		}
		return false
	case "integer", "int":
		return value.kind == kindInt || value.kind == kindUint || value.kind == kindBigInt
	case "string", "list", "tensor", "bool", "null", "enum", "function", "module", "task", "stream", "error", "result", "option":
		return value.Kind() == name
	default:
		return false
	}
}

func cloneMatchBindings(bindings map[string]Value) map[string]Value {
	clone := make(map[string]Value, len(bindings))
	for name, value := range bindings {
		clone[name] = value
	}
	return clone
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
