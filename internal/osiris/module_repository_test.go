package osiris

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixtureRepositoryProvider struct {
	path     string
	snapshot RepositorySnapshot
	resolves []string
}

func (p *fixtureRepositoryProvider) Identity() string { return "fixture-provider-v1" }
func (p *fixtureRepositoryProvider) Supports(path string) bool {
	return path == p.path || strings.HasPrefix(path, p.path+"/")
}
func (p *fixtureRepositoryProvider) Resolve(_ context.Context, path, selector string) (RepositoryVersion, error) {
	p.resolves = append(p.resolves, selector)
	return p.snapshot.Version, nil
}
func (p *fixtureRepositoryProvider) Fetch(_ context.Context, version RepositoryVersion) (RepositorySnapshot, error) {
	p.snapshot.Version = version
	return p.snapshot, nil
}

func TestModuleManifestRoundTripIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	manifest := ModuleManifest{
		Module:       "github.com/acme/orbit",
		Requirements: []ModuleRequirement{{Path: "github.com/acme/z", Selector: "v1.2.0"}, {Path: "github.com/acme/a", Selector: "latest", Indirect: true}},
		Replacements: []ModuleReplacement{{Path: "github.com/acme/z", Target: "../z"}},
		Excludes:     []ModuleExclusion{{Path: "github.com/acme/z", Version: "v1.0.0"}},
	}
	if err := WriteModuleManifest(dir, manifest); err != nil {
		t.Fatal(err)
	}
	got, err := LoadModuleManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Module != manifest.Module || got.Source != "main.osr" || got.Requirements[0].Path != "github.com/acme/a" {
		t.Fatalf("manifest = %#v", got)
	}
	data, err := os.ReadFile(filepath.Join(dir, moduleManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "module github.com/acme/orbit") || !strings.Contains(string(data), "replace (") {
		t.Fatalf("manifest text = %s", data)
	}
}

func TestRepositoryProviderImportAndVerifiedLock(t *testing.T) {
	dir := t.TempDir()
	if _, err := ModuleInit(dir, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.osr"), []byte("import \"github.com/acme/lib\" as lib\nemit lib.answer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	version := RepositoryVersion{ModulePath: "github.com/acme/lib", Selector: "v1.2.3", Version: "v1.2.3", Tag: "v1.2.3", Commit: strings.Repeat("a", 40), Provider: "fixture-provider-v1"}
	provider := &fixtureRepositoryProvider{path: "github.com/acme/lib", snapshot: snapshotFromFiles(map[string][]byte{
		"main.osr": []byte("package lib 1.2.3\nexport answer\nlet answer = 42\n"),
	}, version)}
	rt := NewRuntime()
	rt.SetRepositoryProviders(provider)
	prog, err := Parse(filepath.Join(dir, "main.osr"), string(mustRead(t, filepath.Join(dir, "main.osr"))))
	if err != nil {
		t.Fatal(err)
	}
	out, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].String() != "42u64" {
		t.Fatalf("output = %#v", out)
	}
	if err := ModuleVerify(dir); err != nil {
		t.Fatal(err)
	}
	lockData, err := os.ReadFile(filepath.Join(dir, ".osiris.lock"))
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		Modules map[string]lockEntry `json:"modules"`
	}
	if err := json.Unmarshal(lockData, &lock); err != nil {
		t.Fatal(err)
	}
	entry, ok := lock.Modules["github.com/acme/lib"]
	if !ok {
		t.Fatalf("lock = %s", lockData)
	}
	if entry.Commit != strings.Repeat("a", 40) || entry.Provider != "fixture-provider-v1" || entry.TreeHash == "" {
		t.Fatalf("entry = %#v", entry)
	}
	if len(provider.resolves) != 1 || provider.resolves[0] != "latest" {
		t.Fatalf("resolve selectors = %#v", provider.resolves)
	}
}

func TestModuleTidyDownloadAndOfflineVerify(t *testing.T) {
	dir := t.TempDir()
	if _, err := ModuleInit(dir, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.osr"), []byte("import \"github.com/acme/lib\" as lib\nemit lib.answer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	version := RepositoryVersion{ModulePath: "github.com/acme/lib", Selector: "latest", Version: "v1.0.0", Commit: strings.Repeat("b", 40), Provider: "fixture-provider-v1"}
	provider := &fixtureRepositoryProvider{path: "github.com/acme/lib", snapshot: snapshotFromFiles(map[string][]byte{"main.osr": []byte("package lib 1.0.0\nexport answer\nlet answer = 7\n")}, version)}
	manifest, err := ModuleTidy(context.Background(), dir, provider)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Requirements) != 1 || manifest.Requirements[0].Path != "github.com/acme/lib" {
		t.Fatalf("tidy manifest = %#v", manifest)
	}
	if err := ModuleDownload(context.Background(), dir, provider); err != nil {
		t.Fatal(err)
	}
	if err := ModuleVerify(dir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OSIRIS_OFFLINE", "1")
	rt := NewRuntime()
	rt.SetRepositoryProviders(provider)
	prog, err := Parse(filepath.Join(dir, "main.osr"), string(mustRead(t, filepath.Join(dir, "main.osr"))))
	if err != nil {
		t.Fatal(err)
	}
	if out, err := rt.Run(context.Background(), prog); err != nil || len(out) != 1 || out[0].String() != "7u64" {
		t.Fatalf("offline run = %#v, %v", out, err)
	}
}

func TestSnapshotFromTarRejectsTraversalAndLinks(t *testing.T) {
	var archive bytes.Buffer
	writeTarHeader(t, &archive, "../escape", []byte("bad"), 0)
	if _, err := SnapshotFromTar(context.Background(), &archive, RepositoryVersion{}); err == nil {
		t.Fatal("traversal archive accepted")
	}
}

func TestWorkspaceMembersAreSortedAndContained(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "z")
	second := filepath.Join(dir, "a")
	for path, name := range map[string]string{first: "github.com/acme/z", second: "github.com/acme/a"} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := ModuleInit(path, name); err != nil {
			t.Fatal(err)
		}
	}
	if err := WriteWorkspaceManifest(dir, WorkspaceManifest{Version: 1, Members: []string{first, second}}); err != nil {
		t.Fatal(err)
	}
	modules, err := WorkspaceModules(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) != 2 || modules[0].Module != "github.com/acme/a" {
		t.Fatalf("modules = %#v", modules)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeTarHeader(t *testing.T, b *bytes.Buffer, name string, data []byte, typeflag byte) {
	t.Helper()
	tw := tar.NewWriter(b)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: typeflag}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
}
