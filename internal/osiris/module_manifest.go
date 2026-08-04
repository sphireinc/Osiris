package osiris

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	moduleManifestFile    = "osiris.mod"
	workspaceManifestFile = "osiris.work"
	moduleManifestVersion = 1
)

// ModuleRequirement is a human-authored module dependency. Selector accepts
// latest, a semantic version, a compatible major/minor selector, or a full
// commit SHA.
type ModuleRequirement struct {
	Path     string `json:"path"`
	Selector string `json:"selector"`
	Indirect bool   `json:"indirect,omitempty"`
}

// ModuleReplacement maps a canonical import path to a local directory or an
// explicitly pinned repository source. Replacements never change import
// identity.
type ModuleReplacement struct {
	Path    string `json:"path"`
	Target  string `json:"target"`
	Version string `json:"version,omitempty"`
}

// ModuleExclusion prevents a selector from being selected for a module.
type ModuleExclusion struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

// ModuleManifest is the checked-in, human-authored repository contract.
// .osiris.lock and .osiris/cache are generated and must not be used as a
// substitute for this file.
type ModuleManifest struct {
	Version      int                 `json:"version"`
	Module       string              `json:"module"`
	Source       string              `json:"source,omitempty"`
	Requirements []ModuleRequirement `json:"requirements,omitempty"`
	Replacements []ModuleReplacement `json:"replacements,omitempty"`
	Excludes     []ModuleExclusion   `json:"excludes,omitempty"`
}

// WorkspaceManifest describes a deterministic multi-module workspace.
type WorkspaceManifest struct {
	Version      int                 `json:"version"`
	Members      []string            `json:"members"`
	Replacements []ModuleReplacement `json:"replacements,omitempty"`
}

func moduleManifestPath(root string) string    { return filepath.Join(root, moduleManifestFile) }
func workspaceManifestPath(root string) string { return filepath.Join(root, workspaceManifestFile) }

// ModuleRootFor finds the nearest repository root containing osiris.mod or
// osiris.work. It returns the absolute working directory when no manifest is
// present so existing source-only projects remain usable.
func ModuleRootFor(start string) string {
	if start == "" || strings.HasPrefix(start, "<") {
		start, _ = os.Getwd()
	} else if info, err := os.Stat(start); err == nil && !info.IsDir() {
		start = filepath.Dir(start)
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return start
	}
	for dir := abs; ; dir = filepath.Dir(dir) {
		if fileExists(filepath.Join(dir, moduleManifestFile)) || fileExists(filepath.Join(dir, workspaceManifestFile)) || dir == abs {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return abs
		}
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// LoadModuleManifest reads and validates a canonical osiris.mod file.
func LoadModuleManifest(root string) (ModuleManifest, error) {
	root = ModuleRootFor(root)
	return parseModuleManifest(moduleManifestPath(root))
}

func parseModuleManifest(path string) (ModuleManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ModuleManifest{}, err
	}
	var manifest ModuleManifest
	if err := parseManifestText(string(data), &manifest); err != nil {
		return ModuleManifest{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := validateModuleManifest(manifest); err != nil {
		return ModuleManifest{}, fmt.Errorf("validate %s: %w", path, err)
	}
	if manifest.Source == "" {
		manifest.Source = "main.osr"
	}
	return manifest, nil
}

// WriteModuleManifest writes a stable, human-readable manifest. Existing
// requirements and replacements are sorted and de-duplicated before writing.
func WriteModuleManifest(root string, manifest ModuleManifest) error {
	if err := validateModuleManifest(manifest); err != nil {
		return err
	}
	manifest = canonicalManifest(manifest)
	var b strings.Builder
	fmt.Fprintf(&b, "module %s\n", manifest.Module)
	if manifest.Source != "" && manifest.Source != "main.osr" {
		fmt.Fprintf(&b, "source %s\n", manifest.Source)
	}
	if len(manifest.Requirements) > 0 {
		b.WriteString("\nrequire (\n")
		for _, req := range manifest.Requirements {
			fmt.Fprintf(&b, "\t%s %s", req.Path, req.Selector)
			if req.Indirect {
				b.WriteString(" // indirect")
			}
			b.WriteByte('\n')
		}
		b.WriteString(")\n")
	}
	if len(manifest.Replacements) > 0 {
		b.WriteString("\nreplace (\n")
		for _, replacement := range manifest.Replacements {
			fmt.Fprintf(&b, "\t%s => %s", replacement.Path, replacement.Target)
			if replacement.Version != "" {
				fmt.Fprintf(&b, " %s", replacement.Version)
			}
			b.WriteByte('\n')
		}
		b.WriteString(")\n")
	}
	if len(manifest.Excludes) > 0 {
		b.WriteString("\nexclude (\n")
		for _, exclusion := range manifest.Excludes {
			fmt.Fprintf(&b, "\t%s %s\n", exclusion.Path, exclusion.Version)
		}
		b.WriteString(")\n")
	}
	return os.WriteFile(moduleManifestPath(root), []byte(b.String()), 0o644)
}

func parseManifestText(data string, out *ModuleManifest) error {
	scanner := bufio.NewScanner(strings.NewReader(data))
	section := ""
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if i := strings.Index(raw, "//"); i >= 0 {
			raw = strings.TrimSpace(raw[:i])
		}
		if raw == "" {
			continue
		}
		if raw == "require (" || raw == "replace (" || raw == "exclude (" {
			section = strings.TrimSuffix(raw, " (")
			continue
		}
		if raw == ")" {
			section = ""
			continue
		}
		fields := strings.Fields(raw)
		if len(fields) < 2 {
			return fmt.Errorf("line %d: expected directive and value", line)
		}
		if section == "" {
			switch fields[0] {
			case "module":
				if len(fields) != 2 {
					return fmt.Errorf("line %d: module expects one path", line)
				}
				out.Module = fields[1]
			case "source":
				if len(fields) != 2 {
					return fmt.Errorf("line %d: source expects one path", line)
				}
				out.Source = fields[1]
			case "osiris":
				if len(fields) != 2 || fields[1] != "1" {
					return fmt.Errorf("line %d: unsupported manifest version", line)
				}
				out.Version = 1
			default:
				return fmt.Errorf("line %d: unknown directive %q", line, fields[0])
			}
			continue
		}
		switch section {
		case "require":
			if len(fields) < 2 || len(fields) > 3 {
				return fmt.Errorf("line %d: invalid require", line)
			}
			req := ModuleRequirement{Path: fields[0], Selector: fields[1]}
			if len(fields) == 3 && fields[2] == "indirect" {
				req.Indirect = true
			}
			out.Requirements = append(out.Requirements, req)
		case "replace":
			if len(fields) < 3 || fields[1] != "=>" || len(fields) > 4 {
				return fmt.Errorf("line %d: invalid replace", line)
			}
			replacement := ModuleReplacement{Path: fields[0], Target: fields[2]}
			if len(fields) == 4 {
				replacement.Version = fields[3]
			}
			out.Replacements = append(out.Replacements, replacement)
		case "exclude":
			if len(fields) != 2 {
				return fmt.Errorf("line %d: invalid exclude", line)
			}
			out.Excludes = append(out.Excludes, ModuleExclusion{Path: fields[0], Version: fields[1]})
		default:
			return fmt.Errorf("line %d: unknown section %q", line, section)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if out.Version == 0 {
		out.Version = moduleManifestVersion
	}
	return nil
}

func validateModuleManifest(manifest ModuleManifest) error {
	if manifest.Version != 0 && manifest.Version != moduleManifestVersion {
		return fmt.Errorf("unsupported osiris.mod version %d", manifest.Version)
	}
	if !validModulePath(manifest.Module) {
		return fmt.Errorf("module path %q is not canonical", manifest.Module)
	}
	for _, req := range manifest.Requirements {
		if !validModulePath(req.Path) || strings.TrimSpace(req.Selector) == "" {
			return fmt.Errorf("invalid requirement %q", req.Path)
		}
	}
	for _, replacement := range manifest.Replacements {
		if !validModulePath(replacement.Path) || strings.TrimSpace(replacement.Target) == "" {
			return fmt.Errorf("invalid replacement %q", replacement.Path)
		}
	}
	for _, exclusion := range manifest.Excludes {
		if !validModulePath(exclusion.Path) || strings.TrimSpace(exclusion.Version) == "" {
			return fmt.Errorf("invalid exclusion %q", exclusion.Path)
		}
	}
	return nil
}

func validModulePath(path string) bool {
	if path == "" || strings.HasPrefix(path, ".") || strings.ContainsAny(path, "\\\t\r\n") {
		return false
	}
	parts := strings.Split(path, "/")
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return false
	}
	for _, part := range parts {
		if part == "." || part == ".." || strings.Contains(part, "@") {
			return false
		}
	}
	return true
}

func canonicalManifest(manifest ModuleManifest) ModuleManifest {
	manifest.Version = moduleManifestVersion
	manifest.Source = strings.TrimSpace(manifest.Source)
	if manifest.Source == "" {
		manifest.Source = "main.osr"
	}
	manifest.Requirements = append([]ModuleRequirement(nil), manifest.Requirements...)
	sort.Slice(manifest.Requirements, func(i, j int) bool {
		if manifest.Requirements[i].Path != manifest.Requirements[j].Path {
			return manifest.Requirements[i].Path < manifest.Requirements[j].Path
		}
		return manifest.Requirements[i].Selector < manifest.Requirements[j].Selector
	})
	manifest.Replacements = append([]ModuleReplacement(nil), manifest.Replacements...)
	sort.Slice(manifest.Replacements, func(i, j int) bool { return manifest.Replacements[i].Path < manifest.Replacements[j].Path })
	manifest.Excludes = append([]ModuleExclusion(nil), manifest.Excludes...)
	sort.Slice(manifest.Excludes, func(i, j int) bool {
		if manifest.Excludes[i].Path != manifest.Excludes[j].Path {
			return manifest.Excludes[i].Path < manifest.Excludes[j].Path
		}
		return manifest.Excludes[i].Version < manifest.Excludes[j].Version
	})
	return manifest
}

// LoadWorkspaceManifest reads osiris.work and resolves members relative to
// the workspace root. Members are sorted and must remain inside the workspace.
func LoadWorkspaceManifest(root string) (WorkspaceManifest, error) {
	root = ModuleRootFor(root)
	data, err := os.ReadFile(workspaceManifestPath(root))
	if err != nil {
		return WorkspaceManifest{}, err
	}
	var manifest WorkspaceManifest
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if i := strings.Index(line, "//"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		if line == "use (" || line == "replace (" {
			section = strings.TrimSuffix(line, " (")
			continue
		}
		if line == ")" {
			section = ""
			continue
		}
		fields := strings.Fields(line)
		if section == "" {
			if len(fields) == 2 && fields[0] == "osiris" && fields[1] == "1" {
				manifest.Version = 1
				continue
			}
			return WorkspaceManifest{}, fmt.Errorf("invalid workspace directive %q", line)
		}
		if section == "use" && len(fields) == 1 {
			manifest.Members = append(manifest.Members, filepath.Clean(fields[0]))
			continue
		}
		if section == "replace" && len(fields) >= 3 && fields[1] == "=>" {
			replacement := ModuleReplacement{Path: fields[0], Target: fields[2]}
			if len(fields) == 4 {
				replacement.Version = fields[3]
			}
			manifest.Replacements = append(manifest.Replacements, replacement)
			continue
		}
		return WorkspaceManifest{}, fmt.Errorf("invalid workspace line %q", line)
	}
	if err := scanner.Err(); err != nil {
		return WorkspaceManifest{}, err
	}
	if manifest.Version != 1 {
		return WorkspaceManifest{}, fmt.Errorf("unsupported osiris.work version %d", manifest.Version)
	}
	for i, member := range manifest.Members {
		abs, err := filepath.Abs(filepath.Join(root, member))
		if err != nil {
			return WorkspaceManifest{}, err
		}
		if !pathWithinRoot(root, abs) {
			return WorkspaceManifest{}, fmt.Errorf("workspace member %q escapes root", member)
		}
		manifest.Members[i] = filepath.Clean(abs)
	}
	sort.Strings(manifest.Members)
	return manifest, nil
}

// WriteWorkspaceManifest writes a stable workspace manifest.
func WriteWorkspaceManifest(root string, manifest WorkspaceManifest) error {
	if manifest.Version == 0 {
		manifest.Version = 1
	}
	if manifest.Version != 1 {
		return fmt.Errorf("unsupported osiris.work version %d", manifest.Version)
	}
	root = ModuleRootFor(root)
	members := append([]string(nil), manifest.Members...)
	sort.Strings(members)
	var b strings.Builder
	b.WriteString("osiris 1\n\nuse (\n")
	for _, member := range members {
		rel, err := filepath.Rel(root, member)
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "\t%s\n", filepath.ToSlash(rel))
	}
	b.WriteString(")\n")
	return os.WriteFile(workspaceManifestPath(root), []byte(b.String()), 0o644)
}

// ModuleInit creates a minimal module manifest and refuses to overwrite an
// existing one.
func ModuleInit(root, modulePath string) (ModuleManifest, error) {
	root = ModuleRootFor(root)
	if fileExists(moduleManifestPath(root)) {
		return ModuleManifest{}, fmt.Errorf("%s already exists", moduleManifestPath(root))
	}
	manifest := ModuleManifest{Version: 1, Module: modulePath, Source: "main.osr"}
	if err := WriteModuleManifest(root, manifest); err != nil {
		return ModuleManifest{}, err
	}
	return manifest, nil
}

// ModuleManifestDigest is the canonical digest used in audit output and lock
// metadata. It excludes filesystem metadata and serializes sorted fields.
func ModuleManifestDigest(manifest ModuleManifest) string {
	data, _ := json.Marshal(canonicalManifest(manifest))
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ModuleTidy derives direct and transitive repository requirements from source
// imports without executing dependency code. Local relative imports are left
// out because they are source files, not module requirements.
func ModuleTidy(ctx context.Context, root string, providers ...RepositoryProvider) (ModuleManifest, error) {
	manifest, err := LoadModuleManifest(root)
	if err != nil {
		return ModuleManifest{}, err
	}
	root = ModuleRootFor(root)
	sourcePath := filepath.Join(root, manifest.Source)
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return ModuleManifest{}, err
	}
	rt := NewRuntime()
	rt.SetRepositoryProviders(providers...)
	report, err := rt.BuildDependencyReport(sourcePath, string(data))
	if err != nil {
		return ModuleManifest{}, err
	}
	reqs := make(map[string]ModuleRequirement)
	for ref := range report.Nodes {
		ref = registryEntryPath(ref)
		if strings.HasPrefix(ref, "std/") || filepath.IsAbs(ref) || strings.HasPrefix(ref, "<") {
			continue
		}
		if !validModulePath(ref) {
			continue
		}
		reqs[ref] = ModuleRequirement{Path: ref, Selector: "latest"}
	}
	manifest.Requirements = manifest.Requirements[:0]
	for _, req := range reqs {
		manifest.Requirements = append(manifest.Requirements, req)
	}
	manifest = canonicalManifest(manifest)
	if err := WriteModuleManifest(root, manifest); err != nil {
		return ModuleManifest{}, err
	}
	return manifest, nil
}

// ModuleDownload resolves every manifest requirement and writes verified source
// entries into the generated lock/cache state. It does not parse or execute
// dependency programs.
func ModuleDownload(ctx context.Context, root string, providers ...RepositoryProvider) error {
	manifest, err := LoadModuleManifest(root)
	if err != nil {
		return err
	}
	root = ModuleRootFor(root)
	rt := NewRuntime()
	rt.SetRepositoryProviders(providers...)
	reg := rt.registryFor(root)
	reg.mu.Lock()
	if err := reg.loadLocked(); err != nil {
		reg.mu.Unlock()
		return err
	}
	reg.lock.Module = manifest.Module
	reg.mu.Unlock()
	for _, requirement := range manifest.Requirements {
		if requirement.Selector == "none" {
			continue
		}
		reg.mu.Lock()
		if entry, ok := reg.lock.Modules[requirement.Path]; ok && entry.Selector != requirement.Selector {
			entry.Selector = requirement.Selector
			entry.Version, entry.Tag, entry.Commit, entry.Hash, entry.Cache = "", "", "", "", ""
			reg.lock.Modules[requirement.Path] = entry
		}
		reg.mu.Unlock()
		if _, _, err := reg.materializeContext(ctx, requirement.Path, root, requirement.Selector); err != nil {
			return err
		}
	}
	reg.mu.Lock()
	defer reg.mu.Unlock()
	return reg.saveLocked()
}

// ModuleVerify validates every cached source digest and the lock's canonical
// repository metadata without invoking Git or executing package code.
func ModuleVerify(root string) error {
	root = ModuleRootFor(root)
	reg := newModuleRegistry(root)
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if err := reg.loadLocked(); err != nil {
		return err
	}
	for path, entry := range reg.lock.Modules {
		if entry.Cache == "" || entry.Hash == "" {
			return fmt.Errorf("lock entry %s has incomplete cache metadata", path)
		}
		data, err := os.ReadFile(entry.Cache)
		if err != nil {
			return fmt.Errorf("cached module %s is unavailable: %w", path, err)
		}
		if !digestMatches(data, entry.Hash) {
			return fmt.Errorf("cached module %s hash mismatch", path)
		}
		if entry.Provider != "" && entry.Commit == "" {
			return fmt.Errorf("cached module %s has provider metadata without a commit", path)
		}
	}
	return nil
}

// ModuleGet adds or updates one manifest requirement. The special selector
// `none` removes it, matching the explicit removal form used by Go tooling.
func ModuleGet(root, modulePath, selector string) (ModuleManifest, error) {
	root = ModuleRootFor(root)
	manifest, err := LoadModuleManifest(root)
	if err != nil {
		return ModuleManifest{}, err
	}
	if !validModulePath(modulePath) {
		return ModuleManifest{}, fmt.Errorf("module path %q is not canonical", modulePath)
	}
	if selector == "" {
		selector = "latest"
	}
	found := false
	filtered := manifest.Requirements[:0]
	for _, req := range manifest.Requirements {
		if req.Path != modulePath {
			filtered = append(filtered, req)
			continue
		}
		found = true
		if selector != "none" {
			filtered = append(filtered, ModuleRequirement{Path: modulePath, Selector: selector, Indirect: req.Indirect})
		}
	}
	if !found && selector != "none" {
		filtered = append(filtered, ModuleRequirement{Path: modulePath, Selector: selector})
	}
	manifest.Requirements = filtered
	manifest = canonicalManifest(manifest)
	if err := WriteModuleManifest(root, manifest); err != nil {
		return ModuleManifest{}, err
	}
	return manifest, nil
}

var errModuleSourceUnavailable = errors.New("module source unavailable")

// ModuleManifestFile returns the canonical manifest filename for callers that
// need to display or inspect it without duplicating the format contract.
func ModuleManifestFile() string { return moduleManifestFile }

// ModuleWorkspaceFile returns the canonical workspace filename.
func ModuleWorkspaceFile() string { return workspaceManifestFile }

// ensureManifestSource keeps an empty source field compatible with older
// manifests while making the selected entry explicit to providers.
func ensureManifestSource(manifest ModuleManifest) string {
	if manifest.Source == "" {
		return "main.osr"
	}
	return filepath.ToSlash(filepath.Clean(manifest.Source))
}

// safeRelativePath rejects archive entries that could escape a destination or
// follow an unsafe link during provider materialization.
func safeRelativePath(name string) bool {
	name = filepath.ToSlash(filepath.Clean(name))
	return name != "." && name != "" && !strings.HasPrefix(name, "../") && name != ".." && !filepath.IsAbs(name)
}
