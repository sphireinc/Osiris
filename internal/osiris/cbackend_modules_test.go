package osiris

import "testing"

func TestNativeModuleIdentitiesMergesAndLoops(t *testing.T) {
	mathImport := IRInstr{Dest: "math", Op: "import", Text: `"std/math"`}
	textImport := IRInstr{Dest: "text", Op: "import", Text: `"std/text"`}
	fn := &IRFunction{Name: "main", Blocks: []*IRBlock{
		{Name: "entry", Instrs: []IRInstr{mathImport, textImport}, Term: &irBranch{Cond: "condition", TrueTarget: "same", TrueArgs: []string{"math"}, FalseTarget: "same", FalseArgs: []string{"math"}}},
		{Name: "same", Params: []string{"merged"}, Term: &irJump{Target: "loop", Args: []string{"merged"}}},
		{Name: "loop", Params: []string{"carried"}, Term: &irBranch{Cond: "condition", TrueTarget: "loop", TrueArgs: []string{"carried"}, FalseTarget: "conflict", FalseArgs: []string{"carried"}}},
		{Name: "conflict", Params: []string{"selected"}, Term: &irBranch{Cond: "condition", TrueTarget: "end", TrueArgs: []string{"selected"}, FalseTarget: "end", FalseArgs: []string{"text"}}},
		{Name: "end", Params: []string{"result"}, Term: &irReturn{}},
	}}
	identities := nativeModuleIdentities(fn, nil)
	for _, value := range []string{"merged", "carried", "selected"} {
		if got := identities[value]; got != "std/math" {
			t.Fatalf("identity[%q] = %q, want std/math", value, got)
		}
	}
	if _, ok := identities["result"]; ok {
		t.Fatalf("conflicting branch retained module identity %q", identities["result"])
	}
}

func TestNativeModuleIdentitiesCapturedValues(t *testing.T) {
	child := &IRFunction{Name: "child", CaptureValues: []string{"outer.math"}, Blocks: []*IRBlock{{
		Name:   "entry",
		Instrs: []IRInstr{{Dest: "sqrt", Op: "member", Args: []string{"outer.math"}, Text: "sqrt"}, {Dest: "result", Op: "call", Text: "sqrt"}},
		Term:   &irReturn{Value: "result", HasValue: true},
	}}}
	main := &IRFunction{Name: "main", Blocks: []*IRBlock{{
		Name:   "entry",
		Instrs: []IRInstr{{Dest: "outer.math", Op: "import", Text: `"std/math"`}, {Dest: "closure", Op: "makefunc", Text: "child", Args: []string{"outer.math"}}, {Dest: "again", Op: "makefunc", Text: "child", Args: []string{"outer.math"}}},
		Term:   &irReturn{},
	}}}
	identities := nativeModuleIdentitiesByFunction(&IRProgram{Functions: []*IRFunction{main, child}})
	if got := identities["child"]["outer.math"]; got != "std/math" {
		t.Fatalf("captured identity = %q, want std/math", got)
	}
}

func TestNativeModuleIdentitiesConflictingCapturesBecomeUnknown(t *testing.T) {
	child := &IRFunction{Name: "child", CaptureValues: []string{"captured"}, Blocks: []*IRBlock{{Name: "entry", Term: &irReturn{}}}}
	main := &IRFunction{Name: "main", Blocks: []*IRBlock{{
		Name:   "entry",
		Instrs: []IRInstr{{Dest: "math", Op: "import", Text: `"std/math"`}, {Dest: "text", Op: "import", Text: `"std/text"`}, {Dest: "one", Op: "makefunc", Text: "child", Args: []string{"math"}}, {Dest: "two", Op: "makefunc", Text: "child", Args: []string{"text"}}},
		Term:   &irReturn{},
	}}}
	identities := nativeModuleIdentitiesByFunction(&IRProgram{Functions: []*IRFunction{main, child}})
	if _, ok := identities["child"]["captured"]; ok {
		t.Fatalf("conflicting capture retained identity %q", identities["child"]["captured"])
	}
}
