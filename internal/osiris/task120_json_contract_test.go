package osiris

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestTask120JSONRFC8259ValuesRoundTripAndCanonicalOutput(t *testing.T) {
	tests := []struct {
		name string
		src  string
		kind Task120ConfigKind
	}{
		{name: "null", src: "null", kind: Task120ConfigNull},
		{name: "boolean", src: " true ", kind: Task120ConfigBool},
		{name: "number", src: "-0.000e+12", kind: Task120ConfigNumber},
		{name: "string", src: `"escaped \" / \u20ac"`, kind: Task120ConfigString},
		{name: "array", src: `[null,false,1,"x"]`, kind: Task120ConfigArray},
		{name: "object", src: `{"b":2,"a":[true,null]}`, kind: Task120ConfigObject},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := Task120ParseJSONConfig(context.Background(), []byte(tt.src), Task120ConfigLimits{})
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
			roundTrip, err := Task120ParseJSONConfig(context.Background(), encoded, Task120ConfigLimits{})
			if err != nil {
				t.Fatalf("canonical output is not parseable: %v; output=%s", err, encoded)
			}
			if !reflect.DeepEqual(roundTrip, value) {
				t.Fatalf("round trip changed value:\nfirst=%#v\nsecond=%#v", value, roundTrip)
			}
		})
	}
}

func TestTask120JSONCanonicalOutputPreservesOrderNumbersAndEscaping(t *testing.T) {
	src := ` { "b" : 1.2300e+004, "a": "x", "slash": "a/b", "escaped": "\u0061", "quote": "a\"b", "back": "a\\b" } `
	value, err := Task120ParseJSONConfig(context.Background(), []byte(src), Task120ConfigLimits{})
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

func TestTask120JSONCanonicalNumberExponentForm(t *testing.T) {
	value := Task120ConfigValue{
		Kind: Task120ConfigObject,
		Object: []Task120ConfigField{
			{Key: "large", Value: Task120ConfigValue{Kind: Task120ConfigNumber, Number: "1e21"}},
			{Key: "small", Value: Task120ConfigValue{Kind: Task120ConfigNumber, Number: "1e-7"}},
		},
	}
	encoded, err := Task120JSONCanonicalize(value)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"large":1e+21,"small":1e-7}` {
		t.Fatalf("canonical exponent form = %s", encoded)
	}
}

func TestTask120JSONRejectsInvalidRFC8259AndDuplicateObjects(t *testing.T) {
	invalid := []string{
		``, `01`, `+1`, `NaN`, `{"a":}`, `{"a":1} {"b":2}`, `[1,]`,
		`{"a":01}`, `{"a":"unterminated}`, `{"a":true,}`,
	}
	for _, src := range invalid {
		t.Run(strconv.Quote(src), func(t *testing.T) {
			if _, err := Task120ParseJSONConfig(context.Background(), []byte(src), Task120ConfigLimits{}); err == nil {
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
			if _, err := Task120ParseJSONConfig(context.Background(), []byte(src), Task120ConfigLimits{}); !errors.Is(err, ErrTask120Invalid) {
				t.Fatalf("parse(%q) error = %v, want duplicate rejection", src, err)
			}
		})
	}
	if _, err := Task120JSONParse(context.Background(), []byte{'"', 0xff, '"'}, Task120JSONLimits{}); !errors.Is(err, ErrTask120Invalid) {
		t.Fatalf("invalid UTF-8 error = %v, want ErrTask120Invalid", err)
	}
}

func TestTask120JSONNumbersRemainExactAndMarshalRejectsInvalidNumbers(t *testing.T) {
	const source = `{"integer":900719925474099312345678901234567890,"negative_zero":-0,"decimal":123.4500,"exponent":6.02214076e+23}`
	value, err := Task120ParseJSONConfig(context.Background(), []byte(source), Task120ConfigLimits{})
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
	invalid := []Task120ConfigValue{
		{Kind: Task120ConfigNumber, Number: "01"},
		{Kind: Task120ConfigNumber, Number: "1 2"},
		{Kind: Task120ConfigNumber, Number: "true"},
		{Kind: Task120ConfigNumber, Number: "null"},
	}
	for _, candidate := range invalid {
		if _, err := candidate.JSON(); !errors.Is(err, ErrTask120Invalid) {
			t.Fatalf("invalid number %q error = %v, want ErrTask120Invalid", candidate.Number, err)
		}
	}
}

func TestTask120ConfigPathsEscapeObjectKeysDeterministically(t *testing.T) {
	key := `quote"slash\`
	value, err := Task120ParseJSONConfig(context.Background(), []byte(`{"quote\"slash\\":1}`), Task120ConfigLimits{})
	if err != nil {
		t.Fatal(err)
	}
	err = Task120ValidateConfig(value, Task120ConfigSchema{
		Type:            Task120SchemaObject,
		AllowAdditional: false,
	})
	var validation *Task120ConfigValidationError
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

func TestTask120JSONConfigLimits(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		limits Task120ConfigLimits
	}{
		{name: "bytes", input: `{"a":1}`, limits: Task120ConfigLimits{MaxBytes: 3}},
		{name: "depth", input: `[[0]]`, limits: Task120ConfigLimits{MaxDepth: 1}},
		{name: "nodes", input: `[0,1]`, limits: Task120ConfigLimits{MaxNodes: 2}},
		{name: "string", input: `"abc"`, limits: Task120ConfigLimits{MaxStringBytes: 2}},
		{name: "object-fields", input: `{"a":1,"b":2}`, limits: Task120ConfigLimits{MaxObjectFields: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Task120ParseJSONConfig(context.Background(), []byte(tt.input), tt.limits); !errors.Is(err, ErrTask120Limit) {
				t.Fatalf("error = %v, want ErrTask120Limit", err)
			}
		})
	}
}

func TestTask120JSONConfigCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Task120ParseJSONConfig(ctx, []byte(`{"a":[1,2,3]}`), Task120ConfigLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("JSON cancellation error = %v", err)
	}
	if _, err := Task120ParsePropertiesConfig(ctx, []byte("a = 1\nb = 2\n"), Task120ConfigLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("properties cancellation error = %v", err)
	}
}

func TestTask120JSONPatchAndMergePatchContractSurface(t *testing.T) {
	ctx := context.Background()
	document, err := Task120JSONParse(ctx, []byte(`{"name":"old","items":[1,2],"meta":{"remove":true}}`), Task120JSONLimits{})
	if err != nil {
		t.Fatal(err)
	}
	value := Task120ConfigValue{Kind: Task120ConfigString, String: "new"}
	patched, err := Task120ApplyJSONPatch(ctx, document, []Task120JSONPatchOperation{
		{Op: "replace", Path: "/name", Value: value},
		{Op: "add", Path: "/items/-", Value: Task120ConfigValue{Kind: Task120ConfigNumber, Number: "3"}},
		{Op: "copy", Path: "/meta/copied", From: "/name"},
		{Op: "move", Path: "/meta/moved", From: "/items/0"},
		{Op: "test", Path: "/name", Value: value},
		{Op: "remove", Path: "/meta/remove"},
	}, Task120JSONLimits{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Task120JSONStringify(patched)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"name":"new","items":[2,3],"meta":{"copied":"new","moved":1}}`
	if string(encoded) != want {
		t.Fatalf("patched JSON = %s, want %s", encoded, want)
	}
	if _, err := Task120ApplyJSONPatch(ctx, document, []Task120JSONPatchOperation{
		{Op: "test", Path: "/name", Value: Task120ConfigValue{Kind: Task120ConfigString, String: "wrong"}},
	}, Task120JSONLimits{}); !errors.Is(err, ErrTask120Invalid) {
		t.Fatalf("failed test error = %v, want ErrTask120Invalid", err)
	}

	mergePatch, err := Task120JSONParse(ctx, []byte(`{"name":"updated","meta":{"remove":null,"added":true}}`), Task120JSONLimits{})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := Task120ApplyJSONMergePatch(ctx, document, mergePatch, Task120JSONLimits{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = Task120JSONStringify(merged)
	if err != nil {
		t.Fatal(err)
	}
	const wantMerged = `{"name":"updated","items":[1,2],"meta":{"added":true}}`
	if string(encoded) != wantMerged {
		t.Fatalf("merge-patched JSON = %s, want %s", encoded, wantMerged)
	}
}
