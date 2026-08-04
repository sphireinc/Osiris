package osiris

import (
	"testing"
)

func task137CryptoCall(t *testing.T, name string, args ...Value) Value {
	t.Helper()
	member, ok := task137CryptoModule()[name]
	if !ok || member.kind != kindFunction || member.fn == nil || member.fn.builtin == nil {
		t.Fatalf("crypto member %q is not a builtin function", name)
	}
	value, err := member.fn.builtin(args)
	if err != nil {
		t.Fatalf("crypto.%s failed: %v", name, err)
	}
	return value
}

func TestTask137CryptoModuleSurface(t *testing.T) {
	members := task137CryptoModule()
	for _, name := range []string{
		"version", "max_input_bytes", "max_output_bytes", "max_key_bytes", "max_token_bytes",
		"sha256", "sha512", "sha512_256", "digest", "digest_bytes", "hmac", "compare_digest",
		"hex_encode", "hex_decode", "base64_encode", "base64_decode", "base64url_encode",
		"base64url_decode", "derive_key", "random_bytes", "token", "uuid_v5", "algorithm_supported",
	} {
		if _, ok := members[name]; !ok {
			t.Fatalf("crypto module is missing %q", name)
		}
	}
	if got := members["version"].str; got != task137CryptoVersion {
		t.Fatalf("crypto version = %q, want %q", got, task137CryptoVersion)
	}
}

func TestTask137CryptoDigestsEncodingsAndConstantTimeComparison(t *testing.T) {
	data := stringValue("abc")
	if got := task137CryptoCall(t, "digest", data, stringValue("sha256")).str; got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("sha256 digest = %q", got)
	}
	raw := task137CryptoCall(t, "digest_bytes", data, stringValue("sha512_256"))
	if got, err := bytesFromValue(raw); err != nil || len(got) != 32 {
		t.Fatalf("sha512/256 digest bytes = %x, %v", got, err)
	}
	hexEncoded := task137CryptoCall(t, "hex_encode", bytesListValue([]byte{0, 1, 254, 255}))
	if hexEncoded.str != "0001feff" {
		t.Fatalf("hex encoding = %q", hexEncoded.str)
	}
	if got, err := bytesFromValue(task137CryptoCall(t, "hex_decode", hexEncoded)); err != nil || string(got) != string([]byte{0, 1, 254, 255}) {
		t.Fatalf("hex round trip = %x, %v", got, err)
	}
	base64Encoded := task137CryptoCall(t, "base64url_encode", bytesListValue([]byte("hello?")))
	if base64Encoded.str != "aGVsbG8_" {
		t.Fatalf("base64url encoding = %q", base64Encoded.str)
	}
	if got, err := bytesFromValue(task137CryptoCall(t, "base64url_decode", base64Encoded)); err != nil || string(got) != "hello?" {
		t.Fatalf("base64url round trip = %q, %v", got, err)
	}
	if !task137CryptoCall(t, "compare_digest", stringValue("same"), stringValue("same")).boo {
		t.Fatal("equal digests were not equal")
	}
	if task137CryptoCall(t, "compare_digest", stringValue("same"), stringValue("different")).boo {
		t.Fatal("different digests were equal")
	}
}

func TestTask137CryptoHMACDerivationTokensAndUUID(t *testing.T) {
	if got := task137CryptoCall(t, "hmac", stringValue("key"), stringValue("The quick brown fox jumps over the lazy dog"), stringValue("sha256")).str; got != "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8" {
		t.Fatalf("HMAC = %q", got)
	}
	first := task137CryptoCall(t, "derive_key", stringValue("secret"), stringValue("context"), numberValue(48))
	second := task137CryptoCall(t, "derive_key", stringValue("secret"), stringValue("context"), numberValue(48))
	firstBytes, _ := bytesFromValue(first)
	secondBytes, _ := bytesFromValue(second)
	if string(firstBytes) != string(secondBytes) || len(firstBytes) != 48 {
		t.Fatalf("derive_key is not deterministic and bounded: len=%d", len(firstBytes))
	}
	randomBytes := task137CryptoCall(t, "random_bytes", stringValue("seed"), numberValue(32))
	if got, err := bytesFromValue(randomBytes); err != nil || len(got) != 32 {
		t.Fatalf("random_bytes = %x, %v", got, err)
	}
	token := task137CryptoCall(t, "token", stringValue("seed"), numberValue(24))
	if len(token.str) != 32 { // 24 raw bytes, unpadded base64url.
		t.Fatalf("token length = %d, want 32", len(token.str))
	}
	const dnsNamespace = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	if got := task137CryptoCall(t, "uuid_v5", stringValue(dnsNamespace), stringValue("www.widgets.com")).str; got != "21f7f8de-8051-5b89-8680-0195ef798b6a" {
		t.Fatalf("UUID v5 = %q", got)
	}
	if !task137CryptoCall(t, "algorithm_supported", stringValue("SHA256")).boo || task137CryptoCall(t, "algorithm_supported", stringValue("md5")).boo {
		t.Fatal("algorithm support metadata is incorrect")
	}
}

func TestTask137CryptoLimitsAndErrors(t *testing.T) {
	tooLarge := stringValue(string(make([]byte, task137CryptoMaxInputBytes+1)))
	member := task137CryptoModule()["digest"]
	if _, err := member.fn.builtin([]Value{tooLarge, stringValue("sha256")}); err == nil {
		t.Fatal("digest accepted input beyond the bounded limit")
	}
	if _, err := task137CryptoModule()["derive_key"].fn.builtin([]Value{stringValue("key"), stringValue("context"), numberValue(task137CryptoMaxTokenBytes + 1)}); err == nil {
		t.Fatal("derive_key accepted output beyond the bounded limit")
	}
	if _, err := task137CryptoModule()["uuid_v5"].fn.builtin([]Value{stringValue("bad"), stringValue("name")}); err == nil {
		t.Fatal("uuid_v5 accepted a malformed namespace")
	}
}
