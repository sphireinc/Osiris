package osiris

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type IRProgram struct {
	SourceName     string
	PackageName    string
	PackageVersion string
	Exports        []string
	Functions      []*IRFunction
}

type IRFunction struct {
	Name          string
	SourceName    string
	Params        []string
	ParamTypes    []string
	ResultType    string
	Captures      []string
	CaptureValues []string
	Blocks        []*IRBlock
}

type IRBlock struct {
	Name   string
	Params []string
	Instrs []IRInstr
	Term   IRTerminator
}

type IRInstr struct {
	Dest  string
	Op    string
	Args  []string
	Text  string
	Value Value
	Span  sourceSpan
}

type IRTerminator interface {
	irTerminator()
}

type irJump struct {
	Target string
	Args   []string
}

type irBranch struct {
	Cond        string
	TrueTarget  string
	TrueArgs    []string
	FalseTarget string
	FalseArgs   []string
}

type irReturn struct {
	Value    string
	HasValue bool
}

type irUnreachable struct{}

// irTerminator operates on irJump values and keeps the irjump-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (*irJump) irTerminator() {}

// irTerminator operates on irBranch values and keeps the irbranch-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (*irBranch) irTerminator() {}

// irTerminator operates on irReturn values and keeps the irreturn-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (*irReturn) irTerminator() {}

// irTerminator operates on irUnreachable values and keeps the
// irunreachable-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (*irUnreachable) irTerminator() {}

// String operates on IRProgram values and keeps the irprogram-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (p *IRProgram) String() string {
	return FormatIRProgram(p)
}

// FormatIRProgram formats the irprogram into a stable textual rendering. The
// implementation is intentionally deterministic so round-tripping and
// diff-based review stay predictable.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func FormatIRProgram(prog *IRProgram) string {
	var b strings.Builder
	if prog == nil {
		return ""
	}
	b.WriteString("program ")
	if prog.SourceName != "" {
		b.WriteString(prog.SourceName)
	} else {
		b.WriteString("<unknown>")
	}
	b.WriteByte('\n')
	if prog.PackageName != "" {
		b.WriteString("package ")
		b.WriteString(prog.PackageName)
		if prog.PackageVersion != "" {
			b.WriteByte(' ')
			b.WriteString(prog.PackageVersion)
		}
		b.WriteByte('\n')
	}
	if len(prog.Exports) > 0 {
		b.WriteString("exports ")
		b.WriteString(strings.Join(prog.Exports, ", "))
		b.WriteByte('\n')
	}
	if len(prog.Functions) > 0 {
		b.WriteByte('\n')
	}
	for i, fn := range prog.Functions {
		if i > 0 {
			b.WriteByte('\n')
		}
		formatIRFunction(&b, fn)
	}
	if len(prog.Functions) > 0 {
		b.WriteByte('\n')
	}
	return b.String()
}

// formatIRFunction handles the format irfunction step in the surrounding
// package and returns no result.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func formatIRFunction(b *strings.Builder, fn *IRFunction) {
	if fn == nil {
		b.WriteString("fn <nil>\n")
		return
	}
	b.WriteString("fn ")
	b.WriteString(fn.Name)
	b.WriteByte('(')
	for i, param := range fn.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(param)
		if i < len(fn.ParamTypes) && fn.ParamTypes[i] != "" {
			b.WriteByte(' ')
			b.WriteString(fn.ParamTypes[i])
		}
	}
	b.WriteByte(')')
	if fn.ResultType != "" {
		b.WriteByte(' ')
		b.WriteString(fn.ResultType)
	}
	if len(fn.Captures) > 0 {
		b.WriteString(" captures [")
		b.WriteString(strings.Join(fn.Captures, ", "))
		b.WriteByte(']')
	}
	b.WriteString(" {\n")
	for _, block := range fn.Blocks {
		formatIRBlock(b, block)
	}
	b.WriteString("}\n")
}

// formatIRBlock handles the format irblock step in the surrounding package and
// returns no result.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func formatIRBlock(b *strings.Builder, block *IRBlock) {
	b.WriteString("  block ")
	b.WriteString(block.Name)
	if len(block.Params) > 0 {
		b.WriteByte('(')
		b.WriteString(strings.Join(block.Params, ", "))
		b.WriteByte(')')
	}
	b.WriteString(":\n")
	for _, instr := range block.Instrs {
		b.WriteString("    ")
		b.WriteString(formatIRInstr(instr))
		b.WriteByte('\n')
	}
	if block.Term != nil {
		b.WriteString("    ")
		b.WriteString(formatIRTerminator(block.Term))
		b.WriteByte('\n')
	}
}

// formatIRInstr handles the format irinstr step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatIRInstr(instr IRInstr) string {
	op := instr.Op
	if op == "" {
		op = "<?>"
	}
	switch op {
	case "emit":
		if len(instr.Args) == 0 {
			return "emit"
		}
		return "emit " + strings.Join(instr.Args, ", ")
	case "makefunc":
		if len(instr.Args) == 0 {
			return instr.Dest + " = makefunc @" + instr.Text
		}
		return instr.Dest + " = makefunc @" + instr.Text + " captures [" + strings.Join(instr.Args, ", ") + "]"
	case "const":
		return instr.Dest + " = const " + instr.Value.String()
	case "builtin", "global", "import", "undef", "self":
		if instr.Text == "" {
			return instr.Dest + " = " + op
		}
		return instr.Dest + " = " + op + " " + instr.Text
	case "copy":
		return instr.Dest + " = copy " + strings.Join(instr.Args, ", ")
	case "asserttype":
		return instr.Dest + " = asserttype " + instr.Text + " " + strings.Join(instr.Args, ", ")
	case "unary":
		return instr.Dest + " = " + instr.Text + " " + strings.Join(instr.Args, ", ")
	case "binary":
		return instr.Dest + " = " + strings.Join(instr.Args[:1], ", ") + " " + instr.Text + " " + strings.Join(instr.Args[1:], ", ")
	case "call":
		if len(instr.Args) == 0 {
			return instr.Dest + " = call " + instr.Text
		}
		return instr.Dest + " = call " + instr.Text + "(" + strings.Join(instr.Args, ", ") + ")"
	case "async":
		return instr.Dest + " = async." + instr.Text + "(" + strings.Join(instr.Args, ", ") + ")"
	case "member":
		return instr.Dest + " = member " + strings.Join(instr.Args, ", ") + "." + instr.Text
	case "list":
		return instr.Dest + " = list [" + strings.Join(instr.Args, ", ") + "]"
	case "phi":
		return instr.Dest + " = phi [" + strings.Join(instr.Args, ", ") + "]"
	default:
		if instr.Dest != "" {
			if len(instr.Args) > 0 {
				return instr.Dest + " = " + op + " " + strings.Join(instr.Args, ", ")
			}
			if instr.Text != "" {
				return instr.Dest + " = " + op + " " + instr.Text
			}
			return instr.Dest + " = " + op
		}
		if len(instr.Args) > 0 {
			return op + " " + strings.Join(instr.Args, ", ")
		}
		if instr.Text != "" {
			return op + " " + instr.Text
		}
		return op
	}
}

// formatIRTerminator handles the format irterminator step in the surrounding
// package and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatIRTerminator(term IRTerminator) string {
	switch t := term.(type) {
	case *irJump:
		if len(t.Args) == 0 {
			return "jump " + t.Target
		}
		return "jump " + t.Target + "(" + strings.Join(t.Args, ", ") + ")"
	case *irBranch:
		left := "branch " + t.Cond + " ? " + t.TrueTarget
		if len(t.TrueArgs) > 0 {
			left += "(" + strings.Join(t.TrueArgs, ", ") + ")"
		}
		left += " : " + t.FalseTarget
		if len(t.FalseArgs) > 0 {
			left += "(" + strings.Join(t.FalseArgs, ", ") + ")"
		}
		return left
	case *irReturn:
		if !t.HasValue || t.Value == "" {
			return "return"
		}
		return "return " + t.Value
	case *irUnreachable:
		return "unreachable"
	default:
		return "<term>"
	}
}

// BuildIR operates on Runtime values and keeps the runtime-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns *IRProgram and an error. Callers should only trust the value when
// the error is nil.
func (rt *Runtime) BuildIR(ctx context.Context, prog *Program) (*IRProgram, error) {
	if rt == nil {
		rt = NewRuntime()
	}
	if prog == nil {
		return nil, fmt.Errorf("cannot build IR for nil program")
	}
	sess := NewSession(rt, ctx, prog.SourceName)
	expanded, err := sess.ExpandProgram(prog)
	if err != nil {
		return nil, err
	}
	if err := validateConcurrentEmits(expanded); err != nil {
		return nil, err
	}
	if _, err := rt.CheckProgram(ctx, expanded); err != nil {
		return nil, err
	}

	pb := &irProgramBuilder{
		rt:       rt,
		prog:     &IRProgram{SourceName: expanded.SourceName},
		nextFunc: 0,
	}
	if p, ok := firstPackage(expanded); ok {
		pb.prog.PackageName = p.Name
		pb.prog.PackageVersion = p.Version
	}
	for _, stmt := range expanded.Statements {
		if e, ok := stmt.(exportStmt); ok {
			pb.prog.Exports = append(pb.prog.Exports, e.Names...)
		}
	}

	mainFn := pb.newFunction("main", "main", nil)
	mainBuilder := newIRBuilder(pb, mainFn, "main")
	if err := mainBuilder.lowerProgram(expanded); err != nil {
		return nil, err
	}
	if !mainBuilder.terminated() {
		mainBuilder.emitTerm(&irReturn{})
	}
	pb.prog.Functions = append([]*IRFunction{mainFn}, pb.prog.Functions...)
	return pb.prog, nil
}

// firstPackage handles the first package step in the surrounding package and
// returns packageStmt, bool.
//
// It returns packageStmt, bool so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func firstPackage(prog *Program) (packageStmt, bool) {
	if prog == nil {
		return packageStmt{}, false
	}
	for _, stmt := range prog.Statements {
		if p, ok := stmt.(packageStmt); ok {
			return p, true
		}
	}
	return packageStmt{}, false
}

type irProgramBuilder struct {
	rt       *Runtime
	prog     *IRProgram
	nextFunc int
	enums    map[string]map[string]enumCase
}

// newFunction operates on irProgramBuilder values and keeps the
// irprogrambuilder-specific behavior attached to the receiver so callers can
// use the method as the natural entry point for that data structure.
//
// It returns *IRFunction so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (pb *irProgramBuilder) newFunction(sourceName, label string, params []string) *IRFunction {
	fn := &IRFunction{
		Name:       label,
		SourceName: sourceName,
		Params:     append([]string(nil), params...),
	}
	return fn
}

// nextFuncLabel operates on irProgramBuilder values and keeps the
// irprogrambuilder-specific behavior attached to the receiver so callers can
// use the method as the natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (pb *irProgramBuilder) nextFuncLabel(source string) string {
	label := fmt.Sprintf("%s.fn%d", source, pb.nextFunc)
	pb.nextFunc++
	return label
}

// appendFunction operates on irProgramBuilder values and keeps the
// irprogrambuilder-specific behavior attached to the receiver so callers can
// use the method as the natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (pb *irProgramBuilder) appendFunction(fn *IRFunction) {
	pb.prog.Functions = append(pb.prog.Functions, fn)
}

type irFnState struct {
	blocks  []*IRBlock
	tempSeq int
	bindSeq map[string]int
}

type irBuilder struct {
	pb        *irProgramBuilder
	fn        *IRFunction
	state     *irFnState
	block     *IRBlock
	prefix    string
	env       map[string]string
	locals    map[string]struct{}
	loopStack []irLoopCtx
}

type irLoopCtx struct {
	kind      string
	header    *IRBlock
	exit      *IRBlock
	carry     []string
	loopVar   string
	loopIter  string
	loopLen   string
	loopIndex string
}

// newIRBuilder handles the new irbuilder step in the surrounding package and
// returns *irBuilder.
//
// It returns *irBuilder so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func newIRBuilder(pb *irProgramBuilder, fn *IRFunction, prefix string) *irBuilder {
	st := &irFnState{
		blocks:  make([]*IRBlock, 0, 8),
		bindSeq: make(map[string]int),
	}
	entry := &IRBlock{Name: prefix + ".entry"}
	st.blocks = append(st.blocks, entry)
	return &irBuilder{
		pb:     pb,
		fn:     fn,
		state:  st,
		block:  entry,
		prefix: prefix,
		env:    make(map[string]string),
		locals: make(map[string]struct{}),
	}
}

// fork operates on irBuilder values and keeps the irbuilder-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns *irBuilder so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (b *irBuilder) fork() *irBuilder {
	return &irBuilder{
		pb:        b.pb,
		fn:        b.fn,
		state:     b.state,
		block:     b.block,
		prefix:    b.prefix,
		env:       cloneStringMap(b.env),
		locals:    cloneNameSet(b.locals),
		loopStack: append([]irLoopCtx(nil), b.loopStack...),
	}
}

// cloneNameSet handles the clone name set step in the surrounding package and
// returns map[string]value.
//
// It returns map[string]value so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func cloneNameSet(src map[string]struct{}) map[string]struct{} {
	if len(src) == 0 {
		return make(map[string]struct{})
	}
	dst := make(map[string]struct{}, len(src))
	for k := range src {
		dst[k] = struct{}{}
	}
	return dst
}

// terminated operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func (b *irBuilder) terminated() bool {
	return b.block != nil && b.block.Term != nil
}

// newTemp operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (b *irBuilder) newTemp() string {
	name := fmt.Sprintf("%s.t%d", b.prefix, b.state.tempSeq)
	b.state.tempSeq++
	return name
}

// newBindName operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (b *irBuilder) newBindName(source string) string {
	idx := b.state.bindSeq[source]
	b.state.bindSeq[source] = idx + 1
	return fmt.Sprintf("%s.%s.%d", b.prefix, source, idx)
}

// newBlock operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns *IRBlock so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (b *irBuilder) newBlock(prefix string) *IRBlock {
	name := fmt.Sprintf("%s.%s%d", b.prefix, prefix, len(b.state.blocks))
	block := &IRBlock{Name: name}
	b.state.blocks = append(b.state.blocks, block)
	return block
}

// emitInstr operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (b *irBuilder) emitInstr(instr IRInstr) {
	if b.block == nil {
		return
	}
	b.block.Instrs = append(b.block.Instrs, instr)
}

// emitTerm operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (b *irBuilder) emitTerm(term IRTerminator) {
	if b.block == nil {
		return
	}
	b.block.Term = term
}

// valueOrUndef operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (b *irBuilder) valueOrUndef(name string) string {
	if v, ok := b.env[name]; ok {
		return v
	}
	tmp := b.newTemp()
	b.emitInstr(IRInstr{Dest: tmp, Op: "undef"})
	return tmp
}

// resolveIdent operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string and an error. Callers should only trust the value when the
// error is nil.
func (b *irBuilder) resolveIdent(name string, span sourceSpan) (string, error) {
	if v, ok := b.env[name]; ok {
		if _, local := b.locals[name]; !local {
			// The name came from an outer scope and is therefore a capture.
			b.recordCapture(name)
		}
		return v, nil
	}
	if v, ok := b.pb.rt.Constants[name]; ok {
		tmp := b.newTemp()
		b.emitInstr(IRInstr{Dest: tmp, Op: "global", Text: name, Value: v, Span: span})
		return tmp, nil
	}
	if _, ok := b.pb.rt.Builtins[name]; ok {
		tmp := b.newTemp()
		b.emitInstr(IRInstr{Dest: tmp, Op: "builtin", Text: name, Span: span})
		return tmp, nil
	}
	if _, ok := b.pb.rt.ContextBuiltins[name]; ok {
		tmp := b.newTemp()
		b.emitInstr(IRInstr{Dest: tmp, Op: "builtin", Text: name, Span: span})
		return tmp, nil
	}
	return "", fmt.Errorf("undefined name %q", name)
}

// recordCapture operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (b *irBuilder) recordCapture(name string) {
	// Capture bookkeeping lives on the function value, so only function builders
	// need to care. The top-level builder can ignore it.
	if b.fn == nil || b.fn.Name == "" || b.fn.Name == "main" {
		return
	}
	if b.fn.Captures == nil {
		b.fn.Captures = make([]string, 0, 4)
	}
	if _, seen := captureSeen(b.fn.Captures, name); seen {
		return
	}
	b.fn.Captures = append(b.fn.Captures, name)
}

// captureSeen handles the capture seen step in the surrounding package and
// returns int, bool.
//
// It returns int, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func captureSeen(list []string, name string) (int, bool) {
	for i, item := range list {
		if item == name {
			return i, true
		}
	}
	return -1, false
}

// lowerProgram operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns an error when the operation cannot complete cleanly.
func (b *irBuilder) lowerProgram(prog *Program) error {
	for _, stmt := range prog.Statements {
		terminated, err := b.lowerStmt(stmt)
		if err != nil {
			return err
		}
		if terminated {
			break
		}
	}
	b.fn.Blocks = append(b.fn.Blocks, b.state.blocks...)
	return nil
}

// lowerStmt operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns bool and an error. Callers should only trust the value when the
// error is nil.
func (b *irBuilder) lowerStmt(stmt Stmt) (bool, error) {
	switch st := stmt.(type) {
	case packageStmt:
		b.pb.prog.PackageName = st.Name
		b.pb.prog.PackageVersion = st.Version
		return false, nil
	case exportStmt:
		b.pb.prog.Exports = append(b.pb.prog.Exports, st.Names...)
		return false, nil
	case enumStmt:
		if b.pb.enums == nil {
			b.pb.enums = make(map[string]map[string]enumCase)
		}
		cases := make(map[string]enumCase, len(st.Cases))
		for _, item := range st.Cases {
			cases[item.Name] = item
		}
		b.pb.enums[st.Name] = cases
		return false, nil
	case structStmt, typeDeclStmt, newtypeStmt:
		return false, fmt.Errorf("native C backend: product-type declarations are not yet supported")
	case letStmt:
		if len(st.Names) > 0 {
			return false, fmt.Errorf("native C backend: tuple destructuring is not yet supported")
		}
		child := b.fork()
		value, err := child.evalExpr(st.Expr)
		if err != nil {
			return false, err
		}
		b.block = child.block
		if b.terminated() {
			return true, nil
		}
		if st.Unwrap {
			unwrapped := b.newTemp()
			b.emitInstr(IRInstr{Dest: unwrapped, Op: "resultbind", Args: []string{value}, Span: st.Span})
			value = unwrapped
		}
		if st.Type != "" {
			checked := b.newTemp()
			b.emitInstr(IRInstr{Dest: checked, Op: "asserttype", Text: st.Type, Args: []string{value}})
			value = checked
		}
		dest := b.newBindName(st.Name)
		b.emitInstr(IRInstr{Dest: dest, Op: "copy", Args: []string{value}})
		b.env[st.Name] = dest
		b.locals[st.Name] = struct{}{}
		return false, nil
	case importStmt:
		dest := b.newBindName(st.Alias)
		b.emitInstr(IRInstr{Dest: dest, Op: "import", Text: quoteString(st.Path), Span: st.PathSpan})
		b.env[st.Alias] = dest
		b.locals[st.Alias] = struct{}{}
		return false, nil
	case emitStmt:
		child := b.fork()
		value, err := child.evalExpr(st.Expr)
		if err != nil {
			return false, err
		}
		b.block = child.block
		if b.terminated() {
			return true, nil
		}
		b.emitInstr(IRInstr{Op: "emit", Args: []string{value}})
		return false, nil
	case funcStmt:
		return b.lowerFuncStmt(st)
	case whileStmt:
		return b.lowerWhileStmt(st)
	case forStmt:
		return b.lowerForStmt(st)
	case returnStmt:
		if st.Expr == nil {
			b.emitTerm(&irReturn{})
			return true, nil
		}
		child := b.fork()
		value, err := child.evalExpr(st.Expr)
		if err != nil {
			return false, err
		}
		b.block = child.block
		if b.terminated() {
			return true, nil
		}
		if b.fn.ResultType != "" {
			checked := b.newTemp()
			b.emitInstr(IRInstr{Dest: checked, Op: "asserttype", Text: b.fn.ResultType, Args: []string{value}, Span: st.Span})
			value = checked
		}
		b.emitTerm(&irReturn{Value: value, HasValue: true})
		return true, nil
	case breakStmt:
		if len(b.loopStack) == 0 {
			return false, fmt.Errorf("break used outside of a loop")
		}
		ctx := b.loopStack[len(b.loopStack)-1]
		b.emitTerm(&irJump{Target: ctx.exit.Name, Args: b.argsForNames(ctx.carry)})
		return true, nil
	case continueStmt:
		if len(b.loopStack) == 0 {
			return false, fmt.Errorf("continue used outside of a loop")
		}
		ctx := b.loopStack[len(b.loopStack)-1]
		args := b.argsForNames(ctx.carry)
		if ctx.kind == "for" {
			args = append(args, ctx.loopIter, ctx.loopLen, b.nextLoopIndex(ctx.loopIndex))
		}
		b.emitTerm(&irJump{Target: ctx.header.Name, Args: args})
		return true, nil
	default:
		return false, fmt.Errorf("internal error: unknown statement type %T", stmt)
	}
}

// nextLoopIndex operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (b *irBuilder) nextLoopIndex(name string) string {
	// For the current IR pass, loop indices are represented as a simple
	// incremented temp so the control-flow graph is explicit and stable.
	one := b.newTemp()
	tmp := b.newTemp()
	b.emitInstr(IRInstr{Dest: one, Op: "const", Value: numberValue(1)})
	b.emitInstr(IRInstr{Dest: tmp, Op: "binary", Text: "+", Args: []string{name, one}})
	return tmp
}

// lowerFuncStmt operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns bool and an error. Callers should only trust the value when the
// error is nil.
func (b *irBuilder) lowerFuncStmt(st funcStmt) (bool, error) {
	label := b.pb.nextFuncLabel(st.Name)
	fn := &IRFunction{
		Name:       label,
		SourceName: st.Name,
		Params:     append([]string(nil), st.Params...),
		ParamTypes: append([]string(nil), st.ParamTypes...),
		ResultType: st.ReturnType,
		Captures:   nil,
	}

	// Bind the function value in the enclosing scope before lowering the body so
	// recursive calls resolve to the same SSA name.
	dest := b.newBindName(st.Name)
	outerEnv := cloneStringMap(b.env)
	outerEnv[st.Name] = dest

	child := newIRBuilder(b.pb, fn, label)
	child.env = cloneStringMap(outerEnv)
	child.locals = make(map[string]struct{}, len(st.Params)+1)
	child.env[st.Name] = dest
	child.locals[st.Name] = struct{}{}
	for _, param := range st.Params {
		paramName := fmt.Sprintf("%s.%s.0", label, param)
		child.env[param] = paramName
		child.locals[param] = struct{}{}
	}
	for i, param := range st.Params {
		if i >= len(st.ParamTypes) || st.ParamTypes[i] == "" {
			continue
		}
		checked := child.newTemp()
		span := st.Span
		if i < len(st.ParamSpans) {
			span = st.ParamSpans[i]
		}
		child.emitInstr(IRInstr{Dest: checked, Op: "asserttype", Text: st.ParamTypes[i], Args: []string{child.env[param]}, Span: span})
		child.env[param] = checked
	}

	result, err := child.evalExpr(st.Body)
	if err != nil {
		return false, err
	}
	if !child.terminated() {
		if st.ReturnType != "" {
			checked := child.newTemp()
			child.emitInstr(IRInstr{Dest: checked, Op: "asserttype", Text: st.ReturnType, Args: []string{result}, Span: st.ReturnTypeSpan})
			result = checked
		}
		child.emitTerm(&irReturn{Value: result, HasValue: true})
	}
	fn.Blocks = append(fn.Blocks, child.state.blocks...)
	if len(child.fn.Captures) > 0 {
		caps := append([]string(nil), child.fn.Captures...)
		sort.Strings(caps)
		fn.Captures = caps
		fn.CaptureValues = captureValues(outerEnv, caps)
	}
	b.pb.appendFunction(fn)
	b.emitInstr(IRInstr{Dest: dest, Op: "makefunc", Text: fn.Name, Args: captureValues(b.env, fn.Captures)})
	b.env[st.Name] = dest
	b.locals[st.Name] = struct{}{}
	return false, nil
}

// lowerDeferredExpr lowers an expression into a zero-argument thunk, or into
// a handler closure when params are supplied. Keeping the expression in its
// own function is what makes spawn, parallel, and try boundaries observable
// in IR instead of evaluating their bodies while constructing the boundary.
func (b *irBuilder) lowerDeferredExpr(expr Expr, source string, params []string) (string, error) {
	label := b.pb.nextFuncLabel(source)
	fn := b.pb.newFunction(source, label, params)
	child := newIRBuilder(b.pb, fn, label)
	child.env = cloneStringMap(b.env)
	child.locals = make(map[string]struct{}, len(params))
	for _, param := range params {
		value := fmt.Sprintf("%s.%s.0", label, param)
		child.env[param] = value
		child.locals[param] = struct{}{}
	}
	result, err := child.evalExpr(expr)
	if err != nil {
		return "", err
	}
	if !child.terminated() {
		child.emitTerm(&irReturn{Value: result, HasValue: true})
	}
	fn.Blocks = append(fn.Blocks, child.state.blocks...)
	if len(fn.Captures) > 0 {
		captures := append([]string(nil), fn.Captures...)
		sort.Strings(captures)
		fn.Captures = captures
		fn.CaptureValues = captureValues(b.env, captures)
	}
	b.pb.appendFunction(fn)

	closure := b.newTemp()
	b.emitInstr(IRInstr{
		Dest: closure,
		Op:   "makefunc",
		Text: fn.Name,
		Args: captureValues(b.env, fn.Captures),
		Span: exprSpan(expr),
	})
	return closure, nil
}

// lowerWhileStmt operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns bool and an error. Callers should only trust the value when the
// error is nil.
func (b *irBuilder) lowerWhileStmt(st whileStmt) (bool, error) {
	carryNames := collectScopeNamesExpr(st.Body)
	for name := range b.env {
		carryNames[name] = struct{}{}
	}
	carry := sortedNames(carryNames)
	headerParams := make([]string, len(carry))
	for i := range headerParams {
		headerParams[i] = b.newTemp()
	}

	header := b.newBlock("while.header")
	body := b.newBlock("while.body")
	exit := b.newBlock("while.exit")
	bodyParams := make([]string, len(carry))
	exitParams := make([]string, len(carry))
	for i := range bodyParams {
		bodyParams[i] = b.newTemp()
		exitParams[i] = b.newTemp()
	}
	header.Params = append([]string(nil), headerParams...)
	body.Params = append([]string(nil), bodyParams...)
	exit.Params = append([]string(nil), exitParams...)

	b.recordLoopCaptures(carry)
	b.emitTerm(&irJump{Target: header.Name, Args: b.argsForNames(carry)})

	b.block = header
	for _, name := range carry {
		b.locals[name] = struct{}{}
	}
	b.env = envFromParamValues(carry, headerParams, b.env)

	condBuilder := b.fork()
	cond, err := condBuilder.evalExpr(st.Cond)
	if err != nil {
		return false, err
	}
	b.block = condBuilder.block
	if b.terminated() {
		return true, nil
	}
	b.emitTerm(&irBranch{
		Cond:        cond,
		TrueTarget:  body.Name,
		TrueArgs:    append([]string(nil), headerParams...),
		FalseTarget: exit.Name,
		FalseArgs:   append([]string(nil), headerParams...),
	})

	loopCtx := irLoopCtx{kind: "while", header: header, exit: exit, carry: carry}
	b.loopStack = append(b.loopStack, loopCtx)

	b.block = body
	bodyEnv := envFromParamValues(carry, bodyParams, b.env)
	bodyBuilder := b.fork()
	bodyBuilder.env = bodyEnv
	bodyBuilder.locals = cloneNameSet(b.locals)
	for _, name := range carry {
		bodyBuilder.locals[name] = struct{}{}
	}
	if _, err := bodyBuilder.evalExpr(st.Body); err != nil {
		return false, err
	}
	if !bodyBuilder.terminated() {
		b.emitOn(bodyBuilder.block, &irJump{Target: header.Name, Args: captureValues(bodyBuilder.env, carry)})
	}

	b.loopStack = b.loopStack[:len(b.loopStack)-1]

	b.block = exit
	b.env = envFromParamValues(carry, exitParams, bodyBuilder.env)
	for _, name := range carry {
		b.locals[name] = struct{}{}
	}
	return false, nil
}

// lowerForStmt operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns bool and an error. Callers should only trust the value when the
// error is nil.
func (b *irBuilder) lowerForStmt(st forStmt) (bool, error) {
	carryNames := collectScopeNamesExpr(st.Body)
	carryNames[st.Name] = struct{}{}
	for name := range b.env {
		carryNames[name] = struct{}{}
	}
	carry := sortedNames(carryNames)
	headerParams := make([]string, len(carry)+3)
	for i := range headerParams {
		headerParams[i] = b.newTemp()
	}

	iterBuilder := b.fork()
	iterValue, err := iterBuilder.evalExpr(st.Iter)
	if err != nil {
		return false, err
	}
	b.block = iterBuilder.block
	if b.terminated() {
		return true, nil
	}

	header := b.newBlock("for.header")
	body := b.newBlock("for.body")
	exit := b.newBlock("for.exit")
	iterTemp := headerParams[len(carry)]
	lenTemp := headerParams[len(carry)+1]
	idxTemp := headerParams[len(carry)+2]
	bodyParams := make([]string, len(headerParams))
	exitParams := make([]string, len(carry))
	for i := range bodyParams {
		bodyParams[i] = b.newTemp()
	}
	for i := range exitParams {
		exitParams[i] = b.newTemp()
	}
	header.Params = append([]string(nil), headerParams...)
	body.Params = append([]string(nil), bodyParams...)
	exit.Params = append([]string(nil), exitParams...)

	preLen := b.newTemp()
	zeroTemp := b.newTemp()
	b.emitInstr(IRInstr{Dest: preLen, Op: "call", Text: "len", Args: []string{iterValue}})
	b.emitInstr(IRInstr{Dest: zeroTemp, Op: "const", Value: numberValue(0)})
	b.recordLoopCaptures(carry)
	b.emitTerm(&irJump{Target: header.Name, Args: append(append(b.argsForNames(carry), iterValue, preLen), zeroTemp)})

	b.block = header
	for _, name := range carry {
		b.locals[name] = struct{}{}
	}
	b.env = envFromParamValues(carry, headerParams[:len(carry)], b.env)
	condTemp := b.newTemp()
	b.emitInstr(IRInstr{Dest: condTemp, Op: "binary", Text: "<", Args: []string{idxTemp, lenTemp}})
	b.emitTerm(&irBranch{
		Cond:        condTemp,
		TrueTarget:  body.Name,
		TrueArgs:    append([]string(nil), headerParams...),
		FalseTarget: exit.Name,
		FalseArgs:   b.argsForNames(carry),
	})

	loopCtx := irLoopCtx{
		kind:      "for",
		header:    header,
		exit:      exit,
		carry:     carry,
		loopVar:   st.Name,
		loopIter:  iterTemp,
		loopLen:   lenTemp,
		loopIndex: idxTemp,
	}
	b.loopStack = append(b.loopStack, loopCtx)

	b.block = body
	bodyEnv := envFromParamValues(carry, bodyParams[:len(carry)], b.env)
	itemTemp := b.newTemp()
	b.emitInstr(IRInstr{Dest: itemTemp, Op: "call", Text: "at", Args: []string{iterTemp, idxTemp}})
	bodyEnv[st.Name] = itemTemp
	bodyBuilder := b.fork()
	bodyBuilder.env = bodyEnv
	bodyBuilder.locals = cloneNameSet(b.locals)
	for _, name := range carry {
		bodyBuilder.locals[name] = struct{}{}
	}
	bodyBuilder.locals[st.Name] = struct{}{}
	if _, err := bodyBuilder.evalExpr(st.Body); err != nil {
		return false, err
	}
	if !bodyBuilder.terminated() {
		one := bodyBuilder.newTemp()
		nextIdx := bodyBuilder.newTemp()
		bodyBuilder.emitInstr(IRInstr{Dest: one, Op: "const", Value: numberValue(1)})
		bodyBuilder.emitInstr(IRInstr{Dest: nextIdx, Op: "binary", Text: "+", Args: []string{idxTemp, one}})
		b.emitOn(bodyBuilder.block, &irJump{
			Target: header.Name,
			Args:   append(captureValues(bodyBuilder.env, carry), iterTemp, lenTemp, nextIdx),
		})
	}

	b.loopStack = b.loopStack[:len(b.loopStack)-1]

	b.block = exit
	b.env = envFromParamValues(carry, exitParams, bodyBuilder.env)
	for _, name := range carry {
		b.locals[name] = struct{}{}
	}
	b.locals[st.Name] = struct{}{}
	return false, nil
}

func (b *irBuilder) recordLoopCaptures(names []string) {
	for _, name := range names {
		if _, local := b.locals[name]; local {
			continue
		}
		if _, exists := b.env[name]; exists {
			b.recordCapture(name)
		}
	}
}

// emitOn operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (b *irBuilder) emitOn(block *IRBlock, term IRTerminator) {
	if block == nil {
		return
	}
	block.Term = term
}

// evalExpr operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string and an error. Callers should only trust the value when the
// error is nil.
func (b *irBuilder) evalExpr(expr Expr) (string, error) {
	switch n := expr.(type) {
	case *literalExpr:
		tmp := b.newTemp()
		b.emitInstr(IRInstr{Dest: tmp, Op: "const", Value: n.value})
		return tmp, nil
	case *identExpr:
		return b.resolveIdent(n.name, n.Span)
	case *unaryExpr:
		child := b.fork()
		inner, err := child.evalExpr(n.expr)
		if err != nil {
			return "", err
		}
		b.block = child.block
		tmp := b.newTemp()
		b.emitInstr(IRInstr{Dest: tmp, Op: "unary", Text: n.op.String(), Args: []string{inner}})
		return tmp, nil
	case *binaryExpr:
		switch n.op {
		case tokenAndAnd:
			return b.lowerLogicalBinary(n, false)
		case tokenOrOr:
			return b.lowerLogicalBinary(n, true)
		}
		leftBuilder := b.fork()
		left, err := leftBuilder.evalExpr(n.left)
		if err != nil {
			return "", err
		}
		b.block = leftBuilder.block
		rightBuilder := b.fork()
		right, err := rightBuilder.evalExpr(n.right)
		if err != nil {
			return "", err
		}
		b.block = rightBuilder.block
		tmp := b.newTemp()
		b.emitInstr(IRInstr{Dest: tmp, Op: "binary", Text: n.op.String(), Args: []string{left, right}})
		return tmp, nil
	case *callExpr:
		if member, ok := n.callee.(*memberExpr); ok {
			if object, ok := member.object.(*identExpr); ok {
				if cases, ok := b.pb.enums[object.name]; ok {
					item, ok := cases[member.name]
					if !ok {
						return "", fmt.Errorf("enum %s has no case %q", object.name, member.name)
					}
					if len(n.args) != item.Arity() {
						return "", fmt.Errorf("enum case %s.%s expects %d arguments", object.name, member.name, item.Arity())
					}
					args := make([]string, 0, len(n.args))
					for _, arg := range n.args {
						child := b.fork()
						value, err := child.evalExpr(arg)
						if err != nil {
							return "", err
						}
						b.block = child.block
						args = append(args, value)
					}
					tmp := b.newTemp()
					b.emitInstr(IRInstr{Dest: tmp, Op: "enum", Text: object.name + "." + member.name, Args: args})
					return tmp, nil
				}
			}
		}
		if name, ok := specialFormName(n.callee); ok {
			switch name {
			case "if":
				return b.lowerIfExpr(n)
			case "and":
				return b.lowerLogicalCall(n, false)
			case "or":
				return b.lowerLogicalCall(n, true)
			case "spawn":
				if len(n.args) != 1 {
					return "", fmt.Errorf("spawn expects exactly one expression")
				}
				thunk, err := b.lowerDeferredExpr(n.args[0], "spawn.task", nil)
				if err != nil {
					return "", err
				}
				tmp := b.newTemp()
				b.emitInstr(IRInstr{Dest: tmp, Op: "async", Text: name, Args: []string{thunk}, Span: n.Span})
				return tmp, nil
			case "parallel":
				thunks := make([]string, 0, len(n.args))
				for _, arg := range n.args {
					thunk, err := b.lowerDeferredExpr(arg, "parallel.branch", nil)
					if err != nil {
						return "", err
					}
					thunks = append(thunks, thunk)
				}
				tmp := b.newTemp()
				b.emitInstr(IRInstr{Dest: tmp, Op: "async", Text: name, Args: thunks, Span: n.Span})
				return tmp, nil
			default:
				args := make([]string, 0, len(n.args))
				for _, arg := range n.args {
					child := b.fork()
					value, err := child.evalExpr(arg)
					if err != nil {
						return "", err
					}
					b.block = child.block
					args = append(args, value)
				}
				tmp := b.newTemp()
				b.emitInstr(IRInstr{Dest: tmp, Op: "async", Text: name, Args: args})
				return tmp, nil
			}
		}
		calleeBuilder := b.fork()
		callee, err := calleeBuilder.evalExpr(n.callee)
		if err != nil {
			return "", err
		}
		b.block = calleeBuilder.block
		args := make([]string, 0, len(n.args))
		for _, arg := range n.args {
			argBuilder := b.fork()
			val, err := argBuilder.evalExpr(arg)
			if err != nil {
				return "", err
			}
			b.block = argBuilder.block
			args = append(args, val)
		}
		tmp := b.newTemp()
		b.emitInstr(IRInstr{Dest: tmp, Op: "call", Text: callee, Args: args})
		return tmp, nil
	case *memberExpr:
		if object, ok := n.object.(*identExpr); ok {
			if cases, ok := b.pb.enums[object.name]; ok {
				item, ok := cases[n.name]
				if !ok {
					return "", fmt.Errorf("enum %s has no case %q", object.name, n.name)
				}
				if item.Arity() != 0 {
					return "", fmt.Errorf("enum case %s.%s expects %d arguments", object.name, n.name, item.Arity())
				}
				tmp := b.newTemp()
				b.emitInstr(IRInstr{Dest: tmp, Op: "enum", Text: object.name + "." + n.name})
				return tmp, nil
			}
		}
		objBuilder := b.fork()
		obj, err := objBuilder.evalExpr(n.object)
		if err != nil {
			return "", err
		}
		b.block = objBuilder.block
		tmp := b.newTemp()
		b.emitInstr(IRInstr{Dest: tmp, Op: "member", Text: n.name, Args: []string{obj}, Span: n.NameSpan})
		return tmp, nil
	case *listExpr:
		items := make([]string, 0, len(n.items))
		for _, item := range n.items {
			itemBuilder := b.fork()
			val, err := itemBuilder.evalExpr(item)
			if err != nil {
				return "", err
			}
			b.block = itemBuilder.block
			items = append(items, val)
		}
		tmp := b.newTemp()
		b.emitInstr(IRInstr{Dest: tmp, Op: "list", Args: items})
		return tmp, nil
	case *tupleExpr, *structExpr, *structUpdateExpr, *indexExpr:
		return "", fmt.Errorf("native C backend: product-type expressions are not yet supported")
	case *blockExpr:
		for _, stmt := range n.stmts {
			terminated, err := b.lowerStmt(stmt)
			if err != nil {
				return "", err
			}
			if terminated {
				return b.newTemp(), nil
			}
		}
		if n.tail == nil {
			tmp := b.newTemp()
			b.emitInstr(IRInstr{Dest: tmp, Op: "const", Value: nullValue()})
			return tmp, nil
		}
		return b.evalExpr(n.tail)
	case *tryExpr:
		body, err := b.lowerDeferredExpr(n.body, "try.body", nil)
		if err != nil {
			return "", err
		}
		handler, err := b.lowerDeferredExpr(n.handler, "try.catch", []string{n.catchName})
		if err != nil {
			return "", err
		}
		tmp := b.newTemp()
		b.emitInstr(IRInstr{Dest: tmp, Op: "try", Args: []string{body, handler}, Span: n.Span})
		return tmp, nil
	case *propagateExpr:
		return b.evalExpr(n.expr)
	case *matchExpr:
		return b.lowerMatchExpr(n)
	default:
		return "", fmt.Errorf("internal error: unknown expression type %T", expr)
	}
}

// lowerMatchExpr lowers supported enum patterns into ordinary CFG branches.
// Pattern tests are emitted as IR; the source evaluator is never consulted
// while compiling a match expression.
func (b *irBuilder) lowerMatchExpr(n *matchExpr) (string, error) {
	subjectBuilder := b.fork()
	subject, err := subjectBuilder.evalExpr(n.subject)
	if err != nil {
		return "", err
	}
	b.block = subjectBuilder.block
	join := b.newBlock("match.join")
	result := b.newTemp()
	join.Params = []string{result}
	current := b.block
	for index, arm := range n.arms {
		armBlock := b.newBlock("match.arm")
		b.block = current
		if arm.isDefault {
			if index != len(n.arms)-1 {
				return "", fmt.Errorf("default match arm must be final")
			}
			b.emitTerm(&irJump{Target: armBlock.Name})
			current = nil
		} else {
			bindingPattern, err := nativeMatchPattern(arm.pattern)
			if err != nil {
				return "", err
			}
			next := b.newBlock("match.next")
			matchTarget := armBlock
			if arm.guard != nil {
				guardBlock := b.newBlock("match.guard")
				matchTarget = guardBlock
				guardBuilder := b.fork()
				guardBuilder.block = guardBlock
				if err := bindNativeMatchPattern(guardBuilder, bindingPattern, subject); err != nil {
					return "", err
				}
				guard, err := guardBuilder.evalExpr(arm.guard)
				if err != nil {
					return "", err
				}
				guardBuilder.emitTerm(&irBranch{Cond: guard, TrueTarget: armBlock.Name, FalseTarget: next.Name})
			}
			if err := lowerNativeEnumPatternBranch(b, arm.pattern, subject, matchTarget, next, make(map[string]string)); err != nil {
				return "", err
			}
			current = next
		}
		bodyBuilder := b.fork()
		bodyBuilder.block = armBlock
		if !arm.isDefault {
			bindingPattern, err := nativeMatchPattern(arm.pattern)
			if err != nil {
				return "", err
			}
			if err := bindNativeMatchPattern(bodyBuilder, bindingPattern, subject); err != nil {
				return "", err
			}
		}
		value, err := bodyBuilder.evalExpr(arm.body)
		if err != nil {
			return "", err
		}
		if !bodyBuilder.terminated() {
			bodyBuilder.emitTerm(&irJump{Target: join.Name, Args: []string{value}})
		}
	}
	if current != nil {
		b.block = current
		failure := b.newTemp()
		b.emitInstr(IRInstr{Dest: failure, Op: "matchfail", Text: "unmatched match case"})
		b.emitTerm(&irJump{Target: join.Name, Args: []string{failure}})
	}
	b.block = join
	return result, nil
}

func lowerNativeEnumPatternBranch(b *irBuilder, pattern Pattern, subject string, matched, failed *IRBlock, bindings map[string]string) error {
	switch pattern := pattern.(type) {
	case enumPattern:
		condition := b.newTemp()
		b.emitInstr(IRInstr{Dest: condition, Op: "matchenum", Text: pattern.typeName + "." + pattern.caseName, Args: []string{subject}, Value: uintValue(uint64(len(pattern.items)), 64)})
		itemBlock := matched
		if len(pattern.items) != 0 {
			itemBlock = b.newBlock("match.enum.item")
		}
		b.emitTerm(&irBranch{Cond: condition, TrueTarget: itemBlock.Name, FalseTarget: failed.Name})
		for index, item := range pattern.items {
			b.block = itemBlock
			payload := b.newTemp()
			b.emitInstr(IRInstr{Dest: payload, Op: "enumpayload", Args: []string{subject}, Value: uintValue(uint64(index), 64)})
			next := matched
			if index+1 < len(pattern.items) {
				next = b.newBlock("match.enum.item")
			}
			if err := lowerNativeEnumPatternBranch(b, item, payload, next, failed, bindings); err != nil {
				return err
			}
			itemBlock = next
		}
		return nil
	case listPattern:
		condition := b.newTemp()
		b.emitInstr(IRInstr{Dest: condition, Op: "matchlist", Args: []string{subject}, Value: uintValue(uint64(len(pattern.items)), 64)})
		itemBlock := matched
		if len(pattern.items) != 0 {
			itemBlock = b.newBlock("match.list.item")
		}
		b.emitTerm(&irBranch{Cond: condition, TrueTarget: itemBlock.Name, FalseTarget: failed.Name})
		for index, item := range pattern.items {
			b.block = itemBlock
			value := b.newTemp()
			b.emitInstr(IRInstr{Dest: value, Op: "listitem", Args: []string{subject}, Value: uintValue(uint64(index), 64)})
			next := matched
			if index+1 < len(pattern.items) {
				next = b.newBlock("match.list.item")
			}
			if err := lowerNativeEnumPatternBranch(b, item, value, next, failed, bindings); err != nil {
				return err
			}
			itemBlock = next
		}
		return nil
	case wildcardPattern:
		b.emitTerm(&irJump{Target: matched.Name})
		return nil
	case bindPattern:
		if previous, exists := bindings[pattern.name]; exists {
			condition := b.newTemp()
			b.emitInstr(IRInstr{Dest: condition, Op: "binary", Text: "==", Args: []string{subject, previous}})
			b.emitTerm(&irBranch{Cond: condition, TrueTarget: matched.Name, FalseTarget: failed.Name})
			return nil
		}
		bindings[pattern.name] = subject
		b.emitTerm(&irJump{Target: matched.Name})
		return nil
	case literalPattern:
		literal := b.newTemp()
		condition := b.newTemp()
		b.emitInstr(IRInstr{Dest: literal, Op: "const", Value: pattern.value})
		b.emitInstr(IRInstr{Dest: condition, Op: "binary", Text: "==", Args: []string{subject, literal}})
		b.emitTerm(&irBranch{Cond: condition, TrueTarget: matched.Name, FalseTarget: failed.Name})
		return nil
	case relationalPattern:
		literal := b.newTemp()
		condition := b.newTemp()
		b.emitInstr(IRInstr{Dest: literal, Op: "const", Value: pattern.value})
		b.emitInstr(IRInstr{Dest: condition, Op: "binary", Text: pattern.op.String(), Args: []string{subject, literal}})
		b.emitTerm(&irBranch{Cond: condition, TrueTarget: matched.Name, FalseTarget: failed.Name})
		return nil
	case typePattern:
		condition := b.newTemp()
		b.emitInstr(IRInstr{Dest: condition, Op: "matchtype", Text: pattern.typeName, Args: []string{subject}})
		b.emitTerm(&irBranch{Cond: condition, TrueTarget: matched.Name, FalseTarget: failed.Name})
		return nil
	case andPattern:
		right := b.newBlock("match.and")
		if err := lowerNativeEnumPatternBranch(b, pattern.left, subject, right, failed, bindings); err != nil {
			return err
		}
		b.block = right
		return lowerNativeEnumPatternBranch(b, pattern.right, subject, matched, failed, bindings)
	case notPattern:
		return lowerNativeEnumPatternBranch(b, pattern.inner, subject, failed, matched, cloneStringMap(bindings))
	case orPattern:
		if len(pattern.alternatives) == 0 {
			return fmt.Errorf("native lowering does not support an empty grouped pattern")
		}
		for index, alternative := range pattern.alternatives {
			next := failed
			if index+1 < len(pattern.alternatives) {
				next = b.newBlock("match.alternative")
			}
			if err := lowerNativeEnumPatternBranch(b, alternative, subject, matched, next, cloneStringMap(bindings)); err != nil {
				return err
			}
			if next != failed {
				b.block = next
			}
		}
		return nil
	default:
		return fmt.Errorf("native lowering does not yet support this match pattern")
	}
}

func nativeMatchPattern(pattern Pattern) (Pattern, error) {
	if err := validateNativeMatchPattern(pattern); err != nil {
		return nil, err
	}
	return pattern, nil
}

func validateNativeMatchPattern(pattern Pattern) error {
	switch pattern := pattern.(type) {
	case wildcardPattern, bindPattern, literalPattern, relationalPattern, typePattern:
		return nil
	case enumPattern, listPattern:
		_, err := nativeEnumPatternBindingLayout(pattern, "")
		return err
	case andPattern:
		if err := validateNativeMatchPattern(pattern.left); err != nil {
			return err
		}
		return validateNativeMatchPattern(pattern.right)
	case notPattern:
		layout, err := nativeEnumPatternBindingLayout(pattern.inner, "")
		if err != nil {
			return err
		}
		if len(layout) != 0 {
			return fmt.Errorf("native negated patterns cannot bind names")
		}
		return validateNativeMatchPattern(pattern.inner)
	case orPattern:
		if len(pattern.alternatives) == 0 {
			return fmt.Errorf("native lowering does not support an empty grouped pattern")
		}
		if err := validateNativeMatchPattern(pattern.alternatives[0]); err != nil {
			return err
		}
		firstLayout, err := nativeEnumPatternBindingLayout(pattern.alternatives[0], "")
		if err != nil {
			return err
		}
		for _, alternative := range pattern.alternatives[1:] {
			if err := validateNativeMatchPattern(alternative); err != nil {
				return err
			}
			nextLayout, err := nativeEnumPatternBindingLayout(alternative, "")
			if err != nil {
				return err
			}
			if len(nextLayout) != len(firstLayout) {
				return fmt.Errorf("native grouped patterns must bind the same layout")
			}
			for index, binding := range firstLayout {
				if binding != nextLayout[index] {
					return fmt.Errorf("native grouped patterns must bind the same names")
				}
			}
		}
		return nil
	default:
		return fmt.Errorf("native lowering does not yet support this match pattern")
	}
}

func nativeEnumPatternBindingLayout(pattern Pattern, path string) ([]string, error) {
	switch pattern := pattern.(type) {
	case wildcardPattern:
		return nil, nil
	case bindPattern:
		return []string{path + "=" + pattern.name}, nil
	case literalPattern, relationalPattern:
		return nil, nil
	case typePattern:
		if pattern.binding == "" || pattern.binding == "_" {
			return nil, nil
		}
		return []string{path + "=" + pattern.binding}, nil
	case enumPattern:
		layout := make([]string, 0, len(pattern.items))
		for index, item := range pattern.items {
			bindings, err := nativeEnumPatternBindingLayout(item, fmt.Sprintf("%s.e%d", path, index))
			if err != nil {
				return nil, err
			}
			layout = append(layout, bindings...)
		}
		return layout, nil
	case listPattern:
		layout := make([]string, 0, len(pattern.items))
		for index, item := range pattern.items {
			bindings, err := nativeEnumPatternBindingLayout(item, fmt.Sprintf("%s.l%d", path, index))
			if err != nil {
				return nil, err
			}
			layout = append(layout, bindings...)
		}
		return layout, nil
	case andPattern:
		left, err := nativeEnumPatternBindingLayout(pattern.left, path)
		if err != nil {
			return nil, err
		}
		right, err := nativeEnumPatternBindingLayout(pattern.right, path)
		if err != nil {
			return nil, err
		}
		return append(left, right...), nil
	case notPattern:
		return nil, nil
	case orPattern:
		if len(pattern.alternatives) == 0 {
			return nil, fmt.Errorf("native lowering does not support an empty grouped pattern")
		}
		return nativeEnumPatternBindingLayout(pattern.alternatives[0], path)
	default:
		return nil, fmt.Errorf("native lowering does not yet support this enum payload pattern")
	}
}

func bindNativeMatchPattern(b *irBuilder, pattern Pattern, subject string) error {
	return bindNativeEnumPatternValue(b, pattern, subject)
}

func bindNativeEnumPatternValue(b *irBuilder, pattern Pattern, subject string) error {
	switch pattern := pattern.(type) {
	case wildcardPattern:
		return nil
	case bindPattern:
		b.env[pattern.name] = subject
		b.locals[pattern.name] = struct{}{}
		return nil
	case literalPattern, relationalPattern:
		return nil
	case typePattern:
		if pattern.binding != "" && pattern.binding != "_" {
			b.env[pattern.binding] = subject
			b.locals[pattern.binding] = struct{}{}
		}
		return nil
	case enumPattern:
		for index, item := range pattern.items {
			payload := b.newTemp()
			b.emitInstr(IRInstr{Dest: payload, Op: "enumpayload", Args: []string{subject}, Value: uintValue(uint64(index), 64)})
			if err := bindNativeEnumPatternValue(b, item, payload); err != nil {
				return err
			}
		}
		return nil
	case listPattern:
		for index, item := range pattern.items {
			value := b.newTemp()
			b.emitInstr(IRInstr{Dest: value, Op: "listitem", Args: []string{subject}, Value: uintValue(uint64(index), 64)})
			if err := bindNativeEnumPatternValue(b, item, value); err != nil {
				return err
			}
		}
		return nil
	case andPattern:
		if err := bindNativeEnumPatternValue(b, pattern.left, subject); err != nil {
			return err
		}
		return bindNativeEnumPatternValue(b, pattern.right, subject)
	case notPattern:
		return nil
	case orPattern:
		if len(pattern.alternatives) == 0 {
			return fmt.Errorf("native lowering does not support an empty grouped pattern")
		}
		return bindNativeEnumPatternValue(b, pattern.alternatives[0], subject)
	default:
		return fmt.Errorf("native lowering does not yet support this enum payload pattern")
	}
}

// lowerIfExpr operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string and an error. Callers should only trust the value when the
// error is nil.
func (b *irBuilder) lowerIfExpr(n *callExpr) (string, error) {
	if len(n.args) != 3 {
		return "", fmt.Errorf("if expects 3 arguments")
	}
	condBuilder := b.fork()
	cond, err := condBuilder.evalExpr(n.args[0])
	if err != nil {
		return "", err
	}
	b.block = condBuilder.block

	thenBlock := b.newBlock("if.then")
	elseBlock := b.newBlock("if.else")
	joinBlock := b.newBlock("if.join")
	phi := b.newTemp()

	b.emitTerm(&irBranch{
		Cond:        cond,
		TrueTarget:  thenBlock.Name,
		TrueArgs:    nil,
		FalseTarget: elseBlock.Name,
		FalseArgs:   nil,
	})

	thenBuilder := b.fork()
	thenBuilder.block = thenBlock
	thenVal, err := thenBuilder.evalExpr(n.args[1])
	if err != nil {
		return "", err
	}
	if !thenBuilder.terminated() {
		thenBuilder.emitTerm(&irJump{Target: joinBlock.Name, Args: []string{thenVal}})
	}

	elseBuilder := b.fork()
	elseBuilder.block = elseBlock
	elseVal, err := elseBuilder.evalExpr(n.args[2])
	if err != nil {
		return "", err
	}
	if !elseBuilder.terminated() {
		elseBuilder.emitTerm(&irJump{Target: joinBlock.Name, Args: []string{elseVal}})
	}

	joinBlock.Params = []string{phi}
	b.block = joinBlock
	return phi, nil
}

// lowerLogicalBinary operates on irBuilder values and keeps the
// irbuilder-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns string and an error. Callers should only trust the value when the
// error is nil.
func (b *irBuilder) lowerLogicalBinary(n *binaryExpr, isOr bool) (string, error) {
	leftBuilder := b.fork()
	left, err := leftBuilder.evalExpr(n.left)
	if err != nil {
		return "", err
	}
	b.block = leftBuilder.block

	thenBlock := b.newBlock("logic.then")
	elseBlock := b.newBlock("logic.else")
	joinBlock := b.newBlock("logic.join")
	phi := b.newTemp()

	cond := left
	if isOr {
		b.emitTerm(&irBranch{
			Cond:        cond,
			TrueTarget:  joinBlock.Name,
			TrueArgs:    []string{left},
			FalseTarget: elseBlock.Name,
			FalseArgs:   nil,
		})
	} else {
		b.emitTerm(&irBranch{
			Cond:        cond,
			TrueTarget:  thenBlock.Name,
			TrueArgs:    nil,
			FalseTarget: joinBlock.Name,
			FalseArgs:   []string{left},
		})
	}

	if isOr {
		elseBuilder := b.fork()
		elseBuilder.block = elseBlock
		right, err := elseBuilder.evalExpr(n.right)
		if err != nil {
			return "", err
		}
		if !elseBuilder.terminated() {
			elseBuilder.emitTerm(&irJump{Target: joinBlock.Name, Args: []string{right}})
		}
		joinBlock.Params = []string{phi}
		b.block = joinBlock
		return phi, nil
	}

	thenBuilder := b.fork()
	thenBuilder.block = thenBlock
	right, err := thenBuilder.evalExpr(n.right)
	if err != nil {
		return "", err
	}
	if !thenBuilder.terminated() {
		thenBuilder.emitTerm(&irJump{Target: joinBlock.Name, Args: []string{right}})
	}
	joinBlock.Params = []string{phi}
	b.block = joinBlock
	return phi, nil
}

// lowerLogicalCall operates on irBuilder values and keeps the
// irbuilder-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns string and an error. Callers should only trust the value when the
// error is nil.
func (b *irBuilder) lowerLogicalCall(n *callExpr, isOr bool) (string, error) {
	// The call form shares the same lowering strategy as the operator form.
	if len(n.args) != 2 {
		if isOr {
			return "", fmt.Errorf("or expects 2 arguments")
		}
		return "", fmt.Errorf("and expects 2 arguments")
	}
	binary := &binaryExpr{op: tokenAndAnd, left: n.args[0], right: n.args[1]}
	if isOr {
		binary.op = tokenOrOr
	}
	return b.lowerLogicalBinary(binary, isOr)
}

// argsForNames operates on irBuilder values and keeps the irbuilder-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns []string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (b *irBuilder) argsForNames(names []string) []string {
	args := make([]string, 0, len(names))
	for _, name := range names {
		args = append(args, b.valueOrUndef(name))
	}
	return args
}

// captureValues handles the capture values step in the surrounding package and
// returns []string.
//
// It returns []string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func captureValues(env map[string]string, names []string) []string {
	args := make([]string, 0, len(names))
	for _, name := range names {
		if v, ok := env[name]; ok {
			args = append(args, v)
			continue
		}
		args = append(args, "undef")
	}
	return args
}

// envFromParamValues handles the env from param values step in the surrounding
// package and returns map[string]string.
//
// It returns map[string]string so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func envFromParamValues(names []string, params []string, fallback map[string]string) map[string]string {
	env := make(map[string]string, len(names))
	for i, name := range names {
		if i < len(params) {
			env[name] = params[i]
			continue
		}
		if v, ok := fallback[name]; ok {
			env[name] = v
			continue
		}
		env[name] = "undef"
	}
	return env
}

// collectScopeNamesExpr handles the collect scope names expr step in the
// surrounding package and returns map[string]value.
//
// It returns map[string]value so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func collectScopeNamesExpr(expr Expr) map[string]struct{} {
	out := make(map[string]struct{})
	collectScopeNamesExprInto(expr, out)
	return out
}

// collectScopeNamesExprInto handles the collect scope names expr into step in
// the surrounding package and returns no result.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func collectScopeNamesExprInto(expr Expr, out map[string]struct{}) {
	switch n := expr.(type) {
	case *blockExpr:
		collectScopeNamesBlock(n, out)
	}
}

// collectScopeNamesBlock handles the collect scope names block step in the
// surrounding package and returns no result.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func collectScopeNamesBlock(n *blockExpr, out map[string]struct{}) {
	if n == nil {
		return
	}
	for _, stmt := range n.stmts {
		collectScopeNamesStmt(stmt, out)
	}
	if n.tail != nil {
		collectScopeNamesExprInto(n.tail, out)
	}
}

// collectScopeNamesStmt handles the collect scope names stmt step in the
// surrounding package and returns no result.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func collectScopeNamesStmt(stmt Stmt, out map[string]struct{}) {
	switch st := stmt.(type) {
	case letStmt:
		out[st.Name] = struct{}{}
	case importStmt:
		out[st.Alias] = struct{}{}
	case funcStmt:
		out[st.Name] = struct{}{}
	case whileStmt:
		collectScopeNamesExprInto(st.Body, out)
	case forStmt:
		out[st.Name] = struct{}{}
		collectScopeNamesExprInto(st.Body, out)
	case returnStmt:
		if st.Expr != nil {
			collectScopeNamesExprInto(st.Expr, out)
		}
	}
}

// sortedNames handles the sorted names step in the surrounding package and
// returns []string.
//
// It returns []string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func sortedNames(src map[string]struct{}) []string {
	if len(src) == 0 {
		return nil
	}
	out := make([]string, 0, len(src))
	for name := range src {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
