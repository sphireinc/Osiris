package osiris

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// TestNativeCoreCodecResultOptionErrorParity exercises the OSR1 Result,
// Option, and structured Error records through the interpreter, VM, and
// generated-C executable. The fixed byte comparisons make the tag and field
// order contract executable rather than relying only on rendered values.
func TestNativeCoreCodecResultOptionErrorParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available for native codec parity")
	}

	errorRecord := errorValueFromRecord(newOsirisError(ErrorKindInvalidArgument, "bad"))
	wantError, err := encodeValueBytes(errorRecord)
	if err != nil {
		t.Fatal(err)
	}
	wantResultOK, err := encodeValueBytes(resultOkValue(uintValue(41, 64)))
	if err != nil {
		t.Fatal(err)
	}
	wantResultError, err := encodeValueBytes(resultErrValue(errorRecord.errorRecord()))
	if err != nil {
		t.Fatal(err)
	}
	wantOptionSome, err := encodeValueBytes(optionSomeValue(uintValue(41, 64)))
	if err != nil {
		t.Fatal(err)
	}
	wantOptionNone, err := encodeValueBytes(optionNoneValue())
	if err != nil {
		t.Fatal(err)
	}

	source := fmt.Sprintf(`let failure = error("invalid_argument", "bad")
let successful = ok(41u64)
let failed = err(failure)
let present = some(41u64)
let absent = none()
emit bytes(encode(failure)) == %s
emit bytes(encode(successful)) == %s
emit bytes(encode(failed)) == %s
emit bytes(encode(present)) == %s
emit bytes(encode(absent)) == %s
emit decode(encode(successful))
emit decode(encode(failed))
emit decode(encode(present))
emit decode(encode(absent))
emit decode(encode(failure)).kind
emit decode(encode(failure)).message
`, nativeBytesExpr(wantError), nativeBytesExpr(wantResultOK), nativeBytesExpr(wantResultError), nativeBytesExpr(wantOptionSome), nativeBytesExpr(wantOptionNone))
	runDifferentialFixture(t, differentialFixture{
		Name:   "nativeCore-native-result-option-error-codec",
		Source: source,
	})

	program, err := Parse("<nativeCore-native-result-option-error-codec-contract>", source)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"static int ocodecerror(",
		"static int ocodecerrorDecode(",
		"static int ocodecmapdecode(",
		"static int ocodecstackdecode(",
		"case O_ERROR:",
		"case O_RESULT:",
		"case O_OPTION:",
		"case 0x0a:",
		"case 0x16:",
		"case 0x17:",
		"OCODEC_LIMIT",
		"OCODEC_DEPTH",
		"otrack(",
	} {
		if !strings.Contains(artifact.Source, marker) {
			t.Fatalf("generated codec artifact does not contain %q", marker)
		}
	}
	generatedMain := artifact.Source
	if start := strings.Index(generatedMain, "// Osiris native C lowering from"); start >= 0 {
		generatedMain = generatedMain[start:]
	}
	if strings.Contains(generatedMain, "ostd_dispatch(") || strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatalf("pure Result/Option/Error codec unexpectedly introduced a host fallback or capability (main-dispatch=%t capability=%t)", strings.Contains(generatedMain, "ostd_dispatch("), strings.Contains(artifact.Source, "#define OSIRIS_CAP_"))
	}
}

// TestNativeCoreCodecResultOptionErrorRejectsMalformedRecords ensures that
// malformed presence flags, unsupported error versions, and truncated
// records fail inside the generated decoder without an
// allocation proportional to an attacker-controlled count.
func TestNativeCoreCodecResultOptionErrorRejectsMalformedRecords(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available for native codec bounds tests")
	}
	source := `emit try { decode(bytes([79u8, 83u8, 82u8, 49u8, 22u8, 2u8])) } catch err { "presence" }
emit try { decode(bytes([79u8, 83u8, 82u8, 49u8, 23u8, 2u8])) } catch err { "presence" }
emit try { decode(bytes([79u8, 83u8, 82u8, 49u8, 10u8, 2u8])) } catch err { "version" }
emit try { decode(bytes([79u8, 83u8, 82u8, 49u8, 10u8, 1u8, 0u8, 0u8, 0u8, 0u8, 0u8, 0u8, 0u8, 5u8, 101u8, 114u8, 114u8, 111u8, 114u8])) } catch err { "truncated" }
`
	runDifferentialFixture(t, differentialFixture{
		Name:   "nativeCore-native-result-option-error-codec-malformed",
		Source: source,
	})
}
