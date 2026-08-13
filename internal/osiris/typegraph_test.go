package osiris

import "testing"

func typeFromSource(t *testing.T, source string) staticType {
	t.Helper()
	typ, err := parseStaticType(source)
	if err != nil {
		t.Fatal(err)
	}
	return typ
}

func TestTypeContextInternsStructuralTypesAndFormatsSeparately(t *testing.T) {
	types := NewTypeContext()
	first := types.InternStatic(typeFromSource(t, "list<Lazy<u32>>"))
	second := types.InternStatic(typeFromSource(t, "list<Lazy<u32>>"))
	if first == 0 || first != second {
		t.Fatalf("interned IDs = %d, %d; want equal nonzero identity", first, second)
	}
	if got, want := types.Format(first), "list<Lazy<u32>>"; got != want {
		t.Fatalf("format = %q, want %q", got, want)
	}
	if types.Equal(first, types.InternStatic(typeFromSource(t, "list<Lazy<u64>>"))) {
		t.Fatal("different structural payloads shared a TypeID")
	}
}

func TestTypeContextPreservesNominalAndAliasIdentity(t *testing.T) {
	types := NewTypeContext()
	pointA := types.Intern(TypeNode{Kind: TypeNominal, Name: "module-a.Point"})
	pointB := types.Intern(TypeNode{Kind: TypeNominal, Name: "module-b.Point"})
	if types.Equal(pointA, pointB) {
		t.Fatal("distinct nominal declarations shared identity")
	}
	alias := types.InternStatic(typeFromSource(t, "u32"))
	underlying := types.InternStatic(typeFromSource(t, "u32"))
	if !types.Equal(alias, underlying) {
		t.Fatal("identical alias target did not share canonical identity")
	}
}

func TestTypeContextDisplayMetadataCannotChangeIdentity(t *testing.T) {
	types := NewTypeContext()
	canonical := types.Intern(TypeNode{Kind: TypeNominal, Name: "math.Vector", Declaration: 41, Display: "Vector"})
	renamed := types.Intern(TypeNode{Kind: TypeNominal, Name: "math.Vector", Declaration: 41, Display: "Vec"})
	if canonical != renamed {
		t.Fatalf("display spelling changed canonical identity: %d != %d", canonical, renamed)
	}
	if got := types.Format(canonical, TypeFormatDiagnostic); got != "Vector" {
		t.Fatalf("first canonical display = %q, want Vector", got)
	}
	otherDeclaration := types.Intern(TypeNode{Kind: TypeNominal, Name: "math.Vector", Declaration: 42, Display: "Vector"})
	if canonical == otherDeclaration {
		t.Fatal("separate nominal declarations shared a TypeID")
	}
}

func TestTypeContextsAreCompilationLocal(t *testing.T) {
	first, second := NewTypeContext(), NewTypeContext()
	firstU32 := first.InternStatic(typeFromSource(t, "u32"))
	secondU32 := second.InternStatic(typeFromSource(t, "u32"))
	first.Intern(TypeNode{Kind: TypeNominal, Name: "first-only"})
	if len(first.nodes) == len(second.nodes) || first.nodes[0].Kind != TypeInvalid || second.nodes[0].Kind != TypeInvalid || firstU32 == 0 || secondU32 == 0 {
		t.Fatal("type contexts did not own independent arenas")
	}
}

func TestTypeContextSubstitutionUsesIDsWithoutFormatParse(t *testing.T) {
	types := NewTypeContext()
	parameter := types.Intern(TypeNode{Kind: TypeGenericParameter, Name: "T"})
	list := types.Intern(TypeNode{Kind: TypeList, Name: "list", Args: []TypeID{parameter}})
	u32 := types.InternStatic(typeFromSource(t, "u32"))
	result := types.Substitute(list, map[TypeID]TypeID{parameter: u32})
	if got, want := types.Format(result), "list<u32>"; got != want {
		t.Fatalf("substitution format = %q, want %q", got, want)
	}
	if result != types.InternStatic(typeFromSource(t, "list<u32>")) {
		t.Fatal("substitution did not return canonical interned node")
	}
}

func TestGenericParameterIdentityIncludesDeclaringOwner(t *testing.T) {
	types := NewTypeContext()
	typ := typeFromSource(t, "list<T>")
	first := types.InternStaticWithGenericParameters(typ, []typeParam{{Name: "T"}}, 1)
	second := types.InternStaticWithGenericParameters(typ, []typeParam{{Name: "T"}}, 2)
	if first == second {
		t.Fatal("generic parameters from separate declarations shared identity")
	}
	firstNode, _ := types.Node(first)
	secondNode, _ := types.Node(second)
	if len(firstNode.Args) != 1 || len(secondNode.Args) != 1 || firstNode.Args[0] == secondNode.Args[0] {
		t.Fatalf("generic argument identities = %#v and %#v; want distinct owner-scoped parameters", firstNode, secondNode)
	}
	u32 := types.InternStatic(typeFromSource(t, "u32"))
	if got := types.Substitute(first, map[TypeID]TypeID{firstNode.Args[0]: u32}); got != types.InternStatic(typeFromSource(t, "list<u32>")) {
		t.Fatal("owner-scoped generic substitution did not use canonical IDs")
	}
}

func TestCheckedNominalDeclarationsHaveDistinctCanonicalIDs(t *testing.T) {
	program, err := Parse("<typegraph-nominal>", "struct Left { value u32 }\nstruct Right { value u32 }\n")
	if err != nil {
		t.Fatal(err)
	}
	checked, err := NewRuntime().CheckProgram(t.Context(), program)
	if err != nil {
		t.Fatal(err)
	}
	left, right := checked.Nominals["Left"], checked.Nominals["Right"]
	if left == 0 || right == 0 || left == right {
		t.Fatalf("nominal IDs = Left:%d Right:%d; want distinct nonzero IDs", left, right)
	}
	leftNode, _ := checked.Types.Node(left)
	rightNode, _ := checked.Types.Node(right)
	if leftNode.Declaration == 0 || rightNode.Declaration == 0 || leftNode.Declaration == rightNode.Declaration {
		t.Fatalf("nominal declarations = %#v and %#v; want unique declaration identity", leftNode, rightNode)
	}
}

func TestCheckedAliasAndNewtypeIdentity(t *testing.T) {
	program, err := Parse("<typegraph-alias-newtype>", "type Count = u32\nnewtype UserID u32\nenum Status { Ready, Stopped }\n")
	if err != nil {
		t.Fatal(err)
	}
	checked, err := NewRuntime().CheckProgram(t.Context(), program)
	if err != nil {
		t.Fatal(err)
	}
	u32 := checked.Types.InternStatic(typeFromSource(t, "u32"))
	alias := checked.Aliases["Count"]
	if alias != u32 {
		t.Fatal("transparent alias should resolve to its canonical target in checked graph")
	}
	if checked.Nominals["UserID"] == 0 || checked.Nominals["Status"] == 0 || checked.Nominals["UserID"] == checked.Nominals["Status"] {
		t.Fatalf("newtype/enum nominal IDs = %#v", checked.Nominals)
	}
}

func TestCheckedProgramOwnsDiscardableTypeContext(t *testing.T) {
	program, err := Parse("<typegraph>", "let value Lazy<u32> = lazy { u32(1) }\n")
	if err != nil {
		t.Fatal(err)
	}
	checked, err := NewRuntime().CheckProgram(t.Context(), program)
	if err != nil {
		t.Fatal(err)
	}
	if checked.Types == nil || len(checked.Bindings) != 1 || checked.Bindings[0].TypeID == 0 {
		t.Fatalf("checked graph = %#v", checked)
	}
	if got, want := checked.Types.Format(checked.Bindings[0].TypeID), "Lazy<u32>"; got != want {
		t.Fatalf("checked type = %q, want %q", got, want)
	}
}

func TestIRCarriesCanonicalTypeIDsFromTypedProgram(t *testing.T) {
	parsed, err := ParseSurface("<typegraph-ir>", "func keep(value Lazy<u32>) Lazy<u32> => value\n")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	typed, err := rt.Frontend(t.Context(), parsed)
	if err != nil {
		t.Fatal(err)
	}
	ir, err := rt.BuildIRFromTyped(t.Context(), typed)
	if err != nil {
		t.Fatal(err)
	}
	if ir.Types != typed.Checked().Types || len(ir.Functions) < 2 {
		t.Fatal("IR did not retain the typed program's type context")
	}
	fn := ir.Functions[1]
	if fn.ParamTypeIDs[0] == 0 || fn.ResultTypeID == 0 || fn.ParamTypeIDs[0] != fn.ResultTypeID {
		t.Fatalf("IR type IDs = params=%v result=%d; want one canonical Lazy<u32> ID", fn.ParamTypeIDs, fn.ResultTypeID)
	}
	if got := ir.Types.Format(fn.ResultTypeID, TypeFormatDiagnostic); got != "Lazy<u32>" {
		t.Fatalf("IR type diagnostic = %q, want Lazy<u32>", got)
	}
}

func TestIRRetainsCheckedGenericParameterIdentity(t *testing.T) {
	parsed, err := ParseSurface("<typegraph-ir-generic>", "func keep<T>(value T) T => value\n")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	typed, err := rt.Frontend(t.Context(), parsed)
	if err != nil {
		t.Fatal(err)
	}
	ir, err := rt.BuildIRFromTyped(t.Context(), typed)
	if err != nil {
		t.Fatal(err)
	}
	if len(typed.Checked().Functions) != 1 || len(ir.Functions) < 2 {
		t.Fatal("missing checked or lowered generic function")
	}
	checkedSignature, _ := typed.Checked().Types.Node(typed.Checked().Functions[0].TypeID)
	fn := ir.Functions[1]
	if len(checkedSignature.Args) != 1 || fn.ParamTypeIDs[0] != checkedSignature.Args[0] || fn.ResultTypeID != checkedSignature.Result {
		t.Fatalf("IR generic IDs = params=%v result=%d; checked signature=%#v", fn.ParamTypeIDs, fn.ResultTypeID, checkedSignature)
	}
}

func TestVMTypeAssertionsCarryCanonicalIDs(t *testing.T) {
	parsed, err := ParseSurface("<typegraph-vm>", "let value u32 = u32(7)\n")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	typed, err := rt.Frontend(t.Context(), parsed)
	if err != nil {
		t.Fatal(err)
	}
	compiler := newVMCompiler(parsed.SourceName(), typed.Checked().Types)
	chunk, err := compiler.compileProgram(typed.resolved.expanded.program)
	if err != nil {
		t.Fatal(err)
	}
	var assertion *vmInstr
	for i := range chunk.code {
		if chunk.code[i].op == vmOpTypeAssert {
			assertion = &chunk.code[i]
			break
		}
	}
	if assertion == nil || assertion.typeID == 0 {
		t.Fatalf("VM assertion = %#v; want canonical TypeID", assertion)
	}
	if got := chunk.types.Format(assertion.typeID, TypeFormatDiagnostic); got != "u32" {
		t.Fatalf("VM assertion type = %q, want u32", got)
	}
}

func TestVMFunctionSignaturesCarryCanonicalIDs(t *testing.T) {
	parsed, err := ParseSurface("<typegraph-vm-function>", "func keep(value Lazy<u32>) Lazy<u32> => value\n")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	typed, err := rt.Frontend(t.Context(), parsed)
	if err != nil {
		t.Fatal(err)
	}
	compiler := newVMCompiler(parsed.SourceName(), typed.Checked().Types)
	compiler.functionTypes = checkedFunctionTypeIDs(typed.Checked())
	chunk, err := compiler.compileProgram(typed.resolved.expanded.program)
	if err != nil {
		t.Fatal(err)
	}
	var function *vmInstr
	for i := range chunk.code {
		if chunk.code[i].op == vmOpFunc {
			function = &chunk.code[i]
			break
		}
	}
	checkedSignature := typed.Checked().Functions[0].TypeID
	if function == nil || function.signatureTypeID != checkedSignature || len(function.paramTypeIDs) != 1 || function.paramTypeIDs[0] == 0 || function.returnTypeID == 0 || function.paramTypeIDs[0] != function.returnTypeID {
		t.Fatalf("VM function type IDs = %#v; want canonical Lazy<u32> parameter/result", function)
	}
}

func TestInterpreterFunctionsRetainCheckedCanonicalTypes(t *testing.T) {
	program, err := Parse("<typegraph-interpreter>", "func keep(value Lazy<u32>) Lazy<u32> => value\n")
	if err != nil {
		t.Fatal(err)
	}
	session := NewSession(NewRuntime(), t.Context(), program.SourceName)
	if _, err := session.ExecProgram(program, false, nil); err != nil {
		t.Fatal(err)
	}
	binding, ok := session.scope.lookup("keep")
	if !ok {
		t.Fatal("interpreter did not retain function binding")
	}
	literal, ok := binding.expr.(*literalExpr)
	if !ok || literal.value.fn == nil {
		t.Fatalf("function binding = %#v", binding)
	}
	fn := literal.value.fn
	if fn.types == nil || fn.signatureTypeID == 0 || len(fn.paramTypeIDs) != 1 || fn.paramTypeIDs[0] != fn.returnTypeID || literal.value.FunctionType() != "func(Lazy<u32>) Lazy<u32>" {
		t.Fatalf("interpreter function type metadata = %#v", fn)
	}
}

func TestVMRetainsCheckedGenericParameterIdentity(t *testing.T) {
	parsed, err := ParseSurface("<typegraph-vm-generic>", "func keep<T>(value T) T => value\n")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	typed, err := rt.Frontend(t.Context(), parsed)
	if err != nil {
		t.Fatal(err)
	}
	compiler := newVMCompiler(parsed.SourceName(), typed.Checked().Types)
	compiler.functionTypes = checkedFunctionTypeIDs(typed.Checked())
	chunk, err := compiler.compileProgram(typed.resolved.expanded.program)
	if err != nil {
		t.Fatal(err)
	}
	var function *vmInstr
	for i := range chunk.code {
		if chunk.code[i].op == vmOpFunc {
			function = &chunk.code[i]
			break
		}
	}
	signature, _ := typed.Checked().Types.Node(typed.Checked().Functions[0].TypeID)
	if function == nil || len(signature.Args) != 1 || function.paramTypeIDs[0] != signature.Args[0] || function.returnTypeID != signature.Result {
		t.Fatalf("VM generic IDs = %#v; checked signature = %#v", function, signature)
	}
}
