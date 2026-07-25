package osiris

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestOptimizeProgramWithReportIsStableAndPreservesExecution(t *testing.T) {
	prog, err := Parse("report.osr", `
let dead = 1 + 2
let value = 2 * (3 + 4)
emit value
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	optimized, first := OptimizeProgramWithReport(prog)
	_, second := OptimizeProgramWithReport(prog)
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal first report: %v", err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("marshal second report: %v", err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("optimization report is not deterministic\nfirst:  %s\nsecond: %s", firstJSON, secondJSON)
	}
	if got, want := first.OptimizedSource, FormatProgram(optimized); got != want {
		t.Fatalf("report optimized source mismatch\ngot:  %q\nwant: %q", got, want)
	}
	if len(first.Passes) != 6 {
		t.Fatalf("pass count = %d, want 6", len(first.Passes))
	}
	if got, want := first.Passes[0].Name, "simplify-and-propagate"; got != want {
		t.Fatalf("first pass = %q, want %q", got, want)
	}
	if got, want := first.Passes[1].Name, "dead-statement-elimination"; got != want {
		t.Fatalf("second pass = %q, want %q", got, want)
	}

	rt := NewRuntime()
	before, beforeErr := rt.Run(context.Background(), prog)
	after, afterErr := rt.Run(context.Background(), optimized)
	if beforeErr != nil || afterErr != nil {
		t.Fatalf("execution errors: before=%v after=%v", beforeErr, afterErr)
	}
	if got, want := valuesOutput(after), valuesOutput(before); got != want {
		t.Fatalf("optimized interpreter output changed\ngot:  %q\nwant: %q", got, want)
	}
	beforeVM, beforeVMErr := rt.RunVM(context.Background(), prog)
	afterVM, afterVMErr := rt.RunVM(context.Background(), optimized)
	if beforeVMErr != nil || afterVMErr != nil {
		t.Fatalf("VM execution errors: before=%v after=%v", beforeVMErr, afterVMErr)
	}
	if got, want := valuesOutput(afterVM), valuesOutput(beforeVM); got != want {
		t.Fatalf("optimized VM output changed\ngot:  %q\nwant: %q", got, want)
	}
}

func TestOptimizeProgramWithReportExplainsUnavailablePasses(t *testing.T) {
	prog, err := Parse("report.osr", "emit 1\n")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	_, report := OptimizeProgramWithReport(prog)
	want := []string{
		"loop-invariant-code-motion",
		"bounds-check-elimination",
		"loop-unrolling",
		"scalarization",
	}
	for i, name := range want {
		pass := report.Passes[i+2]
		if pass.Name != name || pass.Status != "not_implemented" || pass.Reason == "" {
			t.Fatalf("pass %d = %#v, want named unavailable pass with reason", i+2, pass)
		}
		if pass.Before != pass.After {
			t.Fatalf("unavailable pass %q changed source", name)
		}
	}
	if strings.Contains(report.Passes[2].Reason, "applied") {
		t.Fatalf("unavailable pass reason implies application: %q", report.Passes[2].Reason)
	}
	for _, pass := range report.Passes[:2] {
		if pass.Status != "no_change" || pass.Reason == "" {
			t.Fatalf("unchanged pass = %#v, want no_change with reason", pass)
		}
	}
}
