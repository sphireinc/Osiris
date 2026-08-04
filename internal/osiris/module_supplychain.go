package osiris

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ModuleAuditFinding is a stable machine-readable supply-chain finding.
type ModuleAuditFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Module   string `json:"module,omitempty"`
	Message  string `json:"message"`
}

// ModuleAuditReport is deliberately offline: it inspects manifest and lock
// metadata and never executes dependency code or contacts a discovery service.
type ModuleAuditReport struct {
	SchemaVersion int                  `json:"schema_version"`
	Manifest      string               `json:"manifest"`
	Lock          string               `json:"lock"`
	Findings      []ModuleAuditFinding `json:"findings,omitempty"`
}

func AuditModule(root string) (ModuleAuditReport, error) {
	root = ModuleRootFor(root)
	report := ModuleAuditReport{SchemaVersion: 1, Manifest: moduleManifestPath(root), Lock: filepath.Join(root, ".osiris.lock")}
	manifest, err := LoadModuleManifest(root)
	if err != nil {
		report.Findings = append(report.Findings, ModuleAuditFinding{Code: "manifest.missing_or_invalid", Severity: "error", Message: err.Error()})
		return report, nil
	}
	if manifest.Module == "" {
		report.Findings = append(report.Findings, ModuleAuditFinding{Code: "manifest.module_missing", Severity: "error", Message: "module path is empty"})
	}
	if err := ModuleVerify(root); err != nil {
		report.Findings = append(report.Findings, ModuleAuditFinding{Code: "lock.cache_unverified", Severity: "error", Message: err.Error()})
	}
	reg := newModuleRegistry(root)
	reg.mu.Lock()
	if err := reg.loadLocked(); err == nil {
		for path, entry := range reg.lock.Modules {
			if strings.HasPrefix(path, "github.com/") && entry.Provider == "" {
				report.Findings = append(report.Findings, ModuleAuditFinding{Code: "provenance.missing", Severity: "error", Module: path, Message: "repository entry has no provider identity"})
			}
			if entry.Commit == "" && entry.Version != "" && strings.HasPrefix(path, "github.com/") {
				report.Findings = append(report.Findings, ModuleAuditFinding{Code: "provenance.commit_missing", Severity: "error", Module: path, Message: "repository entry has no immutable commit"})
			}
		}
	} else {
		report.Findings = append(report.Findings, ModuleAuditFinding{Code: "lock.invalid", Severity: "error", Message: err.Error()})
	}
	reg.mu.Unlock()
	return report, nil
}

// WorkspaceModules returns sorted, validated workspace member manifests.
func WorkspaceModules(root string) ([]ModuleManifest, error) {
	workspace, err := LoadWorkspaceManifest(root)
	if err != nil {
		return nil, err
	}
	modules := make([]ModuleManifest, 0, len(workspace.Members))
	for _, member := range workspace.Members {
		manifest, err := LoadModuleManifest(member)
		if err != nil {
			return nil, fmt.Errorf("workspace member %s: %w", member, err)
		}
		modules = append(modules, manifest)
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].Module < modules[j].Module })
	return modules, nil
}

// ModuleVendor copies verified cached source into vendor/modules. It is
// intentionally explicit and never vendors arbitrary release assets.
func ModuleVendor(root, destination string) error {
	root = ModuleRootFor(root)
	if err := ModuleVerify(root); err != nil {
		return err
	}
	reg := newModuleRegistry(root)
	reg.mu.Lock()
	if err := reg.loadLocked(); err != nil {
		reg.mu.Unlock()
		return err
	}
	entries := make(map[string]lockEntry, len(reg.lock.Modules))
	for path, entry := range reg.lock.Modules {
		entries[path] = entry
	}
	reg.mu.Unlock()
	if destination == "" {
		destination = filepath.Join(root, "vendor", "modules")
	}
	for path, entry := range entries {
		rel := strings.TrimPrefix(path, "github.com/")
		if rel == path || strings.Contains(rel, "..") {
			return fmt.Errorf("unsafe vendor module path %q", path)
		}
		target := filepath.Join(destination, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(entry.Cache)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ModuleDownloadAndVerify is a convenience used by release and CI tooling.
func ModuleDownloadAndVerify(ctx context.Context, root string, providers ...RepositoryProvider) error {
	if err := ModuleDownload(ctx, root, providers...); err != nil {
		return err
	}
	return ModuleVerify(root)
}

// EncodeModuleAudit returns the stable JSON representation used by CLI and
// external audit tooling.
func EncodeModuleAudit(report ModuleAuditReport) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}
