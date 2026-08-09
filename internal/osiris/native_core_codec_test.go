package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeCoreTaggedCodecParity covers the tagged Result, Option, and
// structured Error records that are part of the OSR1 value codec. The native
// backend must preserve the wrapper kind and payload instead of silently
// dropping it during encode/decode.
func TestNativeCoreTaggedCodecParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeCore-tagged-codec",
		Source: `emit decode(encode(ok(7u64)))
emit decode(encode(some("value")))
emit decode(encode(none()))
emit decode(encode(err(error(InvalidArgument, "bad"))))
`,
	})
}

// TestNativeCoreTaggedCodecContract checks the exact wire tags and the
// ownership boundary independently from value output. Result and Option
// payloads are boxed by the existing tracked ABI; decoded error strings are
// copied before the input buffer is released.
func TestNativeCoreTaggedCodecContract(t *testing.T) {
	program, err := Parse("<nativeCore-tagged-codec-contract>", `emit decode(encode(ok(7u64)))
emit decode(encode(some("value")))
emit decode(encode(none()))
emit decode(encode(err(error(InvalidArgument, "bad"))))
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"ocodecstring", "case O_ERROR:", "case O_RESULT:", "case O_OPTION:",
		"case 0x0a:", "case 0x16:", "case 0x17:", "ocodecdecodeString",
	} {
		if !strings.Contains(artifact.Source, marker) {
			t.Fatalf("generated tagged codec artifact does not contain %q", marker)
		}
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("tagged value codec unexpectedly defines a host capability")
	}
	for _, member := range []string{"encode", "decode"} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "builtin/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("builtin %s inventory entry = %+v, want implemented native capability", member, entry)
		}
	}
}
