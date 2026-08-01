package osiris

import (
	"context"
	"fmt"
)

type TraceBinding struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type TraceFrame struct {
	Index     int            `json:"index"`
	Kind      string         `json:"kind"`
	Statement string         `json:"statement"`
	Range     SourceRange    `json:"range"`
	Stack     []string       `json:"stack,omitempty"`
	Outputs   []string       `json:"outputs,omitempty"`
	Scope     []TraceBinding `json:"scope,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// Trace operates on Runtime values and keeps the runtime-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns []TraceFrame and an error. Callers should only trust the value
// when the error is nil.
func (rt *Runtime) Trace(ctx context.Context, prog *Program) ([]TraceFrame, error) {
	return NewSession(rt, ctx, prog.SourceName).TraceProgram(prog, false, nil)
}

// TraceProgram operates on Session values and keeps the session-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns []TraceFrame and an error. Callers should only trust the value
// when the error is nil.
func (s *Session) TraceProgram(prog *Program, moduleMode bool, stack []string) ([]TraceFrame, error) {
	frames := make([]TraceFrame, 0, len(prog.Statements))
	for i, stmt := range prog.Statements {
		frame := TraceFrame{
			Index:     i,
			Kind:      statementKind(stmt),
			Statement: formatStmt(stmt),
			Range:     toSourceRange(stmtSpan(stmt)),
		}
		s.ev.beginTraceFrame()
		outputs, ok, err := s.execStmt(stmt, prog.SourceName, moduleMode, stack)
		frame.Stack = s.ev.endTraceFrame()
		if err != nil {
			frame.Error = err.Error()
			frames = append(frames, frame)
			return frames, err
		}
		if ok {
			frame.Outputs = valuesToStrings(outputs)
		}
		frame.Scope = s.snapshotScope()
		frames = append(frames, frame)
	}
	if moduleMode {
		if s.packageName == "" {
			return frames, fmt.Errorf("module %s is missing a package declaration", prog.SourceName)
		}
		if len(s.exportOrder) == 0 {
			return frames, fmt.Errorf("module %s must declare at least one export", prog.SourceName)
		}
	}
	return frames, nil
}

// statementKind handles the statement kind step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func statementKind(stmt Stmt) string {
	switch stmt.(type) {
	case packageStmt:
		return "package"
	case exportStmt:
		return "export"
	case letStmt:
		return "let"
	case structStmt:
		return "struct"
	case typeDeclStmt:
		return "type"
	case newtypeStmt:
		return "newtype"
	case emitStmt:
		return "emit"
	case importStmt:
		return "import"
	case macroStmt:
		return "macro"
	case funcStmt:
		return "func"
	case returnStmt:
		return "return"
	case whileStmt:
		return "while"
	case forStmt:
		return "for"
	case breakStmt:
		return "break"
	case continueStmt:
		return "continue"
	default:
		return fmt.Sprintf("%T", stmt)
	}
}

// valuesToStrings handles the values to strings step in the surrounding
// package and returns []string.
//
// It returns []string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func valuesToStrings(values []Value) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	for i := range values {
		out[i] = values[i].String()
	}
	return out
}
