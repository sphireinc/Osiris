package osiris

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestStdlibJSONRFC8259ValuesRoundTripAndCanonicalOutput(t *testing.T) {
	tests := []struct {
		name string
		src  string
		kind StdlibConfigKind
	}{
		{name: "null", src: "null", kind: StdlibConfigNull},
		{name: "boolean", src: " true ", kind: StdlibConfigBool},
		{name: "number", src: "-0.000e+12", kind: StdlibConfigNumber},
		{name: "string", src: `"escaped \" / \u20ac"`, kind: StdlibConfigString},
		{name: "array", src: `[null,false,1,"x"]`, kind: StdlibConfigArray},
		{name: "object", src: `{"b":2,"a":[true,null]}`, kind: StdlibConfigObject},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := StdlibParseJSONConfig(context.Background(), []byte(tt.src), StdlibConfigLimits{})
			if err != nil {
				t.Fatal(err)
			}
			if value.Kind != tt.kind {
				t.Fatalf("kind = %q, want %q", value.Kind, tt.kind)
			}
			encoded, err := value.JSON()
			if err != nil {
				t.Fatal(err)
			}
			if string(encoded) == "" {
				t.Fatal("canonical output is empty")
			}
			roundTrip, err := StdlibParseJSONConfig(context.Background(), encoded, StdlibConfigLimits{})
			if err != nil {
				t.Fatalf("canonical output is not parseable: %v; output=%s", err, encoded)
			}
			if !reflect.DeepEqual(roundTrip, value) {
				t.Fatalf("round trip changed value:\nfirst=%#v\nsecond=%#v", value, roundTrip)
			}
		})
	}
}

func TestStdlibJSONCanonicalOutputPreservesOrderNumbersAndEscaping(t *testing.T) {
	src := ` { "b" : 1.2300e+004, "a": "x", "slash": "a/b", "escaped": "\u0061", "quote": "a\"b", "back": "a\\b" } `
	value, err := StdlibParseJSONConfig(context.Background(), []byte(src), StdlibConfigLimits{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := value.JSON()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"b":1.2300e+004,"a":"x","slash":"a/b","escaped":"a","quote":"a\"b","back":"a\\b"}`
	if string(encoded) != want {
		t.Fatalf("canonical JSON = %s, want %s", encoded, want)
	}
	if value.Object[0].Key != "b" || value.Object[1].Key != "a" {
		t.Fatalf("object source order changed: %#v", value.Object)
	}
	if value.Object[0].Value.Number != "1.2300e+004" {
		t.Fatalf("number spelling changed: %q", value.Object[0].Value.Number)
	}
}

func TestStdlibJSONCanonicalNumberExponentForm(t *testing.T) {
	value := StdlibConfigValue{
		Kind: StdlibConfigObject,
		Object: []StdlibConfigField{
			{Key: "large", Value: StdlibConfigValue{Kind: StdlibConfigNumber, Number: "1e21"}},
			{Key: "small", Value: StdlibConfigValue{Kind: StdlibConfigNumber, Number: "1e-7"}},
		},
	}
	encoded, err := StdlibJSONCanonicalize(value)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"large":1e+21,"small":1e-7}` {
		t.Fatalf("canonical exponent form = %s", encoded)
	}
}

func TestStdlibJSONRejectsInvalidRFC8259AndDuplicateObjects(t *testing.T) {
	invalid := []string{
		``, `01`, `+1`, `NaN`, `{"a":}`, `{"a":1} {"b":2}`, `[1,]`,
		`{"a":01}`, `{"a":"unterminated}`, `{"a":true,}`,
	}
	for _, src := range invalid {
		t.Run(strconv.Quote(src), func(t *testing.T) {
			if _, err := StdlibParseJSONConfig(context.Background(), []byte(src), StdlibConfigLimits{}); err == nil {
				t.Fatalf("parse(%q) unexpectedly succeeded", src)
			}
		})
	}
	duplicates := []string{
		`{"a":1,"a":2}`,
		`{"outer":{"x":1,"x":2}}`,
		`[{"x":1,"x":2}]`,
	}
	for _, src := range duplicates {
		t.Run("duplicate-"+strconv.Quote(src), func(t *testing.T) {
			if _, err := StdlibParseJSONConfig(context.Background(), []byte(src), StdlibConfigLimits{}); !errors.Is(err, ErrStdlibInvalid) {
				t.Fatalf("parse(%q) error = %v, want duplicate rejection", src, err)
			}
		})
	}
	if _, err := StdlibJSONParse(context.Background(), []byte{'"', 0xff, '"'}, StdlibJSONLimits{}); !errors.Is(err, ErrStdlibInvalid) {
		t.Fatalf("invalid UTF-8 error = %v, want ErrStdlibInvalid", err)
	}
}

func TestStdlibJSONNumbersRemainExactAndMarshalRejectsInvalidNumbers(t *testing.T) {
	const source = `{"integer":900719925474099312345678901234567890,"negative_zero":-0,"decimal":123.4500,"exponent":6.02214076e+23}`
	value, err := StdlibParseJSONConfig(context.Background(), []byte(source), StdlibConfigLimits{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"900719925474099312345678901234567890",
		"-0",
		"123.4500",
		"6.02214076e+23",
	}
	for i, field := range value.Object {
		if field.Value.Number != want[i] {
			t.Fatalf("field %q number = %q, want %q", field.Key, field.Value.Number, want[i])
		}
	}
	invalid := []StdlibConfigValue{
		{Kind: StdlibConfigNumber, Number: "01"},
		{Kind: StdlibConfigNumber, Number: "1 2"},
		{Kind: StdlibConfigNumber, Number: "true"},
		{Kind: StdlibConfigNumber, Number: "null"},
	}
	for _, candidate := range invalid {
		if _, err := candidate.JSON(); !errors.Is(err, ErrStdlibInvalid) {
			t.Fatalf("invalid number %q error = %v, want ErrStdlibInvalid", candidate.Number, err)
		}
	}
}

func TestStdlibConfigPathsEscapeObjectKeysDeterministically(t *testing.T) {
	key := `quote"slash\`
	value, err := StdlibParseJSONConfig(context.Background(), []byte(`{"quote\"slash\\":1}`), StdlibConfigLimits{})
	if err != nil {
		t.Fatal(err)
	}
	err = StdlibValidateConfig(value, StdlibConfigSchema{
		Type:            StdlibSchemaObject,
		AllowAdditional: false,
	})
	var validation *StdlibConfigValidationError
	if !errors.As(err, &validation) || len(validation.Errors) != 1 {
		t.Fatalf("validation = %#v, %v", validation, err)
	}
	wantPath := `$[` + strconv.Quote(key) + `]`
	if validation.Errors[0].Path != wantPath {
		t.Fatalf("escaped path = %q, want %q", validation.Errors[0].Path, wantPath)
	}
	if !strings.Contains(validation.Errors[0].Path, `\"`) || !strings.Contains(validation.Errors[0].Path, `\\`) {
		t.Fatalf("path did not preserve both quote and backslash escaping: %q", validation.Errors[0].Path)
	}
}

func TestStdlibJSONConfigLimits(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		limits StdlibConfigLimits
	}{
		{name: "bytes", input: `{"a":1}`, limits: StdlibConfigLimits{MaxBytes: 3}},
		{name: "depth", input: `[[0]]`, limits: StdlibConfigLimits{MaxDepth: 1}},
		{name: "nodes", input: `[0,1]`, limits: StdlibConfigLimits{MaxNodes: 2}},
		{name: "string", input: `"abc"`, limits: StdlibConfigLimits{MaxStringBytes: 2}},
		{name: "object-fields", input: `{"a":1,"b":2}`, limits: StdlibConfigLimits{MaxObjectFields: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := StdlibParseJSONConfig(context.Background(), []byte(tt.input), tt.limits); !errors.Is(err, ErrStdlibLimit) {
				t.Fatalf("error = %v, want ErrStdlibLimit", err)
			}
		})
	}
}

func TestStdlibJSONConfigCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := StdlibParseJSONConfig(ctx, []byte(`{"a":[1,2,3]}`), StdlibConfigLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("JSON cancellation error = %v", err)
	}
	if _, err := StdlibParsePropertiesConfig(ctx, []byte("a = 1\nb = 2\n"), StdlibConfigLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("properties cancellation error = %v", err)
	}
}

func TestStdlibJSONPatchAndMergePatchContractSurface(t *testing.T) {
	ctx := context.Background()
	document, err := StdlibJSONParse(ctx, []byte(`{"name":"old","items":[1,2],"meta":{"remove":true}}`), StdlibJSONLimits{})
	if err != nil {
		t.Fatal(err)
	}
	value := StdlibConfigValue{Kind: StdlibConfigString, String: "new"}
	patched, err := StdlibApplyJSONPatch(ctx, document, []StdlibJSONPatchOperation{
		{Op: "replace", Path: "/name", Value: value},
		{Op: "add", Path: "/items/-", Value: StdlibConfigValue{Kind: StdlibConfigNumber, Number: "3"}},
		{Op: "copy", Path: "/meta/copied", From: "/name"},
		{Op: "move", Path: "/meta/moved", From: "/items/0"},
		{Op: "test", Path: "/name", Value: value},
		{Op: "remove", Path: "/meta/remove"},
	}, StdlibJSONLimits{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := StdlibJSONStringify(patched)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"name":"new","items":[2,3],"meta":{"copied":"new","moved":1}}`
	if string(encoded) != want {
		t.Fatalf("patched JSON = %s, want %s", encoded, want)
	}
	if _, err := StdlibApplyJSONPatch(ctx, document, []StdlibJSONPatchOperation{
		{Op: "test", Path: "/name", Value: StdlibConfigValue{Kind: StdlibConfigString, String: "wrong"}},
	}, StdlibJSONLimits{}); !errors.Is(err, ErrStdlibInvalid) {
		t.Fatalf("failed test error = %v, want ErrStdlibInvalid", err)
	}

	mergePatch, err := StdlibJSONParse(ctx, []byte(`{"name":"updated","meta":{"remove":null,"added":true}}`), StdlibJSONLimits{})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := StdlibApplyJSONMergePatch(ctx, document, mergePatch, StdlibJSONLimits{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = StdlibJSONStringify(merged)
	if err != nil {
		t.Fatal(err)
	}
	const wantMerged = `{"name":"updated","items":[1,2],"meta":{"added":true}}`
	if string(encoded) != wantMerged {
		t.Fatalf("merge-patched JSON = %s, want %s", encoded, wantMerged)
	}
}
