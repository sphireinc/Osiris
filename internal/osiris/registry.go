package osiris

import (
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
	Modules map[string]lockEntry `json:"modules"`
}

type lockEntry struct {
	Source  string `json:"source"`
	Hash    string `json:"hash"`
	Cache   string `json:"cache"`
	Package string `json:"package,omitempty"`
	Version string `json:"version,omitempty"`
}

const moduleLockFormatVersion = 2

type moduleRegistry struct {
	root     string
	lockPath string
	cacheDir string

	mu     sync.Mutex
	loaded bool
	lock   lockfile
}

// newModuleRegistry handles the new module registry step in the surrounding
// package and returns *moduleRegistry.
//
// It returns *moduleRegistry so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func newModuleRegistry(root string) *moduleRegistry {
	if root == "" {
		root = "."
	}
	return &moduleRegistry{
		root:     root,
		lockPath: filepath.Join(root, ".osiris.lock"),
		cacheDir: filepath.Join(root, ".osiris", "cache", "modules"),
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
func (r *moduleRegistry) materialize(ref, base string) (string, string, error) {
	abs, kind, err := resolveModulePath(ref, base)
	if err != nil && kind != moduleKindFile {
		return "", "", err
	}
	if kind == moduleKindStdlib {
		return ref, "", nil
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
		r.lock.Modules[abs] = lockEntry{Source: abs, Hash: hash, Cache: cache}
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
	if lf.Version == 1 {
		// Version 2 only added optional package metadata, so a v1 lock can be
		// upgraded losslessly. The next save writes the canonical v2 form.
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
