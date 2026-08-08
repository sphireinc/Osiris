package osiris

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestStructTupleAliasAndNewtypeInterpreterVMParity(t *testing.T) {
	source := "struct Spacecraft { name string `json:\"name\" bson:\"craft_name\"`, health f64 `json:\"health,omitempty\"` }\n" +
		"type Count = u64\n" +
		"newtype MissionID u64\n" +
		"let craft = Spacecraft { name: \"Odyssey\", health: f64(0.75) }\n" +
		"let revised = craft with { health: f64(0.9) }\n" +
		"let id = MissionID(u64(7))\n" +
		"let (name, score) = (craft.name, u64(3))\n" +
		"emit craft.name\n" +
		"emit revised.health\n" +
		"emit id\n" +
		"emit id.value\n" +
		"emit name, score\n"
	prog, err := Parse("<structs>", source)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := FormatSource("<structs>", source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse("<structs-formatted>", formatted); err != nil {
		t.Fatalf("formatted source does not parse: %v\n%s", err, formatted)
	}
	rt := NewRuntime()
	interpreter, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if len(interpreter) != len(vm) {
		t.Fatalf("interpreter=%v vm=%v", interpreter, vm)
	}
	for i := range interpreter {
		if !interpreter[i].Equal(vm[i]) {
			t.Fatalf("output %d differs: interpreter=%s vm=%s", i, interpreter[i], vm[i])
		}
	}
	if got := interpreter[0].String(); got != `"Odyssey"` {
		t.Fatalf("name = %s", got)
	}
	if got := interpreter[1].String(); got != "0.9f64" {
		t.Fatalf("health = %s", got)
	}
	if got := interpreter[2].Kind(); got != "MissionID" {
		t.Fatalf("newtype kind = %s", got)
	}
}

func TestStructJSONTagsAndMetadata(t *testing.T) {
	prog, err := Parse("<struct-json>", "struct Item { name string `json:\"label\" bson:\"item_name\"`, empty string `json:\"empty,omitempty\"` }\nlet item = Item { name: \"x\", empty: \"\" }\nemit item\n")
	if err != nil {
		t.Fatal(err)
	}
	out, err := NewRuntime().Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(out[0])
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `{"label":"x"}`; got != want {
		t.Fatalf("json = %s, want %s", got, want)
	}
}

func TestStructTagDiagnostics(t *testing.T) {
	for _, source := range []string{
		"struct Bad { value string `json:\"x\" json:\"y\"` }\n",
		"struct Bad { value string `json:x` }\n",
		"struct Bad { value string `json:\"unterminated` }\n",
	} {
		if _, err := Parse("<struct-tag>", source); err == nil {
			t.Fatalf("Parse(%q) succeeded", source)
		}
	}
}

func TestStructTupleCodecPreservesNominalMetadata(t *testing.T) {
	source := "struct Item { name string `json:\"label\" bson:\"item_name\"`, count u64 `json:\"count\"` }\n" +
		"let item = Item { name: \"x\", count: u64(2) }\n" +
		"emit (item, item.count)\n"
	prog, err := Parse("<struct-codec>", source)
	if err != nil {
		t.Fatal(err)
	}
	out, err := NewRuntime().Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := encodeValueBytes(out[0])
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeValueBytes(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.Equal(out[0]) {
		t.Fatalf("codec changed value: %s != %s", decoded, out[0])
	}
	if decoded.tuple[0].structType == nil || decoded.tuple[0].structType.fields[0].tagValues["json"] != "label" {
		t.Fatalf("codec lost struct tag metadata: %#v", decoded)
	}
}

func TestStructNativeTypeAliasBoundaryIsExplicit(t *testing.T) {
	prog, err := Parse("<struct-native>", "type Point = u64\nlet p = Point(1u64)\nemit p\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileC(context.Background(), NewRuntime(), prog); err == nil || !strings.Contains(err.Error(), "product-type declarations") {
		t.Fatalf("native compile error = %v", err)
	}
}
