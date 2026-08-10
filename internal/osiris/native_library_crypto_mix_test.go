package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestNativeLibraryCryptoMixParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-crypto-mix",
		Source: `import "std/crypto" as crypto
emit crypto.mix()
emit crypto.mix("osiris", 7u64, true)
emit crypto.mix([1u8, 2u8], (3, "four"))
`,
	})
}

func TestNativeLibraryCryptoMixContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-crypto-mix-contract>", `import "std/crypto" as crypto
func mixValue(value) { return crypto.mix(value) }
emit mixValue("osiris")
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range []string{"ocrypto_mix", "orender", "oownedstring"} {
		if !strings.Contains(artifact.Source, symbol+"(") {
			t.Fatalf("generated crypto mix artifact does not contain ownership boundary %q", symbol)
		}
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/crypto.mix lowering unexpectedly defines a host capability")
	}
	entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/crypto/mix")
	if !ok || entry.Native != BackendImplemented {
		t.Fatalf("std/crypto.mix inventory entry = %#v, want implemented", entry)
	}
}
