package osiris

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeCoreMathLinalgProviderLoweringOwnershipAndCapabilityContract(t *testing.T) {
	providerPath := filepath.Join("testdata", "linalg_provider_diagonal.c")
	tests := []struct {
		name           string
		source         string
		helper         string
		wantNoProvider string
	}{
		{
			name:           "eigh",
			source:         "import \"std/math\" as math\nemit math.eigh(math.identity(2))",
			helper:         "oeigh(",
			wantNoProvider: "eigh requires a configured native numerical provider",
		},
		{
			name:           "svd",
			source:         "import \"std/math\" as math\nemit math.svd(math.identity(2))",
			helper:         "osvd(",
			wantNoProvider: "svd requires a configured native numerical provider",
		},
		{
			name:           "eig",
			source:         "import \"std/math\" as math\nemit math.eig(math.identity(2))",
			helper:         "oeig(",
			wantNoProvider: "eig requires a configured ABI v2 native numerical provider",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program, err := Parse("<native-linalg-contract>", test.source)
			if err != nil {
				t.Fatal(err)
			}

			artifact, err := CompileCWithOptions(context.Background(), NewRuntime(), program, CCompileOptions{LinalgProviderSource: providerPath})
			if err != nil {
				t.Fatalf("provider compile failed: %v", err)
			}
			for _, needle := range []string{test.helper, "osiris_linalg_provider_v1", "olist"} {
				if !strings.Contains(artifact.Source, needle) {
					t.Fatalf("generated source is missing %q", needle)
				}
			}
			if test.name == "eig" {
				if !strings.Contains(artifact.Source, "ocomplextensordata") {
					t.Fatal("eig generated source does not use owned complex tensor results")
				}
			} else if !strings.Contains(artifact.Source, "otensordata") {
				t.Fatalf("%s generated source does not use owned tensor results", test.name)
			}

			if _, err := CompileC(context.Background(), NewRuntime(), program); err == nil || !strings.Contains(err.Error(), test.wantNoProvider) {
				t.Fatalf("without provider error = %v, want %q", err, test.wantNoProvider)
			}
		})
	}
}
