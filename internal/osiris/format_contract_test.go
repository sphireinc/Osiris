package osiris

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCCompileManifestRejectsCorruptAndIncompatibleFormats(t *testing.T) {
	expected := &CCompileManifest{
		FormatVersion:    cCompileManifestVersion,
		Key:              "key",
		SourceSHA256:     "source",
		Compiler:         "cc",
		CompilerIdentity: "cc version",
		Flags:            []string{"-O3"},
		Target:           "native-executable/test/test",
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(`{"format_version":0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateCCompileManifest(path, expected); err == nil || !strings.Contains(err.Error(), "unsupported format version") {
		t.Fatalf("incompatible manifest error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateCCompileManifest(path, expected); err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("corrupt manifest error = %v", err)
	}
}

func TestModuleLockUpgradesV1AndRejectsUnknownFormat(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".osiris.lock"), []byte(`{"version":1,"modules":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := newModuleRegistry(dir)
	if err := registry.loadLocked(); err != nil || registry.lock.Version != moduleLockFormatVersion {
		t.Fatalf("v1 lock upgrade: version=%d err=%v", registry.lock.Version, err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".osiris.lock"), []byte(`{"version":99,"modules":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	registry = newModuleRegistry(dir)
	if err := registry.loadLocked(); err == nil || !strings.Contains(err.Error(), "unsupported .osiris.lock format version 99") {
		t.Fatalf("unknown lock error = %v", err)
	}
}

func TestModuleRegistryRejectsCorruptCachedSource(t *testing.T) {
	dir := t.TempDir()
	module := filepath.Join(dir, "module.osr")
	if err := os.WriteFile(module, []byte("package module\nexport answer\nlet answer = 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := newModuleRegistry(dir)
	if _, _, err := registry.materialize(module, module); err != nil {
		t.Fatalf("materialize source: %v", err)
	}
	entry := registry.lock.Modules[module]
	if err := os.Remove(module); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry.Cache, []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry = newModuleRegistry(dir)
	if _, _, err := registry.materialize(module, module); err == nil || !strings.Contains(err.Error(), "cache hash mismatch") {
		t.Fatalf("corrupt cache error = %v", err)
	}
}

func TestModuleRegistryRejectsRelativeTraversalAndSupportsOfflineCache(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside.osr")
	if err := os.WriteFile(outside, []byte("package outside\nexport value\nlet value = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outside)
	registry := newModuleRegistry(root)
	if _, _, err := registry.materialize(filepath.Join("..", filepath.Base(outside)), filepath.Join(root, "main.osr")); err == nil || !strings.Contains(err.Error(), "escapes project root") {
		t.Fatalf("traversal error = %v", err)
	}
	module := filepath.Join(root, "cached.osr")
	if err := os.WriteFile(module, []byte("package cached\nexport value\nlet value = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := registry.materialize(module, module); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(module); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OSIRIS_OFFLINE", "1")
	registry = newModuleRegistry(root)
	if _, source, err := registry.materialize(module, module); err != nil || !strings.Contains(source, "package cached") {
		t.Fatalf("offline cache result source=%q err=%v", source, err)
	}
	if _, _, err := registry.materialize(filepath.Join(root, "missing.osr"), module); err == nil || !strings.Contains(err.Error(), "offline module") {
		t.Fatalf("offline missing diagnostic = %v", err)
	}
}

func TestGPUArtifactJSONIsVersioned(t *testing.T) {
	artifact := GPUArtifact{SchemaVersion: gpuArtifactSchemaVersion, Source: "kernel", Kernels: []GPUKernel{{Name: "k", Kind: "test"}}}
	data, err := EncodeGPUArtifactJSON(artifact)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SchemaVersion != gpuArtifactSchemaVersion {
		t.Fatalf("GPU schema version = %d, want %d", decoded.SchemaVersion, gpuArtifactSchemaVersion)
	}
}
