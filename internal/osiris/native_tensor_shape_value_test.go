package osiris

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeTensorShapeValueOperationsParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	program, err := Parse("<native-tensor-shape-value>", `import "std/math" as math
emit math.squeeze(math.tensor([2, 1], [1.0, 2.0]))
emit math.expand_dims(math.tensor([2], [3.0, 4.0]), -1)
emit math.broadcast_to(math.tensor([2], [5.0, 6.0]), [2, 2])
emit math.clip(math.tensor([3], [-1.0, 2.0, 9.0]), 0.0, 5.0)
emit math.where(math.tensor([2], [1.0, 0.0]), math.tensor([2], [7.0, 8.0]), 9.0)
emit math.concat(math.tensor([2], [1.0, 2.0]), math.tensor([1], [3.0]))
emit math.concat(14.0, 15.0)
emit math.stack(math.tensor([2], [10.0, 11.0]), math.tensor([2], [12.0, 13.0]))
`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	wantValues, err := NewRuntime().Run(context.Background(), program)
	if err != nil {
		t.Fatalf("interpreter failed: %v", err)
	}
	var want strings.Builder
	for _, value := range wantValues {
		want.WriteString(value.String())
		want.WriteByte('\n')
	}

	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatalf("compile c failed: %v", err)
	}
	for _, helper := range []string{
		"otensor_squeeze(", "otensor_expand_dims(", "otensor_broadcast_to(",
		"otensor_clip(", "otensor_where(", "otensor_concat(", "otensor_stack(",
	} {
		if !strings.Contains(artifact.Source, helper) {
			t.Fatalf("generated C omitted direct tensor helper %q", helper)
		}
	}
	if strings.Contains(artifact.Source, `ostd_dispatch("std/math"`) {
		t.Fatal("std/math tensor shape/value operations unexpectedly used module dispatch")
	}

	executable := filepath.Join(t.TempDir(), "native-tensor-shape-value")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatalf("compile executable failed: %v\n--- C ---\n%s", err, artifact.Source)
	}
	out, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("running executable failed: %v\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("native output = %q, want %q\n--- C ---\n%s", got, want.String(), artifact.Source)
	}
}

func TestNativeTensorShapeValueOperationsRejectInvalidShapes(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	tests := []struct {
		name    string
		source  string
		message string
	}{
		{
			name:    "expand axis",
			source:  `import "std/math" as math; emit math.expand_dims(math.tensor([2], [1.0, 2.0]), 3)`,
			message: "expand_dims axis out of range",
		},
		{
			name:    "broadcast shape",
			source:  `import "std/math" as math; emit math.broadcast_to(math.tensor([2], [1.0, 2.0]), [3])`,
			message: "broadcast shape mismatch",
		},
		{
			name:    "concat shape",
			source:  `import "std/math" as math; emit math.concat(math.tensor([2, 1], [1.0, 2.0]), math.tensor([1, 2], [3.0, 4.0]))`,
			message: "concat shape mismatch",
		},
		{
			name:    "stack shape",
			source:  `import "std/math" as math; emit math.stack(math.tensor([2], [1.0, 2.0]), math.tensor([3], [3.0, 4.0, 5.0]))`,
			message: "stack shape mismatch",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program, err := Parse("<native-tensor-shape-value-error>", test.source)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			artifact, err := CompileC(context.Background(), NewRuntime(), program)
			if err != nil {
				t.Fatalf("compile c failed: %v", err)
			}
			executable := filepath.Join(t.TempDir(), "native-tensor-shape-value-error")
			if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
				t.Fatalf("compile executable failed: %v", err)
			}
			out, runErr := exec.Command(executable).CombinedOutput()
			if runErr == nil {
				t.Fatalf("invalid shape unexpectedly succeeded: %s", out)
			}
			if !strings.Contains(string(out), "osiris native error: "+test.message) {
				t.Fatalf("native error = %q, want message %q", out, test.message)
			}
		})
	}
}
