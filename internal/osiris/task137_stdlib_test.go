package osiris

import (
	"context"
	"testing"
)

// TestTask137ExpandedModulesInterpreterAndVM verifies that the six expanded
// language modules are registered through both public execution paths. The
// per-module tests exercise detailed edge cases; this smoke test protects the
// integration seam and prevents an additive helper from becoming unreachable.
func TestTask137ExpandedModulesInterpreterAndVM(t *testing.T) {
	source := `import "std/crypto" as crypto
import "std/list" as list
import "std/signal" as signal
import "std/regex" as regex
import "std/json" as json
import "std/i18n" as i18n

emit crypto.digest("hello", crypto.sha256)
emit list.sum([1, 2, 3])
emit signal.mean([1, 2, 3])
emit regex.capture_count("(a)")
emit json.type_of([true])
emit i18n.format_currency("en-US", "12.50", "USD")
`
	program, err := Parse("<task-137-expanded-modules>", source)
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
			if len(values) != 6 {
				t.Fatalf("expanded module outputs = %#v", values)
			}
			if values[0].str != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
				t.Fatalf("digest = %s", values[0])
			}
			if values[4].str != "array" || values[5].str != "$12.50" {
				t.Fatalf("expanded module values = %#v", values)
			}
		})
	}
}
