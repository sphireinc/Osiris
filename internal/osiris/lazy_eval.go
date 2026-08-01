package osiris

import "fmt"

const iterativeLazyExpressionDepth = 128

type simpleFingerprintResult struct {
	hash  string
	depth int
}

type simpleFingerprintFrame struct {
	expr     Expr
	env      *scope
	key      fpKey
	stage    uint8
	children []simpleFingerprintResult
	active   bool
}

func isSimpleLazyExpr(expr Expr) bool {
	switch expr.(type) {
	case *literalExpr, *identExpr, *unaryExpr, *binaryExpr:
		return true
	default:
		return false
	}
}

// fingerprintSimple computes the potentially very deep identifier, unary,
// and binary dependency graph with an explicit continuation stack. Other
// expression forms retain the established recursive implementation; when one
// appears as a leaf of a simple graph it is fingerprinted as one opaque leaf.
func (ev *evaluator) fingerprintSimple(expr Expr, env *scope) (string, int, bool, error) {
	if !isSimpleLazyExpr(expr) {
		return "", 0, false, nil
	}
	frames := []simpleFingerprintFrame{{expr: expr, env: env}}
	active := make(map[fpKey]struct{})
	var root simpleFingerprintResult
	steps := 0

	complete := func(result simpleFingerprintResult, cache bool) {
		index := len(frames) - 1
		frame := frames[index]
		if frame.active {
			delete(active, frame.key)
		}
		if cache {
			ev.fpMu.Lock()
			ev.fpMemo[frame.key] = result.hash
			ev.fpDepth[frame.key] = result.depth
			ev.fpMu.Unlock()
		}
		frames = frames[:index]
		if len(frames) == 0 {
			root = result
			return
		}
		parent := &frames[len(frames)-1]
		parent.children = append(parent.children, result)
	}

	for len(frames) > 0 {
		steps++
		if steps&255 == 0 {
			if err := ev.ctx.Err(); err != nil {
				return "", 0, true, err
			}
		}
		frame := &frames[len(frames)-1]
		if frame.stage == 0 {
			frame.key = fpKey{expr: frame.expr, scope: frame.env}
			ev.fpMu.Lock()
			memo, memoized := ev.fpMemo[frame.key]
			depth := ev.fpDepth[frame.key]
			ev.fpMu.Unlock()
			if memoized && depth > 0 {
				complete(simpleFingerprintResult{hash: memo, depth: depth}, false)
				continue
			}
			if _, cycle := active[frame.key]; cycle {
				complete(simpleFingerprintResult{
					hash:  hashParts("cycle", fmt.Sprintf("%T", frame.expr)),
					depth: iterativeLazyExpressionDepth,
				}, false)
				continue
			}
			active[frame.key] = struct{}{}
			frame.active = true

			switch n := frame.expr.(type) {
			case *literalExpr:
				complete(simpleFingerprintResult{
					hash:  hashParts("lit", fingerprintValue(n.value)),
					depth: 1,
				}, true)
			case *identExpr:
				binding, ok := frame.env.lookup(n.name)
				if ok {
					frame.stage = 1
					frames = append(frames, simpleFingerprintFrame{expr: binding.expr, env: binding.env})
					continue
				}
				if value, exists := ev.rt.Constants[n.name]; exists {
					complete(simpleFingerprintResult{
						hash:  hashParts("const", n.name, value.String()),
						depth: 1,
					}, true)
					continue
				}
				if _, exists := ev.builtin[n.name]; exists {
					complete(simpleFingerprintResult{hash: hashParts("builtin", n.name), depth: 1}, true)
					continue
				}
				if _, exists := ev.rt.ContextBuiltins[n.name]; exists {
					complete(simpleFingerprintResult{hash: hashParts("context-builtin", n.name), depth: 1}, true)
					continue
				}
				return "", 0, true, fmt.Errorf("undefined name %q", n.name)
			case *unaryExpr:
				frame.stage = 1
				frames = append(frames, simpleFingerprintFrame{expr: n.expr, env: frame.env})
			case *binaryExpr:
				frame.stage = 1
				frames = append(frames, simpleFingerprintFrame{expr: n.left, env: frame.env})
			default:
				fingerprint, err := ev.fingerprintRecursive(frame.expr, frame.env)
				if err != nil {
					return "", 0, true, err
				}
				complete(simpleFingerprintResult{hash: fingerprint, depth: 1}, true)
			}
			continue
		}

		switch n := frame.expr.(type) {
		case *identExpr:
			child := frame.children[0]
			complete(simpleFingerprintResult{hash: child.hash, depth: child.depth + 1}, true)
		case *unaryExpr:
			child := frame.children[0]
			complete(simpleFingerprintResult{
				hash:  hashParts("unary", n.op.String(), child.hash),
				depth: child.depth + 1,
			}, true)
		case *binaryExpr:
			if frame.stage == 1 {
				frame.stage = 2
				frames = append(frames, simpleFingerprintFrame{expr: n.right, env: frame.env})
				continue
			}
			left, right := frame.children[0], frame.children[1]
			depth := left.depth
			if right.depth > depth {
				depth = right.depth
			}
			complete(simpleFingerprintResult{
				hash:  hashParts("binary", n.op.String(), left.hash, right.hash),
				depth: depth + 1,
			}, true)
		default:
			return "", 0, true, fmt.Errorf("internal error: unsupported simple fingerprint frame %T", frame.expr)
		}
	}
	return root.hash, root.depth, true, nil
}

func (ev *evaluator) shouldEvaluateSimpleIteratively(expr Expr, env *scope) bool {
	if !isSimpleLazyExpr(expr) {
		return false
	}
	key := fpKey{expr: expr, scope: env}
	ev.fpMu.Lock()
	depth := ev.fpDepth[key]
	ev.fpMu.Unlock()
	return depth >= iterativeLazyExpressionDepth
}

type simpleEvaluationFrame struct {
	expr   Expr
	env    *scope
	key    fpKey
	hash   string
	stage  uint8
	values []Value
	active bool
}

// evalSimpleIterative forces a deep simple dependency graph without adding a
// Go frame for every lazy binding. It is entered only after the fingerprint
// walk proves the graph is deep; ordinary shallow expressions retain their
// established scheduler and error-shaping path.
func (ev *evaluator) evalSimpleIterative(expr Expr, env *scope) (Value, error) {
	frames := []simpleEvaluationFrame{{expr: expr, env: env}}
	active := make(map[fpKey]struct{})
	var root Value
	steps := 0

	complete := func(value Value, cache bool) {
		index := len(frames) - 1
		frame := frames[index]
		if frame.active {
			delete(active, frame.key)
		}
		if cache {
			ev.mu.Lock()
			ev.cache[frame.hash] = value
			ev.mu.Unlock()
		}
		frames = frames[:index]
		if len(frames) == 0 {
			root = value
			return
		}
		parent := &frames[len(frames)-1]
		parent.values = append(parent.values, value)
	}

	for len(frames) > 0 {
		steps++
		if steps&255 == 0 {
			if err := ev.ctx.Err(); err != nil {
				return nullValue(), err
			}
		}
		frame := &frames[len(frames)-1]
		if frame.stage == 0 {
			frame.key = fpKey{expr: frame.expr, scope: frame.env}
			ev.fpMu.Lock()
			frame.hash = ev.fpMemo[frame.key]
			ev.fpMu.Unlock()
			if frame.hash == "" {
				return nullValue(), fmt.Errorf("internal error: missing lazy fingerprint for %T", frame.expr)
			}
			ev.mu.Lock()
			cached, memoized := ev.cache[frame.hash]
			ev.mu.Unlock()
			if memoized {
				complete(cached, false)
				continue
			}
			if _, cycle := active[frame.key]; cycle {
				return nullValue(), fmt.Errorf("cyclic lazy binding dependency")
			}
			active[frame.key] = struct{}{}
			frame.active = true

			switch n := frame.expr.(type) {
			case *literalExpr:
				complete(n.value, true)
			case *identExpr:
				binding, ok := frame.env.lookup(n.name)
				if ok {
					frame.stage = 1
					frames = append(frames, simpleEvaluationFrame{expr: binding.expr, env: binding.env})
					continue
				}
				if value, exists := ev.rt.Constants[n.name]; exists {
					complete(value, true)
					continue
				}
				if fn, exists := ev.rt.ContextBuiltins[n.name]; exists {
					complete(contextFunctionValueOf(n.name, fn), true)
					continue
				}
				if fn, exists := ev.builtin[n.name]; exists {
					complete(functionValueOf(n.name, fn), true)
					continue
				}
				return nullValue(), fmt.Errorf("undefined name %q", n.name)
			case *unaryExpr:
				frame.stage = 1
				frames = append(frames, simpleEvaluationFrame{expr: n.expr, env: frame.env})
			case *binaryExpr:
				frame.stage = 1
				frames = append(frames, simpleEvaluationFrame{expr: n.left, env: frame.env})
			default:
				value, err := computeExpr(ev, frame.expr, frame.env, "")
				if err != nil {
					return nullValue(), err
				}
				complete(value, true)
			}
			continue
		}

		switch n := frame.expr.(type) {
		case *identExpr:
			complete(frame.values[0], true)
		case *unaryExpr:
			value, err := applyUnary(n.op, frame.values[0])
			if err != nil {
				return nullValue(), err
			}
			complete(value, true)
		case *binaryExpr:
			if frame.stage == 1 {
				left := frame.values[0]
				if n.op == tokenAndAnd && !left.Truthy() {
					complete(boolValue(false), true)
					continue
				}
				if n.op == tokenOrOr && left.Truthy() {
					complete(boolValue(true), true)
					continue
				}
				frame.stage = 2
				frames = append(frames, simpleEvaluationFrame{expr: n.right, env: frame.env})
				continue
			}
			left, right := frame.values[0], frame.values[1]
			if n.op == tokenAndAnd || n.op == tokenOrOr {
				complete(boolValue(right.Truthy()), true)
				continue
			}
			var (
				value Value
				err   error
			)
			if n.op == tokenAmp || n.op == tokenPipe || n.op == tokenCaret || n.op == tokenShiftLeft || n.op == tokenShiftRight {
				value, err = evalBitwiseBinary(n.op, left, right)
			} else {
				value, err = vmBinary(ev.ctx, n.op, left, right)
			}
			if err != nil {
				return nullValue(), err
			}
			complete(value, true)
		default:
			return nullValue(), fmt.Errorf("internal error: unsupported simple evaluation frame %T", frame.expr)
		}
	}
	return root, nil
}
