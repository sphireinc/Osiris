package osiris

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestTask137JSONQueryAndStructuralHelpers(t *testing.T) {
	ctx := context.Background()
	document, err := Task120JSONParse(ctx, []byte(`{"users":[{"name":"Ada","roles":["admin"]},{"name":"Lin","roles":["reader"]}],"active":true}`), Task120JSONLimits{})
	if err != nil {
		t.Fatal(err)
	}
	results, err := Task137JSONQuery(ctx, document, "$.users[*].name")
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{results[0].String, results[1].String}; !reflect.DeepEqual(got, []string{"Ada", "Lin"}) {
		t.Fatalf("query names = %#v", got)
	}
	roles, err := Task137JSONQuery(ctx, document, `$.users[0]["roles"][*]`)
	if err != nil || len(roles) != 1 || roles[0].String != "admin" {
		t.Fatalf("quoted/wildcard query = %#v, %v", roles, err)
	}
	one, err := Task137JSONQueryOne(ctx, document, "$.active")
	if err != nil || !one.Bool {
		t.Fatalf("query one = %#v, %v", one, err)
	}
	exists, err := Task137JSONPointerExists(document, "/users/4")
	if err != nil || exists {
		t.Fatalf("missing pointer exists = %v, %v", exists, err)
	}
	if _, err := Task137JSONQuery(ctx, document, "users"); !errors.Is(err, ErrTask120Invalid) {
		t.Fatalf("invalid query error = %v", err)
	}

	left, _ := Task120JSONParse(ctx, []byte(`{"n":1,"nested":{"a":true}}`), Task120JSONLimits{})
	right, _ := Task120JSONParse(ctx, []byte(`{"n":1.0,"nested":{"b":2}}`), Task120JSONLimits{})
	equal, err := Task137JSONEqual(left, right)
	if err != nil || equal {
		t.Fatalf("ordered structural equality = %v, %v", equal, err)
	}
	numberOne, _ := Task120JSONParse(ctx, []byte(`1e0`), Task120JSONLimits{})
	numberTwo, _ := Task120JSONParse(ctx, []byte(`1.00`), Task120JSONLimits{})
	equal, err = Task137JSONEqual(numberOne, numberTwo)
	if err != nil || !equal {
		t.Fatalf("numeric equality = %v, %v", equal, err)
	}
	merged, err := Task137JSONMerge(left, right)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Task120JSONStringify(merged)
	if err != nil || string(encoded) != `{"n":1.0,"nested":{"a":true,"b":2}}` {
		t.Fatalf("merged = %s, %v", encoded, err)
	}
}

func TestTask137JSONLinesAndExtendedSchema(t *testing.T) {
	ctx := context.Background()
	values, err := Task137JSONParseLines(ctx, []byte("{\"id\":1}\n\n[true]\r\n"))
	if err != nil || len(values) != 2 {
		t.Fatalf("JSON Lines = %#v, %v", values, err)
	}
	encoded, err := Task137JSONStringifyLines(values)
	if err != nil || string(encoded) != "{\"id\":1}\n[true]\n" {
		t.Fatalf("JSON Lines output = %q, %v", encoded, err)
	}
	if _, err := Task137JSONParseLines(ctx, []byte("1\n"), Task120JSONLimits{MaxBytes: 1}); err == nil {
		t.Fatalf("expected bounded JSON Lines error")
	}

	schemaValue, err := Task120JSONParse(ctx, []byte(`{"type":"object","required":["id"],"additional_properties":false,"properties":{"id":{"type":"integer","minimum":1},"name":{"type":"string","pattern":"^[A-Z]"}}}`), Task120JSONLimits{})
	if err != nil {
		t.Fatal(err)
	}
	schema, err := Task137JSONSchemaFromValue(schemaValue)
	if err != nil {
		t.Fatal(err)
	}
	invalid, _ := Task120JSONParse(ctx, []byte(`{"id":0,"name":"ada","extra":true}`), Task120JSONLimits{})
	issues, err := Task137JSONValidateSchema(ctx, invalid, schema)
	if err != nil || len(issues) != 3 {
		t.Fatalf("schema issues = %#v, %v", issues, err)
	}
	if got := []string{issues[0].Keyword, issues[1].Keyword, issues[2].Keyword}; !reflect.DeepEqual(got, []string{"minimum", "pattern", "additionalProperties"}) {
		t.Fatalf("schema keyword order = %#v", got)
	}
	valid, _ := Task120JSONParse(ctx, []byte(`{"id":2,"name":"Ada"}`), Task120JSONLimits{})
	ok, err := Task137JSONSchemaValid(ctx, valid, schema)
	if err != nil || !ok {
		t.Fatalf("schema valid = %v, %v", ok, err)
	}
	if _, err := Task137JSONQuery(ctx, valid, "$.name[*]", Task137JSONQueryLimits{MaxSteps: 1}); err == nil {
		t.Fatal("expected query step limit")
	}
}

func TestTask137JSONModuleHelperIsAdditive(t *testing.T) {
	module := Task137JSONModule()
	for _, name := range []string{"task137_version", "type_of", "keys", "length", "equal", "merge", "query", "parse_lines", "stringify_lines"} {
		if _, ok := module[name]; !ok {
			t.Fatalf("JSON module missing %q", name)
		}
	}
	if strings.Contains(Task120JSONVersion, Task137JSONVersion) {
		t.Fatal("task versions unexpectedly overlap")
	}
}
