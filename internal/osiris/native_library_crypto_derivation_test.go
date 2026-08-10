package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeLibraryCryptoDerivationParity covers the deterministic byte
// derivation members whose seed is explicit. They must not consult ambient
// randomness, clocks, or host capabilities, so the same source is meaningful
// across interpreter, VM, generated C, and the native executable.
func TestNativeLibraryCryptoDerivationParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-crypto-derivation",
		Source: `import "std/crypto" as crypto
emit crypto.derive_key("secret", "context", 40)
emit crypto.random_bytes("seed", 17)
emit crypto.token("seed", 17)
emit crypto.uuid_v5("6ba7b810-9dad-11d1-80b4-00c04fd430c8", "www.example.com")
emit try { crypto.derive_key("secret", "context", -1) } catch err { "caught-length" }
emit try { crypto.random_bytes("seed", 4097) } catch err { "caught-bound" }
`,
	})
}

// TestNativeLibraryCryptoDerivationOwnershipAndCapabilityContract checks the
// three exported members individually. Results are copied into owned native
// byte lists or strings and no capability macro is introduced.
func TestNativeLibraryCryptoDerivationOwnershipAndCapabilityContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-crypto-derivation-contract>", `import "std/crypto" as crypto
emit crypto.derive_key("secret", "context", 8)
emit crypto.random_bytes("seed", 8)
emit crypto.token("seed", 8)
emit crypto.uuid_v5("6ba7b810-9dad-11d1-80b4-00c04fd430c8", "www.example.com")
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("deterministic crypto derivation unexpectedly defines a host capability")
	}
	for _, symbol := range []string{
		"ostd_crypto_hmac_sha256_raw",
		"ostd_crypto_expand_bytes",
		"ostd_crypto_derive_key",
		"ostd_crypto_random_bytes",
		"ostd_crypto_token",
		"osha1block",
		"ostd_crypto_uuid_v5",
		"ostd_dispatch",
	} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated crypto artifact does not contain %q", symbol)
		}
	}
	for _, member := range []string{"derive_key", "random_bytes", "token", "uuid_v5"} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/crypto/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/crypto.%s inventory entry = %+v, want implemented native capability", member, entry)
		}
	}
}
