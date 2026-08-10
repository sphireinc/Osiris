package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestNativeLibraryCryptoNoiseParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-crypto-noise",
		Source: `import "std/crypto" as crypto
emit crypto.noise()
emit crypto.noise("osiris", 7u64, true)
emit crypto.noise([1u8, 2u8], (3, "four"))
`,
	})
}

func TestNativeLibraryCryptoNoiseContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-crypto-noise-contract>", `import "std/crypto" as crypto
func noiseValue(value) { return crypto.noise(value) }
emit noiseValue("osiris")
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range []string{"ocrypto_noise", "ocrypto_noise_hash", "orender", "oownedstring"} {
		if !strings.Contains(artifact.Source, symbol+"(") {
			t.Fatalf("generated crypto noise artifact does not contain ownership boundary %q", symbol)
		}
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/crypto.noise lowering unexpectedly defines a host capability")
	}
	entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/crypto/noise")
	if !ok || entry.Native != BackendImplemented {
		t.Fatalf("std/crypto.noise inventory entry = %#v, want implemented", entry)
	}
}
