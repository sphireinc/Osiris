package osiris

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type HeapBinding struct {
	Name  string `json:"name"`
	Kind  string `json:"kind,omitempty"`
	Value string `json:"value"`
}

type HeapModule struct {
	Ref     string        `json:"ref"`
	Kind    string        `json:"kind"`
	Package string        `json:"package,omitempty"`
	Version string        `json:"version,omitempty"`
	Imports []string      `json:"imports,omitempty"`
	Exports []HeapBinding `json:"exports,omitempty"`
}

type HeapReport struct {
	Root         string           `json:"root"`
	Scope        []HeapBinding    `json:"scope,omitempty"`
	Modules      []HeapModule     `json:"modules,omitempty"`
	Dependencies DependencyReport `json:"dependencies"`
}

type HeapInspector struct {
	rt     *Runtime
	ctx    context.Context
	sess   *Session
	report HeapReport
}

// OpenHeapInspector operates on Runtime values and keeps the runtime-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns *HeapInspector and an error. Callers should only trust the value
// when the error is nil.
func (rt *Runtime) OpenHeapInspector(ctx context.Context, sourceName, src string) (*HeapInspector, error) {
	if rt == nil {
		rt = NewRuntime()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	prog, err := Parse(sourceName, src)
	if err != nil {
		return nil, err
	}
	sess := NewSession(rt, ctx, sourceName)
	for _, stmt := range prog.Statements {
		if _, ok := stmt.(emitStmt); ok {
			continue
		}
		if _, _, err := sess.execStmt(stmt, prog.SourceName, false, nil); err != nil {
			return nil, err
		}
	}
	deps, err := rt.BuildDependencyReport(sourceName, src)
	if err != nil {
		return nil, err
	}
	report := HeapReport{
		Root:         sourceName,
		Scope:        sess.snapshotHeapScope(),
		Modules:      rt.snapshotHeapModules(deps),
		Dependencies: deps,
	}
	return &HeapInspector{
		rt:     rt,
		ctx:    ctx,
		sess:   sess,
		report: report,
	}, nil
}

// Report operates on HeapInspector values and keeps the heapinspector-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns HeapReport so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (h *HeapInspector) Report() HeapReport {
	if h == nil {
		return HeapReport{}
	}
	return h.report
}

// Query operates on HeapInspector values and keeps the heapinspector-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func (h *HeapInspector) Query(expr string) (Value, error) {
	if h == nil || h.sess == nil {
		return nullValue(), fmt.Errorf("heap inspector is not initialized")
	}
	prog, err := Parse("<heap>", "emit "+expr)
	if err != nil {
		return nullValue(), err
	}
	emit, ok := prog.Statements[0].(emitStmt)
	if !ok {
		return nullValue(), fmt.Errorf("heap query parser failed")
	}
	return h.sess.eval(emit.Expr, h.sess.scope)
}

// Binding operates on HeapInspector values and keeps the
// heapinspector-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns HeapBinding, bool so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func (h *HeapInspector) Binding(name string) (HeapBinding, bool) {
	if h == nil || h.sess == nil {
		return HeapBinding{}, false
	}
	b, ok := h.sess.scope.lookup(name)
	if !ok {
		return HeapBinding{}, false
	}
	v, stored := storedBindingValue(b)
	var err error
	if !stored {
		v, err = h.sess.eval(b.expr, b.env)
	}
	if err != nil {
		return HeapBinding{Name: name, Value: "<error: " + err.Error() + ">"}, true
	}
	return HeapBinding{Name: name, Kind: v.Kind(), Value: v.String()}, true
}

// Module operates on HeapInspector values and keeps the heapinspector-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns HeapModule, bool so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func (h *HeapInspector) Module(name string) (HeapModule, bool) {
	if h == nil || h.sess == nil {
		return HeapModule{}, false
	}
	b, ok := h.sess.scope.lookup(name)
	if !ok {
		return HeapModule{}, false
	}
	v, stored := storedBindingValue(b)
	var err error
	if !stored {
		v, err = h.sess.eval(b.expr, b.env)
	}
	if err != nil {
		return HeapModule{
			Ref:  name,
			Kind: "binding",
		}, true
	}
	mod, err := v.AsModule()
	if err != nil {
		return HeapModule{}, false
	}
	return heapModuleFromValue(name, "binding", mod), true
}

// snapshotHeapModules operates on Runtime values and keeps the
// runtime-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns []HeapModule so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (rt *Runtime) snapshotHeapModules(report DependencyReport) []HeapModule {
	if rt == nil {
		return nil
	}
	rt.moduleMu.Lock()
	defer rt.moduleMu.Unlock()

	order := report.Order
	if len(order) == 0 {
		order = make([]string, 0, len(report.Nodes))
		for ref := range report.Nodes {
			order = append(order, ref)
		}
		sort.Strings(order)
	}

	out := make([]HeapModule, 0, len(order))
	for _, ref := range order {
		node, ok := report.Nodes[ref]
		if !ok {
			continue
		}
		mod, ok := rt.moduleValueForRefLocked(ref)
		if !ok {
			continue
		}
		heapMod := heapModuleFromValue(ref, node.Kind, mod.mod)
		if node.Package != "" {
			heapMod.Package = node.Package
		}
		if node.Version != "" {
			heapMod.Version = node.Version
		}
		heapMod.Imports = append([]string(nil), node.Imports...)
		out = append(out, heapMod)
	}
	return out
}

// moduleValueForRefLocked operates on Runtime values and keeps the
// runtime-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns Value, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (rt *Runtime) moduleValueForRefLocked(ref string) (Value, bool) {
	if mod, ok := rt.modules[ref]; ok {
		return mod, true
	}
	for key, mod := range rt.modules {
		if registryEntryPath(key) == ref {
			return mod, true
		}
	}
	return Value{}, false
}

// heapModuleFromValue handles the heap module from value step in the
// surrounding package and returns HeapModule.
//
// It returns HeapModule so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func heapModuleFromValue(ref, kind string, mod *moduleValue) HeapModule {
	if mod == nil {
		return HeapModule{Ref: ref, Kind: kind}
	}
	hm := HeapModule{
		Ref:     ref,
		Kind:    kind,
		Package: mod.name,
		Version: mod.version,
	}
	if len(mod.members) == 0 {
		return hm
	}
	names := make([]string, 0, len(mod.members))
	for name := range mod.members {
		names = append(names, name)
	}
	sort.Strings(names)
	hm.Exports = make([]HeapBinding, 0, len(names))
	for _, name := range names {
		v := mod.members[name]
		hm.Exports = append(hm.Exports, HeapBinding{Name: name, Kind: v.Kind(), Value: v.String()})
	}
	return hm
}

// snapshotHeapScope operates on Session values and keeps the session-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns []HeapBinding so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (s *Session) snapshotHeapScope() []HeapBinding {
	if s == nil || s.scope == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []HeapBinding
	var walk func(*scope)
	walk = func(sc *scope) {
		if sc == nil {
			return
		}
		walk(sc.parent)
		names := make([]string, 0, len(sc.items))
		for name := range sc.items {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			b := sc.items[name]
			v, err := s.eval(b.expr, b.env)
			if err != nil {
				out = append(out, HeapBinding{Name: name, Value: "<error: " + err.Error() + ">"})
				continue
			}
			out = append(out, HeapBinding{Name: name, Kind: v.Kind(), Value: v.String()})
		}
	}
	walk(s.scope)
	return out
}

// snapshotScope operates on Session values and keeps the session-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns []TraceBinding so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (s *Session) snapshotScope() []TraceBinding {
	heap := s.snapshotHeapScope()
	if len(heap) == 0 {
		return nil
	}
	out := make([]TraceBinding, len(heap))
	for i := range heap {
		out[i] = TraceBinding{Name: heap[i].Name, Value: heap[i].Value}
	}
	return out
}

// String operates on HeapReport values and keeps the heapreport-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (r HeapReport) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "root: %s\n", r.Root)
	if len(r.Scope) > 0 {
		b.WriteString("scope:\n")
		for _, bind := range r.Scope {
			if bind.Kind != "" {
				fmt.Fprintf(&b, "  %s [%s] = %s\n", bind.Name, bind.Kind, bind.Value)
				continue
			}
			fmt.Fprintf(&b, "  %s = %s\n", bind.Name, bind.Value)
		}
	}
	if len(r.Modules) > 0 {
		b.WriteString("modules:\n")
		for _, mod := range r.Modules {
			label := mod.Ref
			if mod.Package != "" {
				label = mod.Package
			}
			if mod.Version != "" {
				label += "@" + mod.Version
			}
			fmt.Fprintf(&b, "  %s [%s]\n", label, mod.Kind)
			for _, exp := range mod.Exports {
				fmt.Fprintf(&b, "    %s [%s] = %s\n", exp.Name, exp.Kind, exp.Value)
			}
		}
	}
	if len(r.Dependencies.Cycles) > 0 {
		b.WriteString("cycles:\n")
		for _, cycle := range r.Dependencies.Cycles {
			fmt.Fprintf(&b, "  - %s\n", strings.Join(cycle, " -> "))
		}
	}
	return b.String()
}
