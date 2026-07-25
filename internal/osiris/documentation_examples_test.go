package osiris

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCanonicalDocumentationExamples executes every self-contained `emit`
// example in the canonical React reference. Examples in the I/O group are
// intentionally excluded: they require a sandbox fixture and are documented
// as host interaction rather than runnable snippets (task 040's non-goal).
func TestCanonicalDocumentationExamples(t *testing.T) {
	type item struct {
		Name    string `json:"name"`
		Example string `json:"example"`
	}
	type group struct {
		ID    string `json:"id"`
		Items []item `json:"items"`
	}
	var reference struct {
		Groups []group `json:"groups"`
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "newdocs", "src", "legacy-reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, group := range reference.Groups {
		if group.ID == "io" {
			continue
		}
		for _, item := range group.Items {
			if !strings.HasPrefix(item.Example, "emit ") {
				continue
			}
			source := item.Example + "\n"
			prelude := ""
			if group.ID == "text" || strings.Contains(source, "text.") {
				prelude += "import \"std/text\" as text\n"
			}
			if strings.Contains(source, "math.") {
				prelude += "import \"std/math\" as math\n"
			}
			source = prelude + source
			t.Run(group.ID+"/"+item.Name, func(t *testing.T) {
				prog, err := Parse("newdocs/"+group.ID+"/"+item.Name+".osr", source)
				if err != nil {
					t.Fatalf("documented example does not parse:\n%s\n%v", source, err)
				}
				if _, err := NewSession(NewRuntime(), context.Background(), "<docs>").ExecProgram(prog, false, nil); err != nil {
					t.Fatalf("documented example fails at runtime:\n%s\n%v", source, err)
				}
			})
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no canonical documentation examples were checked")
	}
}
