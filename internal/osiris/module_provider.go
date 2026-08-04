package osiris

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// RepositoryVersion is an immutable provider result. Commit is always the
// authority for source bytes; tag and Version are presentation/selection
// metadata retained for lock and audit output.
type RepositoryVersion struct {
	ModulePath    string `json:"module_path"`
	Selector      string `json:"selector"`
	Version       string `json:"version"`
	Tag           string `json:"tag,omitempty"`
	Commit        string `json:"commit"`
	Provider      string `json:"provider"`
	RepositoryURL string `json:"repository_url"`
}

// RepositorySnapshot is the verified source tree returned by a provider.
// Files contain only regular source files; providers must reject submodules,
// unsafe paths, and links that leave the repository tree.
type RepositorySnapshot struct {
	Version    RepositoryVersion
	Files      map[string][]byte
	TreeDigest string
}

// RepositoryProvider separates repository discovery/resolution from package
// import syntax. Implementations may use GitHub, another Git host, or a
// controlled vanity-path mapping without changing module identity.
type RepositoryProvider interface {
	Identity() string
	Supports(modulePath string) bool
	Resolve(context.Context, string, string) (RepositoryVersion, error)
	Fetch(context.Context, RepositoryVersion) (RepositorySnapshot, error)
}

// GitHubProvider is the first complete repository provider. It uses the
// user's configured Git credentials through the git executable; credentials
// never enter manifests, locks, caches, diagnostics, or telemetry.
type GitHubProvider struct {
	Git string
}

func NewGitHubProvider() *GitHubProvider { return &GitHubProvider{Git: "git"} }

func (p *GitHubProvider) Identity() string { return "github-git-v1" }

func (p *GitHubProvider) Supports(modulePath string) bool {
	parts := strings.Split(modulePath, "/")
	return len(parts) >= 3 && parts[0] == "github.com" && parts[1] != "" && parts[2] != ""
}

func (p *GitHubProvider) Resolve(ctx context.Context, modulePath, selector string) (RepositoryVersion, error) {
	if !p.Supports(modulePath) {
		return RepositoryVersion{}, fmt.Errorf("GitHub provider does not support %q", modulePath)
	}
	parts := strings.Split(modulePath, "/")
	url := "https://github.com/" + parts[1] + "/" + parts[2] + ".git"
	if selector == "" {
		selector = "latest"
	}
	ref := selector
	if ref == "latest" || strings.HasPrefix(ref, "^") || strings.HasPrefix(ref, "~") || looksLikeSemverSelector(ref) {
		ref = "refs/tags/" + strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(ref, "^"), "~"), "v")
		if selector == "latest" {
			ref = "refs/tags/*"
		}
	}
	output, err := p.run(ctx, "ls-remote", "--tags", "--refs", url, ref)
	if err != nil {
		return RepositoryVersion{}, fmt.Errorf("resolve %s@%s: %w", modulePath, selector, err)
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return RepositoryVersion{}, fmt.Errorf("repository %s has no matching tag or commit for %q", modulePath, selector)
	}
	type tagCommit struct{ tag, commit string }
	var matches []tagCommit
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != 40 {
			continue
		}
		tag := strings.TrimPrefix(fields[1], "refs/tags/")
		version, err := parseSemVersion(tag)
		if err != nil {
			continue
		}
		if selector != "latest" && !versionMatchesSelector(version.String(), selector) {
			continue
		}
		matches = append(matches, tagCommit{tag: tag, commit: fields[0]})
	}
	if len(matches) > 0 {
		major := moduleMajorPath(modulePath)
		if major > 0 {
			filtered := matches[:0]
			for _, match := range matches {
				if version, err := parseSemVersion(match.tag); err == nil && version.major == major {
					filtered = append(filtered, match)
				}
			}
			matches = filtered
		}
	}
	if len(matches) > 0 {
		sort.Slice(matches, func(i, j int) bool {
			vi, _ := parseSemVersion(matches[i].tag)
			vj, _ := parseSemVersion(matches[j].tag)
			if vi.major != vj.major {
				return vi.major < vj.major
			}
			if vi.minor != vj.minor {
				return vi.minor < vj.minor
			}
			return vi.patch < vj.patch
		})
		chosen := matches[len(matches)-1]
		return RepositoryVersion{ModulePath: modulePath, Selector: selector, Version: normalizeTagVersion(chosen.tag), Tag: chosen.tag, Commit: chosen.commit, Provider: p.Identity(), RepositoryURL: url}, nil
	}
	if isCommitSHA(selector) {
		commitOutput, err := p.run(ctx, "ls-remote", url, selector)
		if err != nil || strings.TrimSpace(commitOutput) == "" {
			return RepositoryVersion{}, fmt.Errorf("commit %s is not available for %s", selector, modulePath)
		}
		commit := strings.Fields(strings.TrimSpace(strings.Split(commitOutput, "\n")[0]))[0]
		return RepositoryVersion{ModulePath: modulePath, Selector: selector, Version: pseudoVersion(selector, commit), Commit: commit, Provider: p.Identity(), RepositoryURL: url}, nil
	}
	if selector == "latest" {
		commitOutput, err := p.run(ctx, "ls-remote", url, "HEAD")
		if err == nil && strings.TrimSpace(commitOutput) != "" {
			commit := strings.Fields(strings.TrimSpace(strings.Split(commitOutput, "\n")[0]))[0]
			return RepositoryVersion{ModulePath: modulePath, Selector: selector, Version: pseudoVersion(selector, commit), Commit: commit, Provider: p.Identity(), RepositoryURL: url}, nil
		}
	}
	return RepositoryVersion{}, fmt.Errorf("no semantic-version tag satisfies %s@%s", modulePath, selector)
}

func moduleMajorPath(modulePath string) int {
	parts := strings.Split(modulePath, "/")
	if len(parts) <= 3 {
		return 0
	}
	last := parts[len(parts)-1]
	if len(last) < 2 || last[0] != 'v' {
		return 0
	}
	version, err := parseSemVersion(last[1:])
	if err != nil || version.minor != 0 || version.patch != 0 {
		return 0
	}
	return version.major
}

func normalizeTagVersion(tag string) string {
	version, err := parseSemVersion(tag)
	if err != nil {
		return tag
	}
	return "v" + version.String()
}

func pseudoVersion(selector, commit string) string {
	base := "0.0.0"
	if version, err := parseSemVersion(strings.TrimPrefix(selector, "v")); err == nil {
		base = version.String()
	}
	return "v" + base + "-0.00000000000-" + commit[:12]
}

func isCommitSHA(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

func (p *GitHubProvider) Fetch(ctx context.Context, version RepositoryVersion) (RepositorySnapshot, error) {
	if version.Commit == "" || version.RepositoryURL == "" {
		return RepositorySnapshot{}, fmt.Errorf("repository version is incomplete")
	}
	tmp, err := os.MkdirTemp("", "osiris-git-")
	if err != nil {
		return RepositorySnapshot{}, err
	}
	defer os.RemoveAll(tmp)
	if _, err := p.runIn(ctx, tmp, "clone", "--quiet", "--no-checkout", "--depth", "1", version.RepositoryURL, "."); err != nil {
		return RepositorySnapshot{}, fmt.Errorf("clone %s: %w", version.ModulePath, err)
	}
	if _, err := p.runIn(ctx, tmp, "checkout", "--quiet", "--detach", version.Commit); err != nil {
		// A commit or non-tip tag may not be available in a shallow clone.
		if _, fetchErr := p.runIn(ctx, tmp, "fetch", "--quiet", "--depth", "1", "origin", version.Commit); fetchErr != nil {
			return RepositorySnapshot{}, fmt.Errorf("fetch %s: %w", version.ModulePath, fetchErr)
		}
		if _, checkoutErr := p.runIn(ctx, tmp, "checkout", "--quiet", "--detach", version.Commit); checkoutErr != nil {
			return RepositorySnapshot{}, fmt.Errorf("checkout %s: %w", version.ModulePath, checkoutErr)
		}
	}
	return snapshotFromTree(tmp, version)
}

func (p *GitHubProvider) run(ctx context.Context, args ...string) (string, error) {
	return p.runIn(ctx, "", args...)
}

func (p *GitHubProvider) runIn(ctx context.Context, dir string, args ...string) (string, error) {
	git := p.Git
	if git == "" {
		git = "git"
	}
	cmd := exec.CommandContext(ctx, git, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(output), nil
}

func snapshotFromTree(root string, version RepositoryVersion) (RepositorySnapshot, error) {
	files := make(map[string][]byte)
	var names []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == ".gitmodules" {
			return fmt.Errorf("repository contains unsupported submodules")
		}
		if rel == "." || info.IsDir() {
			return nil
		}
		if !safeRelativePath(rel) {
			return fmt.Errorf("repository entry %q escapes source root", rel)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("repository contains unsupported symlink %q", rel)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("repository entry %q is not a regular file", rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		files[name] = data
		names = append(names, name)
		return nil
	})
	if err != nil {
		return RepositorySnapshot{}, err
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		fmt.Fprintf(hash, "%d:%s\n", len(name), name)
		fmt.Fprintf(hash, "%d:", len(files[name]))
		_, _ = hash.Write(files[name])
		hash.Write([]byte("\n"))
	}
	return RepositorySnapshot{Version: version, Files: files, TreeDigest: hex.EncodeToString(hash.Sum(nil))}, nil
}

// SnapshotFromTar is a safe fixture/provider helper. It rejects traversal,
// links, special files, submodules, and duplicate entries before hashing.
func SnapshotFromTar(ctx context.Context, r io.Reader, version RepositoryVersion) (RepositorySnapshot, error) {
	files := make(map[string][]byte)
	tr := tar.NewReader(r)
	for {
		if err := ctx.Err(); err != nil {
			return RepositorySnapshot{}, err
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return RepositorySnapshot{}, err
		}
		name := filepath.ToSlash(filepath.Clean(hdr.Name))
		if !safeRelativePath(name) {
			return RepositorySnapshot{}, fmt.Errorf("archive entry %q escapes source root", hdr.Name)
		}
		if hdr.Typeflag != tar.TypeReg {
			return RepositorySnapshot{}, fmt.Errorf("archive entry %q is not a regular file", hdr.Name)
		}
		if _, exists := files[name]; exists {
			return RepositorySnapshot{}, fmt.Errorf("archive contains duplicate entry %q", name)
		}
		limited := io.LimitReader(tr, 64<<20+1)
		data, err := io.ReadAll(limited)
		if err != nil {
			return RepositorySnapshot{}, err
		}
		if len(data) > 64<<20 {
			return RepositorySnapshot{}, fmt.Errorf("archive entry %q exceeds 64 MiB", hdr.Name)
		}
		files[name] = data
	}
	return snapshotFromFiles(files, version), nil
}

func snapshotFromFiles(files map[string][]byte, version RepositoryVersion) RepositorySnapshot {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		fmt.Fprintf(hash, "%d:%s\n", len(name), name)
		fmt.Fprintf(hash, "%d:", len(files[name]))
		_, _ = hash.Write(files[name])
		hash.Write([]byte("\n"))
	}
	return RepositorySnapshot{Version: version, Files: files, TreeDigest: hex.EncodeToString(hash.Sum(nil))}
}
