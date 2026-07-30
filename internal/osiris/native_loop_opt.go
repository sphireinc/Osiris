package osiris

import "fmt"

// optimizeNativeLoops performs small, proof-gated IR transformations used only
// by the native backend. It deliberately does not run the evaluator: every
// transformation is justified from IR definitions and control-flow shape.
func optimizeNativeLoops(ir *IRProgram) {
	if ir == nil {
		return
	}
	for _, fn := range ir.Functions {
		nativeLoopInvariantCodeMotion(fn)
		nativeForListBoundsCheckElimination(fn)
		nativeUnrollSmallStaticForLists(fn)
	}
}

const (
	nativeUnrollMaxTrips      = 4
	nativeUnrollMaxBodyInstrs = 8
	nativeUnrollMaxGrowth     = 24
)

func nativeUnrollSmallStaticForLists(fn *IRFunction) {
	if fn == nil {
		return
	}
	blocks := map[string]*IRBlock{}
	for _, b := range fn.Blocks {
		blocks[b.Name] = b
	}
	for _, header := range fn.Blocks {
		shape, ok := canonicalLoopShape(fn, header)
		if !ok || shape.Kind != "for" {
			continue
		}
		body := blocks[shape.Body]
		exit := blocks[shape.Exit]
		if body == nil || exit == nil || len(body.Instrs) == 0 || len(body.Instrs) > nativeUnrollMaxBodyInstrs {
			continue
		}
		pre := nativeLoopPreheader(fn, header.Name, body.Name)
		if pre == nil {
			continue
		}
		carry := len(header.Params) - 3
		jump, ok := pre.Term.(*irJump)
		back, backOK := body.Term.(*irJump)
		branch, branchOK := header.Term.(*irBranch)
		if !ok || !backOK || !branchOK || back.Target != header.Name || len(jump.Args) != len(header.Params) || len(branch.TrueArgs) != len(body.Params) || len(back.Args) != len(header.Params) {
			continue
		}
		list, ok := nativePreheaderListInstr(pre, jump.Args[carry])
		trips := len(list.Args)
		if !ok || trips == 0 || trips > nativeUnrollMaxTrips || trips*len(body.Instrs) > nativeUnrollMaxGrowth {
			continue
		}
		if !nativeUnrollBodyIsClosed(body, header, branch) {
			continue
		}

		// incoming is the SSA edge payload for the next header entry.  Every
		// body clone receives that payload through its block parameters, then
		// the original backedge supplies the following payload.  This is an
		// edge rewrite, not a textual substitution of loop variable names.
		incoming := append([]string(nil), jump.Args...)
		out := make([]IRInstr, 0, trips*len(body.Instrs))
		for iter, item := range list.Args {
			m := make(map[string]string, len(header.Params)+len(body.Params)+len(body.Instrs))
			for i := range header.Params {
				m[header.Params[i]] = incoming[i]
				// The current IR builder retains the incoming lexical SSA names in
				// a for-body in addition to recording block parameters for CFG
				// emission. Rewrite both representations at this edge.
				m[jump.Args[i]] = incoming[i]
			}
			for i, param := range body.Params {
				m[param] = m[branch.TrueArgs[i]]
			}
			for _, in := range body.Instrs {
				c := in
				c.Args = append([]string(nil), in.Args...)
				c.Dest = nativeUnrollName(in.Dest, iter)
				for i, a := range c.Args {
					if v, yes := m[a]; yes {
						c.Args[i] = v
					}
				}
				if nativeUnrollIsIterationRead(in, header.Params[carry], header.Params[carry+2], jump.Args[carry], jump.Args[carry+2]) {
					c.Op = "copy"
					c.Args = []string{item}
				}
				m[in.Dest] = c.Dest
				out = append(out, c)
			}
			for i, value := range back.Args {
				if mapped, exists := m[value]; exists {
					incoming[i] = mapped
				} else {
					incoming[i] = value
				}
			}
		}
		pre.Instrs = append(pre.Instrs, out...)
		pre.Term = &irJump{Target: exit.Name, Args: append([]string(nil), incoming[:carry]...)}
	}
}

// nativeUnrollBodyIsClosed verifies the exact single-block CFG shape that the
// cloner understands. Values defined outside the loop remain valid at the
// preheader; all body-local values must be introduced by an instruction or a
// block parameter before the clone uses them.
func nativeUnrollBodyIsClosed(body, header *IRBlock, branch *irBranch) bool {
	if body == nil || header == nil || branch == nil || len(body.Params) != len(branch.TrueArgs) {
		return false
	}
	available := make(map[string]struct{}, len(body.Params)+len(header.Params)+len(body.Instrs))
	for _, name := range body.Params {
		available[name] = struct{}{}
	}
	for _, name := range header.Params {
		available[name] = struct{}{}
	}
	for _, in := range body.Instrs {
		for _, arg := range in.Args {
			if _, ok := available[arg]; !ok {
				// It is an outer SSA definition. It dominates the preheader too.
				continue
			}
		}
		if in.Dest == "" {
			return false
		}
		available[in.Dest] = struct{}{}
	}
	return true
}

func nativeUnrollIsIterationRead(in IRInstr, iter, index, incomingIter, incomingIndex string) bool {
	if len(in.Args) != 2 || !((in.Args[0] == iter && in.Args[1] == index) || (in.Args[0] == incomingIter && in.Args[1] == incomingIndex)) {
		return false
	}
	return in.Op == "listatunchecked" || (in.Op == "call" && in.Text == "at")
}

func nativeUnrollName(name string, iteration int) string {
	return fmt.Sprintf("%s.unroll%d", name, iteration)
}

func nativePreheaderListInstr(pre *IRBlock, value string) (IRInstr, bool) {
	defs := map[string]IRInstr{}
	for _, in := range pre.Instrs {
		defs[in.Dest] = in
	}
	for {
		in, ok := defs[value]
		if !ok {
			return IRInstr{}, false
		}
		if in.Op == "list" {
			return in, true
		}
		if in.Op != "copy" || len(in.Args) != 1 {
			return IRInstr{}, false
		}
		value = in.Args[0]
	}
}

// nativeForListBoundsCheckElimination replaces at(iter, index) only when a
// canonical for-loop supplies both operands and its initial iterator is a
// directly constructed list. The loop header is the range proof: it branches
// to the body only after index < len(iter), while lowering initializes index
// to zero and increments it by one. No dynamic container, tensor/string
// indexing, or user-written at call is admitted.
func nativeForListBoundsCheckElimination(fn *IRFunction) {
	if fn == nil {
		return
	}
	blocks := make(map[string]*IRBlock, len(fn.Blocks))
	for _, block := range fn.Blocks {
		blocks[block.Name] = block
	}
	for _, header := range fn.Blocks {
		shape, ok := canonicalLoopShape(fn, header)
		if !ok || shape.Kind != "for" {
			continue
		}
		body := blocks[shape.Body]
		if body == nil {
			continue
		}
		preheader := nativeLoopPreheader(fn, header.Name, body.Name)
		if preheader == nil {
			continue
		}
		carryCount := len(header.Params) - 3
		if carryCount < 0 || len(body.Params) < carryCount+3 {
			continue
		}
		jump, ok := preheader.Term.(*irJump)
		if !ok || len(jump.Args) < carryCount+3 || !nativePreheaderList(preheader, jump.Args[carryCount]) {
			continue
		}
		// The for-body expression environment intentionally references the
		// header's carried iterator/index values; body parameters only carry
		// them across the branch edge.
		iter, index := header.Params[carryCount], header.Params[carryCount+2]
		for i := range body.Instrs {
			in := &body.Instrs[i]
			if in.Op == "call" && in.Text == "at" && len(in.Args) == 2 && in.Args[0] == iter && in.Args[1] == index {
				in.Op = "listatunchecked"
			}
		}
	}
}

func nativePreheaderList(preheader *IRBlock, value string) bool {
	defs := make(map[string]IRInstr, len(preheader.Instrs))
	for _, in := range preheader.Instrs {
		defs[in.Dest] = in
	}
	for {
		in, ok := defs[value]
		if !ok {
			return false
		}
		if in.Op == "list" {
			return true
		}
		if in.Op != "copy" || len(in.Args) != 1 {
			return false
		}
		value = in.Args[0]
	}
}

// nativeLoopInvariantCodeMotion moves only numeric constant expressions out of
// a canonical while body. A candidate must be independent of every body
// parameter, must depend solely on previously proven invariant constants, and
// must be an operation that cannot introduce a language error for the admitted
// numeric values. This intentionally narrow first slice is preferable to
// moving effectful calls, allocation, division, indexing, or mutable values.
func nativeLoopInvariantCodeMotion(fn *IRFunction) {
	if fn == nil {
		return
	}
	blocks := make(map[string]*IRBlock, len(fn.Blocks))
	for _, block := range fn.Blocks {
		blocks[block.Name] = block
	}
	for _, header := range fn.Blocks {
		shape, ok := canonicalLoopShape(fn, header)
		if !ok || !shape.Canonical || shape.Kind != "while" {
			continue
		}
		body := blocks[shape.Body]
		if body == nil {
			continue
		}
		preheader := nativeLoopPreheader(fn, header.Name, body.Name)
		if preheader == nil {
			continue
		}
		nativeHoistLoopConstants(preheader, body)
	}
}

func nativeLoopPreheader(fn *IRFunction, header, body string) *IRBlock {
	var preheader *IRBlock
	for _, block := range fn.Blocks {
		jump, ok := block.Term.(*irJump)
		if !ok || jump.Target != header || block.Name == body {
			continue
		}
		if preheader != nil {
			return nil
		}
		preheader = block
	}
	return preheader
}

func nativeHoistLoopConstants(preheader, body *IRBlock) {
	if preheader == nil || body == nil {
		return
	}
	invariant := make(map[string]struct{}, len(preheader.Instrs)+len(body.Instrs))
	for _, in := range preheader.Instrs {
		if in.Dest != "" {
			invariant[in.Dest] = struct{}{}
		}
	}
	for _, param := range body.Params {
		delete(invariant, param)
	}
	hoisted := make([]IRInstr, 0, len(body.Instrs))
	kept := make([]IRInstr, 0, len(body.Instrs))
	for _, in := range body.Instrs {
		if nativeHoistableInvariant(in, invariant) {
			hoisted = append(hoisted, in)
			invariant[in.Dest] = struct{}{}
			continue
		}
		kept = append(kept, in)
	}
	if len(hoisted) == 0 {
		return
	}
	preheader.Instrs = append(preheader.Instrs, hoisted...)
	body.Instrs = kept
}

func nativeHoistableInvariant(in IRInstr, invariant map[string]struct{}) bool {
	if in.Dest == "" {
		return false
	}
	switch in.Op {
	case "const":
		return nativeSafeLoopConstant(in.Value)
	case "binary":
		if in.Text != "+" && in.Text != "-" && in.Text != "*" && in.Text != "<" && in.Text != "<=" && in.Text != ">" && in.Text != ">=" && in.Text != "==" && in.Text != "!=" {
			return false
		}
		for _, arg := range in.Args {
			if _, ok := invariant[arg]; !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func nativeSafeLoopConstant(value Value) bool {
	switch value.kind {
	case kindBool, kindInt, kindUint, kindNumber:
		return true
	default:
		return false
	}
}
