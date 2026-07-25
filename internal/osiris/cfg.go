package osiris

import (
	"context"
	"fmt"
	"strings"
)

type CFGProgram struct {
	Program   *IRProgram
	Functions []CFGFunction
}

type CFGFunction struct {
	Name       string
	SourceName string
	Entry      string
	Blocks     []CFGBlock
}

type CFGBlock struct {
	Name   string
	Params []string
	Preds  []string
	Succs  []string
	Async  []string
}

// String operates on CFGProgram values and keeps the cfgprogram-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (cp *CFGProgram) String() string {
	return FormatCFGProgram(cp)
}

// BuildCFG operates on Runtime values and keeps the runtime-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns *CFGProgram and an error. Callers should only trust the value
// when the error is nil.
func (rt *Runtime) BuildCFG(ctx context.Context, prog *Program) (*CFGProgram, error) {
	ssaProg, err := rt.BuildSSA(ctx, prog)
	if err != nil {
		return nil, err
	}
	return BuildCFGProgram(ssaProg)
}

// BuildCFGProgram builds the cfgprogram representation from its input form,
// carrying structural information forward and returning an error if the source
// cannot be lowered safely.
//
// It returns *CFGProgram and an error. Callers should only trust the value
// when the error is nil.
func BuildCFGProgram(prog *SSAProgram) (*CFGProgram, error) {
	if prog == nil || prog.Program == nil {
		return nil, fmt.Errorf("cannot build CFG for nil program")
	}
	out := &CFGProgram{Program: prog.Program}
	for _, fn := range prog.Program.Functions {
		cfgFn, err := buildCFGFunction(fn)
		if err != nil {
			return nil, err
		}
		out.Functions = append(out.Functions, cfgFn)
	}
	return out, nil
}

// FormatCFGProgram formats the cfgprogram into a stable textual rendering. The
// implementation is intentionally deterministic so round-tripping and
// diff-based review stay predictable.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func FormatCFGProgram(prog *CFGProgram) string {
	if prog == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("cfg ")
	if prog.Program != nil && prog.Program.SourceName != "" {
		b.WriteString(prog.Program.SourceName)
	} else {
		b.WriteString("<unknown>")
	}
	b.WriteByte('\n')
	if prog.Program != nil {
		if prog.Program.PackageName != "" {
			b.WriteString("package ")
			b.WriteString(prog.Program.PackageName)
			if prog.Program.PackageVersion != "" {
				b.WriteByte(' ')
				b.WriteString(prog.Program.PackageVersion)
			}
			b.WriteByte('\n')
		}
		if len(prog.Program.Exports) > 0 {
			b.WriteString("exports ")
			b.WriteString(strings.Join(prog.Program.Exports, ", "))
			b.WriteByte('\n')
		}
	}
	if len(prog.Functions) > 0 {
		b.WriteByte('\n')
		for i, fn := range prog.Functions {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString("fn ")
			b.WriteString(fn.Name)
			if fn.SourceName != "" && fn.SourceName != fn.Name {
				b.WriteString(" (")
				b.WriteString(fn.SourceName)
				b.WriteByte(')')
			}
			fmt.Fprintf(&b, ": entry=%s blocks=%d\n", fn.Entry, len(fn.Blocks))
			for _, block := range fn.Blocks {
				b.WriteString("  block ")
				b.WriteString(block.Name)
				if len(block.Params) > 0 {
					b.WriteByte('(')
					b.WriteString(strings.Join(block.Params, ", "))
					b.WriteByte(')')
				}
				b.WriteString(" preds [")
				b.WriteString(strings.Join(block.Preds, ", "))
				b.WriteString("] succs [")
				b.WriteString(strings.Join(block.Succs, ", "))
				b.WriteString("]")
				if len(block.Async) > 0 {
					b.WriteString(" async [")
					b.WriteString(strings.Join(block.Async, ", "))
					b.WriteByte(']')
				}
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

// buildCFGFunction handles the build cfgfunction step in the surrounding
// package and returns CFGFunction, error.
//
// It returns CFGFunction and an error. Callers should only trust the value
// when the error is nil.
func buildCFGFunction(fn *IRFunction) (CFGFunction, error) {
	cfgFn := CFGFunction{Name: fn.Name, SourceName: fn.SourceName}
	if len(fn.Blocks) == 0 {
		return cfgFn, nil
	}
	blockMap := make(map[string]*IRBlock, len(fn.Blocks))
	cfgBlocks := make(map[string]*CFGBlock, len(fn.Blocks))
	order := make([]string, 0, len(fn.Blocks))
	for _, block := range fn.Blocks {
		if block == nil {
			return cfgFn, fmt.Errorf("cfg build: function %s has nil block", fn.Name)
		}
		if block.Name == "" {
			return cfgFn, fmt.Errorf("cfg build: function %s has unnamed block", fn.Name)
		}
		if _, exists := blockMap[block.Name]; exists {
			return cfgFn, fmt.Errorf("cfg build: function %s has duplicate block %q", fn.Name, block.Name)
		}
		blockMap[block.Name] = block
		order = append(order, block.Name)
		async := make([]string, 0)
		for _, instr := range block.Instrs {
			if instr.Op == "async" {
				async = append(async, instr.Text)
			}
		}
		cfgBlocks[block.Name] = &CFGBlock{Name: block.Name, Params: append([]string(nil), block.Params...), Async: async}
	}
	for _, block := range fn.Blocks {
		cfgBlock := cfgBlocks[block.Name]
		for _, succ := range cfgSuccessors(block.Term) {
			if succ == "" {
				continue
			}
			if _, ok := blockMap[succ]; !ok {
				return cfgFn, fmt.Errorf("cfg build: function %s block %s targets unknown block %q", fn.Name, block.Name, succ)
			}
			cfgBlock.Succs = append(cfgBlock.Succs, succ)
			cfgBlocks[succ].Preds = append(cfgBlocks[succ].Preds, block.Name)
		}
	}
	cfgFn.Entry = order[0]
	cfgFn.Blocks = make([]CFGBlock, 0, len(order))
	for _, name := range order {
		block := cfgBlocks[name]
		if block == nil {
			continue
		}
		cfgFn.Blocks = append(cfgFn.Blocks, *block)
	}
	return cfgFn, nil
}

// cfgSuccessors handles the cfg successors step in the surrounding package and
// returns []string.
//
// It returns []string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func cfgSuccessors(term IRTerminator) []string {
	switch t := term.(type) {
	case *irJump:
		return []string{t.Target}
	case *irBranch:
		return []string{t.TrueTarget, t.FalseTarget}
	default:
		return nil
	}
}
