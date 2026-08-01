package osiris

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFFIManifestRequiresCapabilityAndPinnedLibrary(t *testing.T) {
	dir := t.TempDir()
	library := filepath.Join(dir, "libfixture.so")
	contents := []byte("fixture")
	if err := os.WriteFile(library, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	manifest := FFIManifest{
		ABIVersion:   1,
		Library:      library,
		Version:      "1.0.0",
		Platform:     runtime.GOOS,
		Architecture: runtime.GOARCH,
		SHA256:       hex.EncodeToString(digest[:]),
		Capabilities: []string{"ffi.fixture"},
		SearchPolicy: "absolute",
		Functions:    []FFIFunction{{Name: "add", Symbol: "fixture_add", Args: []string{"i32", "i32"}, Result: "i32", Ownership: "borrowed", CallingConvention: "c", ThreadAffinity: "any"}},
	}
	if err := manifest.Validate(nil); err == nil {
		t.Fatal("manifest unexpectedly passed without a capability grant")
	}
	if err := manifest.Validate(map[string]bool{"ffi.fixture": true}); err != nil {
		t.Fatal(err)
	}
}

func TestFFIHandleReleaseIsExactlyOnce(t *testing.T) {
	handle, err := NewFFIHandle("handle:fixture", struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	releases := 0
	release := func(any) error { releases++; return nil }
	if err := handle.Close(release); err != nil {
		t.Fatal(err)
	}
	if err := handle.Close(release); err != nil {
		t.Fatal(err)
	}
	if releases != 1 {
		t.Fatalf("releases = %d, want 1", releases)
	}
}
