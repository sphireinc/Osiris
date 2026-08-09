package osiris

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeHostPOSIXFilesystemMutationSlice(t *testing.T) {
	directory := t.TempDir()
	nested := filepath.Join(directory, "parent", "child")
	file := filepath.Join(nested, "before.txt")
	renamed := filepath.Join(nested, "after.txt")
	linkTarget := filepath.Join(directory, "target.txt")
	link := filepath.Join(directory, "target.link")
	hardLink := filepath.Join(directory, "target.hard")
	createdLink := filepath.Join(directory, "target.created.link")
	if err := os.WriteFile(linkTarget, []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.txt", link); err != nil {
		t.Fatal(err)
	}

	got := runNativeHostResourceProgram(t, fmt.Sprintf(`import "std/os" as os
emit os.mkdir(%q, 493)
emit os.mkdir_all(%q, 493)
let output = os.create(%q)
emit os.write_string(output, "native")
emit os.close(output)
emit os.truncate(%q, 3)
emit os.chmod(%q, 384)
emit os.rename(%q, %q)
emit os.remove(%q)
emit os.remove_all(%q)
emit os.readlink(%q)
emit os.link(%q, %q)
emit os.symlink("target.txt", %q)
emit os.readlink(%q)
`, filepath.Join(directory, "single"), nested, file, file, file, file, renamed, renamed, filepath.Join(directory, "parent"), link, linkTarget, hardLink, createdLink, createdLink),
		"os.file_mutate", "os.file_write", "os.file_read", "os.symlink")

	if !strings.Contains(got, "6u64") || !strings.Contains(got, `"target.txt"`) {
		t.Fatalf("native mutation output = %q, want write count and readlink target", got)
	}
	if _, err := os.Stat(filepath.Join(directory, "single")); err != nil {
		t.Fatalf("mkdir result missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "parent")); !os.IsNotExist(err) {
		t.Fatalf("remove_all left parent directory: err=%v", err)
	}
}

func TestNativeHostPOSIXFilesystemMutationCapabilityGate(t *testing.T) {
	program, err := Parse("nativeHost-native-filesystem-mutation-capability.osr", `import "std/os" as os
emit os.mkdir_all("/tmp/osiris-nativeHost-missing-capability", 493)
`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileCWithOptions(context.Background(), NewRuntime(), program, CCompileOptions{}); err == nil || !strings.Contains(err.Error(), `native capability "os.file_mutate"`) {
		t.Fatalf("CompileC error = %v, want os.file_mutate capability diagnostic", err)
	}
}
