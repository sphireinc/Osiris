package osiris

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestManifestPinnedNativeFFIScalarCalls(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("fixture build is maintained on macOS and Linux")
	}
	compiler, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("C compiler is unavailable")
	}
	root := t.TempDir()
	fixture := filepath.Join(root, "libosiris_ffi_fixture")
	args := []string{"-shared", "-fPIC", "-o", fixture, filepath.Join("testdata", "ffi_fixture.c")}
	if runtime.GOOS == "darwin" {
		fixture += ".dylib"
		args = []string{"-dynamiclib", "-o", fixture, filepath.Join("testdata", "ffi_fixture.c")}
	}
	command := exec.Command(compiler, args...)
	command.Dir, _ = os.Getwd()
	if output, err := command.CombinedOutput(); err != nil {
		t.Skipf("fixture compiler unavailable: %v (%s)", err, output)
	}
	libraryBytes, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(libraryBytes)
	manifestPath := filepath.Join(root, "ffi.json")
	manifest := FFIManifest{
		ABIVersion: 1, Library: fixture, Version: "1.0.0", Platform: runtime.GOOS,
		Architecture: runtime.GOARCH, SHA256: hex.EncodeToString(digest[:]),
		Capabilities: []string{"ffi.test"}, SearchPolicy: "absolute",
		Functions: []FFIFunction{
			{Name: "add_i64", Symbol: "osiris_ffi_add_i64", Args: []string{"i64", "i64"}, Result: "i64", Ownership: "borrowed", CallingConvention: "c", ThreadAffinity: "any"},
			{Name: "mul_f64", Symbol: "osiris_ffi_mul_f64", Args: []string{"f64", "f64"}, Result: "f64", Ownership: "borrowed", CallingConvention: "c", ThreadAffinity: "any"},
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	library, err := LoadFFILibrary(manifestPath, map[string]bool{"ffi.test": true})
	if err != nil {
		t.Fatal(err)
	}
	defer library.Close()
	integer, err := library.Call("add_i64", []Value{intValue(bigIntForTest(20), 64), intValue(bigIntForTest(22), 64)})
	if err != nil || integer.String() != "42i64" {
		t.Fatalf("integer call = %s, err = %v", integer.String(), err)
	}
	product, err := library.Call("mul_f64", []Value{numberValue(1.5), numberValue(2.0)})
	if err != nil || math.Abs(product.num-3.0) > 1e-12 {
		t.Fatalf("float call = %s, err = %v", product.String(), err)
	}
}

func bigIntForTest(value int64) *big.Int { return big.NewInt(value) }
