package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestNativeLibraryCryptoBase64Parity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-crypto-base64",
		Source: `import "std/crypto" as crypto
emit crypto.base64_encode("hello?")
emit crypto.base64_decode("aGVsbG8/")
emit crypto.base64url_encode("hello?")
emit crypto.base64url_decode("aGVsbG8_")
`,
	})
}

func TestNativeLibraryCryptoBase64OwnershipAndErrorContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-crypto-base64-contract>", `import "std/crypto" as crypto
emit crypto.base64_encode("hello")
emit crypto.base64_decode("aGVsbG8=")
emit crypto.base64url_encode("hello?")
emit crypto.base64url_decode("aGVsbG8_")
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, helper := range []string{"ostd_crypto_base64_value", "ostd_crypto_base64_encode", "ostd_crypto_base64_decode"} {
		if !strings.Contains(artifact.Source, helper+"(") {
			t.Fatalf("generated artifact omitted crypto helper %q", helper)
		}
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure base64 codec unexpectedly requires a host capability")
	}
	for _, member := range []string{"base64_encode", "base64_decode", "base64url_encode", "base64url_decode"} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/crypto/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("inventory status for std/crypto.%s = %#v, want implemented", member, entry)
		}
	}
}
