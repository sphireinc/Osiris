package osiris

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type ClosureProgram struct {
	Program      *Program
	Environments []ClosureEnvironment
	Functions    []ClosureFunction
}

type ClosureEnvironment struct {
	Name     string
	Function string
	Fields   []ClosureField
}

type ClosureField struct {
	Name  string
	Value string
}

type ClosureFunction struct {
	Name       string
	SourceName string
	Params     []string
	Env        string
	Captures   []string
}

// String operates on ClosureProgram values and keeps the
// closureprogram-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (p *ClosureProgram) String() string {
	return FormatClosureProgram(p)
}

// AnalyzeClosure operates on Runtime values and keeps the runtime-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns *ClosureProgram and an error. Callers should only trust the value
// when the error is nil.
func (rt *Runtime) AnalyzeClosure(ctx context.Context, prog *Program) (*ClosureProgram, error) {
	if prog == nil {
		return nil, fmt.Errorf("cannot analyze closures for nil program")
	}
	ir, err := rt.BuildIR(ctx, prog)
	if err != nil {
		return nil, err
	}
	return analyzeClosureProgram(prog, ir), nil
}

// FormatClosureProgram formats the closure program into a stable textual
// rendering. The implementation is intentionally deterministic so
// round-tripping and diff-based review stay predictable.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func FormatClosureProgram(prog *ClosureProgram) string {
	if prog == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("closure ")
	if prog.Program != nil && prog.Program.SourceName != "" {
		b.WriteString(prog.Program.SourceName)
	} else {
		b.WriteString("<unknown>")
	}
	b.WriteByte('\n')
	if prog.Program != nil {
		if p, ok := firstPackage(prog.Program); ok {
			b.WriteString("package ")
			b.WriteString(p.Name)
			if p.Version != "" {
				b.WriteByte(' ')
				b.WriteString(p.Version)
			}
			b.WriteByte('\n')
		}
	}
	if len(prog.Environments) > 0 {
		b.WriteByte('\n')
		b.WriteString("environments\n")
		for _, env := range prog.Environments {
			b.WriteString("  env ")
			b.WriteString(env.Name)
			if env.Function != "" {
				b.WriteString(" @ ")
				b.WriteString(env.Function)
			}
			if len(env.Fields) > 0 {
				b.WriteString(" fields [")
				fields := make([]string, 0, len(env.Fields))
				for _, field := range env.Fields {
					fields = append(fields, field.Name+" -> "+field.Value)
				}
				b.WriteString(strings.Join(fields, ", "))
				b.WriteByte(']')
			}
			b.WriteByte('\n')
		}
	}
	if len(prog.Functions) > 0 {
		b.WriteByte('\n')
		b.WriteString("functions\n")
		for _, fn := range prog.Functions {
			b.WriteString("  fn ")
			b.WriteString(fn.Name)
			if fn.SourceName != "" && fn.SourceName != fn.Name {
				b.WriteString(" (")
				b.WriteString(fn.SourceName)
				b.WriteByte(')')
			}
			hasMeta := len(fn.Params) > 0 || fn.Env != "" || len(fn.Captures) > 0
			if hasMeta {
				b.WriteByte(':')
			}
			if len(fn.Params) > 0 {
				b.WriteString(" params [")
				b.WriteString(strings.Join(fn.Params, ", "))
				b.WriteByte(']')
			}
			if fn.Env != "" {
				b.WriteString(" env=")
				b.WriteString(fn.Env)
			}
			if len(fn.Captures) > 0 {
				b.WriteString(" captures [")
				b.WriteString(strings.Join(fn.Captures, ", "))
				b.WriteByte(']')
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// analyzeClosureProgram handles the analyze closure program step in the
// surrounding package and returns *ClosureProgram.
//
// It returns *ClosureProgram so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func analyzeClosureProgram(prog *Program, ir *IRProgram) *ClosureProgram {
	out := &ClosureProgram{Program: prog}
	if ir == nil {
		return out
	}
	for _, fn := range ir.Functions {
		caps := append([]string(nil), fn.Captures...)
		sort.Strings(caps)
		envName := ""
		if len(caps) > 0 {
			envName = fn.Name + ".env"
			fields := make([]ClosureField, 0, len(caps))
			for i, capture := range caps {
				value := ""
				if i < len(fn.CaptureValues) {
					value = fn.CaptureValues[i]
				}
				fields = append(fields, ClosureField{Name: capture, Value: value})
			}
			sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
			out.Environments = append(out.Environments, ClosureEnvironment{
				Name:     envName,
				Function: fn.Name,
				Fields:   fields,
			})
		}
		out.Functions = append(out.Functions, ClosureFunction{
			Name:       fn.Name,
			SourceName: fn.SourceName,
			Params:     append([]string(nil), fn.Params...),
			Env:        envName,
			Captures:   caps,
		})
	}
	sort.Slice(out.Environments, func(i, j int) bool {
		return out.Environments[i].Name < out.Environments[j].Name
	})
	sort.Slice(out.Functions, func(i, j int) bool {
		return out.Functions[i].Name < out.Functions[j].Name
	})
	return out
}
