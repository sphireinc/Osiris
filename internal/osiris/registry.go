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

type moduleRegistry struct {
	root     string
	lockPath string
	cacheDir string

	mu     sync.Mutex
	loaded bool
	lock   lockfile
}

func newModuleRegistry(root string) *moduleRegistry {
	if root == "" {
		root = "."
	}
	return &moduleRegistry{
		root:     root,
		lockPath: filepath.Join(root, ".osiris.lock"),
		cacheDir: filepath.Join(root, ".osiris", "cache", "modules"),
		lock: lockfile{
			Version: 2,
			Modules: make(map[string]lockEntry),
		},
	}
}

func (r *moduleRegistry) materialize(ref, base string) (string, string, error) {
	abs, kind, err := resolveModulePath(ref, base)
	if err != nil && kind != moduleKindFile {
		return "", "", err
	}
	if kind == moduleKindStdlib {
		return ref, "", nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(r.cacheDir, 0o755); err != nil {
		return "", "", err
	}

	if src, err := os.ReadFile(abs); err == nil {
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
		return "", "", fmt.Errorf("module %s is not cached and source is unavailable", abs)
	}
	src, err := os.ReadFile(entry.Cache)
	if err != nil {
		return "", "", fmt.Errorf("module %s cache missing at %s", abs, entry.Cache)
	}
	return abs + "@" + entry.Hash, string(src), nil
}

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
	if lf.Version == 0 {
		lf.Version = 2
	}
	if lf.Modules == nil {
		lf.Modules = make(map[string]lockEntry)
	}
	r.lock = lf
	r.loaded = true
	return nil
}

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

func registryEntryPath(key string) string {
	if i := strings.LastIndex(key, "@"); i > 0 {
		return key[:i]
	}
	return key
}
