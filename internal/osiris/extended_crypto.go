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
	extendedCryptoVersion        = "1.1.0"
	extendedCryptoMaxInputBytes  = 1 << 20
	extendedCryptoMaxOutputBytes = 1 << 20
	extendedCryptoMaxKeyBytes    = 1 << 16
	extendedCryptoMaxTokenBytes  = 4096
)

// extendedCryptoModule is the integration surface for the expanded std/crypto
// module. Its operations are pure with respect to the Osiris runtime: no
// ambient random source, process state, clock, or mutable package state is
// consulted. The module integration can merge this map with the existing
// std/crypto members without changing any existing symbol.
func extendedCryptoModule() map[string]Value {
	return map[string]Value{
		"version":             stringValue(extendedCryptoVersion),
		"max_input_bytes":     uintValue(extendedCryptoMaxInputBytes, 64),
		"max_output_bytes":    uintValue(extendedCryptoMaxOutputBytes, 64),
		"max_key_bytes":       uintValue(extendedCryptoMaxKeyBytes, 64),
		"max_token_bytes":     uintValue(extendedCryptoMaxTokenBytes, 64),
		"sha256":              stringValue("sha256"),
		"sha512":              stringValue("sha512"),
		"sha512_256":          stringValue("sha512_256"),
		"digest":              functionValueOf("std/crypto.digest", extendedCryptoDigest),
		"digest_bytes":        functionValueOf("std/crypto.digest_bytes", extendedCryptoDigestBytes),
		"hmac":                functionValueOf("std/crypto.hmac", extendedCryptoHMAC),
		"compare_digest":      functionValueOf("std/crypto.compare_digest", extendedCryptoCompareDigest),
		"hex_encode":          functionValueOf("std/crypto.hex_encode", extendedCryptoHexEncode),
		"hex_decode":          functionValueOf("std/crypto.hex_decode", extendedCryptoHexDecode),
		"base64_encode":       functionValueOf("std/crypto.base64_encode", extendedCryptoBase64Encode),
		"base64_decode":       functionValueOf("std/crypto.base64_decode", extendedCryptoBase64Decode),
		"base64url_encode":    functionValueOf("std/crypto.base64url_encode", extendedCryptoBase64URLEncode),
		"base64url_decode":    functionValueOf("std/crypto.base64url_decode", extendedCryptoBase64URLDecode),
		"derive_key":          functionValueOf("std/crypto.derive_key", extendedCryptoDeriveKey),
		"random_bytes":        functionValueOf("std/crypto.random_bytes", extendedCryptoRandomBytes),
		"token":               functionValueOf("std/crypto.token", extendedCryptoToken),
		"uuid_v5":             functionValueOf("std/crypto.uuid_v5", extendedCryptoUUIDV5),
		"algorithm_supported": functionValueOf("std/crypto.algorithm_supported", extendedCryptoAlgorithmSupported),
	}
}

func extendedCryptoBytes(value Value, name string, max int) ([]byte, error) {
	data, err := bytesFromValue(value)
	if err != nil {
		return nil, fmt.Errorf("%s: expected byte-compatible value: %w", name, err)
	}
	if len(data) > max {
		return nil, fmt.Errorf("%s exceeds the %d-byte limit", name, max)
	}
	return data, nil
}

func extendedCryptoAlgorithm(value Value) (string, error) {
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

func extendedCryptoHash(name string) (hash.Hash, error) {
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

func extendedCryptoDigest(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("crypto.digest expects data and algorithm")
	}
	data, err := extendedCryptoBytes(args[0], "crypto.digest data", extendedCryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	algorithm, err := extendedCryptoAlgorithm(args[1])
	if err != nil {
		return nullValue(), err
	}
	hasher, err := extendedCryptoHash(algorithm)
	if err != nil {
		return nullValue(), err
	}
	_, _ = hasher.Write(data)
	return stringValue(hex.EncodeToString(hasher.Sum(nil))), nil
}

func extendedCryptoDigestBytes(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("crypto.digest_bytes expects data and algorithm")
	}
	data, err := extendedCryptoBytes(args[0], "crypto.digest_bytes data", extendedCryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	algorithm, err := extendedCryptoAlgorithm(args[1])
	if err != nil {
		return nullValue(), err
	}
	hasher, err := extendedCryptoHash(algorithm)
	if err != nil {
		return nullValue(), err
	}
	_, _ = hasher.Write(data)
	return bytesListValue(hasher.Sum(nil)), nil
}

func extendedCryptoHMAC(args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("crypto.hmac expects key, data, and algorithm")
	}
	key, err := extendedCryptoBytes(args[0], "crypto.hmac key", extendedCryptoMaxKeyBytes)
	if err != nil {
		return nullValue(), err
	}
	data, err := extendedCryptoBytes(args[1], "crypto.hmac data", extendedCryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	algorithm, err := extendedCryptoAlgorithm(args[2])
	if err != nil {
		return nullValue(), err
	}
	constructor := func() hash.Hash {
		hasher, _ := extendedCryptoHash(algorithm)
		return hasher
	}
	mac := hmac.New(constructor, key)
	_, _ = mac.Write(data)
	return stringValue(hex.EncodeToString(mac.Sum(nil))), nil
}

func extendedCryptoCompareDigest(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("crypto.compare_digest expects two byte-compatible values")
	}
	left, err := extendedCryptoBytes(args[0], "crypto.compare_digest left", extendedCryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	right, err := extendedCryptoBytes(args[1], "crypto.compare_digest right", extendedCryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	return boolValue(subtle.ConstantTimeCompare(left, right) == 1), nil
}

func extendedCryptoHexEncode(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("crypto.hex_encode expects one byte-compatible value")
	}
	data, err := extendedCryptoBytes(args[0], "crypto.hex_encode data", extendedCryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	return stringValue(hex.EncodeToString(data)), nil
}

func extendedCryptoHexDecode(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("crypto.hex_decode expects one string")
	}
	encoded, err := args[0].AsString()
	if err != nil {
		return nullValue(), fmt.Errorf("crypto.hex_decode expects a string")
	}
	if len(encoded) > extendedCryptoMaxOutputBytes*2 {
		return nullValue(), fmt.Errorf("crypto.hex_decode exceeds the output limit")
	}
	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		return nullValue(), fmt.Errorf("crypto.hex_decode: %w", err)
	}
	return bytesListValue(decoded), nil
}

func extendedCryptoBase64Encode(args []Value) (Value, error) {
	return extendedCryptoBase64EncodeWith(args, base64.StdEncoding, "base64_encode")
}

func extendedCryptoBase64URLEncode(args []Value) (Value, error) {
	return extendedCryptoBase64EncodeWith(args, base64.RawURLEncoding, "base64url_encode")
}

func extendedCryptoBase64EncodeWith(args []Value, encoding *base64.Encoding, operation string) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("crypto.%s expects one byte-compatible value", operation)
	}
	data, err := extendedCryptoBytes(args[0], "crypto."+operation+" data", extendedCryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	return stringValue(encoding.EncodeToString(data)), nil
}

func extendedCryptoBase64Decode(args []Value) (Value, error) {
	return extendedCryptoBase64DecodeWith(args, base64.StdEncoding, "base64_decode")
}

func extendedCryptoBase64URLDecode(args []Value) (Value, error) {
	return extendedCryptoBase64DecodeWith(args, base64.RawURLEncoding, "base64url_decode")
}

func extendedCryptoBase64DecodeWith(args []Value, encoding *base64.Encoding, operation string) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("crypto.%s expects one string", operation)
	}
	encoded, err := args[0].AsString()
	if err != nil {
		return nullValue(), fmt.Errorf("crypto.%s expects a string", operation)
	}
	if len(encoded) > extendedCryptoMaxInputBytes*2 {
		return nullValue(), fmt.Errorf("crypto.%s exceeds the input limit", operation)
	}
	decoded, err := encoding.DecodeString(encoded)
	if err != nil {
		return nullValue(), fmt.Errorf("crypto.%s: %w", operation, err)
	}
	if len(decoded) > extendedCryptoMaxOutputBytes {
		return nullValue(), fmt.Errorf("crypto.%s exceeds the output limit", operation)
	}
	return bytesListValue(decoded), nil
}

func extendedCryptoLength(value Value, name string, max int) (int, error) {
	number, err := value.AsNumber()
	if err != nil || number < 0 || number != float64(int(number)) || number > float64(max) {
		return 0, fmt.Errorf("%s must be an integer from 0 through %d", name, max)
	}
	return int(number), nil
}

func extendedCryptoDeriveKey(args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("crypto.derive_key expects secret, context, and length")
	}
	secret, err := extendedCryptoBytes(args[0], "crypto.derive_key secret", extendedCryptoMaxKeyBytes)
	if err != nil {
		return nullValue(), err
	}
	context, err := extendedCryptoBytes(args[1], "crypto.derive_key context", extendedCryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	length, err := extendedCryptoLength(args[2], "crypto.derive_key length", extendedCryptoMaxTokenBytes)
	if err != nil {
		return nullValue(), err
	}
	return bytesListValue(extendedCryptoExpand(secret, context, length)), nil
}

func extendedCryptoRandomBytes(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("crypto.random_bytes expects seed and length")
	}
	seed, err := extendedCryptoBytes(args[0], "crypto.random_bytes seed", extendedCryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	length, err := extendedCryptoLength(args[1], "crypto.random_bytes length", extendedCryptoMaxTokenBytes)
	if err != nil {
		return nullValue(), err
	}
	return bytesListValue(extendedCryptoExpand(seed, []byte("random_bytes"), length)), nil
}

func extendedCryptoToken(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("crypto.token expects seed and length")
	}
	seed, err := extendedCryptoBytes(args[0], "crypto.token seed", extendedCryptoMaxInputBytes)
	if err != nil {
		return nullValue(), err
	}
	length, err := extendedCryptoLength(args[1], "crypto.token length", extendedCryptoMaxTokenBytes)
	if err != nil {
		return nullValue(), err
	}
	raw := extendedCryptoExpand(seed, []byte("token"), length)
	return stringValue(base64.RawURLEncoding.EncodeToString(raw)), nil
}

func extendedCryptoExpand(key, context []byte, length int) []byte {
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

func extendedCryptoUUIDV5(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("crypto.uuid_v5 expects namespace and name")
	}
	namespace, err := args[0].AsString()
	if err != nil {
		return nullValue(), fmt.Errorf("crypto.uuid_v5 namespace must be a UUID string")
	}
	name, err := args[1].AsString()
	if err != nil || len(name) > extendedCryptoMaxInputBytes {
		return nullValue(), fmt.Errorf("crypto.uuid_v5 name must be a bounded string")
	}
	namespaceBytes, err := extendedCryptoParseUUID(namespace)
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

func extendedCryptoParseUUID(value string) ([]byte, error) {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return nil, fmt.Errorf("crypto.uuid_v5 namespace must be a canonical UUID")
	}
	decoded, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	if err != nil || len(decoded) != 16 {
		return nil, fmt.Errorf("crypto.uuid_v5 namespace must be a canonical UUID")
	}
	return decoded, nil
}

func extendedCryptoAlgorithmSupported(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("crypto.algorithm_supported expects one algorithm")
	}
	_, err := extendedCryptoAlgorithm(args[0])
	return boolValue(err == nil), nil
}
