package osiris

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestExtendedJSONQueryAndStructuralHelpers(t *testing.T) {
	ctx := context.Background()
	document, err := StdlibJSONParse(ctx, []byte(`{"users":[{"name":"Ada","roles":["admin"]},{"name":"Lin","roles":["reader"]}],"active":true}`), StdlibJSONLimits{})
	if err != nil {
		t.Fatal(err)
	}
	results, err := ExtendedJSONQuery(ctx, document, "$.users[*].name")
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{results[0].String, results[1].String}; !reflect.DeepEqual(got, []string{"Ada", "Lin"}) {
		t.Fatalf("query names = %#v", got)
	}
	roles, err := ExtendedJSONQuery(ctx, document, `$.users[0]["roles"][*]`)
	if err != nil || len(roles) != 1 || roles[0].String != "admin" {
		t.Fatalf("quoted/wildcard query = %#v, %v", roles, err)
	}
	one, err := ExtendedJSONQueryOne(ctx, document, "$.active")
	if err != nil || !one.Bool {
		t.Fatalf("query one = %#v, %v", one, err)
	}
	exists, err := ExtendedJSONPointerExists(document, "/users/4")
	if err != nil || exists {
		t.Fatalf("missing pointer exists = %v, %v", exists, err)
	}
	if _, err := ExtendedJSONQuery(ctx, document, "users"); !errors.Is(err, ErrStdlibInvalid) {
		t.Fatalf("invalid query error = %v", err)
	}

	left, _ := StdlibJSONParse(ctx, []byte(`{"n":1,"nested":{"a":true}}`), StdlibJSONLimits{})
	right, _ := StdlibJSONParse(ctx, []byte(`{"n":1.0,"nested":{"b":2}}`), StdlibJSONLimits{})
	equal, err := ExtendedJSONEqual(left, right)
	if err != nil || equal {
		t.Fatalf("ordered structural equality = %v, %v", equal, err)
	}
	numberOne, _ := StdlibJSONParse(ctx, []byte(`1e0`), StdlibJSONLimits{})
	numberTwo, _ := StdlibJSONParse(ctx, []byte(`1.00`), StdlibJSONLimits{})
	equal, err = ExtendedJSONEqual(numberOne, numberTwo)
	if err != nil || !equal {
		t.Fatalf("numeric equality = %v, %v", equal, err)
	}
	merged, err := ExtendedJSONMerge(left, right)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := StdlibJSONStringify(merged)
	if err != nil || string(encoded) != `{"n":1.0,"nested":{"a":true,"b":2}}` {
		t.Fatalf("merged = %s, %v", encoded, err)
	}
}

func TestExtendedJSONLinesAndExtendedSchema(t *testing.T) {
	ctx := context.Background()
	values, err := ExtendedJSONParseLines(ctx, []byte("{\"id\":1}\n\n[true]\r\n"))
	if err != nil || len(values) != 2 {
		t.Fatalf("JSON Lines = %#v, %v", values, err)
	}
	encoded, err := ExtendedJSONStringifyLines(values)
	if err != nil || string(encoded) != "{\"id\":1}\n[true]\n" {
		t.Fatalf("JSON Lines output = %q, %v", encoded, err)
	}
	if _, err := ExtendedJSONParseLines(ctx, []byte("1\n"), StdlibJSONLimits{MaxBytes: 1}); err == nil {
		t.Fatalf("expected bounded JSON Lines error")
	}

	schemaValue, err := StdlibJSONParse(ctx, []byte(`{"type":"object","required":["id"],"additional_properties":false,"properties":{"id":{"type":"integer","minimum":1},"name":{"type":"string","pattern":"^[A-Z]"}}}`), StdlibJSONLimits{})
	if err != nil {
		t.Fatal(err)
	}
	schema, err := ExtendedJSONSchemaFromValue(schemaValue)
	if err != nil {
		t.Fatal(err)
	}
	invalid, _ := StdlibJSONParse(ctx, []byte(`{"id":0,"name":"ada","extra":true}`), StdlibJSONLimits{})
	issues, err := ExtendedJSONValidateSchema(ctx, invalid, schema)
	if err != nil || len(issues) != 3 {
		t.Fatalf("schema issues = %#v, %v", issues, err)
	}
	if got := []string{issues[0].Keyword, issues[1].Keyword, issues[2].Keyword}; !reflect.DeepEqual(got, []string{"minimum", "pattern", "additionalProperties"}) {
		t.Fatalf("schema keyword order = %#v", got)
	}
	valid, _ := StdlibJSONParse(ctx, []byte(`{"id":2,"name":"Ada"}`), StdlibJSONLimits{})
	ok, err := ExtendedJSONSchemaValid(ctx, valid, schema)
	if err != nil || !ok {
		t.Fatalf("schema valid = %v, %v", ok, err)
	}
	if _, err := ExtendedJSONQuery(ctx, valid, "$.name[*]", ExtendedJSONQueryLimits{MaxSteps: 1}); err == nil {
		t.Fatal("expected query step limit")
	}
}

func TestExtendedJSONModuleHelperIsAdditive(t *testing.T) {
	module := ExtendedJSONModule()
	for _, name := range []string{"extensions_version", "type_of", "keys", "length", "equal", "merge", "query", "parse_lines", "stringify_lines"} {
		if _, ok := module[name]; !ok {
			t.Fatalf("JSON module missing %q", name)
		}
	}
	if strings.Contains(StdlibJSONVersion, ExtendedJSONVersion) {
		t.Fatal("task versions unexpectedly overlap")
	}
}
