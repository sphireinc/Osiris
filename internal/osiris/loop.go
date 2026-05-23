package osiris

import (
	"context"
	"strings"
)

type LoopProgram struct {
	SourceName string
	Functions  []*LoopFunction
}

type LoopFunction struct {
	Name  string
	Loops []*LoopShape
}

type LoopShape struct {
	Kind      string
	Header    string
	Body      string
	Exit      string
	Carry     []string
	Cond      string
	Iter      string
	Len       string
	Index     string
	Canonical bool
}

func (p *LoopProgram) String() string { return FormatLoopProgram(p) }

func FormatLoopProgram(prog *LoopProgram) string {
	if prog == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("loops ")
	if prog.SourceName != "" {
		b.WriteString(prog.SourceName)
	} else {
		b.WriteString("<unknown>")
	}
	b.WriteByte('\n')
	for i, fn := range prog.Functions {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("fn ")
		b.WriteString(fn.Name)
		b.WriteByte('\n')
		for _, loop := range fn.Loops {
			b.WriteString("  ")
			b.WriteString(loop.Kind)
			b.WriteString(" loop header=")
			b.WriteString(loop.Header)
			b.WriteString(" body=")
			b.WriteString(loop.Body)
			b.WriteString(" exit=")
			b.WriteString(loop.Exit)
			if len(loop.Carry) > 0 {
				b.WriteString(" carry=[")
				b.WriteString(strings.Join(loop.Carry, ", "))
				b.WriteByte(']')
			}
			if loop.Cond != "" {
				b.WriteString(" cond=")
				b.WriteString(loop.Cond)
			}
			if loop.Iter != "" {
				b.WriteString(" iter=")
				b.WriteString(loop.Iter)
			}
			if loop.Len != "" {
				b.WriteString(" len=")
				b.WriteString(loop.Len)
			}
			if loop.Index != "" {
				b.WriteString(" index=")
				b.WriteString(loop.Index)
			}
			if loop.Canonical {
				b.WriteString(" canonical")
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (rt *Runtime) BuildLoopProgram(ctx context.Context, prog *Program) (*LoopProgram, error) {
	if rt == nil {
		rt = NewRuntime()
	}
	ir, err := rt.BuildIR(ctx, prog)
	if err != nil {
		return nil, err
	}
	return analyzeLoopProgram(ir), nil
}

func analyzeLoopProgram(ir *IRProgram) *LoopProgram {
	if ir == nil {
		return nil
	}
	out := &LoopProgram{SourceName: ir.SourceName, Functions: make([]*LoopFunction, 0, len(ir.Functions))}
	for _, fn := range ir.Functions {
		lfn := &LoopFunction{Name: fn.Name}
		for _, block := range fn.Blocks {
			shape, ok := canonicalLoopShape(block)
			if ok {
				lfn.Loops = append(lfn.Loops, shape)
			}
		}
		out.Functions = append(out.Functions, lfn)
	}
	return out
}

func canonicalLoopShape(block *IRBlock) (*LoopShape, bool) {
	if block == nil {
		return nil, false
	}
	branch, ok := block.Term.(*irBranch)
	if !ok {
		return nil, false
	}
	if isCanonicalWhileHeader(block.Name) && len(block.Params) >= 1 {
		if !strings.Contains(branch.TrueTarget, ".body") || !strings.Contains(branch.FalseTarget, ".exit") {
			return nil, false
		}
		return &LoopShape{
			Kind:      "while",
			Header:    block.Name,
			Body:      branch.TrueTarget,
			Exit:      branch.FalseTarget,
			Carry:     append([]string(nil), block.Params...),
			Cond:      branch.Cond,
			Canonical: true,
		}, true
	}
	if isCanonicalForHeader(block.Name) && len(block.Params) >= 4 {
		if !strings.Contains(branch.TrueTarget, ".body") || !strings.Contains(branch.FalseTarget, ".exit") {
			return nil, false
		}
		carryCount := len(block.Params) - 3
		return &LoopShape{
			Kind:      "for",
			Header:    block.Name,
			Body:      branch.TrueTarget,
			Exit:      branch.FalseTarget,
			Carry:     append([]string(nil), block.Params[:carryCount]...),
			Cond:      branch.Cond,
			Iter:      block.Params[carryCount],
			Len:       block.Params[carryCount+1],
			Index:     block.Params[carryCount+2],
			Canonical: true,
		}, true
	}
	return nil, false
}

func isCanonicalWhileHeader(name string) bool {
	return strings.Contains(name, ".while.header")
}

func isCanonicalForHeader(name string) bool {
	return strings.Contains(name, ".for.header")
}
