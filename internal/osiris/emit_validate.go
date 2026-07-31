package osiris

// validateConcurrentEmits rejects statically visible output effects before IR
// lowering. Runtime guards remain necessary for dynamically obtained
// callables, but direct syntax and named user functions receive source-spanned
// compile diagnostics.
func validateConcurrentEmits(prog *Program) error {
	if prog == nil {
		return nil
	}
	functions := make(map[string]Expr)
	for _, stmt := range prog.Statements {
		if fn, ok := stmt.(funcStmt); ok {
			functions[fn.Name] = fn.Body
		}
	}
	for _, stmt := range prog.Statements {
		if err := validateConcurrentEmitStmt(stmt, false, functions, make(map[string]bool), prog.SourceName); err != nil {
			return err
		}
	}
	return nil
}

func validateConcurrentEmitStmt(stmt Stmt, concurrent bool, functions map[string]Expr, active map[string]bool, source string) error {
	switch st := stmt.(type) {
	case emitStmt:
		if concurrent {
			return structuredErrorAt(concurrentEmitError(), source, st.Span)
		}
		return validateConcurrentEmitExpr(st.Expr, concurrent, functions, active, source)
	case letStmt:
		return validateConcurrentEmitExpr(st.Expr, concurrent, functions, active, source)
	case funcStmt:
		if concurrent {
			return nil
		}
		return validateConcurrentEmitExpr(st.Body, false, functions, active, source)
	case macroStmt:
		return validateConcurrentEmitExpr(st.Body, concurrent, functions, active, source)
	case returnStmt:
		if st.Expr != nil {
			return validateConcurrentEmitExpr(st.Expr, concurrent, functions, active, source)
		}
	case whileStmt:
		if err := validateConcurrentEmitExpr(st.Cond, concurrent, functions, active, source); err != nil {
			return err
		}
		return validateConcurrentEmitExpr(st.Body, concurrent, functions, active, source)
	case forStmt:
		if err := validateConcurrentEmitExpr(st.Iter, concurrent, functions, active, source); err != nil {
			return err
		}
		return validateConcurrentEmitExpr(st.Body, concurrent, functions, active, source)
	}
	return nil
}

func validateConcurrentEmitExpr(expr Expr, concurrent bool, functions map[string]Expr, active map[string]bool, source string) error {
	switch n := expr.(type) {
	case *callExpr:
		if name, ok := specialFormName(n.callee); ok {
			switch name {
			case "spawn", "parallel", "race", "select":
				for _, arg := range n.args {
					if err := validateConcurrentEmitExpr(arg, true, functions, active, source); err != nil {
						return err
					}
				}
				return nil
			case "timeout":
				for i, arg := range n.args {
					if err := validateConcurrentEmitExpr(arg, i == 1, functions, active, source); err != nil {
						return err
					}
				}
				return nil
			}
		}
		if concurrent {
			if id, ok := n.callee.(*identExpr); ok && !active[id.name] {
				if body, ok := functions[id.name]; ok {
					active[id.name] = true
					err := validateConcurrentEmitExpr(body, true, functions, active, source)
					delete(active, id.name)
					if err != nil {
						return structuredErrorAt(concurrentEmitError(), source, n.Span)
					}
				}
			}
		}
		if err := validateConcurrentEmitExpr(n.callee, concurrent, functions, active, source); err != nil {
			return err
		}
		for _, arg := range n.args {
			if err := validateConcurrentEmitExpr(arg, concurrent, functions, active, source); err != nil {
				return err
			}
		}
	case *blockExpr:
		local := make(map[string]Expr, len(functions)+len(n.stmts))
		for name, body := range functions {
			local[name] = body
		}
		for _, stmt := range n.stmts {
			if fn, ok := stmt.(funcStmt); ok {
				local[fn.Name] = fn.Body
			}
		}
		for _, stmt := range n.stmts {
			if err := validateConcurrentEmitStmt(stmt, concurrent, local, active, source); err != nil {
				return err
			}
		}
		if n.tail != nil {
			return validateConcurrentEmitExpr(n.tail, concurrent, local, active, source)
		}
	case *memberExpr:
		return validateConcurrentEmitExpr(n.object, concurrent, functions, active, source)
	case *unaryExpr:
		return validateConcurrentEmitExpr(n.expr, concurrent, functions, active, source)
	case *binaryExpr:
		if err := validateConcurrentEmitExpr(n.left, concurrent, functions, active, source); err != nil {
			return err
		}
		return validateConcurrentEmitExpr(n.right, concurrent, functions, active, source)
	case *listExpr:
		for _, item := range n.items {
			if err := validateConcurrentEmitExpr(item, concurrent, functions, active, source); err != nil {
				return err
			}
		}
	case *matchExpr:
		if err := validateConcurrentEmitExpr(n.subject, concurrent, functions, active, source); err != nil {
			return err
		}
		for _, arm := range n.arms {
			if arm.guard != nil {
				if err := validateConcurrentEmitExpr(arm.guard, concurrent, functions, active, source); err != nil {
					return err
				}
			}
			if err := validateConcurrentEmitExpr(arm.body, concurrent, functions, active, source); err != nil {
				return err
			}
		}
	case *tryExpr:
		if err := validateConcurrentEmitExpr(n.body, concurrent, functions, active, source); err != nil {
			return err
		}
		return validateConcurrentEmitExpr(n.handler, concurrent, functions, active, source)
	case *propagateExpr:
		return validateConcurrentEmitExpr(n.expr, concurrent, functions, active, source)
	}
	return nil
}
