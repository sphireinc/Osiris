package osiris

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type SSAProgram struct {
	Program   *IRProgram
	Functions []SSAFunctionReport
}

type SSAFunctionReport struct {
	Name          string
	SourceName    string
	Blocks        int
	Instrs        int
	Defs          int
	Uses          int
	Captures      []string
	CaptureValues []string
	FreeRefs      []string
}

func (sp *SSAProgram) String() string {
	return FormatSSAProgram(sp)
}

func (rt *Runtime) BuildSSA(ctx context.Context, prog *Program) (*SSAProgram, error) {
	ir, err := rt.BuildIR(ctx, prog)
	if err != nil {
		return nil, err
	}
	return VerifySSAProgram(ir)
}

func VerifySSAProgram(prog *IRProgram) (*SSAProgram, error) {
	if prog == nil {
		return nil, fmt.Errorf("cannot verify nil IR program")
	}
	out := &SSAProgram{Program: prog}
	for _, fn := range prog.Functions {
		if fn == nil {
			return nil, fmt.Errorf("ssa verification: nil function in program")
		}
		report, err := verifySSAFunction(fn)
		if err != nil {
			return nil, err
		}
		out.Functions = append(out.Functions, report)
	}
	return out, nil
}

func FormatSSAProgram(prog *SSAProgram) string {
	if prog == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("ssa ")
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
			fmt.Fprintf(&b, ": blocks=%d instrs=%d defs=%d uses=%d", fn.Blocks, fn.Instrs, fn.Defs, fn.Uses)
			if len(fn.Captures) > 0 {
				b.WriteString(" captures [")
				for j, name := range fn.Captures {
					if j > 0 {
						b.WriteString(", ")
					}
					b.WriteString(name)
					if j < len(fn.CaptureValues) {
						b.WriteString(" -> ")
						b.WriteString(fn.CaptureValues[j])
					}
				}
				b.WriteByte(']')
			}
			if len(fn.FreeRefs) > 0 {
				b.WriteString(" free [")
				b.WriteString(strings.Join(fn.FreeRefs, ", "))
				b.WriteByte(']')
			}
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	b.WriteString(FormatIRProgram(prog.Program))
	return b.String()
}

func verifySSAFunction(fn *IRFunction) (SSAFunctionReport, error) {
	report := SSAFunctionReport{
		Name:          fn.Name,
		SourceName:    fn.SourceName,
		Blocks:        len(fn.Blocks),
		Captures:      append([]string(nil), fn.Captures...),
		CaptureValues: append([]string(nil), fn.CaptureValues...),
	}
	if len(fn.Captures) != len(fn.CaptureValues) {
		return report, fmt.Errorf("ssa verification: function %s captures and capture values mismatch", fn.Name)
	}
	for _, name := range fn.Captures {
		if name == "" {
			return report, fmt.Errorf("ssa verification: function %s has empty capture name", fn.Name)
		}
	}
	available := make(map[string]struct{}, len(fn.Params)+len(fn.Captures)+len(fn.Blocks)*2)
	defs := make(map[string]struct{})
	blockMap := make(map[string]*IRBlock, len(fn.Blocks))
	for _, param := range fn.Params {
		name := fmt.Sprintf("%s.%s.0", fn.Name, param)
		if err := markSSAName(defs, name, "parameter", fn.Name); err != nil {
			return report, err
		}
		available[name] = struct{}{}
	}
	for i, name := range fn.CaptureValues {
		if name == "" {
			return report, fmt.Errorf("ssa verification: function %s has empty capture value", fn.Name)
		}
		if name == "undef" {
			return report, fmt.Errorf("ssa verification: function %s capture %q is undefined", fn.Name, fn.Captures[i])
		}
		available[name] = struct{}{}
	}
	for _, block := range fn.Blocks {
		if block == nil {
			return report, fmt.Errorf("ssa verification: function %s has nil block", fn.Name)
		}
		if block.Name == "" {
			return report, fmt.Errorf("ssa verification: function %s has unnamed block", fn.Name)
		}
		if _, exists := blockMap[block.Name]; exists {
			return report, fmt.Errorf("ssa verification: function %s has duplicate block %q", fn.Name, block.Name)
		}
		blockMap[block.Name] = block
	}
	for _, block := range fn.Blocks {
		for _, param := range block.Params {
			if param == "" {
				return report, fmt.Errorf("ssa verification: function %s block %s has empty param", fn.Name, block.Name)
			}
			if err := markSSAName(defs, param, "block param", block.Name); err != nil {
				return report, err
			}
			available[param] = struct{}{}
		}
		for _, instr := range block.Instrs {
			report.Instrs++
			if err := verifySSAInstr(fn.Name, block.Name, instr, available, defs, blockMap, &report); err != nil {
				return report, err
			}
			if instr.Dest != "" {
				available[instr.Dest] = struct{}{}
			}
		}
		if block.Term == nil {
			return report, fmt.Errorf("ssa verification: function %s block %s has no terminator", fn.Name, block.Name)
		}
		if err := verifySSATerminator(fn.Name, block.Name, block.Term, available, blockMap, &report); err != nil {
			return report, err
		}
	}
	report.Defs = len(defs)
	report.FreeRefs = nil
	sort.Strings(report.Captures)
	if len(report.CaptureValues) > 0 {
		// Keep capture names aligned with the sorted capture list.
		aligned := make([]string, len(report.Captures))
		for i, capName := range report.Captures {
			for j, name := range fn.Captures {
				if name == capName {
					if j < len(fn.CaptureValues) {
						aligned[i] = fn.CaptureValues[j]
					}
					break
				}
			}
		}
		report.CaptureValues = aligned
	}
	return report, nil
}

func verifySSAInstr(fnName, blockName string, instr IRInstr, available map[string]struct{}, defs map[string]struct{}, blockMap map[string]*IRBlock, report *SSAFunctionReport) error {
	for _, ref := range operandRefsForInstr(instr) {
		if ref == "" {
			return fmt.Errorf("ssa verification: function %s block %s has empty operand in %q", fnName, blockName, instr.Op)
		}
		if ref == "undef" {
			return fmt.Errorf("ssa verification: function %s block %s uses undefined operand in %q", fnName, blockName, instr.Op)
		}
		if _, ok := available[ref]; !ok {
			return fmt.Errorf("ssa verification: function %s block %s uses undefined name %q in %q", fnName, blockName, ref, instr.Op)
		}
		report.Uses++
	}
	if instr.Dest != "" {
		if err := markSSAName(defs, instr.Dest, "instruction", blockName); err != nil {
			return err
		}
	}
	return nil
}

func verifySSATerminator(fnName, blockName string, term IRTerminator, available map[string]struct{}, blockMap map[string]*IRBlock, report *SSAFunctionReport) error {
	switch t := term.(type) {
	case *irJump:
		target, ok := blockMap[t.Target]
		if !ok {
			return fmt.Errorf("ssa verification: function %s block %s jumps to unknown block %q", fnName, blockName, t.Target)
		}
		if len(t.Args) != len(target.Params) {
			return fmt.Errorf("ssa verification: function %s block %s jumps to %s with %d args but block expects %d", fnName, blockName, t.Target, len(t.Args), len(target.Params))
		}
		for _, arg := range t.Args {
			if arg == "" {
				return fmt.Errorf("ssa verification: function %s block %s has empty jump argument", fnName, blockName)
			}
			if _, ok := available[arg]; !ok {
				return fmt.Errorf("ssa verification: function %s block %s jumps with undefined name %q", fnName, blockName, arg)
			}
			report.Uses++
		}
	case *irBranch:
		if _, ok := blockMap[t.TrueTarget]; !ok {
			return fmt.Errorf("ssa verification: function %s block %s branches to unknown block %q", fnName, blockName, t.TrueTarget)
		}
		if _, ok := blockMap[t.FalseTarget]; !ok {
			return fmt.Errorf("ssa verification: function %s block %s branches to unknown block %q", fnName, blockName, t.FalseTarget)
		}
		if t.Cond == "" {
			return fmt.Errorf("ssa verification: function %s block %s has empty branch condition", fnName, blockName)
		}
		if _, ok := available[t.Cond]; !ok {
			return fmt.Errorf("ssa verification: function %s block %s branches on undefined name %q", fnName, blockName, t.Cond)
		}
		report.Uses++
		for _, arg := range append(append([]string(nil), t.TrueArgs...), t.FalseArgs...) {
			if arg == "" {
				return fmt.Errorf("ssa verification: function %s block %s has empty branch argument", fnName, blockName)
			}
			if _, ok := available[arg]; !ok {
				return fmt.Errorf("ssa verification: function %s block %s branches with undefined name %q", fnName, blockName, arg)
			}
			report.Uses++
		}
		if len(t.TrueArgs) != len(blockMap[t.TrueTarget].Params) {
			return fmt.Errorf("ssa verification: function %s block %s branches to %s with %d args but block expects %d", fnName, blockName, t.TrueTarget, len(t.TrueArgs), len(blockMap[t.TrueTarget].Params))
		}
		if len(t.FalseArgs) != len(blockMap[t.FalseTarget].Params) {
			return fmt.Errorf("ssa verification: function %s block %s branches to %s with %d args but block expects %d", fnName, blockName, t.FalseTarget, len(t.FalseArgs), len(blockMap[t.FalseTarget].Params))
		}
	case *irReturn:
		if t.HasValue {
			if t.Value == "" {
				return fmt.Errorf("ssa verification: function %s block %s returns empty value", fnName, blockName)
			}
			if _, ok := available[t.Value]; !ok {
				return fmt.Errorf("ssa verification: function %s block %s returns undefined name %q", fnName, blockName, t.Value)
			}
			report.Uses++
		}
	case *irUnreachable:
		return nil
	default:
		return fmt.Errorf("ssa verification: function %s block %s has unknown terminator %T", fnName, blockName, term)
	}
	return nil
}

func operandRefsForInstr(instr IRInstr) []string {
	switch instr.Op {
	case "copy", "unary", "binary", "emit", "phi", "list", "makefunc":
		return append([]string(nil), instr.Args...)
	case "member":
		if len(instr.Args) == 0 {
			return nil
		}
		return []string{instr.Args[0]}
	case "call":
		out := make([]string, 0, 1+len(instr.Args))
		if instr.Text != "" {
			out = append(out, instr.Text)
		}
		out = append(out, instr.Args...)
		return out
	default:
		return nil
	}
}

func markSSAName(defs map[string]struct{}, name, kind, site string) error {
	if name == "" {
		return fmt.Errorf("ssa verification: empty %s at %s", kind, site)
	}
	if _, exists := defs[name]; exists {
		return fmt.Errorf("ssa verification: duplicate definition of %q at %s", name, site)
	}
	defs[name] = struct{}{}
	return nil
}
