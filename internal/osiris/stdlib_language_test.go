package osiris

import (
	"context"
	"testing"
)

func TestStdlibTo122LanguageModulesInterpreterAndVM(t *testing.T) {
	source := `import "std/regex" as regex
import "std/compress" as compress
import "std/json" as json
import "std/i18n" as i18n
import "std/db" as db
import "std/telemetry" as telemetry

let connection = db.open()
emit db.exec(connection, "CREATE TABLE users (id INTEGER, name TEXT)", [])
emit db.exec(connection, "INSERT INTO users (id, name) VALUES (?, ?)", [1, "Ada"])
let rows = db.query(connection, "SELECT id, name FROM users", [])
emit db.next(rows)
emit telemetry.info("module smoke", "stdlib-122")
emit regex.is_match("a+", "aaa")
emit compress.gunzip(compress.gzip([1u8, 2u8]))
emit json.parse([123u8, 34u8, 120u8, 34u8, 58u8, 49u8, 125u8])
emit i18n.format_number("en-US", 1234.5)
emit db.values(rows)
emit telemetry.dropped()
`
	program, err := Parse("<stdlib-122-modules>", source)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []struct {
		name string
		fn   func(context.Context, *Program) ([]Value, error)
	}{
		{name: "interpreter", fn: NewRuntime().Run},
		{name: "vm", fn: NewRuntime().RunVM},
	} {
		t.Run(run.name, func(t *testing.T) {
			values, err := run.fn(context.Background(), program)
			if err != nil {
				t.Fatal(err)
			}
			if len(values) != 10 || values[7].str != "1,234.5" {
				t.Fatalf("module outputs = %#v", values)
			}
		})
	}
}

func TestStdlibConfigCompatibilityModule(t *testing.T) {
	source := `import "std/config" as config
emit config.parse_json([123u8, 34u8, 120u8, 34u8, 58u8, 49u8, 125u8])`
	program, err := Parse("<stdlib-config-compatibility>", source)
	if err != nil {
		t.Fatal(err)
	}
	values, err := NewRuntime().Run(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 {
		t.Fatalf("compatibility outputs = %#v", values)
	}
}
