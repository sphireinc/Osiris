package osiris

import "strconv"

// nativeModuleIdentities tracks only proven static import identities. Absence
// means unknown/dynamic; merges retain an identity only when every predecessor
// supplies the same path.
func nativeModuleIdentities(fn *IRFunction, seeds map[string]string) map[string]string {
	modules := make(map[string]string, len(seeds))
	for value, path := range seeds {
		modules[value] = path
	}
	blocks := make(map[string]*IRBlock, len(fn.Blocks))
	params := make(map[string]struct{})
	invalid := make(map[string]struct{})
	for _, block := range fn.Blocks {
		blocks[block.Name] = block
		for _, param := range block.Params {
			params[param] = struct{}{}
			delete(modules, param)
		}
	}
	for changed := true; changed; {
		changed = false
		for _, block := range fn.Blocks {
			for _, in := range block.Instrs {
				switch in.Op {
				case "import":
					if path, err := strconv.Unquote(in.Text); err == nil && modules[in.Dest] != path {
						modules[in.Dest] = path
						changed = true
					}
				case "copy":
					if len(in.Args) == 1 {
						if path, ok := modules[in.Args[0]]; ok && modules[in.Dest] != path {
							modules[in.Dest] = path
							changed = true
						}
					}
				}
			}
		}
		type incoming struct {
			path  string
			valid bool
			seen  bool
		}
		values := make(map[string]incoming)
		add := func(target string, args []string) {
			block := blocks[target]
			if block == nil {
				return
			}
			for i, param := range block.Params {
				state := values[param]
				if !state.seen {
					state.valid = true
					state.seen = true
				}
				path, ok := "", false
				arg := ""
				if i < len(args) {
					arg = args[i]
					path, ok = modules[arg]
				}
				if !ok {
					// A block parameter may be a loop-carried value whose identity
					// is established by another incoming edge in a later iteration.
					// A non-parameter unknown is genuinely dynamic and poisons the
					// merge immediately.
					if _, pending := params[arg]; !pending {
						state.valid = false
					}
				} else if state.path == "" {
					state.path = path
				} else if state.path != path {
					state.valid = false
				}
				values[param] = state
			}
		}
		for _, block := range fn.Blocks {
			switch term := block.Term.(type) {
			case *irJump:
				add(term.Target, term.Args)
			case *irBranch:
				add(term.TrueTarget, term.TrueArgs)
				add(term.FalseTarget, term.FalseArgs)
			}
		}
		for param := range params {
			if _, poisoned := invalid[param]; poisoned {
				delete(modules, param)
				continue
			}
			state := values[param]
			if !state.valid {
				invalid[param] = struct{}{}
				if _, exists := modules[param]; exists {
					delete(modules, param)
					changed = true
				}
				continue
			}
			if state.path != "" && modules[param] != state.path {
				modules[param] = state.path
				changed = true
			}
		}
	}
	return modules
}

// nativeModuleCandidates conservatively tracks values which can carry a
// module even when conflicting control-flow inputs prevent a single static
// identity from being proven. It lets the C backend distinguish an ordinary
// value member (for example, error.kind) from an unsafe dynamic module member.
func nativeModuleCandidates(fn *IRFunction, seeds map[string]string) map[string]bool {
	candidates := make(map[string]bool, len(seeds))
	for value := range seeds {
		candidates[value] = true
	}
	blocks := make(map[string]*IRBlock, len(fn.Blocks))
	for _, block := range fn.Blocks {
		blocks[block.Name] = block
	}
	addIncoming := func(target string, args []string) bool {
		block := blocks[target]
		if block == nil {
			return false
		}
		changed := false
		for i, param := range block.Params {
			if i < len(args) && candidates[args[i]] && !candidates[param] {
				candidates[param] = true
				changed = true
			}
		}
		return changed
	}
	for changed := true; changed; {
		changed = false
		for _, block := range fn.Blocks {
			for _, in := range block.Instrs {
				switch in.Op {
				case "import":
					if !candidates[in.Dest] {
						candidates[in.Dest] = true
						changed = true
					}
				case "copy":
					if len(in.Args) == 1 && candidates[in.Args[0]] && !candidates[in.Dest] {
						candidates[in.Dest] = true
						changed = true
					}
				}
			}
			switch term := block.Term.(type) {
			case *irJump:
				if addIncoming(term.Target, term.Args) {
					changed = true
				}
			case *irBranch:
				if addIncoming(term.TrueTarget, term.TrueArgs) {
					changed = true
				}
				if addIncoming(term.FalseTarget, term.FalseArgs) {
					changed = true
				}
			}
		}
	}
	return candidates
}

// nativeModuleIdentitiesByFunction propagates lexical capture identity to a
// fixed point. A capture is static only when every construction site agrees.
func nativeModuleIdentitiesByFunction(prog *IRProgram) map[string]map[string]string {
	functions := make(map[string]*IRFunction, len(prog.Functions))
	for _, fn := range prog.Functions {
		functions[fn.Name] = fn
	}
	seeds := make(map[string]map[string]string, len(functions))
	conflicts := make(map[string]map[string]struct{}, len(functions))
	for name := range functions {
		seeds[name] = make(map[string]string)
		conflicts[name] = make(map[string]struct{})
	}
	for changed := true; changed; {
		changed = false
		resolved := make(map[string]map[string]string, len(functions))
		for name, fn := range functions {
			resolved[name] = nativeModuleIdentities(fn, seeds[name])
		}
		for _, outer := range functions {
			for _, block := range outer.Blocks {
				for _, in := range block.Instrs {
					if in.Op != "makefunc" {
						continue
					}
					child := functions[in.Text]
					if child == nil {
						continue
					}
					for i, captured := range child.CaptureValues {
						if i >= len(in.Args) {
							continue
						}
						if _, conflicted := conflicts[child.Name][captured]; conflicted {
							continue
						}
						path, ok := resolved[outer.Name][in.Args[i]]
						if !ok {
							continue
						}
						if previous, exists := seeds[child.Name][captured]; !exists {
							seeds[child.Name][captured] = path
							changed = true
						} else if previous != path {
							delete(seeds[child.Name], captured)
							conflicts[child.Name][captured] = struct{}{}
							changed = true
						}
					}
				}
			}
		}
	}
	resolved := make(map[string]map[string]string, len(functions))
	for name, fn := range functions {
		resolved[name] = nativeModuleIdentities(fn, seeds[name])
	}
	return resolved
}
