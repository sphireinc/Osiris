package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeLibraryCryptoCodecParity covers the host-independent codec and
// digest-comparison members individually through the interpreter, VM,
// generated C, and native executable. The caught failures keep malformed
// encodings and wrong value kinds part of the cross-backend contract too.
func TestNativeLibraryCryptoCodecParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-crypto-codecs",
		Source: `import "std/crypto" as crypto
emit crypto.hex_encode([0u8, 1u8, 254u8, 255u8])
emit crypto.hex_decode("0001feff")
emit crypto.compare_digest("same", "same")
emit crypto.compare_digest("same", "different")
emit crypto.compare_digest([0u8, 1u8], [0u8, 1u8])
emit try { crypto.hex_decode("0") } catch err { "odd-length" }
emit try { crypto.hex_decode("gg") } catch err { "invalid-digit" }
emit try { crypto.hex_encode(["not-a-byte"]) } catch err { "invalid-input" }
emit try { crypto.compare_digest([7u64], "7") } catch err { "invalid-input" }
`,
	})
}

// TestNativeLibraryCryptoCodecOwnershipErrorAndCapabilityContract checks the
// generated ABI rather than only its output. Codec results are newly owned,
// temporary list-to-byte buffers are released on every path, and no OS
// capability is introduced by these pure operations.
func TestNativeLibraryCryptoCodecOwnershipErrorAndCapabilityContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-crypto-codec-contract>", `import "std/crypto" as crypto
emit crypto.hex_encode([0u8, 1u8])
emit crypto.hex_decode("0001")
emit crypto.compare_digest("left", "right")
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range []string{
		"ostd_crypto_hex", "ostd_crypto_compare_digest", "ostd_bytes_list",
		"oownedstring", "ostd_dispatch",
	} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated crypto codec artifact does not contain %q", symbol)
		}
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/crypto codec lowering unexpectedly defines a host capability")
	}
	for _, message := range []string{
		"hex_decode expects a string",
		"hex_decode expects an even-length string",
		"hex_decode exceeds the 1048576-byte output limit",
		"invalid hexadecimal encoding",
		"hex_encode expects byte-compatible data",
		"hex_encode exceeds the 1048576-byte input limit",
		"compare_digest expects byte-compatible values",
		"compare_digest input exceeds the 1048576-byte limit",
	} {
		if !strings.Contains(artifact.Source, message) {
			t.Fatalf("generated crypto codec artifact does not preserve error contract %q", message)
		}
	}
	if !strings.Contains(artifact.Source, "diff=(unsigned char)(an==bn?0:1)") {
		t.Fatal("generated compare_digest does not preserve constant-time length handling")
	}
	for _, member := range []string{"hex_encode", "hex_decode", "compare_digest"} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/crypto/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/crypto.%s inventory entry = %+v, want implemented native capability", member, entry)
		}
	}
}
