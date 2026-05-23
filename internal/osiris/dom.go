package osiris

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type DomProgram struct {
	Program   *IRProgram
	Functions []DomFunction
}

type DomFunction struct {
	Name       string
	SourceName string
	Entry      string
	Blocks     []DomBlock
}

type DomBlock struct {
	Name     string
	Params   []string
	Preds    []string
	Succs    []string
	IDom     string
	Depth    int
	Children []string
}

func (dp *DomProgram) String() string {
	return FormatDomProgram(dp)
}

func (rt *Runtime) BuildDom(ctx context.Context, prog *Program) (*DomProgram, error) {
	cfgProg, err := rt.BuildCFG(ctx, prog)
	if err != nil {
		return nil, err
	}
	return BuildDomProgram(cfgProg)
}

func BuildDomProgram(prog *CFGProgram) (*DomProgram, error) {
	if prog == nil || prog.Program == nil {
		return nil, fmt.Errorf("cannot build dominator tree for nil program")
	}
	out := &DomProgram{Program: prog.Program}
	for _, fn := range prog.Functions {
		domFn, err := buildDomFunction(fn)
		if err != nil {
			return nil, err
		}
		out.Functions = append(out.Functions, domFn)
	}
	return out, nil
}

func FormatDomProgram(prog *DomProgram) string {
	if prog == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("dom ")
	if prog.Program != nil && prog.Program.SourceName != "" {
		b.WriteString(prog.Program.SourceName)
	} else {
		b.WriteString("<unknown>")
	}
	b.WriteByte('\n')
	if prog.Program != nil {
		if prog.Program.PackageName != "" {
			b.WriteString("package ")
			b.WriteString(prog.Program.PackageName)
			if prog.Program.PackageVersion != "" {
				b.WriteByte(' ')
				b.WriteString(prog.Program.PackageVersion)
			}
			b.WriteByte('\n')
		}
		if len(prog.Program.Exports) > 0 {
			b.WriteString("exports ")
			b.WriteString(strings.Join(prog.Program.Exports, ", "))
			b.WriteByte('\n')
		}
	}
	if len(prog.Functions) > 0 {
		b.WriteByte('\n')
		for i, fn := range prog.Functions {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString("fn ")
			b.WriteString(fn.Name)
			if fn.SourceName != "" && fn.SourceName != fn.Name {
				b.WriteString(" (")
				b.WriteString(fn.SourceName)
				b.WriteByte(')')
			}
			fmt.Fprintf(&b, ": entry=%s blocks=%d\n", fn.Entry, len(fn.Blocks))
			for _, block := range fn.Blocks {
				b.WriteString("  block ")
				b.WriteString(block.Name)
				if len(block.Params) > 0 {
					b.WriteByte('(')
					b.WriteString(strings.Join(block.Params, ", "))
					b.WriteByte(')')
				}
				if block.IDom != "" {
					fmt.Fprintf(&b, " idom=%s depth=%d", block.IDom, block.Depth)
				}
				b.WriteString("\n    preds [")
				b.WriteString(strings.Join(block.Preds, ", "))
				b.WriteString("] succs [")
				b.WriteString(strings.Join(block.Succs, ", "))
				b.WriteString("] children [")
				b.WriteString(strings.Join(block.Children, ", "))
				b.WriteString("]\n")
			}
		}
	}
	return b.String()
}

func buildDomFunction(fn CFGFunction) (DomFunction, error) {
	domFn := DomFunction{Name: fn.Name, SourceName: fn.SourceName, Entry: fn.Entry}
	if len(fn.Blocks) == 0 {
		return domFn, nil
	}
	blockMap := make(map[string]CFGBlock, len(fn.Blocks))
	reachable := reachableCFGBlocks(&fn)
	if len(reachable) == 0 {
		return domFn, nil
	}
	for _, block := range fn.Blocks {
		blockMap[block.Name] = block
	}
	domSets := make(map[string]map[string]struct{}, len(reachable))
	universe := make(map[string]struct{}, len(reachable))
	for _, name := range reachable {
		universe[name] = struct{}{}
	}
	for _, name := range reachable {
		if name == fn.Entry {
			domSets[name] = setOf(name)
			continue
		}
		domSets[name] = cloneSet(universe)
	}
	changed := true
	for changed {
		changed = false
		for _, name := range reachable {
			if name == fn.Entry {
				continue
			}
			block := blockMap[name]
			preds := filterReachable(block.Preds, universe)
			next := setOf(name)
			if len(preds) > 0 {
				next = cloneSet(domSets[preds[0]])
				for _, pred := range preds[1:] {
					next = intersectSets(next, domSets[pred])
				}
				next[name] = struct{}{}
			}
			if !sameSet(next, domSets[name]) {
				domSets[name] = next
				changed = true
			}
		}
	}
	idom := map[string]string{fn.Entry: ""}
	for _, name := range reachable {
		if name == fn.Entry {
			continue
		}
		cands := setMembers(domSets[name])
		sort.Strings(cands)
		best := ""
		for _, cand := range cands {
			if cand == name {
				continue
			}
			ok := true
			for _, other := range cands {
				if other == name || other == cand {
					continue
				}
				if !dominates(domSets, other, cand) {
					ok = false
					break
				}
			}
			if ok {
				best = cand
				break
			}
		}
		idom[name] = best
	}
	children := make(map[string][]string, len(reachable))
	for name, parent := range idom {
		if parent == "" {
			continue
		}
		children[parent] = append(children[parent], name)
	}
	for _, list := range children {
		sort.Strings(list)
	}
	depth := map[string]int{fn.Entry: 0}
	queue := []string{fn.Entry}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		for _, child := range children[name] {
			depth[child] = depth[name] + 1
			queue = append(queue, child)
		}
	}
	for _, name := range reachable {
		block := blockMap[name]
		domBlock := DomBlock{
			Name:     block.Name,
			Params:   append([]string(nil), block.Params...),
			Preds:    append([]string(nil), block.Preds...),
			Succs:    append([]string(nil), block.Succs...),
			IDom:     idom[name],
			Depth:    depth[name],
			Children: append([]string(nil), children[name]...),
		}
		sort.Strings(domBlock.Preds)
		sort.Strings(domBlock.Succs)
		sort.Strings(domBlock.Children)
		domFn.Blocks = append(domFn.Blocks, domBlock)
	}
	sort.Slice(domFn.Blocks, func(i, j int) bool { return domFn.Blocks[i].Name < domFn.Blocks[j].Name })
	return domFn, nil
}

func reachableCFGBlocks(fn *CFGFunction) []string {
	if fn == nil || fn.Entry == "" {
		return nil
	}
	blockMap := make(map[string]CFGBlock, len(fn.Blocks))
	for _, block := range fn.Blocks {
		blockMap[block.Name] = block
	}
	seen := map[string]struct{}{fn.Entry: {}}
	queue := []string{fn.Entry}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		block, ok := blockMap[name]
		if !ok {
			continue
		}
		for _, succ := range block.Succs {
			if _, ok := seen[succ]; ok {
				continue
			}
			seen[succ] = struct{}{}
			queue = append(queue, succ)
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func filterReachable(names []string, reach map[string]struct{}) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := reach[name]; ok {
			out = append(out, name)
		}
	}
	return out
}

func setOf(names ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[name] = struct{}{}
	}
	return out
}

func cloneSet(src map[string]struct{}) map[string]struct{} {
	if len(src) == 0 {
		return make(map[string]struct{})
	}
	out := make(map[string]struct{}, len(src))
	for name := range src {
		out[name] = struct{}{}
	}
	return out
}

func intersectSets(a, b map[string]struct{}) map[string]struct{} {
	if len(a) == 0 || len(b) == 0 {
		return make(map[string]struct{})
	}
	out := make(map[string]struct{}, min(len(a), len(b)))
	if len(a) > len(b) {
		a, b = b, a
	}
	for name := range a {
		if _, ok := b[name]; ok {
			out[name] = struct{}{}
		}
	}
	return out
}

func sameSet(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for name := range a {
		if _, ok := b[name]; !ok {
			return false
		}
	}
	return true
}

func setMembers(src map[string]struct{}) []string {
	out := make([]string, 0, len(src))
	for name := range src {
		out = append(out, name)
	}
	return out
}

func dominates(domSets map[string]map[string]struct{}, a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	set, ok := domSets[b]
	if !ok {
		return false
	}
	_, ok = set[a]
	return ok
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
