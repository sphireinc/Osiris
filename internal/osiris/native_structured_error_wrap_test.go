package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestNativeStructuredErrorWrapParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "native-structured-error-wrap",
		Source: `let base = error("invalid_argument", "bad input")
	let wrapped = wrap(base, "unavailable", "while loading")
emit wrapped.kind
emit wrapped.message
emit wrapped.cause.kind
emit wrapped.cause.message
emit cause(wrapped).kind
	emit cause(base)
	emit try { wrap(1u64, "unavailable", "invalid") } catch err { err.kind }
	emit try { wrap(base, 1u64, "invalid") } catch err { err.kind }
`,
	})
}

func TestNativeStructuredErrorWrapLoweringOwnsCauseWithoutFallback(t *testing.T) {
	program, err := Parse("native-structured-error-wrap-contract.osr", `let base = error("invalid_argument", "bad")
	emit wrap(base, "unavailable", "outer")
	emit cause(base)
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"static OValue owrap(", "static OValue ocause(", "= owrap(", "= ocause(", "result.as.error.cause"} {
		if !strings.Contains(artifact.Source, marker) {
			t.Fatalf("generated artifact omitted wrap marker %q", marker)
		}
	}
	if strings.Contains(artifact.Source, `ostd_dispatch("", "wrap"`) || strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure structured-error wrap crossed a dispatcher or capability boundary")
	}
	if nativeBuiltinStatus("wrap") != BackendImplemented {
		t.Fatalf("wrap inventory status = %q, want implemented", nativeBuiltinStatus("wrap"))
	}
}
