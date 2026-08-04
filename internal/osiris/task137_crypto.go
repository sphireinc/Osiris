package osiris

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"
)

const (
	task137CryptoVersion        = "1.1.0"
	task137CryptoMaxInputBytes  = 1 << 20
	task137CryptoMaxOutputBytes = 1 << 20
	task137CryptoMaxKeyBytes    = 1 << 16
	task137CryptoMaxTokenBytes  = 4096
)

// task137CryptoModule is the integration surface for the expanded std/crypto
// module. Its operations are pure with respect to the Osiris runtime: no
// ambient random source, process state, clock, or mutable package state is
// consulted. The module integration can merge this map with the existing
// std/crypto members without changing any existing symbol.
func task137CryptoModule() map[string]Value {
	return map[string]Value{
		"version":             stringValue(task137CryptoVersion),
		"max_input_bytes":     uintValue(task137CryptoMaxInputBytes, 64),
		"max_output_bytes":    uintValue(task137CryptoMaxOutputBytes, 64),
		"max_key_bytes":       uintValue(task137CryptoMaxKeyBytes, 64),
		"max_token_bytes":     uintValue(task137CryptoMaxTokenBytes, 64),
		"sha256":              stringValue("sha256"),
		"sha512":              stringValue("sha512"),
		"sha512_256":          stringValue("sha512_256"),
		"digest":              functionValueOf("std/crypto.digest", task137CryptoDigest),
		"digest_bytes":        functionValueOf("std/crypto.digest_bytes", task137CryptoDigestBytes),
		"hmac":                functionValueOf("std/crypto.hmac", task137CryptoHMAC),
		"compare_digest":      functionValueOf("std/crypto.compare_digest", task137CryptoCompareDigest),
		"hex_encode":          functionValueOf("std/crypto.hex_encode", task137CryptoHexEncode),
		"hex_decode":          functionValueOf("std/crypto.hex_decode", task137CryptoHexDecode),
		"base64_encode":       functionValueOf("std/crypto.base64_encode", task137CryptoBase64Encode),
		"base64_decode":       functionValueOf("std/crypto.base64_decode", task137CryptoBase64Decode),
		"base64url_encode":    functionValueOf("std/crypto.base64url_encode", task137CryptoBase64URLEncode),
		"base64url_decode":    functionValueOf("std/crypto.base64url_decode", task137CryptoBase64URLDecode),
		"derive_key":          functionValueOf("std/crypto.derive_key", task137CryptoDeriveKey),
		"random_bytes":        functionValueOf("std/crypto.random_bytes", task137CryptoRandomBytes),
		"token":               functionValueOf("std/crypto.token", task137CryptoToken),
		"uuid_v5":             functionValueOf("std/crypto.uuid_v5", task137CryptoUUIDV5),
		"algorithm_supported": functionValueOf("std/crypto.algorithm_supported", task137CryptoAlgorithmSupported),
	}
}

func task137CryptoBytes(value Value, name string, max int) ([]byte, error) {
	data, err := bytesFromValue(value)
	if err != nil {
		return nil, fmt.Errorf("%s: expected byte-compatible value: %w", name, err)
	}
	if len(data) > max {
		return nil, fmt.Errorf("%s exceeds the %d-byte limit", name, max)
	}
	return data, nil
}

func task137CryptoAlgorithm(value Value) (string, error) {
	name, err := value.AsString()
	if err != nil {
		return "", fmt.Errorf("crypto algorithm must be a string")
	}
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "sha256", "sha512", "sha512_256":
		return name, nil
	default:
		return "", fmt.Errorf("unsupported crypto algorithm %q", name)
	}
}

func task137CryptoHash(name string) (hash.Hash, error) {
	switch name {
	case "sha256":
		return sha256.New(), nil
	case "sha512":
		return sha512.New(), nil
	case "sha512_256":
		return sha512.New512_256(), nil
	default:
		return nil, fmt.Errorf("unsupported crypto algorithm %q", name)
	}
}

func task137CryptoDigest(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("crypto.digest expects data and algorithm")
	}
	data, err := task137CryptoBytes(args[0], "crypto.digest data", task137CryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	algorithm, err := task137CryptoAlgorithm(args[1])
	if err != nil {
		return nullValue(), err
	}
	hasher, err := task137CryptoHash(algorithm)
	if err != nil {
		return nullValue(), err
	}
	_, _ = hasher.Write(data)
	return stringValue(hex.EncodeToString(hasher.Sum(nil))), nil
}

func task137CryptoDigestBytes(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("crypto.digest_bytes expects data and algorithm")
	}
	data, err := task137CryptoBytes(args[0], "crypto.digest_bytes data", task137CryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	algorithm, err := task137CryptoAlgorithm(args[1])
	if err != nil {
		return nullValue(), err
	}
	hasher, err := task137CryptoHash(algorithm)
	if err != nil {
		return nullValue(), err
	}
	_, _ = hasher.Write(data)
	return bytesListValue(hasher.Sum(nil)), nil
}

func task137CryptoHMAC(args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("crypto.hmac expects key, data, and algorithm")
	}
	key, err := task137CryptoBytes(args[0], "crypto.hmac key", task137CryptoMaxKeyBytes)
	if err != nil {
		return nullValue(), err
	}
	data, err := task137CryptoBytes(args[1], "crypto.hmac data", task137CryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	algorithm, err := task137CryptoAlgorithm(args[2])
	if err != nil {
		return nullValue(), err
	}
	constructor := func() hash.Hash {
		hasher, _ := task137CryptoHash(algorithm)
		return hasher
	}
	mac := hmac.New(constructor, key)
	_, _ = mac.Write(data)
	return stringValue(hex.EncodeToString(mac.Sum(nil))), nil
}

func task137CryptoCompareDigest(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("crypto.compare_digest expects two byte-compatible values")
	}
	left, err := task137CryptoBytes(args[0], "crypto.compare_digest left", task137CryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	right, err := task137CryptoBytes(args[1], "crypto.compare_digest right", task137CryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	return boolValue(subtle.ConstantTimeCompare(left, right) == 1), nil
}

func task137CryptoHexEncode(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("crypto.hex_encode expects one byte-compatible value")
	}
	data, err := task137CryptoBytes(args[0], "crypto.hex_encode data", task137CryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	return stringValue(hex.EncodeToString(data)), nil
}

func task137CryptoHexDecode(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("crypto.hex_decode expects one string")
	}
	encoded, err := args[0].AsString()
	if err != nil {
		return nullValue(), fmt.Errorf("crypto.hex_decode expects a string")
	}
	if len(encoded) > task137CryptoMaxOutputBytes*2 {
		return nullValue(), fmt.Errorf("crypto.hex_decode exceeds the output limit")
	}
	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		return nullValue(), fmt.Errorf("crypto.hex_decode: %w", err)
	}
	return bytesListValue(decoded), nil
}

func task137CryptoBase64Encode(args []Value) (Value, error) {
	return task137CryptoBase64EncodeWith(args, base64.StdEncoding, "base64_encode")
}

func task137CryptoBase64URLEncode(args []Value) (Value, error) {
	return task137CryptoBase64EncodeWith(args, base64.RawURLEncoding, "base64url_encode")
}

func task137CryptoBase64EncodeWith(args []Value, encoding *base64.Encoding, operation string) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("crypto.%s expects one byte-compatible value", operation)
	}
	data, err := task137CryptoBytes(args[0], "crypto."+operation+" data", task137CryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	return stringValue(encoding.EncodeToString(data)), nil
}

func task137CryptoBase64Decode(args []Value) (Value, error) {
	return task137CryptoBase64DecodeWith(args, base64.StdEncoding, "base64_decode")
}

func task137CryptoBase64URLDecode(args []Value) (Value, error) {
	return task137CryptoBase64DecodeWith(args, base64.RawURLEncoding, "base64url_decode")
}

func task137CryptoBase64DecodeWith(args []Value, encoding *base64.Encoding, operation string) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("crypto.%s expects one string", operation)
	}
	encoded, err := args[0].AsString()
	if err != nil {
		return nullValue(), fmt.Errorf("crypto.%s expects a string", operation)
	}
	if len(encoded) > task137CryptoMaxInputBytes*2 {
		return nullValue(), fmt.Errorf("crypto.%s exceeds the input limit", operation)
	}
	decoded, err := encoding.DecodeString(encoded)
	if err != nil {
		return nullValue(), fmt.Errorf("crypto.%s: %w", operation, err)
	}
	if len(decoded) > task137CryptoMaxOutputBytes {
		return nullValue(), fmt.Errorf("crypto.%s exceeds the output limit", operation)
	}
	return bytesListValue(decoded), nil
}

func task137CryptoLength(value Value, name string, max int) (int, error) {
	number, err := value.AsNumber()
	if err != nil || number < 0 || number != float64(int(number)) || number > float64(max) {
		return 0, fmt.Errorf("%s must be an integer from 0 through %d", name, max)
	}
	return int(number), nil
}

func task137CryptoDeriveKey(args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("crypto.derive_key expects secret, context, and length")
	}
	secret, err := task137CryptoBytes(args[0], "crypto.derive_key secret", task137CryptoMaxKeyBytes)
	if err != nil {
		return nullValue(), err
	}
	context, err := task137CryptoBytes(args[1], "crypto.derive_key context", task137CryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	length, err := task137CryptoLength(args[2], "crypto.derive_key length", task137CryptoMaxTokenBytes)
	if err != nil {
		return nullValue(), err
	}
	return bytesListValue(task137CryptoExpand(secret, context, length)), nil
}

func task137CryptoRandomBytes(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("crypto.random_bytes expects seed and length")
	}
	seed, err := task137CryptoBytes(args[0], "crypto.random_bytes seed", task137CryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	length, err := task137CryptoLength(args[1], "crypto.random_bytes length", task137CryptoMaxTokenBytes)
	if err != nil {
		return nullValue(), err
	}
	return bytesListValue(task137CryptoExpand(seed, []byte("random_bytes"), length)), nil
}

func task137CryptoToken(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("crypto.token expects seed and length")
	}
	seed, err := task137CryptoBytes(args[0], "crypto.token seed", task137CryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	length, err := task137CryptoLength(args[1], "crypto.token length", task137CryptoMaxTokenBytes)
	if err != nil {
		return nullValue(), err
	}
	raw := task137CryptoExpand(seed, []byte("token"), length)
	return stringValue(base64.RawURLEncoding.EncodeToString(raw)), nil
}

func task137CryptoExpand(key, context []byte, length int) []byte {
	output := make([]byte, 0, length)
	for counter := uint32(1); len(output) < length; counter++ {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write(context)
		var counterBytes [4]byte
		binary.BigEndian.PutUint32(counterBytes[:], counter)
		_, _ = mac.Write(counterBytes[:])
		output = append(output, mac.Sum(nil)...)
	}
	return output[:length]
}

func task137CryptoUUIDV5(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("crypto.uuid_v5 expects namespace and name")
	}
	namespace, err := args[0].AsString()
	if err != nil {
		return nullValue(), fmt.Errorf("crypto.uuid_v5 namespace must be a UUID string")
	}
	name, err := args[1].AsString()
	if err != nil || len(name) > task137CryptoMaxInputBytes {
		return nullValue(), fmt.Errorf("crypto.uuid_v5 name must be a bounded string")
	}
	namespaceBytes, err := task137CryptoParseUUID(namespace)
	if err != nil {
		return nullValue(), err
	}
	hasher := sha1.New()
	_, _ = hasher.Write(namespaceBytes)
	_, _ = hasher.Write([]byte(name))
	result := hasher.Sum(nil)[:16]
	result[6] = (result[6] & 0x0f) | 0x50
	result[8] = (result[8] & 0x3f) | 0x80
	return stringValue(fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", result[0:4], result[4:6], result[6:8], result[8:10], result[10:16])), nil
}

func task137CryptoParseUUID(value string) ([]byte, error) {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return nil, fmt.Errorf("crypto.uuid_v5 namespace must be a canonical UUID")
	}
	decoded, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	if err != nil || len(decoded) != 16 {
		return nil, fmt.Errorf("crypto.uuid_v5 namespace must be a canonical UUID")
	}
	return decoded, nil
}

func task137CryptoAlgorithmSupported(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("crypto.algorithm_supported expects one algorithm")
	}
	_, err := task137CryptoAlgorithm(args[0])
	return boolValue(err == nil), nil
}
