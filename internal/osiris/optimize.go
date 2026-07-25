package osiris

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"math/bits"
)

type optimizeEnv map[string]Expr

// OptimizationPassReport records one deterministic optimizer stage. Before and
// after are formatted Osiris source so reports are useful to both people and
// tools without exposing unstable Go AST internals.
type OptimizationPassReport struct {
	Name             string `json:"name"`
	Status           string `json:"status"`
	Reason           string `json:"reason,omitempty"`
	Before           string `json:"before"`
	After            string `json:"after"`
	BeforeStatements int    `json:"before_statements"`
	AfterStatements  int    `json:"after_statements"`
}

// OptimizationReport is the stable, machine-readable account of an optimizer
// run. Passes always appear in pipeline order. A pass reported as
// "not_implemented" is deliberately absent rather than silently speculative.
type OptimizationReport struct {
	Version         int                      `json:"version"`
	SourceName      string                   `json:"source_name"`
	OptimizedSource string                   `json:"optimized_source"`
	Passes          []OptimizationPassReport `json:"passes"`
}

// OptimizeProgram handles the optimize program step in the surrounding package
// and returns *Program.
//
// It returns *Program so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func OptimizeProgram(prog *Program) *Program {
	optimized, _ := OptimizeProgramWithReport(prog)
	return optimized
}

// OptimizeProgramWithReport applies the conservative source optimizer and
// reports every available and intentionally unavailable optimization stage.
// It is the reporting entry point; OptimizeProgram preserves the compact API
// used by the compiler pipeline.
func OptimizeProgramWithReport(prog *Program) (*Program, OptimizationReport) {
	if prog == nil {
		return nil, OptimizationReport{Version: 1}
	}
	report := OptimizationReport{Version: 1, SourceName: prog.SourceName}
	before := prog
	stmts, _, pureNames := optimizeStmtSequence(prog.Statements, nil)
	simplified := &Program{
		SourceName: prog.SourceName,
		Statements: stmts,
	}
	report.Passes = append(report.Passes, optimizationPassReport(
		"simplify-and-propagate", "applied", "", before, simplified,
	))
	optimized := &Program{
		SourceName: prog.SourceName,
		Statements: sweepDeadStatements(simplified.Statements, nil, pureNames),
	}
	report.Passes = append(report.Passes, optimizationPassReport(
		"dead-statement-elimination", "applied", "", simplified, optimized,
	))
	for _, unavailable := range []struct{ name, reason string }{
		{"loop-invariant-code-motion", "the source optimizer does not prove loop invariance across effects or aliases"},
		{"bounds-check-elimination", "the source optimizer has no range proof for index, shape, and mutation semantics"},
		{"loop-unrolling", "the source optimizer has no profitability model or bounded-trip-count proof"},
		{"scalarization", "the source optimizer has no aggregate escape or alias analysis"},
	} {
		report.Passes = append(report.Passes, optimizationPassReport(
			unavailable.name, "not_implemented", unavailable.reason, optimized, optimized,
		))
	}
	report.OptimizedSource = FormatProgram(optimized)
	return optimized, report
}

func optimizationPassReport(name, status, reason string, before, after *Program) OptimizationPassReport {
	beforeSource := FormatProgram(before)
	afterSource := FormatProgram(after)
	if status == "applied" && beforeSource == afterSource {
		status = "no_change"
		reason = "no eligible transformation was proven safe"
	}
	return OptimizationPassReport{
		Name:             name,
		Status:           status,
		Reason:           reason,
		Before:           beforeSource,
		After:            afterSource,
		BeforeStatements: len(before.Statements),
		AfterStatements:  len(after.Statements),
	}
}

// optimizeStmt handles the optimize stmt step in the surrounding package and
// returns Stmt.
//
// It returns Stmt so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func optimizeStmt(stmt Stmt) Stmt {
	opt, _ := optimizeStmtWithEnv(stmt, nil)
	return opt
}

// optimizeStmtWithEnv handles the optimize stmt with env step in the
// surrounding package and returns Stmt, optimizeEnv.
//
// It returns Stmt, optimizeEnv so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func optimizeStmtWithEnv(stmt Stmt, env optimizeEnv) (Stmt, optimizeEnv) {
	switch s := stmt.(type) {
	case letStmt:
		expr := optimizeExprWithEnv(s.Expr, env)
		if env == nil {
			env = make(optimizeEnv)
		}
		if isPureExpr(expr, env, nil) {
			env[s.Name] = expr
		} else {
			delete(env, s.Name)
		}
		return letStmt{Name: s.Name, Expr: expr}, env
	case emitStmt:
		return emitStmt{Expr: optimizeExprWithEnv(s.Expr, env)}, env
	case macroStmt:
		return macroStmt{Name: s.Name, Params: append([]string(nil), s.Params...), Body: optimizeExprWithEnv(s.Body, env)}, env
	case funcStmt:
		fnEnv := cloneOptimizeEnv(env)
		if fnEnv == nil {
			fnEnv = make(optimizeEnv)
		}
		return funcStmt{Name: s.Name, Params: append([]string(nil), s.Params...), Body: optimizeExprWithEnv(s.Body, fnEnv)}, env
	case returnStmt:
		return returnStmt{Expr: optimizeExprWithEnv(s.Expr, env)}, env
	case whileStmt:
		// Loops are optimization barriers for the current source-to-source pass.
		// Their bodies can mutate loop-carried state by shadowing bindings, so we
		// must not let outer bindings substitute into the condition or body and
		// accidentally freeze the loop into a constant form.
		return whileStmt{Cond: optimizeExprWithEnv(s.Cond, nil), Body: optimizeExprWithEnv(s.Body, nil)}, nil
	case forStmt:
		return forStmt{Name: s.Name, Iter: optimizeExprWithEnv(s.Iter, nil), Body: optimizeExprWithEnv(s.Body, nil)}, nil
	case packageStmt, exportStmt, importStmt, breakStmt, continueStmt:
		return stmt, env
	default:
		return stmt, env
	}
}

// optimizeExpr handles the optimize expr step in the surrounding package and
// returns Expr.
//
// It returns Expr so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func optimizeExpr(expr Expr) Expr {
	return optimizeExprWithEnv(expr, nil)
}

// optimizeExprWithEnv handles the optimize expr with env step in the
// surrounding package and returns Expr.
//
// It returns Expr so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func optimizeExprWithEnv(expr Expr, env optimizeEnv) Expr {
	return optimizeExprWithEnvSeen(expr, env, nil)
}

// optimizeExprWithEnvSeen handles the optimize expr with env seen step in the
// surrounding package and returns Expr.
//
// It returns Expr so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func optimizeExprWithEnvSeen(expr Expr, env optimizeEnv, seen map[string]struct{}) Expr {
	switch n := expr.(type) {
	case *literalExpr, *identExpr:
		if id, ok := expr.(*identExpr); ok {
			if env != nil {
				if repl, ok := env[id.name]; ok {
					if seen == nil {
						seen = make(map[string]struct{})
					}
					if _, ok := seen[id.name]; ok {
						return expr
					}
					nextSeen := make(map[string]struct{}, len(seen)+1)
					for name := range seen {
						nextSeen[name] = struct{}{}
					}
					nextSeen[id.name] = struct{}{}
					return optimizeExprWithEnvSeen(repl, env, nextSeen)
				}
			}
			if named, ok := namedConstantValue(id.name); ok {
				return &literalExpr{value: named}
			}
		}
		return expr
	case *memberExpr:
		return &memberExpr{object: optimizeExprWithEnvSeen(n.object, env, seen), name: n.name}
	case *unaryExpr:
		inner := optimizeExprWithEnvSeen(n.expr, env, seen)
		if lit, ok := inner.(*literalExpr); ok {
			if folded, err := applyUnary(n.op, lit.value); err == nil {
				return &literalExpr{value: folded}
			}
		}
		return &unaryExpr{op: n.op, expr: inner}
	case *binaryExpr:
		left := optimizeExprWithEnvSeen(n.left, env, seen)
		right := optimizeExprWithEnvSeen(n.right, env, seen)
		if lf, lok := left.(*literalExpr); lok {
			if rf, rok := right.(*literalExpr); rok {
				if folded, err := vmBinary(context.Background(), n.op, lf.value, rf.value); err == nil {
					return &literalExpr{value: folded}
				}
				if n.op == tokenAmp || n.op == tokenPipe || n.op == tokenCaret || n.op == tokenShiftLeft || n.op == tokenShiftRight {
					if folded, err := evalBitwiseBinary(n.op, lf.value, rf.value); err == nil {
						return &literalExpr{value: folded}
					}
				}
			}
		}
		if lit, ok := left.(*literalExpr); ok {
			if optimized := optimizeBinaryWithLeftLiteral(n.op, lit.value, right); optimized != nil {
				return optimized
			}
		}
		if lit, ok := right.(*literalExpr); ok {
			if optimized := optimizeBinaryWithRightLiteral(n.op, left, lit.value); optimized != nil {
				return optimized
			}
		}
		if optimized := strengthReduceBinary(n.op, left, right, env); optimized != nil {
			return optimized
		}
		return &binaryExpr{op: n.op, left: left, right: right}
	case *callExpr:
		callee := optimizeExprWithEnvSeen(n.callee, env, seen)
		args := make([]Expr, len(n.args))
		allConst := true
		for i, arg := range n.args {
			args[i] = optimizeExprWithEnvSeen(arg, env, seen)
			if _, ok := constValueExpr(args[i]); !ok {
				allConst = false
			}
		}
		if name, ok := callee.(*identExpr); ok {
			if allConst {
				if values, ok := constValues(args); ok {
					if builtin, ok := pureBuiltinByName(name.name); ok {
						if v, err := builtin(values); err == nil {
							return &literalExpr{value: v}
						}
					}
				}
			}
		}
		if name, ok := specialFormName(callee); ok {
			switch name {
			case "if":
				return optimizeIfCall(args)
			case "and":
				return optimizeAndCall(args)
			case "or":
				return optimizeOrCall(args)
			}
		}
		return &callExpr{callee: callee, args: args}
	case *listExpr:
		items := make([]Expr, len(n.items))
		for i, item := range n.items {
			items[i] = optimizeExprWithEnvSeen(item, env, seen)
		}
		return &listExpr{items: items}
	case *blockExpr:
		stmts, nextEnv, pureNames := optimizeStmtSequence(n.stmts, env)
		var tail Expr
		if n.tail != nil {
			tail = optimizeExprWithEnvSeen(n.tail, nextEnv, seen)
		}
		return &blockExpr{stmts: sweepDeadStatements(stmts, tail, pureNames), tail: tail}
	case *tryExpr:
		return &tryExpr{body: optimizeExprWithEnvSeen(n.body, env, seen), catchName: n.catchName, handler: optimizeExprWithEnvSeen(n.handler, env, seen)}
	case *propagateExpr:
		return &propagateExpr{expr: optimizeExprWithEnvSeen(n.expr, env, seen), Span: n.Span}
	case *matchExpr:
		arms := make([]matchArm, len(n.arms))
		for i, arm := range n.arms {
			arms[i] = matchArm{pattern: arm.pattern, guard: optimizeExprWithEnvSeen(arm.guard, env, seen), body: optimizeExprWithEnvSeen(arm.body, env, seen), isDefault: arm.isDefault}
		}
		return &matchExpr{subject: optimizeExprWithEnvSeen(n.subject, env, seen), arms: arms, keyword: n.keyword}
	default:
		return expr
	}
}

// optimizeStmtSequence handles the optimize stmt sequence step in the
// surrounding package and returns []Stmt, optimizeEnv, map[string]value.
//
// It returns []Stmt, optimizeEnv, map[string]value so callers can continue
// with the next stage of the pipeline without re-deriving the same
// information.
func optimizeStmtSequence(stmts []Stmt, env optimizeEnv) ([]Stmt, optimizeEnv, map[string]struct{}) {
	localEnv := cloneOptimizeEnv(env)
	if localEnv == nil {
		localEnv = make(optimizeEnv)
	}
	seen := make(map[string]string)
	pureNames := make(map[string]struct{})
	optimized := make([]Stmt, 0, len(stmts))
	for _, stmt := range stmts {
		opt, nextEnv := optimizeStmtWithEnv(stmt, localEnv)
		if letv, ok := opt.(letStmt); ok {
			if isPureExpr(letv.Expr, localEnv, nil) {
				key := formatExpr(letv.Expr, 0)
				if prev, seenOK := seen[key]; seenOK && prev != letv.Name {
					letv.Expr = &identExpr{name: prev}
					opt = letv
				} else {
					seen[key] = letv.Name
				}
			}
			if nextEnv == nil {
				nextEnv = make(optimizeEnv)
			}
			nextEnv[letv.Name] = letv.Expr
			if isPureExpr(letv.Expr, localEnv, nil) {
				pureNames[letv.Name] = struct{}{}
			}
		}
		if fnv, ok := opt.(funcStmt); ok {
			pureNames[fnv.Name] = struct{}{}
		}
		if opt != nil {
			optimized = append(optimized, opt)
		}
		localEnv = nextEnv
	}
	return optimized, localEnv, pureNames
}

// cloneOptimizeEnv handles the clone optimize env step in the surrounding
// package and returns optimizeEnv.
//
// It returns optimizeEnv so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func cloneOptimizeEnv(env optimizeEnv) optimizeEnv {
	if len(env) == 0 {
		return nil
	}
	out := make(optimizeEnv, len(env))
	for k, v := range env {
		out[k] = v
	}
	return out
}

// sweepDeadStatements handles the sweep dead statements step in the
// surrounding package and returns []Stmt.
//
// It returns []Stmt so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func sweepDeadStatements(stmts []Stmt, tail Expr, pureNames map[string]struct{}) []Stmt {
	if len(stmts) == 0 {
		return nil
	}
	live := collectRawRefsExpr(tail)
	out := make([]Stmt, 0, len(stmts))
	for i := len(stmts) - 1; i >= 0; i-- {
		stmt := stmts[i]
		refs := collectRawRefsStmt(stmt)
		switch st := stmt.(type) {
		case letStmt:
			if !containsName(live, st.Name) && isPureExpr(st.Expr, nil, pureNames) {
				continue
			}
			delete(live, st.Name)
			addNames(live, refs)
			out = append(out, stmt)
		case funcStmt:
			if !containsName(live, st.Name) {
				continue
			}
			delete(live, st.Name)
			addNames(live, refs)
			out = append(out, stmt)
		default:
			addNames(live, refs)
			out = append(out, stmt)
		}
	}
	reverseStatements(out)
	return out
}

// reverseStatements handles the reverse statements step in the surrounding
// package and returns no result.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func reverseStatements(stmts []Stmt) {
	for i, j := 0, len(stmts)-1; i < j; i, j = i+1, j-1 {
		stmts[i], stmts[j] = stmts[j], stmts[i]
	}
}

// collectRawRefsExpr handles the collect raw refs expr step in the surrounding
// package and returns map[string]value.
//
// It returns map[string]value so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func collectRawRefsExpr(expr Expr) map[string]struct{} {
	out := make(map[string]struct{})
	collectRawRefsExprInto(expr, out)
	return out
}

// collectRawRefsExprInto handles the collect raw refs expr into step in the
// surrounding package and returns no result.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func collectRawRefsExprInto(expr Expr, out map[string]struct{}) {
	switch n := expr.(type) {
	case nil, *literalExpr:
		return
	case *identExpr:
		out[n.name] = struct{}{}
	case *memberExpr:
		collectRawRefsExprInto(n.object, out)
	case *unaryExpr:
		collectRawRefsExprInto(n.expr, out)
	case *binaryExpr:
		collectRawRefsExprInto(n.left, out)
		collectRawRefsExprInto(n.right, out)
	case *callExpr:
		collectRawRefsExprInto(n.callee, out)
		for _, arg := range n.args {
			collectRawRefsExprInto(arg, out)
		}
	case *listExpr:
		for _, item := range n.items {
			collectRawRefsExprInto(item, out)
		}
	case *blockExpr:
		for _, stmt := range n.stmts {
			collectRawRefsStmtInto(stmt, out)
		}
		collectRawRefsExprInto(n.tail, out)
	case *tryExpr:
		collectRawRefsExprInto(n.body, out)
		collectRawRefsExprInto(n.handler, out)
	case *propagateExpr:
		collectRawRefsExprInto(n.expr, out)
	case *matchExpr:
		collectRawRefsExprInto(n.subject, out)
		for _, arm := range n.arms {
			collectRawRefsExprInto(arm.body, out)
		}
	}
}

// collectRawRefsStmt handles the collect raw refs stmt step in the surrounding
// package and returns map[string]value.
//
// It returns map[string]value so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func collectRawRefsStmt(stmt Stmt) map[string]struct{} {
	out := make(map[string]struct{})
	collectRawRefsStmtInto(stmt, out)
	return out
}

// collectRawRefsStmtInto handles the collect raw refs stmt into step in the
// surrounding package and returns no result.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func collectRawRefsStmtInto(stmt Stmt, out map[string]struct{}) {
	switch st := stmt.(type) {
	case letStmt:
		collectRawRefsExprInto(st.Expr, out)
	case emitStmt:
		collectRawRefsExprInto(st.Expr, out)
	case macroStmt:
		collectRawRefsExprInto(st.Body, out)
	case funcStmt:
		collectRawRefsExprInto(st.Body, out)
	}
	switch st := stmt.(type) {
	case whileStmt:
		collectRawRefsExprInto(st.Cond, out)
		collectRawRefsExprInto(st.Body, out)
	case forStmt:
		collectRawRefsExprInto(st.Iter, out)
		collectRawRefsExprInto(st.Body, out)
	case returnStmt:
		collectRawRefsExprInto(st.Expr, out)
	case importStmt, packageStmt, exportStmt, breakStmt, continueStmt:
	}
}

// addNames handles the add names step in the surrounding package and returns
// no result.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func addNames(dst map[string]struct{}, src map[string]struct{}) {
	for name := range src {
		dst[name] = struct{}{}
	}
}

// containsName handles the contains name step in the surrounding package and
// returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func containsName(set map[string]struct{}, name string) bool {
	_, ok := set[name]
	return ok
}

// isPureExpr handles the is pure expr step in the surrounding package and
// returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func isPureExpr(expr Expr, env optimizeEnv, pureNames map[string]struct{}) bool {
	return isPureExprSeen(expr, env, pureNames, nil)
}

// isPureExprSeen handles the is pure expr seen step in the surrounding package
// and returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func isPureExprSeen(expr Expr, env optimizeEnv, pureNames map[string]struct{}, seen map[string]struct{}) bool {
	switch n := expr.(type) {
	case nil, *literalExpr:
		return true
	case *identExpr:
		if env != nil {
			if repl, ok := env[n.name]; ok {
				if seen == nil {
					seen = make(map[string]struct{})
				}
				if _, ok := seen[n.name]; ok {
					return false
				}
				nextSeen := make(map[string]struct{}, len(seen)+1)
				for name := range seen {
					nextSeen[name] = struct{}{}
				}
				nextSeen[n.name] = struct{}{}
				return isPureExprSeen(repl, env, pureNames, nextSeen)
			}
		}
		if pureNames != nil {
			if _, ok := pureNames[n.name]; ok {
				return true
			}
		}
		_, ok := purityBuiltinValueInfo(n.name)
		return ok
	case *memberExpr:
		return isPureExprSeen(n.object, env, pureNames, seen)
	case *unaryExpr:
		return isPureExprSeen(n.expr, env, pureNames, seen)
	case *binaryExpr:
		return isPureExprSeen(n.left, env, pureNames, seen) && isPureExprSeen(n.right, env, pureNames, seen)
	case *callExpr:
		if name, ok := specialFormName(n.callee); ok {
			switch name {
			case "if":
				return len(n.args) == 3 && isPureExprSeen(n.args[0], env, pureNames, seen) && isPureExprSeen(n.args[1], env, pureNames, seen) && isPureExprSeen(n.args[2], env, pureNames, seen)
			case "and", "or":
				return len(n.args) == 2 && isPureExprSeen(n.args[0], env, pureNames, seen) && isPureExprSeen(n.args[1], env, pureNames, seen)
			}
		}
		if ident, ok := n.callee.(*identExpr); ok {
			if _, ok := pureBuiltinByName(ident.name); ok {
				for _, arg := range n.args {
					if !isPureExprSeen(arg, env, pureNames, seen) {
						return false
					}
				}
				return true
			}
		}
		return false
	case *listExpr:
		for _, item := range n.items {
			if !isPureExprSeen(item, env, pureNames, seen) {
				return false
			}
		}
		return true
	case *blockExpr:
		shadow := cloneOptimizeEnv(env)
		for _, stmt := range n.stmts {
			switch st := stmt.(type) {
			case letStmt:
				if !isPureExprSeen(st.Expr, shadow, pureNames, nil) {
					return false
				}
				if shadow == nil {
					shadow = make(optimizeEnv)
				}
				shadow[st.Name] = st.Expr
			case funcStmt, importStmt, emitStmt, returnStmt, whileStmt, forStmt, breakStmt, continueStmt, packageStmt, exportStmt, macroStmt:
				return false
			}
		}
		return isPureExprSeen(n.tail, shadow, pureNames, nil)
	case *tryExpr, *matchExpr, *propagateExpr:
		return false
	default:
		return false
	}
}

// optimizeBinaryWithLeftLiteral handles the optimize binary with left literal
// step in the surrounding package and returns Expr.
//
// It returns Expr so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func optimizeBinaryWithLeftLiteral(op tokenKind, left Value, right Expr) Expr {
	if isZeroValue(left) {
		switch op {
		case tokenPlus:
			return right
		case tokenMinus:
			return &unaryExpr{op: tokenMinus, expr: right}
		case tokenStar:
			return &literalExpr{value: left}
		case tokenSlash:
			return &literalExpr{value: left}
		}
	}
	return nil
}

// optimizeBinaryWithRightLiteral handles the optimize binary with right
// literal step in the surrounding package and returns Expr.
//
// It returns Expr so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func optimizeBinaryWithRightLiteral(op tokenKind, left Expr, right Value) Expr {
	if isZeroValue(right) {
		switch op {
		case tokenPlus, tokenMinus:
			return left
		case tokenStar:
			return &literalExpr{value: right}
		}
	}
	if isOneValue(right) && op == tokenStar {
		return left
	}
	if isOneValue(right) && op == tokenSlash {
		return left
	}
	return nil
}

// strengthReduceBinary handles the strength reduce binary step in the
// surrounding package and returns Expr.
//
// It returns Expr so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func strengthReduceBinary(op tokenKind, left, right Expr, env optimizeEnv) Expr {
	switch op {
	case tokenStar:
		if shift, ok := powerOfTwoShift(left); ok && isUintishExpr(right, env) {
			return &binaryExpr{op: tokenShiftLeft, left: right, right: uintShiftLiteral(shift)}
		}
		if shift, ok := powerOfTwoShift(right); ok && isUintishExpr(left, env) {
			return &binaryExpr{op: tokenShiftLeft, left: left, right: uintShiftLiteral(shift)}
		}
	case tokenSlash:
		if shift, ok := powerOfTwoShift(right); ok && isUintishExpr(left, env) {
			return &binaryExpr{op: tokenShiftRight, left: left, right: uintShiftLiteral(shift)}
		}
	}
	return nil
}

// powerOfTwoShift handles the power of two shift step in the surrounding
// package and returns uint64, bool.
//
// It returns uint64, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func powerOfTwoShift(expr Expr) (uint64, bool) {
	lit, ok := expr.(*literalExpr)
	if !ok {
		return 0, false
	}
	switch lit.value.kind {
	case kindUint:
		u, _, err := unsignedIntFromValue(lit.value)
		if err != nil || u.Sign() == 0 || u.BitLen() > 64 || u.Uint64() == 0 || u.Uint64()&(u.Uint64()-1) != 0 {
			return 0, false
		}
		return uint64(bits.TrailingZeros64(u.Uint64())), true
	default:
		return 0, false
	}
}

// uintShiftLiteral handles the uint shift literal step in the surrounding
// package and returns Expr.
//
// It returns Expr so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func uintShiftLiteral(shift uint64) Expr {
	return &literalExpr{value: uintValue(shift, 64)}
}

// isUintishExpr handles the is uintish expr step in the surrounding package
// and returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func isUintishExpr(expr Expr, env optimizeEnv) bool {
	switch n := expr.(type) {
	case *literalExpr:
		return n.value.kind == kindUint
	case *identExpr:
		if env != nil {
			if repl, ok := env[n.name]; ok {
				return isUintishExpr(repl, env)
			}
		}
		if named, ok := namedConstantValue(n.name); ok {
			return named.kind == kindUint
		}
		return false
	case *callExpr:
		if ident, ok := n.callee.(*identExpr); ok {
			switch ident.name {
			case "uint", "byte", "u8", "u16", "u32", "u64", "u128", "u256", "u512", "u1024":
				return true
			case "len", "take", "range", "sum", "product", "shape":
				return false
			}
		}
		return false
	case *unaryExpr:
		return isUintishExpr(n.expr, env)
	case *binaryExpr:
		switch n.op {
		case tokenAmp, tokenPipe, tokenCaret, tokenShiftLeft, tokenShiftRight:
			return isUintishExpr(n.left, env) && isUintishExpr(n.right, env)
		default:
			return false
		}
	default:
		return false
	}
}

// optimizeIfCall handles the optimize if call step in the surrounding package
// and returns Expr.
//
// It returns Expr so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func optimizeIfCall(args []Expr) Expr {
	if len(args) != 3 {
		return &callExpr{callee: &identExpr{name: "if"}, args: args}
	}
	if lit, ok := args[0].(*literalExpr); ok {
		if lit.value.Truthy() {
			return args[1]
		}
		return args[2]
	}
	return &callExpr{callee: &identExpr{name: "if"}, args: args}
}

// optimizeAndCall handles the optimize and call step in the surrounding
// package and returns Expr.
//
// It returns Expr so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func optimizeAndCall(args []Expr) Expr {
	if len(args) != 2 {
		return &callExpr{callee: &identExpr{name: "and"}, args: args}
	}
	if lit, ok := args[0].(*literalExpr); ok {
		if !lit.value.Truthy() {
			return args[0]
		}
		return args[1]
	}
	return &callExpr{callee: &identExpr{name: "and"}, args: args}
}

// optimizeOrCall handles the optimize or call step in the surrounding package
// and returns Expr.
//
// It returns Expr so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func optimizeOrCall(args []Expr) Expr {
	if len(args) != 2 {
		return &callExpr{callee: &identExpr{name: "or"}, args: args}
	}
	if lit, ok := args[0].(*literalExpr); ok {
		if lit.value.Truthy() {
			return args[0]
		}
		return args[1]
	}
	return &callExpr{callee: &identExpr{name: "or"}, args: args}
}

// pureBuiltinByName handles the pure builtin by name step in the surrounding
// package and returns Builtin, bool.
//
// It returns Builtin, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func pureBuiltinByName(name string) (Builtin, bool) {
	switch name {
	case "cat":
		return catValue, true
	case "hash":
		return builtinHash, true
	case "mix":
		return builtinMix, true
	case "zeros":
		return zerosValue, true
	case "ones":
		return onesValue, true
	case "full":
		return fullValue, true
	case "eye":
		return eyeValue, true
	case "identity":
		return identityValue, true
	case "arange":
		return arangeValue, true
	case "linspace":
		return linspaceValue, true
	case "logspace":
		return logspaceValue, true
	case "upper":
		return builtinUpper, true
	case "lower":
		return builtinLower, true
	case "trim":
		return builtinTrim, true
	case "replace":
		return builtinReplace, true
	case "repeat":
		return repeatBuiltin, true
	case "join":
		return joinValue, true
	case "split":
		return splitBuiltin, true
	case "chunks":
		return chunksValue, true
	case "len":
		return lenValue, true
	case "take":
		return takeValue, true
	case "slice":
		return sliceValue, true
	case "range":
		return rangeValue, true
	case "sum":
		return sumValue, true
	case "product":
		return productValue, true
	case "min":
		return builtinMin, true
	case "max":
		return builtinMax, true
	case "clamp":
		return builtinClamp, true
	case "floor":
		return builtinFloor, true
	case "ceil":
		return builtinCeil, true
	case "round":
		return builtinRound, true
	case "sinh":
		return sinhValue, true
	case "cosh":
		return coshValue, true
	case "tanh":
		return tanhValue, true
	case "asinh":
		return asinhValue, true
	case "acosh":
		return acoshValue, true
	case "atanh":
		return atanhValue, true
	case "expm1":
		return expm1Value, true
	case "log1p":
		return log1pValue, true
	case "cbrt":
		return cbrtValue, true
	case "erf":
		return erfValue, true
	case "erfc":
		return erfcValue, true
	case "gamma":
		return gammaValue, true
	case "lgamma":
		return lgammaValue, true
	case "sign":
		return signValue, true
	case "step":
		return stepValue, true
	case "heaviside":
		return heavisideValue, true
	case "relu":
		return reluValue, true
	case "sigmoid":
		return sigmoidValue, true
	case "softplus":
		return softplusValue, true
	case "mean":
		return meanValue, true
	case "var":
		return varValue, true
	case "std":
		return stdValue, true
	case "median":
		return medianValue, true
	case "quantile":
		return quantileValue, true
	case "percentile":
		return percentileValue, true
	case "argmin":
		return argminValue, true
	case "argmax":
		return argmaxValue, true
	case "any":
		return anyValue, true
	case "all":
		return allValue, true
	case "cumsum":
		return cumsumValue, true
	case "cumprod":
		return cumprodValue, true
	case "dec", "decimal":
		return decimalValueFromArgs, true
	case "rat", "rational":
		return decimalValueFromArgs, true
	case "interval":
		return intervalValueFromArgs, true
	case "modint":
		return modIntValueFromArgs, true
	case "fixed":
		return fixedValueFromArgs, true
	case "complex":
		return complexValueFromArgs, true
	case "real":
		return builtinReal, true
	case "imag":
		return builtinImag, true
	case "conjugate":
		return conjugateValue, true
	case "magnitude", "abs":
		return builtinMagnitude, true
	case "pow":
		return builtinPow, true
	case "expt":
		return builtinExpt, true
	case "json":
		return jsonValueFromArgs, true
	case "format":
		return formatValueFromArgs, true
	case "uint":
		return uintValueFromArgs, true
	case "byte":
		return byteValueFromArgs, true
	case "uf8":
		return uf8ValueFromArgs, true
	case "uf16":
		return uf16ValueFromArgs, true
	case "uf32":
		return uf32ValueFromArgs, true
	case "ufnum":
		return ufnumValueFromArgs, true
	case "uf64":
		return uf64ValueFromArgs, true
	case "uf128":
		return uf128ValueFromArgs, true
	case "uf256":
		return uf256ValueFromArgs, true
	case "uf512":
		return uf512ValueFromArgs, true
	case "uf1024":
		return uf1024ValueFromArgs, true
	case "f32":
		return f32ValueFromArgs, true
	case "f64":
		return f64ValueFromArgs, true
	case "bf8":
		return bf8ValueFromArgs, true
	case "bf16":
		return bf16ValueFromArgs, true
	case "bf32":
		return bf32ValueFromArgs, true
	case "bf64":
		return bf64ValueFromArgs, true
	case "bf128":
		return bf128ValueFromArgs, true
	case "bf256":
		return bf256ValueFromArgs, true
	case "bf512":
		return bf512ValueFromArgs, true
	case "bf1024":
		return bf1024ValueFromArgs, true
	case "u8":
		return u8ValueFromArgs, true
	case "u16":
		return u16ValueFromArgs, true
	case "u32":
		return u32ValueFromArgs, true
	case "u64":
		return u64ValueFromArgs, true
	case "u128":
		return u128ValueFromArgs, true
	case "u256":
		return u256ValueFromArgs, true
	case "u512":
		return u512ValueFromArgs, true
	case "u1024":
		return u1024ValueFromArgs, true
	case "i8":
		return builtinI8, true
	case "i16":
		return builtinI16, true
	case "i32":
		return builtinI32, true
	case "i64":
		return builtinI64, true
	case "i128":
		return builtinI128, true
	case "i256":
		return builtinI256, true
	case "i512":
		return builtinI512, true
	case "i1024":
		return builtinI1024, true
	case "bigint":
		return bigIntValueFromArgs, true
	case "bigfloat":
		return bigFloatValueFromArgs, true
	case "tensor":
		return tensorValueFromArgs, true
	case "matrix":
		return matrixValueFromArgs, true
	case "shape":
		return tensorShapeValue, true
	case "flatten":
		return tensorFlattenValue, true
	case "squeeze":
		return tensorSqueezeValue, true
	case "expand_dims":
		return tensorExpandDimsValue, true
	case "concat":
		return tensorConcatValue, true
	case "stack":
		return tensorStackValue, true
	case "broadcast_to":
		return tensorBroadcastToValue, true
	case "where":
		return tensorWhereValue, true
	case "clip":
		return tensorClipValue, true
	case "gather":
		return tensorGatherValue, true
	case "scatter":
		return tensorScatterValue, true
	case "pad":
		return tensorPadValue, true
	case "tile":
		return tensorTileValue, true
	case "transpose":
		return tensorTransposeValue, true
	case "dot":
		return tensorDotValue, true
	case "matmul":
		return tensorMatMulValue, true
	case "outer":
		return outerValue, true
	case "trace":
		return traceValue, true
	case "det":
		return detValue, true
	case "solve":
		return solveValue, true
	case "inv":
		return invValue, true
	case "qr":
		return qrValue, true
	case "cholesky":
		return choleskyValue, true
	case "eig":
		return eigValue, true
	case "eigh":
		return eighValue, true
	case "svd":
		return svdValue, true
	case "reshape":
		return tensorReshapeValue, true
	case "view":
		return tensorViewValue, true
	case "clone":
		return tensorCloneValue, true
	case "sin":
		return builtinSin, true
	case "cos":
		return builtinCos, true
	case "tan":
		return builtinTan, true
	case "asin":
		return builtinAsin, true
	case "acos":
		return builtinAcos, true
	case "atan":
		return builtinAtan, true
	case "exp":
		return builtinExp, true
	case "log":
		return builtinLog, true
	case "log10":
		return builtinLog10, true
	case "log2":
		return builtinLog2, true
	case "hypot":
		return builtinHypot, true
	case "norm":
		return builtinNorm, true
	case "vector", "vec":
		return builtinVector, true
	case "bytes":
		return bytesValue, true
	case "pack":
		return packValue, true
	case "unpack":
		return unpackValue, true
	case "encode":
		return encodeValue, true
	case "decode":
		return decodeValue, true
	case "at":
		return atValue, true
	case "setat":
		return setAtValue, true
	case "lines":
		return linesValue, true
	case "csv":
		return csvValue, true
	case "jsonparse":
		return jsonParseValue, true
	case "noise":
		return builtinNoise, true
	default:
		return nil, false
	}
}

// isZeroValue handles the is zero value step in the surrounding package and
// returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func isZeroValue(v Value) bool {
	switch v.kind {
	case kindNumber:
		return v.num == 0
	case kindUint:
		if v.ub != nil {
			return v.ub.Sign() == 0
		}
		return v.u == 0
	case kindInt:
		return v.i == nil || v.i.Sign() == 0
	case kindBool:
		return !v.boo
	case kindDecimal:
		return v.dec == nil || v.dec.Sign() == 0
	case kindBigInt:
		return v.bi == nil || v.bi.Sign() == 0
	case kindBigFloat:
		return v.bf == nil || v.bf.Sign() == 0
	default:
		return false
	}
}

// isOneValue handles the is one value step in the surrounding package and
// returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func isOneValue(v Value) bool {
	switch v.kind {
	case kindNumber:
		return v.num == 1
	case kindUint:
		if v.ub != nil {
			return v.ub.Cmp(oneBigInt()) == 0
		}
		return v.u == 1
	case kindInt:
		return v.i != nil && v.i.Cmp(oneBigInt()) == 0
	case kindBool:
		return v.boo
	case kindDecimal:
		if v.dec == nil {
			return false
		}
		return v.dec.Cmp(oneRat()) == 0
	case kindBigInt:
		return v.bi != nil && v.bi.Cmp(oneBigInt()) == 0
	case kindBigFloat:
		return v.bf != nil && v.bf.Cmp(oneBigFloat()) == 0
	default:
		return false
	}
}

var (
	oneRatValue      = newRatOne()
	oneBigIntValue   = newBigIntOne()
	oneBigFloatValue = newBigFloatOne()
)

// newRatOne handles the new rat one step in the surrounding package and
// returns *big.Rat.
//
// It returns *big.Rat so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func newRatOne() *big.Rat {
	return new(big.Rat).SetInt64(1)
}

// oneRat handles the one rat step in the surrounding package and returns
// *big.Rat.
//
// It returns *big.Rat so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func oneRat() *big.Rat { return oneRatValue }

// newBigIntOne handles the new big int one step in the surrounding package and
// returns *big.Int.
//
// It returns *big.Int so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func newBigIntOne() *big.Int {
	return big.NewInt(1)
}

// oneBigInt handles the one big int step in the surrounding package and
// returns *big.Int.
//
// It returns *big.Int so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func oneBigInt() *big.Int { return oneBigIntValue }

// newBigFloatOne handles the new big float one step in the surrounding package
// and returns *big.Float.
//
// It returns *big.Float so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func newBigFloatOne() *big.Float {
	return new(big.Float).SetInt64(1)
}

// oneBigFloat handles the one big float step in the surrounding package and
// returns *big.Float.
//
// It returns *big.Float so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func oneBigFloat() *big.Float { return oneBigFloatValue }

// decimalValueFromArgs handles the decimal value from args step in the
// surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func decimalValueFromArgs(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("dec expects 1 argument")
	}
	switch args[0].kind {
	case kindDecimal:
		return decimalValue(args[0].dec), nil
	case kindString:
		return decimalValueFromString(args[0].str)
	default:
		rat, err := decimalFromValue(args[0])
		if err != nil {
			return nullValue(), err
		}
		return decimalValue(rat), nil
	}
}

// uintValueFromArgs handles the uint value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func uintValueFromArgs(args []Value) (Value, error) { return castUnsignedWidthValue(args, 64) }

// byteValueFromArgs handles the byte value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func byteValueFromArgs(args []Value) (Value, error) { return castUnsignedWidthValue(args, 8) }

// uf8ValueFromArgs handles the uf8 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func uf8ValueFromArgs(args []Value) (Value, error) {
	return castPackedUnsignedWidthValue(args, 8, false)
}

// uf16ValueFromArgs handles the uf16 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func uf16ValueFromArgs(args []Value) (Value, error) {
	return castPackedUnsignedWidthValue(args, 16, false)
}

// uf32ValueFromArgs handles the uf32 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func uf32ValueFromArgs(args []Value) (Value, error) {
	return castPackedUnsignedWidthValue(args, 32, false)
}

// ufnumValueFromArgs handles the ufnum value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func ufnumValueFromArgs(args []Value) (Value, error) {
	return castPackedUnsignedWidthValue(args, 64, true)
}

// uf64ValueFromArgs handles the uf64 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func uf64ValueFromArgs(args []Value) (Value, error) {
	return castPackedUnsignedWidthValue(args, 64, false)
}

// uf128ValueFromArgs handles the uf128 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func uf128ValueFromArgs(args []Value) (Value, error) {
	return castPackedUnsignedWidthValue(args, 128, false)
}

// uf256ValueFromArgs handles the uf256 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func uf256ValueFromArgs(args []Value) (Value, error) {
	return castPackedUnsignedWidthValue(args, 256, false)
}

// uf512ValueFromArgs handles the uf512 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func uf512ValueFromArgs(args []Value) (Value, error) {
	return castPackedUnsignedWidthValue(args, 512, false)
}

// uf1024ValueFromArgs handles the uf1024 value from args step in the
// surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func uf1024ValueFromArgs(args []Value) (Value, error) {
	return castPackedUnsignedWidthValue(args, 1024, false)
}

// f32ValueFromArgs handles the f32 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func f32ValueFromArgs(args []Value) (Value, error) { return castFloatWidthValue(args, 32) }

// f64ValueFromArgs handles the f64 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func f64ValueFromArgs(args []Value) (Value, error) { return castFloatWidthValue(args, 64) }

// bf8ValueFromArgs handles the bf8 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func bf8ValueFromArgs(args []Value) (Value, error) { return castBFloatWidthValue(args, 8) }

// bf16ValueFromArgs handles the bf16 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func bf16ValueFromArgs(args []Value) (Value, error) { return castBFloatWidthValue(args, 16) }

// bf32ValueFromArgs handles the bf32 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func bf32ValueFromArgs(args []Value) (Value, error) { return castBFloatWidthValue(args, 32) }

// bf64ValueFromArgs handles the bf64 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func bf64ValueFromArgs(args []Value) (Value, error) { return castBFloatWidthValue(args, 64) }

// bf128ValueFromArgs handles the bf128 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func bf128ValueFromArgs(args []Value) (Value, error) { return castBFloatWidthValue(args, 128) }

// bf256ValueFromArgs handles the bf256 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func bf256ValueFromArgs(args []Value) (Value, error) { return castBFloatWidthValue(args, 256) }

// bf512ValueFromArgs handles the bf512 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func bf512ValueFromArgs(args []Value) (Value, error) { return castBFloatWidthValue(args, 512) }

// bf1024ValueFromArgs handles the bf1024 value from args step in the
// surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func bf1024ValueFromArgs(args []Value) (Value, error) { return castBFloatWidthValue(args, 1024) }

// u8ValueFromArgs handles the u8 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func u8ValueFromArgs(args []Value) (Value, error) { return castUnsignedWidthValue(args, 8) }

// u16ValueFromArgs handles the u16 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func u16ValueFromArgs(args []Value) (Value, error) { return castUnsignedWidthValue(args, 16) }

// u32ValueFromArgs handles the u32 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func u32ValueFromArgs(args []Value) (Value, error) { return castUnsignedWidthValue(args, 32) }

// u64ValueFromArgs handles the u64 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func u64ValueFromArgs(args []Value) (Value, error) { return castUnsignedWidthValue(args, 64) }

// u128ValueFromArgs handles the u128 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func u128ValueFromArgs(args []Value) (Value, error) { return castUnsignedWidthValue(args, 128) }

// u256ValueFromArgs handles the u256 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func u256ValueFromArgs(args []Value) (Value, error) { return castUnsignedWidthValue(args, 256) }

// u512ValueFromArgs handles the u512 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func u512ValueFromArgs(args []Value) (Value, error) { return castUnsignedWidthValue(args, 512) }

// u1024ValueFromArgs handles the u1024 value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func u1024ValueFromArgs(args []Value) (Value, error) { return castUnsignedWidthValue(args, 1024) }

// constValueExpr handles the const value expr step in the surrounding package
// and returns Value, bool.
//
// It returns Value, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func constValueExpr(expr Expr) (Value, bool) {
	switch n := expr.(type) {
	case *literalExpr:
		return n.value, true
	case *listExpr:
		items := make([]Value, len(n.items))
		for i, item := range n.items {
			v, ok := constValueExpr(item)
			if !ok {
				return nullValue(), false
			}
			items[i] = v
		}
		return listValue(items), true
	default:
		return nullValue(), false
	}
}

// constValues handles the const values step in the surrounding package and
// returns []Value, bool.
//
// It returns []Value, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func constValues(exprs []Expr) ([]Value, bool) {
	values := make([]Value, len(exprs))
	for i, expr := range exprs {
		v, ok := constValueExpr(expr)
		if !ok {
			return nil, false
		}
		values[i] = v
	}
	return values, true
}

// jsonValueFromArgs handles the json value from args step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func jsonValueFromArgs(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("json expects 1 argument")
	}
	data, err := json.Marshal(args[0].toInterface())
	if err != nil {
		return nullValue(), err
	}
	return stringValue(string(data)), nil
}

// formatValueFromArgs handles the format value from args step in the
// surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func formatValueFromArgs(args []Value) (Value, error) {
	if len(args) == 0 {
		return nullValue(), fmt.Errorf("format expects at least 1 argument")
	}
	verb, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	rendered := make([]any, 0, len(args)-1)
	for _, arg := range args[1:] {
		rendered = append(rendered, arg.toInterface())
	}
	return stringValue(fmt.Sprintf(verb, rendered...)), nil
}
