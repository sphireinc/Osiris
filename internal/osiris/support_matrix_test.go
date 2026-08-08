package osiris

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type supportMatrix struct {
	SchemaVersion int               `json:"schema_version"`
	States        map[string]string `json:"states"`
	Backends      []string          `json:"backends"`
	Features      []supportFeature  `json:"features"`
}

type supportFeature struct {
	ID            string            `json:"id"`
	Label         string            `json:"label"`
	Documentation []string          `json:"documentation"`
	Support       map[string]string `json:"support"`
	Evidence      supportEvidence   `json:"evidence"`
	Notes         string            `json:"notes"`
}

type supportEvidence struct {
	Tests []string `json:"tests"`
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate support matrix test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(here), "..", ".."))
}

func loadSupportMatrix(t *testing.T) (string, supportMatrix) {
	t.Helper()
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "docs", "src", "support-matrix.json"))
	if err != nil {
		t.Fatal(err)
	}
	var matrix supportMatrix
	if err := json.Unmarshal(data, &matrix); err != nil {
		t.Fatalf("decode docs/src/support-matrix.json: %v", err)
	}
	return root, matrix
}

func TestSupportMatrixSchema(t *testing.T) {
	root, matrix := loadSupportMatrix(t)
	if matrix.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", matrix.SchemaVersion)
	}
	validStates := map[string]bool{"stable": true, "partial": true, "experimental": true, "unsupported": true}
	if len(matrix.States) != len(validStates) {
		t.Fatalf("states = %v, want exactly %v", matrix.States, validStates)
	}
	for state := range validStates {
		if strings.TrimSpace(matrix.States[state]) == "" {
			t.Fatalf("missing description for state %q", state)
		}
	}
	if len(matrix.Backends) == 0 || len(matrix.Features) == 0 {
		t.Fatal("matrix must declare backends and documented features")
	}
	seenIDs := make(map[string]bool)
	coveredDocs := make(map[string]bool)
	for _, feature := range matrix.Features {
		if feature.ID == "" || feature.Label == "" || feature.Notes == "" {
			t.Fatalf("feature has required empty field: %#v", feature)
		}
		if seenIDs[feature.ID] {
			t.Fatalf("duplicate feature id %q", feature.ID)
		}
		seenIDs[feature.ID] = true
		for _, backend := range matrix.Backends {
			if !validStates[feature.Support[backend]] {
				t.Fatalf("feature %q has invalid or absent state for %q: %q", feature.ID, backend, feature.Support[backend])
			}
		}
		if len(feature.Documentation) == 0 || len(feature.Evidence.Tests) == 0 {
			t.Fatalf("feature %q needs documentation and evidence", feature.ID)
		}
		for _, doc := range feature.Documentation {
			if !strings.HasPrefix(doc, "https://sphireinc.github.io/Osiris/#") {
				t.Fatalf("feature %q has invalid documentation path %q", feature.ID, doc)
			}
			coveredDocs[doc] = true
		}
		for _, evidence := range feature.Evidence.Tests {
			parts := strings.SplitN(evidence, "#", 2)
			if len(parts) != 2 || !strings.HasPrefix(parts[0], "internal/osiris/") {
				t.Fatalf("feature %q has invalid test evidence %q", feature.ID, evidence)
			}
			data, err := os.ReadFile(filepath.Join(root, parts[0]))
			if err != nil || !strings.Contains(string(data), "func "+parts[1]+"(") {
				t.Fatalf("feature %q has orphaned test evidence %q", feature.ID, evidence)
			}
		}
	}
	for _, doc := range []string{"https://sphireinc.github.io/Osiris/#core", "https://sphireinc.github.io/Osiris/#language", "https://sphireinc.github.io/Osiris/#tooling", "https://sphireinc.github.io/Osiris/#chapter-errors", "https://sphireinc.github.io/Osiris/#chapter-native", "https://sphireinc.github.io/Osiris/#text", "https://sphireinc.github.io/Osiris/#math", "https://sphireinc.github.io/Osiris/#arrays", "https://sphireinc.github.io/Osiris/#tensor", "https://sphireinc.github.io/Osiris/#stats", "https://sphireinc.github.io/Osiris/#io", "https://sphireinc.github.io/Osiris/#data", "https://sphireinc.github.io/Osiris/#crypto", "https://sphireinc.github.io/Osiris/#list", "https://sphireinc.github.io/Osiris/#types"} {
		if !coveredDocs[doc] {
			t.Fatalf("documented reference page %q is absent from the matrix", doc)
		}
	}
}

func TestSupportMatrixDocumentationPage(t *testing.T) {
	root := repositoryRoot(t)
	checks := map[string]string{
		"docs/src/reference-app.jsx":         "./support-matrix.json",
		"docs/src/components/page-shell.jsx": "Backend support",
	}
	for path, required := range checks {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), required) {
			t.Fatalf("%s does not expose the canonical React support matrix route", path)
		}
	}
}

func TestSupportMatrixStableReleaseGate(t *testing.T) {
	_, matrix := loadSupportMatrix(t)
	var nonStable []string
	for _, feature := range matrix.Features {
		for _, backend := range matrix.Backends {
			state := feature.Support[backend]
			if state != "stable" {
				nonStable = append(nonStable, feature.ID+"/"+backend+"="+state)
			}
		}
	}
	if len(nonStable) != 0 {
		t.Fatalf("support matrix release gate found %d non-stable cells: %s", len(nonStable), strings.Join(nonStable, ", "))
	}
}
