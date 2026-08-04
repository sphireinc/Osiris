package osiris

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTask136DiscoversColocatedSidecarFromProductionFile(t *testing.T) {
	dir := t.TempDir()
	production := filepath.Join(dir, "todo.osr")
	testFile := filepath.Join(dir, "todo.test.osr")
	if err := os.WriteFile(production, []byte("func add(a, b) { return a + b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testFile, []byte("// osiris:test name=adds\nemit add(2, 3)\n// osiris:assert output[0] == 5u64\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := DiscoverLanguageTests([]string{production})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != testFile {
		t.Fatalf("files = %#v, want %q", files, testFile)
	}
	results, err := RunLanguageTests(nil, []string{production}, LanguageTestOptions{Backend: "interpreter", Deterministic: true, NoCache: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("results = %#v", results)
	}
}

func TestTask136DefaultAndDirectoryDiscoveryRetainCompatibilitySuffix(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.osr", "a.test.osr", "b.osr", "b_test.osr", "ignored.osr"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("emit 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := DiscoverLanguageTests([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || filepath.Base(files[0]) != "a.test.osr" || filepath.Base(files[1]) != "b_test.osr" {
		t.Fatalf("files = %#v", files)
	}
}
