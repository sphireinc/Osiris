package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/sphireinc/osiris/internal/osiris"
)

// runHeap handles the run heap command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runHeap(ctx context.Context, args []string, stdout io.Writer, stdin io.Reader) error {
	fs := flag.NewFlagSet("heap", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var jsonOut bool
	var query string
	fs.BoolVar(&jsonOut, "json", false, "emit heap snapshot as JSON")
	fs.StringVar(&query, "query", "", "evaluate an expression against the heap snapshot")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	sourcePath := ""
	if len(rest) > 0 {
		sourcePath = rest[0]
	}
	src, name, err := readSource(sourcePath)
	if err != nil {
		return err
	}

	insp, err := osiris.NewRuntime().OpenHeapInspector(ctx, name, src)
	if err != nil {
		return err
	}
	report := insp.Report()

	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	if query != "" {
		v, err := insp.Query(query)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, v.String())
		return err
	}

	fmt.Fprintf(stdout, "heap snapshot for %s\n", report.Root)
	if len(report.Scope) > 0 {
		fmt.Fprintln(stdout, "scope:")
		for _, bind := range report.Scope {
			if bind.Kind != "" {
				fmt.Fprintf(stdout, "  %s [%s] = %s\n", bind.Name, bind.Kind, bind.Value)
				continue
			}
			fmt.Fprintf(stdout, "  %s = %s\n", bind.Name, bind.Value)
		}
	}
	if len(report.Modules) > 0 {
		fmt.Fprintln(stdout, "modules:")
		for _, mod := range report.Modules {
			label := mod.Ref
			if mod.Package != "" {
				label = mod.Package
			}
			if mod.Version != "" {
				label += "@" + mod.Version
			}
			fmt.Fprintf(stdout, "  %s [%s]\n", label, mod.Kind)
			if len(mod.Exports) == 0 {
				continue
			}
			for _, exp := range mod.Exports {
				if exp.Kind != "" {
					fmt.Fprintf(stdout, "    %s [%s] = %s\n", exp.Name, exp.Kind, exp.Value)
					continue
				}
				fmt.Fprintf(stdout, "    %s = %s\n", exp.Name, exp.Value)
			}
		}
	}
	if len(report.Dependencies.Cycles) > 0 {
		fmt.Fprintln(stdout, "cycles:")
		for _, cycle := range report.Dependencies.Cycles {
			fmt.Fprintf(stdout, "  - %s\n", strings.Join(cycle, " -> "))
		}
	}

	reader := bufio.NewReader(stdin)
	for {
		if _, err := io.WriteString(stdout, "heap> "); err != nil {
			return err
		}
		line, err := reader.ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			return err
		}
		cmd := strings.TrimSpace(line)
		switch {
		case cmd == "", cmd == "help":
			fmt.Fprintln(stdout, "commands: scope, modules, deps, graph, show <name>, module <name>, query <expr>, json, quit|q")
		case cmd == "scope":
			printHeapScope(stdout, report.Scope)
		case cmd == "modules":
			printHeapModules(stdout, report.Modules)
		case cmd == "deps":
			printHeapDependencies(stdout, report.Dependencies)
		case cmd == "graph":
			printHeapGraph(stdout, report.Dependencies)
		case strings.HasPrefix(cmd, "show "):
			name := strings.TrimSpace(strings.TrimPrefix(cmd, "show "))
			if name == "" {
				fmt.Fprintln(stdout, "show requires a name")
				continue
			}
			printHeapBinding(stdout, insp, name)
		case strings.HasPrefix(cmd, "module "):
			name := strings.TrimSpace(strings.TrimPrefix(cmd, "module "))
			if name == "" {
				fmt.Fprintln(stdout, "module requires a name")
				continue
			}
			printHeapModule(stdout, insp, name)
		case strings.HasPrefix(cmd, "query "):
			expr := strings.TrimSpace(strings.TrimPrefix(cmd, "query "))
			if expr == "" {
				fmt.Fprintln(stdout, "query requires an expression")
				continue
			}
			v, err := insp.Query(expr)
			if err != nil {
				fmt.Fprintln(stdout, err)
				continue
			}
			fmt.Fprintln(stdout, v.String())
		case cmd == "json":
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(report); err != nil {
				return err
			}
		case cmd == "q", cmd == "quit", cmd == "exit":
			return nil
		default:
			fmt.Fprintln(stdout, "unknown command; type help")
		}
	}
}

// printHeapScope handles the CLI-facing print heap scope view. It turns
// internal command data into stable text that can be read directly from a
// terminal or redirected to a file.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func printHeapScope(stdout io.Writer, scope []osiris.HeapBinding) {
	if len(scope) == 0 {
		fmt.Fprintln(stdout, "scope is empty")
		return
	}
	for _, bind := range scope {
		if bind.Kind != "" {
			fmt.Fprintf(stdout, "%s [%s] = %s\n", bind.Name, bind.Kind, bind.Value)
			continue
		}
		fmt.Fprintf(stdout, "%s = %s\n", bind.Name, bind.Value)
	}
}

// printHeapModules handles the CLI-facing print heap modules view. It turns
// internal command data into stable text that can be read directly from a
// terminal or redirected to a file.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func printHeapModules(stdout io.Writer, modules []osiris.HeapModule) {
	if len(modules) == 0 {
		fmt.Fprintln(stdout, "no modules loaded")
		return
	}
	for _, mod := range modules {
		label := mod.Ref
		if mod.Package != "" {
			label = mod.Package
		}
		if mod.Version != "" {
			label += "@" + mod.Version
		}
		fmt.Fprintf(stdout, "%s [%s]\n", label, mod.Kind)
		for _, exp := range mod.Exports {
			if exp.Kind != "" {
				fmt.Fprintf(stdout, "  %s [%s] = %s\n", exp.Name, exp.Kind, exp.Value)
				continue
			}
			fmt.Fprintf(stdout, "  %s = %s\n", exp.Name, exp.Value)
		}
	}
}

// printHeapDependencies handles the CLI-facing print heap dependencies view.
// It turns internal command data into stable text that can be read directly
// from a terminal or redirected to a file.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func printHeapDependencies(stdout io.Writer, deps osiris.DependencyReport) {
	fmt.Fprintf(stdout, "root: %s\n", deps.Root)
	if len(deps.Order) > 0 {
		fmt.Fprintln(stdout, "order:")
		for i, ref := range deps.Order {
			node := deps.Nodes[ref]
			fmt.Fprintf(stdout, "  %d. %s (%s) [%s]\n", i+1, node.Label, ref, node.Kind)
		}
	}
	if len(deps.Cycles) == 0 {
		return
	}
	fmt.Fprintln(stdout, "cycles:")
	for _, cycle := range deps.Cycles {
		fmt.Fprintf(stdout, "  - %s\n", strings.Join(cycle, " -> "))
	}
}

// printHeapGraph handles the CLI-facing print heap graph view. It turns
// internal command data into stable text that can be read directly from a
// terminal or redirected to a file.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func printHeapGraph(stdout io.Writer, deps osiris.DependencyReport) {
	if len(deps.Nodes) == 0 {
		fmt.Fprintln(stdout, "graph is empty")
		return
	}
	for _, ref := range deps.Order {
		node := deps.Nodes[ref]
		fmt.Fprintf(stdout, "%s [%s]\n", node.Label, ref)
		for _, dep := range node.Imports {
			fmt.Fprintf(stdout, "  -> %s\n", dep)
		}
	}
}

// printHeapBinding handles the CLI-facing print heap binding view. It turns
// internal command data into stable text that can be read directly from a
// terminal or redirected to a file.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func printHeapBinding(stdout io.Writer, insp *osiris.HeapInspector, name string) {
	if binding, ok := insp.Binding(name); ok {
		if binding.Kind != "" {
			fmt.Fprintf(stdout, "%s [%s] = %s\n", binding.Name, binding.Kind, binding.Value)
		} else {
			fmt.Fprintf(stdout, "%s = %s\n", binding.Name, binding.Value)
		}
		return
	}
	fmt.Fprintf(stdout, "no binding named %q\n", name)
}

// printHeapModule handles the CLI-facing print heap module view. It turns
// internal command data into stable text that can be read directly from a
// terminal or redirected to a file.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func printHeapModule(stdout io.Writer, insp *osiris.HeapInspector, name string) {
	mod, ok := insp.Module(name)
	if !ok {
		fmt.Fprintf(stdout, "no module named %q\n", name)
		return
	}
	label := mod.Ref
	if mod.Package != "" {
		label = mod.Package
	}
	if mod.Version != "" {
		label += "@" + mod.Version
	}
	fmt.Fprintf(stdout, "%s [%s]\n", label, mod.Kind)
	if len(mod.Exports) == 0 {
		fmt.Fprintln(stdout, "  exports: (none)")
		return
	}
	fmt.Fprintln(stdout, "  exports:")
	for _, exp := range mod.Exports {
		if exp.Kind != "" {
			fmt.Fprintf(stdout, "    %s [%s] = %s\n", exp.Name, exp.Kind, exp.Value)
			continue
		}
		fmt.Fprintf(stdout, "    %s = %s\n", exp.Name, exp.Value)
	}
}
