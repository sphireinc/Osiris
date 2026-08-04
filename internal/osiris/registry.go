package osiris

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type lockfile struct {
	Version int                  `json:"version"`
	Module  string               `json:"module,omitempty"`
	Modules map[string]lockEntry `json:"modules"`
}

type lockEntry struct {
	Source         string   `json:"source"`
	Hash           string   `json:"hash"`
	Cache          string   `json:"cache"`
	Package        string   `json:"package,omitempty"`
	Version        string   `json:"version,omitempty"`
	ModulePath     string   `json:"module_path,omitempty"`
	Selector       string   `json:"selector,omitempty"`
	Tag            string   `json:"tag,omitempty"`
	Commit         string   `json:"commit,omitempty"`
	TreeHash       string   `json:"tree_hash,omitempty"`
	Provider       string   `json:"provider,omitempty"`
	Dependencies   []string `json:"dependencies,omitempty"`
	ManifestDigest string   `json:"manifest_digest,omitempty"`
}

const moduleLockFormatVersion = 3

type moduleRegistry struct {
	root         string
	lockPath     string
	cacheDir     string
	providers    []RepositoryProvider
	replacements map[string]ModuleReplacement

	mu     sync.Mutex
	loaded bool
	lock   lockfile
}

// newModuleRegistry handles the new module registry step in the surrounding
// package and returns *moduleRegistry.
//
// It returns *moduleRegistry so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func newModuleRegistry(root string, providers ...RepositoryProvider) *moduleRegistry {
	if root == "" {
		root = "."
	}
	return &moduleRegistry{
		root:         root,
		lockPath:     filepath.Join(root, ".osiris.lock"),
		cacheDir:     filepath.Join(root, ".osiris", "cache", "modules"),
		providers:    append([]RepositoryProvider(nil), providers...),
		replacements: make(map[string]ModuleReplacement),
		lock: lockfile{
			Version: moduleLockFormatVersion,
			Modules: make(map[string]lockEntry),
		},
	}
}

// materialize operates on moduleRegistry values and keeps the
// moduleregistry-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns string, string, error so callers can continue with the next stage
// of the pipeline without re-deriving the same information.
// materialize preserves the pre-task-124 internal helper contract used by
// existing tests and embedders. New resolution paths should use
// materializeContext so cancellation and selectors are retained.
func (r *moduleRegistry) materialize(ref, base string) (string, string, error) {
	return r.materializeContext(context.Background(), ref, base)
}

func (r *moduleRegistry) materializeContext(ctx context.Context, ref, base string, requested ...string) (string, string, error) {
	abs, kind, err := resolveModulePath(ref, base)
	if err != nil && kind != moduleKindFile && kind != moduleKindRepository {
		return "", "", err
	}
	if kind == moduleKindStdlib {
		return ref, "", nil
	}
	if kind == moduleKindRepository {
		return r.materializeRepository(ctx, ref, requested...)
	}
	if !filepath.IsAbs(ref) && !pathWithinRoot(r.root, abs) {
		return "", "", fmt.Errorf("module path %q escapes project root %q", ref, r.root)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(r.cacheDir, 0o755); err != nil {
		return "", "", err
	}

	offline := os.Getenv("OSIRIS_OFFLINE") == "1"
	if src, err := os.ReadFile(abs); err == nil && !offline {
		sum := sha256.Sum256(src)
		hash := hex.EncodeToString(sum[:])
		cache := filepath.Join(r.cacheDir, hash+".osr")
		if _, err := os.Stat(cache); err != nil {
			if err := os.WriteFile(cache, src, 0o644); err != nil {
				return "", "", err
			}
		}
		entry := r.lock.Modules[abs]
		entry.Source = abs
		entry.Hash = hash
		entry.Cache = cache
		r.lock.Modules[abs] = entry
		if err := r.saveLocked(); err != nil {
			return "", "", err
		}
		return abs + "@" + hash, string(src), nil
	}

	entry, ok := r.lock.Modules[abs]
	if !ok {
		if offline {
			return "", "", fmt.Errorf("offline module %s is not present in the lockfile/cache", abs)
		}
		return "", "", fmt.Errorf("module %s is not cached and source is unavailable", abs)
	}
	src, err := os.ReadFile(entry.Cache)
	if err != nil {
		return "", "", fmt.Errorf("module %s cache missing at %s", abs, entry.Cache)
	}
	sum := sha256.Sum256(src)
	if got := hex.EncodeToString(sum[:]); got != entry.Hash {
		return "", "", fmt.Errorf("module %s cache hash mismatch: got %s, lock expects %s", abs, got, entry.Hash)
	}
	return abs + "@" + entry.Hash, string(src), nil
}

func (r *moduleRegistry) materializeRepository(ctx context.Context, modulePath string, requested ...string) (string, string, error) {
	if replacement, ok := r.replacements[modulePath]; ok && replacement.Target != "" {
		return r.materializeContext(ctx, replacement.Target, r.root)
	}
	selector := "latest"
	if len(requested) > 0 && requested[0] != "" {
		selector = requested[0]
	}
	r.mu.Lock()
	if err := r.loadLocked(); err != nil {
		r.mu.Unlock()
		return "", "", err
	}
	entry, locked := r.lock.Modules[modulePath]
	if locked && entry.Selector != "" && (len(requested) == 0 || requested[0] == "") {
		selector = entry.Selector
	}
	r.mu.Unlock()
	if locked && entry.Cache != "" && entry.Version != "" && (len(requested) == 0 || requested[0] == "" || entry.Selector == selector) {
		if data, err := os.ReadFile(entry.Cache); err == nil && digestMatches(data, entry.Hash) && (selector == "latest" || versionMatchesSelector(entry.Version, selector) || entry.Commit == selector) {
			return modulePath + "@" + entry.Version, string(data), nil
		}
	}
	if os.Getenv("OSIRIS_OFFLINE") == "1" {
		if !locked || entry.Cache == "" {
			return "", "", fmt.Errorf("offline module %s is not present in the lockfile/cache", modulePath)
		}
		if selector != "latest" && !versionMatchesSelector(entry.Version, selector) && entry.Commit != selector {
			return "", "", fmt.Errorf("locked module %s version %s does not satisfy %q", modulePath, entry.Version, selector)
		}
		data, err := os.ReadFile(entry.Cache)
		if err != nil {
			return "", "", fmt.Errorf("module %s cache missing at %s: %w", modulePath, entry.Cache, err)
		}
		if !digestMatches(data, entry.Hash) {
			return "", "", fmt.Errorf("module %s cache hash mismatch", modulePath)
		}
		return modulePath + "@" + entry.Version, string(data), nil
	}
	provider := r.providerFor(modulePath)
	if provider == nil {
		return "", "", fmt.Errorf("no repository provider supports %q", modulePath)
	}
	version, err := provider.Resolve(ctx, modulePath, selector)
	if err != nil {
		return "", "", err
	}
	snapshot, err := provider.Fetch(ctx, version)
	if err != nil {
		return "", "", err
	}
	sourceName, source, err := repositorySource(snapshot, modulePath)
	if err != nil {
		return "", "", err
	}
	hash := sha256.Sum256(source)
	sourceHash := hex.EncodeToString(hash[:])
	cacheName := sha256.Sum256([]byte(modulePath + "\x00" + version.Commit + "\x00" + sourceName))
	cache := filepath.Join(r.cacheDir, hex.EncodeToString(cacheName[:])+".osr")
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.MkdirAll(r.cacheDir, 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(cache, source, 0o644); err != nil {
		return "", "", err
	}
	entry = lockEntry{Source: modulePath, Hash: sourceHash, Cache: cache, ModulePath: modulePath, Selector: version.Selector, Version: version.Version, Tag: version.Tag, Commit: version.Commit, TreeHash: snapshot.TreeDigest, Provider: version.Provider}
	if r.lock.Modules == nil {
		r.lock.Modules = make(map[string]lockEntry)
	}
	r.lock.Modules[modulePath] = entry
	if err := r.saveLocked(); err != nil {
		return "", "", err
	}
	return modulePath + "@" + version.Version, string(source), nil
}

func (r *moduleRegistry) providerFor(modulePath string) RepositoryProvider {
	for _, provider := range r.providers {
		if provider != nil && provider.Supports(modulePath) {
			return provider
		}
	}
	if strings.HasPrefix(modulePath, "github.com/") {
		return NewGitHubProvider()
	}
	return nil
}

func repositorySource(snapshot RepositorySnapshot, modulePath string) (string, []byte, error) {
	parts := strings.Split(modulePath, "/")
	if len(parts) < 3 {
		return "", nil, fmt.Errorf("invalid repository module path %q", modulePath)
	}
	entry := "main.osr"
	if data, ok := snapshot.Files["osiris.mod"]; ok {
		var manifest ModuleManifest
		if err := parseManifestText(string(data), &manifest); err == nil {
			entry = ensureManifestSource(manifest)
		}
	}
	if len(parts) > 3 {
		suffix := strings.Join(parts[3:], "/")
		candidates := []string{suffix + ".osr", suffix + "/main.osr", suffix + "/index.osr"}
		for _, candidate := range candidates {
			if _, ok := snapshot.Files[candidate]; ok {
				entry = candidate
				break
			}
		}
	}
	data, ok := snapshot.Files[entry]
	if !ok {
		return "", nil, fmt.Errorf("repository module %s has no source entry %q", modulePath, entry)
	}
	return entry, data, nil
}

func digestMatches(data []byte, want string) bool {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) == want
}

func pathWithinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// recordManifest operates on moduleRegistry values and keeps the
// moduleregistry-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns an error when the operation cannot complete cleanly.
func (r *moduleRegistry) recordManifest(key, packageName, version string) error {
	if packageName == "" && version == "" {
		return nil
	}
	abs := registryEntryPath(key)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return err
	}
	entry := r.lock.Modules[abs]
	entry.Source = abs
	entry.Package = packageName
	entry.Version = version
	r.lock.Modules[abs] = entry
	return r.saveLocked()
}

// loadLocked operates on moduleRegistry values and keeps the
// moduleregistry-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns an error when the operation cannot complete cleanly.
func (r *moduleRegistry) loadLocked() error {
	if r.loaded {
		return nil
	}
	data, err := os.ReadFile(r.lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			r.loaded = true
			return nil
		}
		return err
	}
	var lf lockfile
	if err := json.Unmarshal(data, &lf); err != nil {
		return err
	}
	if lf.Version == 1 || lf.Version == 2 {
		// Versions 1 and 2 contain the same source/hash/cache authority. Version
		// 3 adds repository provenance and selector metadata.
		lf.Version = moduleLockFormatVersion
	} else if lf.Version != moduleLockFormatVersion {
		return fmt.Errorf("unsupported .osiris.lock format version %d (expected %d)", lf.Version, moduleLockFormatVersion)
	}
	if lf.Modules == nil {
		lf.Modules = make(map[string]lockEntry)
	}
	r.lock = lf
	r.loaded = true
	return nil
}

// saveLocked operates on moduleRegistry values and keeps the
// moduleregistry-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns an error when the operation cannot complete cleanly.
func (r *moduleRegistry) saveLocked() error {
	data, err := json.MarshalIndent(r.lock, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.lockPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(r.lockPath, append(data, '\n'), 0o644)
}

// registryRootFor handles the registry root for step in the surrounding
// package and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func registryRootFor(base string) string {
	start := base
	if start == "" || strings.HasPrefix(start, "<") {
		if cwd, err := os.Getwd(); err == nil {
			start = cwd
		}
	} else if info, err := os.Stat(start); err != nil || !info.IsDir() {
		start = filepath.Dir(start)
	}
	if start == "" {
		return "."
	}
	dir := start
	dir, _ = filepath.Abs(dir)
	root := dir
	for {
		if _, err := os.Stat(filepath.Join(dir, ".osiris.lock")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return root
		}
		dir = parent
	}
}

// ModuleStatePaths returns the generated lockfile and module-cache locations
// associated with a source path. Callers can use it for explicit lifecycle
// commands without reconstructing registry path rules.
func ModuleStatePaths(sourceName string) (lockPath, cacheDir string) {
	root := registryRootFor(sourceName)
	return filepath.Join(root, ".osiris.lock"), filepath.Join(root, ".osiris", "cache", "modules")
}

// registryEntryPath handles the registry entry path step in the surrounding
// package and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func registryEntryPath(key string) string {
	if i := strings.LastIndex(key, "@"); i > 0 {
		return key[:i]
	}
	return key
}
