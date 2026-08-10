package osiris

import (
	"context"
	"strings"
	"testing"
)

// TestNativeLibraryCryptoCompleteParity exercises every exported std/crypto
// member, including constants and deterministic seeded helpers. The fixture
// deliberately uses only explicit inputs so native execution has no ambient
// clock, random source, or host capability dependency.
func TestNativeLibraryCryptoCompleteParity(t *testing.T) {
	runDifferentialFixture(t, differentialFixture{
		Name: "nativeLibrary-native-crypto-complete",
		Source: `import "std/crypto" as crypto
emit crypto.version
emit crypto.max_input_bytes
emit crypto.max_output_bytes
emit crypto.max_key_bytes
emit crypto.max_token_bytes
emit crypto.sha256
emit crypto.sha512
emit crypto.sha512_256
emit crypto.random("seed", 0)
emit crypto.hash("osiris")
emit crypto.mix("osiris")
emit crypto.noise("osiris")
emit crypto.digest("osiris", crypto.sha256)
emit crypto.digest_bytes("osiris", crypto.sha256)
emit crypto.hmac("key", "data", crypto.sha256)
emit crypto.hex_encode("osiris")
emit crypto.hex_decode("6f7369726973")
emit crypto.compare_digest("same", "same")
emit crypto.base64_encode("osiris")
emit crypto.base64_decode("b3Npcmlz")
emit crypto.base64url_encode("osiris")
emit crypto.base64url_decode("b3Npcmlz")
emit crypto.derive_key("secret", "context", 8)
emit crypto.random_bytes("seed", 8)
emit crypto.token("seed", 8)
emit crypto.uuid_v5("6ba7b810-9dad-11d1-80b4-00c04fd430c8", "www.example.com")
emit crypto.algorithm_supported(crypto.sha256)
emit try { crypto.hex_decode("bad") } catch err { "caught-hex" }
`,
	})
}

// TestNativeLibraryCryptoCompleteOwnershipAndCapabilityContract verifies the
// complete allow-list and the direct runtime symbols without depending on one
// particular output rendering for every byte-oriented result.
func TestNativeLibraryCryptoCompleteOwnershipAndCapabilityContract(t *testing.T) {
	program, err := Parse("<nativeLibrary-native-crypto-complete-contract>", `import "std/crypto" as crypto
emit crypto.version
emit crypto.max_input_bytes
emit crypto.max_output_bytes
emit crypto.max_key_bytes
emit crypto.max_token_bytes
emit crypto.sha256
emit crypto.sha512
emit crypto.sha512_256
emit crypto.random("seed", 0)
emit crypto.hash("osiris")
emit crypto.mix("osiris")
emit crypto.noise("osiris")
emit crypto.digest("osiris", "sha256")
emit crypto.digest_bytes("osiris", "sha256")
emit crypto.hmac("key", "data", "sha256")
emit crypto.hex_encode("osiris")
emit crypto.hex_decode("6f7369726973")
emit crypto.compare_digest("same", "same")
emit crypto.base64_encode("osiris")
emit crypto.base64_decode("b3Npcmlz")
emit crypto.base64url_encode("osiris")
emit crypto.base64url_decode("b3Npcmlz")
emit crypto.derive_key("secret", "context", 8)
emit crypto.random_bytes("seed", 8)
emit crypto.token("seed", 8)
emit crypto.uuid_v5("6ba7b810-9dad-11d1-80b4-00c04fd430c8", "www.example.com")
emit crypto.algorithm_supported("sha256")
`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(artifact.Source, "#define OSIRIS_CAP_") {
		t.Fatal("pure std/crypto lowering unexpectedly defines a host capability")
	}
	for _, symbol := range []string{
		"ostd_crypto_digest", "ostd_crypto_hmac", "ostd_crypto_hex",
		"ostd_crypto_base64_encode", "ostd_crypto_derive_key",
		"ostd_crypto_random_bytes", "ostd_crypto_token",
		"ostd_crypto_uuid_v5", "ocrypto_random", "ocrypto_hash",
		"ocrypto_mix", "ocrypto_noise", "ostd_dispatch",
	} {
		if !strings.Contains(artifact.Source, symbol) {
			t.Fatalf("generated crypto artifact does not contain %q", symbol)
		}
	}
	for _, member := range []string{
		"version", "max_input_bytes", "max_output_bytes", "max_key_bytes",
		"max_token_bytes", "sha256", "sha512", "sha512_256", "random", "hash",
		"mix", "noise", "digest", "digest_bytes", "hmac", "hex_encode", "hex_decode",
		"compare_digest", "base64_encode", "base64_decode", "base64url_encode",
		"base64url_decode", "derive_key", "random_bytes", "token", "uuid_v5",
		"algorithm_supported",
	} {
		entry, ok := findBackendCapability(BackendCapabilityInventory(), "module/std/crypto/"+member)
		if !ok || entry.Native != BackendImplemented {
			t.Fatalf("std/crypto.%s inventory entry = %+v, want implemented native capability", member, entry)
		}
	}
}
