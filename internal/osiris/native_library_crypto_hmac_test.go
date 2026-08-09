package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestNativeLibraryCryptoHMACParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-crypto-hmac",
		Source: `import "std/crypto" as crypto
emit crypto.hmac("key", "The quick brown fox jumps over the lazy dog", crypto.sha256)
emit crypto.hmac([0u8, 1u8, 254u8, 255u8], "payload", crypto.sha512)
emit crypto.hmac("key", "payload", crypto.sha512_256)
emit try { crypto.hmac("key", "payload", "md5") } catch err { "unsupported" }
emit crypto.hmac([256u64], "payload", crypto.sha256)
emit try { crypto.hmac(["not-a-byte"], "payload", crypto.sha256) } catch err { "invalid-bytes" }
`,
	})
}

func TestNativeLibraryCryptoHMACOwnershipErrorAndCapabilityContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-crypto-hmac-contract>", `import "std/crypto" as crypto
emit crypto.hmac([0u8, 1u8], "payload", crypto.sha256)
emit crypto.sha512
emit crypto.sha512_256
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range []string{"ostd_crypto_hmac", "OSHA256", "OSHA512", "ostd_bytes", "oownedstring", "ostd_dispatch"} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated HMAC artifact does not contain %q", symbol)
		}
	}
	for _, algorithm := range []string{`ostring("sha256")`, `ostring("sha512")`, `ostring("sha512_256")`} {
		if !strings.Contains(artifact.Source, algorithm) {
			t.Fatalf("generated HMAC artifact does not lower algorithm constant %q", algorithm)
		}
	}
	for _, message := range []string{
		"crypto.hmac expects key, data, and algorithm",
		"crypto.hmac expects byte-compatible key and data",
		"crypto.hmac key exceeds the 65536-byte limit",
		"crypto.hmac data exceeds the 1048576-byte limit",
		"unsupported crypto algorithm",
	} {
		if !strings.Contains(artifact.Source, message) {
			t.Fatalf("generated HMAC artifact does not preserve error contract %q", message)
		}
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure HMAC lowering unexpectedly defines a host capability")
	}
	entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/crypto/hmac")
	if !ok || entry.Native != BackendImplemented {
		t.Fatalf("std/crypto.hmac inventory entry = %+v, want implemented native capability", entry)
	}
}
