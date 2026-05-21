package osiris

import (
	"fmt"
	"strings"
)

type DependencyNode struct {
	Ref     string   `json:"ref"`
	Kind    string   `json:"kind"`
	Label   string   `json:"label"`
	Package string   `json:"package,omitempty"`
	Version string   `json:"version,omitempty"`
	Imports []string `json:"imports,omitempty"`
}

type DependencyReport struct {
	Root   string                    `json:"root"`
	Nodes  map[string]DependencyNode `json:"nodes"`
	Order  []string                  `json:"order,omitempty"`
	Cycles [][]string                `json:"cycles,omitempty"`
}

func (rt *Runtime) BuildDependencyReport(sourceName, src string) (DependencyReport, error) {
	if rt == nil {
		rt = NewRuntime()
	}
	report := DependencyReport{
		Root:  sourceName,
		Nodes: make(map[string]DependencyNode),
	}
	visiting := make(map[string]int)
	seen := make(map[string]struct{})
	var stack []string

	var walk func(ref, base, src, selector string) error
	walk = func(ref, base, src, selector string) error {
		if idx, ok := visiting[ref]; ok {
			cycle := append([]string(nil), stack[idx:]...)
			cycle = append(cycle, ref)
			report.Cycles = append(report.Cycles, cycle)
			return nil
		}
		if _, ok := seen[ref]; ok {
			return nil
		}

		visiting[ref] = len(stack)
		stack = append(stack, ref)
		defer func() {
			stack = stack[:len(stack)-1]
			delete(visiting, ref)
			seen[ref] = struct{}{}
			report.Order = append(report.Order, ref)
		}()

		prog, err := Parse(ref, src)
		if err != nil {
			return err
		}

		node := DependencyNode{
			Ref:   ref,
			Kind:  dependencyKindForRef(ref),
			Label: dependencyLabel(ref, prog),
		}
		if p := firstPackageStmt(prog); p != nil {
			node.Package = p.Name
			node.Version = p.Version
		}

		for _, stmt := range prog.Statements {
			imp, ok := stmt.(importStmt)
			if !ok {
				continue
			}
			childRef, selector := splitModuleRef(imp.Path)
			childKey, childSrc, childKind, err := rt.resolveDependencySource(childRef, ref)
			if err != nil {
				return err
			}
			if childKind == moduleKindStdlib {
				version := stdlibVersion(childKey)
				if selector != "" && !versionMatchesSelector(version, selector) {
					return fmt.Errorf("stdlib module %s version %s does not satisfy %q", childKey, version, selector)
				}
				childSrc = "package " + childKey + " " + version
			}
			node.Imports = append(node.Imports, childKey)
			if err := walk(childKey, ref, childSrc, selector); err != nil {
				return err
			}
			if selector != "" && childKind == moduleKindFile {
				childNode := report.Nodes[childKey]
				if childNode.Version == "" {
					return fmt.Errorf("module %s has no version but %q was requested", childKey, selector)
				}
				if !versionMatchesSelector(childNode.Version, selector) {
					return fmt.Errorf("module %s version %s does not satisfy %q", childKey, childNode.Version, selector)
				}
			}
		}

		report.Nodes[ref] = node
		return nil
	}

	if err := walk(sourceName, sourceName, src, ""); err != nil {
		return report, err
	}
	return report, nil
}

func (rt *Runtime) resolveDependencySource(ref, base string) (string, string, moduleKind, error) {
	if strings.HasPrefix(ref, "std/") {
		return ref, "", moduleKindStdlib, nil
	}
	reg := rt.registryFor(base)
	key, src, err := reg.materialize(ref, base)
	if err != nil {
		return "", "", 0, err
	}
	return key, src, moduleKindFile, nil
}

func dependencyKindForRef(ref string) string {
	if strings.HasPrefix(ref, "std/") {
		return "stdlib"
	}
	if strings.HasPrefix(ref, "<") {
		return "root"
	}
	return "file"
}

func dependencyLabel(ref string, prog *Program) string {
	if p := firstPackageStmt(prog); p != nil {
		label := "package " + p.Name
		if p.Version != "" {
			label += " " + p.Version
		}
		return label
	}
	return ref
}

func firstPackageStmt(prog *Program) *packageStmt {
	if prog == nil {
		return nil
	}
	for _, stmt := range prog.Statements {
		if p, ok := stmt.(packageStmt); ok {
			cp := p
			return &cp
		}
	}
	return nil
}

func stdlibVersion(name string) string {
	switch name {
	case "std/text", "std/math", "std/crypto", "std/list":
		return "1.0.0"
	default:
		return ""
	}
}
