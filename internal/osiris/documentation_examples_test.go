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
// example in the canonical React reference. I/O examples use a fresh explicit
// temporary-directory fixture and replace only the documented fixture marker.
func TestCanonicalDocumentationExamples(t *testing.T) {
	type item struct {
		Name    string `json:"name"`
		Example string `json:"example"`
	}
	type group struct {
		ID    string `json:"id"`
		Items []item `json:"items"`
	}
	type languageSection struct {
		Title   string `json:"title"`
		Entries []item `json:"entries"`
	}
	type constantEntry struct {
		Names   []string `json:"names"`
		Code    string   `json:"code"`
		Example string   `json:"example"`
	}
	type constantSection struct {
		Title   string          `json:"title"`
		Entries []constantEntry `json:"entries"`
	}
	type constantPage struct {
		ID       string            `json:"id"`
		Sections []constantSection `json:"sections"`
	}
	var reference struct {
		Groups []group `json:"groups"`
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "src", "reference-data.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join("..", "..", "docs", "src", "language-reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var language []languageSection
	if err := json.Unmarshal(data, &language); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join("..", "..", "docs", "src", "supplemental-reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var supplemental struct {
		Groups           []group           `json:"groups"`
		LanguageSections []languageSection `json:"languageSections"`
	}
	if err := json.Unmarshal(data, &supplemental); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join("..", "..", "docs", "src", "time-reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var timeReference group
	if err := json.Unmarshal(data, &timeReference); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join("..", "..", "docs", "src", "constant-reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var constants struct {
		Global constantPage `json:"global"`
		Errors constantPage `json:"errors"`
	}
	if err := json.Unmarshal(data, &constants); err != nil {
		t.Fatal(err)
	}
	reference.Groups = append(reference.Groups, supplemental.Groups...)
	reference.Groups = append(reference.Groups, timeReference)
	for _, page := range []constantPage{constants.Global, constants.Errors} {
		for _, section := range page.Sections {
			for _, entry := range section.Entries {
				if len(entry.Names) == 0 {
					t.Fatalf("constant reference entry in %s/%s has no names", page.ID, section.Title)
				}
				example := entry.Example
				if example == "" {
					example = "let problem = error(" + entry.Names[0] + ", \"example failure\")\nemit problem.kind # \"" + entry.Code + "\""
				}
				reference.Groups = append(reference.Groups, group{
					ID:    page.ID,
					Items: []item{{Name: entry.Names[0], Example: example}},
				})
			}
		}
	}
	language = append(language, supplemental.LanguageSections...)
	for _, section := range language {
		reference.Groups = append(reference.Groups, group{ID: "language-" + strings.ToLower(strings.ReplaceAll(section.Title, " ", "-")), Items: section.Entries})
	}
	checked := 0
	ioCandidates := 0
	checkedIO := 0
	for _, group := range reference.Groups {
		for _, item := range group.Items {
			// The reference data deliberately retains concise illustrative
			// fragments, which are executable only when they begin with emit.
			// The React-owned language JSON uses complete snippets and may begin
			// with a declaration before its observable emit.
			languageExample := strings.HasPrefix(group.ID, "language-")
			ioExample := group.ID == "io"
			if ioExample {
				ioCandidates++
			}
			if (!languageExample && !ioExample && !strings.HasPrefix(item.Example, "emit ")) ||
				((languageExample || ioExample) && !strings.Contains(item.Example, "emit ")) {
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
			if group.ID == "time" || strings.Contains(source, "time.") {
				prelude += "import \"std/time\" as time\n"
			}
			source = prelude + source
			t.Run(group.ID+"/"+item.Name, func(t *testing.T) {
				executableSource := source
				if ioExample {
					sandbox := filepath.ToSlash(t.TempDir())
					fixtures := map[string][]byte{
						"input.txt":  []byte("first line\nsecond line\n"),
						"input.bin":  {'O', 'S', 'R', '!'},
						"input.csv":  []byte("name,count\nosiris,2\n"),
						"input.json": []byte(`{"ok":true}`),
					}
					for name, data := range fixtures {
						if err := os.WriteFile(filepath.Join(filepath.FromSlash(sandbox), name), data, 0o600); err != nil {
							t.Fatal(err)
						}
					}
					executableSource = strings.ReplaceAll(executableSource, "__OSIRIS_DOC_TMP__", sandbox)
					if strings.Contains(executableSource, "__OSIRIS_DOC_TMP__") {
						t.Fatal("I/O documentation fixture marker was not resolved")
					}
				}
				prog, err := Parse("docs/"+group.ID+"/"+item.Name+".osr", executableSource)
				if err != nil {
					t.Fatalf("documented example does not parse:\n%s\n%v", executableSource, err)
				}
				if _, err := NewSession(NewRuntime(), context.Background(), "<docs>").ExecProgram(prog, false, nil); err != nil {
					t.Fatalf("documented example fails at runtime:\n%s\n%v", executableSource, err)
				}
			})
			checked++
			if ioExample {
				checkedIO++
			}
		}
	}
	if checked == 0 {
		t.Fatal("no canonical documentation examples were checked")
	}
	if checkedIO != ioCandidates {
		t.Fatalf("checked %d of %d canonical I/O examples; every I/O entry must remain executable", checkedIO, ioCandidates)
	}
}
