package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/sphireinc/osiris/internal/osiris"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return nil
	}

	switch args[0] {
	case "run":
		return runProgram(ctx, args[1:], stdout)
	case "vm":
		return runProgramVM(ctx, args[1:], stdout)
	case "eval":
		return runEval(ctx, args[1:], stdout)
	case "demo":
		return runDemo(ctx, stdout)
	case "fmt":
		return runFmt(ctx, args[1:], stdout)
	case "viz":
		return runViz(ctx, args[1:], stdout)
	case "wasm":
		return runWASM(ctx, args[1:], stdout)
	case "lsp":
		return runLSP(ctx, stdout, stderr)
	case "map":
		return runMap(ctx, args[1:], stdout)
	case "deps":
		return runDeps(ctx, args[1:], stdout)
	case "heap":
		return runHeap(ctx, args[1:], stdout, os.Stdin)
	case "trace":
		return runTrace(ctx, args[1:], stdout)
	case "debug":
		return runDebug(ctx, args[1:], stdout, os.Stdin)
	case "repl":
		return runRepl(ctx, stdout)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runProgram(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var sourcePath string
	fs.StringVar(&sourcePath, "f", "", "source file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if sourcePath == "" && len(rest) > 0 {
		sourcePath = rest[0]
	}

	src, name, err := readSource(sourcePath)
	if err != nil {
		return err
	}
	prog, err := osiris.Parse(name, src)
	if err != nil {
		return err
	}

	rt := osiris.NewRuntime()
	outputs, err := rt.Run(ctx, prog)
	if err != nil {
		return err
	}
	for i, v := range outputs {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		fmt.Fprintln(stdout, v.String())
	}
	return nil
}

func runProgramVM(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("vm", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var sourcePath string
	fs.StringVar(&sourcePath, "f", "", "source file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if sourcePath == "" && len(rest) > 0 {
		sourcePath = rest[0]
	}

	src, name, err := readSource(sourcePath)
	if err != nil {
		return err
	}
	prog, err := osiris.Parse(name, src)
	if err != nil {
		return err
	}

	rt := osiris.NewRuntime()
	outputs, err := rt.RunVM(ctx, prog)
	if err != nil {
		return err
	}
	for i, v := range outputs {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		fmt.Fprintln(stdout, v.String())
	}
	return nil
}

func runEval(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("eval expects an expression")
	}
	src := "emit " + strings.Join(args, " ")
	prog, err := osiris.Parse("<eval>", src)
	if err != nil {
		return err
	}
	rt := osiris.NewRuntime()
	outputs, err := rt.Run(ctx, prog)
	if err != nil {
		return err
	}
	for i, v := range outputs {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		fmt.Fprintln(stdout, v.String())
	}
	return nil
}

func runDemo(ctx context.Context, stdout io.Writer) error {
	const demo = `
let seed = "osiris::demo"
let basis = hash(seed)
let salt = mix(basis, "scarab", "moon")
let sigil = join("-", chunks(mix(basis, salt), 4))
emit sigil
emit json([seed, basis, salt, sigil])
`
	prog, err := osiris.Parse("<demo>", demo)
	if err != nil {
		return err
	}
	rt := osiris.NewRuntime()
	outputs, err := rt.Run(ctx, prog)
	if err != nil {
		return err
	}
	for i, v := range outputs {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		fmt.Fprintln(stdout, v.String())
	}
	return nil
}

func runFmt(ctx context.Context, args []string, stdout io.Writer) error {
	_ = ctx
	fs := flag.NewFlagSet("fmt", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var write bool
	fs.BoolVar(&write, "w", false, "write result back to file")
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
	formatted, err := osiris.FormatSource(name, src)
	if err != nil {
		return err
	}
	if write {
		if sourcePath == "" || sourcePath == "-" {
			return fmt.Errorf("-w requires a file path")
		}
		return os.WriteFile(sourcePath, []byte(formatted), 0o644)
	}
	_, err = io.WriteString(stdout, formatted)
	return err
}

func runViz(ctx context.Context, args []string, stdout io.Writer) error {
	_ = ctx
	fs := flag.NewFlagSet("viz", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var imports bool
	fs.BoolVar(&imports, "imports", false, "render the import tree instead of the AST")
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
	if imports {
		dot, err := osiris.VizImportsDOT(name, src)
		if err != nil {
			return err
		}
		_, err = io.WriteString(stdout, dot)
		return err
	}
	prog, err := osiris.Parse(name, src)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, osiris.VizProgramDOT(prog))
	return err
}

func runWASM(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("wasm", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var outputPath string
	var binary bool
	fs.StringVar(&outputPath, "o", "", "output path")
	fs.BoolVar(&binary, "binary", false, "write a binary .wasm module instead of WAT")
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
	prog, err := osiris.Parse(name, src)
	if err != nil {
		return err
	}
	rt := osiris.NewRuntime()
	artifact, err := osiris.CompileWASM(ctx, rt, prog)
	if err != nil {
		return err
	}
	if !binary && strings.HasSuffix(outputPath, ".wasm") {
		binary = true
	}
	if outputPath != "" {
		if binary {
			return os.WriteFile(outputPath, artifact.Binary, 0o644)
		}
		return os.WriteFile(outputPath, []byte(artifact.WAT), 0o644)
	}
	if binary {
		_, err = stdout.Write(artifact.Binary)
		return err
	}
	_, err = io.WriteString(stdout, artifact.WAT)
	return err
}

func runRepl(ctx context.Context, stdout io.Writer) error {
	sess := osiris.NewSession(osiris.NewRuntime(), ctx, "<repl>")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		if _, err := io.WriteString(stdout, "osiris> "); err != nil {
			return err
		}
		if !scanner.Scan() {
			return scanner.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		switch line {
		case "", ";":
			continue
		case "exit", "quit", ":q":
			return nil
		}
		src := line
		if !strings.HasPrefix(line, "let ") &&
			!strings.HasPrefix(line, "emit ") &&
			!strings.HasPrefix(line, "import ") &&
			!strings.HasPrefix(line, "macro ") &&
			!strings.HasPrefix(line, "package ") &&
			!strings.HasPrefix(line, "export ") {
			src = "emit " + line
		}
		out, err := sess.ExecSource(src)
		if err != nil {
			if _, writeErr := fmt.Fprintln(stdout, err); writeErr != nil {
				return writeErr
			}
			continue
		}
		for _, v := range out {
			if _, err := io.WriteString(stdout, v.String()+"\n"); err != nil {
				return err
			}
		}
	}
}

func runLSP(ctx context.Context, stdout, stderr io.Writer) error {
	_ = stderr
	server := osiris.NewLanguageServer(osiris.NewRuntime(), stdout, stderr)
	return server.Serve(ctx, os.Stdin)
}

func runMap(ctx context.Context, args []string, stdout io.Writer) error {
	_ = ctx
	fs := flag.NewFlagSet("map", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
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
	prog, err := osiris.Parse(name, src)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(osiris.BuildSourceMap(prog))
}

func runDeps(ctx context.Context, args []string, stdout io.Writer) error {
	_ = ctx
	fs := flag.NewFlagSet("deps", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "emit dependency report as JSON")
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
	rt := osiris.NewRuntime()
	report, err := rt.BuildDependencyReport(name, src)
	if err != nil {
		return err
	}
	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	fmt.Fprintf(stdout, "root: %s\n", report.Root)
	if len(report.Order) > 0 {
		fmt.Fprintln(stdout, "resolution order:")
		for i, ref := range report.Order {
			node := report.Nodes[ref]
			fmt.Fprintf(stdout, "  %d. %s (%s) [%s]\n", i+1, node.Label, ref, node.Kind)
		}
	}
	if len(report.Cycles) > 0 {
		fmt.Fprintln(stdout, "cycles:")
		for _, cycle := range report.Cycles {
			fmt.Fprintf(stdout, "  - %s\n", strings.Join(cycle, " -> "))
		}
	}
	if len(report.Nodes) > 0 {
		fmt.Fprintln(stdout, "modules:")
		for _, ref := range report.Order {
			node := report.Nodes[ref]
			fmt.Fprintf(stdout, "  %s (%s)\n", node.Label, ref)
			if len(node.Imports) == 0 {
				continue
			}
			fmt.Fprintln(stdout, "    imports:")
			for _, dep := range node.Imports {
				fmt.Fprintf(stdout, "      - %s\n", dep)
			}
		}
	}
	return nil
}

type debugBreakpoint struct {
	Line      int
	Column    int
	Condition string
}

type breakpointList []debugBreakpoint

func (b *breakpointList) String() string {
	if len(*b) == 0 {
		return ""
	}
	parts := make([]string, len(*b))
	for i, bp := range *b {
		if bp.Column > 0 {
			parts[i] = fmt.Sprintf("%d:%d", bp.Line, bp.Column)
			continue
		}
		parts[i] = fmt.Sprintf("%d", bp.Line)
	}
	return strings.Join(parts, ",")
}

func (b *breakpointList) Set(v string) error {
	line, col, err := parseBreakpoint(v)
	if err != nil {
		return err
	}
	*b = append(*b, debugBreakpoint{Line: line, Column: col})
	return nil
}

func runDebug(ctx context.Context, args []string, stdout io.Writer, stdin io.Reader) error {
	fs := flag.NewFlagSet("debug", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var jsonOut bool
	var breaks breakpointList
	fs.BoolVar(&jsonOut, "json", false, "emit debug state as JSON")
	fs.Var(&breaks, "break", "add a breakpoint at line or line:column")
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
	prog, err := osiris.Parse(name, src)
	if err != nil {
		return err
	}
	rt := osiris.NewRuntime()
	frames, err := rt.Trace(ctx, prog)
	if err != nil && len(frames) == 0 {
		return err
	}
	sourceMap := osiris.BuildSourceMap(prog)
	if jsonOut {
		return json.NewEncoder(stdout).Encode(map[string]any{
			"sourceMap": sourceMap,
			"frames":    frames,
			"breaks":    breaks,
		})
	}

	bps := append([]debugBreakpoint(nil), breaks...)

	reader := bufio.NewReader(stdin)
	pc := 0
	var last *osiris.TraceFrame
	for {
		if pc >= len(frames) {
			fmt.Fprintln(stdout, "program finished")
			return nil
		}
		frame := frames[pc]
		last = &frame
		printDebugFrame(stdout, frame)
		if breakpointHit(ctx, rt, prog, frame, bps) {
			fmt.Fprintf(stdout, "breakpoint hit at line %d\n", frame.Range.Start.Line)
		}
		for {
			if _, err := io.WriteString(stdout, "debug> "); err != nil {
				return err
			}
			line, err := reader.ReadString('\n')
			if err != nil && strings.TrimSpace(line) == "" {
				return err
			}
			cmd := strings.TrimSpace(line)
			switch {
			case cmd == "", cmd == "s", cmd == "step":
				pc++
				goto nextFrame
			case cmd == "c", cmd == "continue":
				pc++
				for pc < len(frames) {
					if breakpointHit(ctx, rt, prog, frames[pc], bps) {
						break
					}
					printDebugFrame(stdout, frames[pc])
					last = &frames[pc]
					pc++
				}
				goto nextFrame
			case cmd == "m", cmd == "map":
				if err := json.NewEncoder(stdout).Encode(sourceMap); err != nil {
					return err
				}
			case cmd == "scope", cmd == "locals":
				printDebugScope(stdout, last)
			case cmd == "stack":
				printDebugStack(stdout, last)
			case strings.HasPrefix(cmd, "watch "):
				if last == nil {
					fmt.Fprintln(stdout, "no active frame")
					continue
				}
				expr := strings.TrimSpace(strings.TrimPrefix(cmd, "watch "))
				if expr == "" {
					fmt.Fprintln(stdout, "watch requires an expression")
					continue
				}
				v, err := rt.EvalAt(ctx, prog, last.Index, expr)
				if err != nil {
					fmt.Fprintln(stdout, err)
					continue
				}
				fmt.Fprintln(stdout, v.String())
			case cmd == "frame":
				if last != nil {
					printDebugFrame(stdout, *last)
				}
			case strings.HasPrefix(cmd, "break "):
				bp, err := parseDebugBreakpoint(strings.TrimSpace(strings.TrimPrefix(cmd, "break ")))
				if err != nil {
					fmt.Fprintln(stdout, err)
					continue
				}
				bps = append(bps, bp)
				if bp.Column > 0 {
					fmt.Fprintf(stdout, "added breakpoint at line %d:%d\n", bp.Line, bp.Column)
				} else {
					fmt.Fprintf(stdout, "added breakpoint at line %d\n", bp.Line)
				}
			case cmd == "help":
				fmt.Fprintln(stdout, "commands: step|s, continue|c, break <line[:col]> [if expr], frame, scope|locals, stack, watch <expr>, map, quit|q")
			case cmd == "q", cmd == "quit", cmd == "exit":
				return nil
			default:
				fmt.Fprintln(stdout, "unknown command; type help")
			}
		}
	nextFrame:
		continue
	}
}

func parseBreakpoint(raw string) (int, int, error) {
	if raw == "" {
		return 0, 0, fmt.Errorf("empty breakpoint")
	}
	linePart := raw
	colPart := ""
	if before, after, ok := strings.Cut(raw, ":"); ok {
		linePart = before
		colPart = after
	}
	line, err := strconv.Atoi(linePart)
	if err != nil || line <= 0 {
		return 0, 0, fmt.Errorf("invalid breakpoint %q", raw)
	}
	col := 0
	if colPart != "" {
		col, err = strconv.Atoi(colPart)
		if err != nil || col <= 0 {
			return 0, 0, fmt.Errorf("invalid breakpoint %q", raw)
		}
	}
	return line, col, nil
}

func parseDebugBreakpoint(raw string) (debugBreakpoint, error) {
	spec := raw
	cond := ""
	if before, after, ok := strings.Cut(raw, " if "); ok {
		spec = strings.TrimSpace(before)
		cond = strings.TrimSpace(after)
	}
	line, col, err := parseBreakpoint(spec)
	if err != nil {
		return debugBreakpoint{}, err
	}
	return debugBreakpoint{Line: line, Column: col, Condition: cond}, nil
}

func breakpointHit(ctx context.Context, rt *osiris.Runtime, prog *osiris.Program, frame osiris.TraceFrame, breaks []debugBreakpoint) bool {
	for _, bp := range breaks {
		if bp.Line != frame.Range.Start.Line {
			continue
		}
		if bp.Column > 0 && bp.Column != frame.Range.Start.Column {
			continue
		}
		if bp.Condition == "" {
			return true
		}
		v, err := rt.EvalAt(ctx, prog, frame.Index, bp.Condition)
		if err == nil && v.Truthy() {
			return true
		}
	}
	return false
}

func printDebugFrame(stdout io.Writer, frame osiris.TraceFrame) {
	fmt.Fprintf(stdout, "#%d %s\n", frame.Index+1, frame.Statement)
	if frame.Range.Start.Line > 0 {
		fmt.Fprintf(stdout, "  at %d:%d-%d:%d\n", frame.Range.Start.Line, frame.Range.Start.Column, frame.Range.End.Line, frame.Range.End.Column)
	}
	if len(frame.Outputs) > 0 {
		fmt.Fprintf(stdout, "  outputs: %s\n", strings.Join(frame.Outputs, ", "))
	}
	if len(frame.Scope) > 0 {
		fmt.Fprintln(stdout, "  scope:")
		for _, bind := range frame.Scope {
			fmt.Fprintf(stdout, "    %s = %s\n", bind.Name, bind.Value)
		}
	}
	if len(frame.Stack) > 0 {
		fmt.Fprintln(stdout, "  stack:")
		for i := len(frame.Stack) - 1; i >= 0; i-- {
			fmt.Fprintf(stdout, "    %d: %s\n", len(frame.Stack)-i, frame.Stack[i])
		}
	}
	if frame.Error != "" {
		fmt.Fprintf(stdout, "  error: %s\n", frame.Error)
	}
}

func printDebugScope(stdout io.Writer, frame *osiris.TraceFrame) {
	if frame == nil {
		fmt.Fprintln(stdout, "no frame")
		return
	}
	if len(frame.Scope) == 0 {
		fmt.Fprintln(stdout, "scope is empty")
		return
	}
	for _, bind := range frame.Scope {
		fmt.Fprintf(stdout, "%s = %s\n", bind.Name, bind.Value)
	}
}

func printDebugStack(stdout io.Writer, frame *osiris.TraceFrame) {
	if frame == nil {
		fmt.Fprintln(stdout, "no frame")
		return
	}
	if len(frame.Stack) == 0 {
		fmt.Fprintln(stdout, "stack is empty")
		return
	}
	for i := len(frame.Stack) - 1; i >= 0; i-- {
		fmt.Fprintf(stdout, "%d: %s\n", len(frame.Stack)-i, frame.Stack[i])
	}
}

func runTrace(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("trace", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "emit trace frames as JSON")
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
	prog, err := osiris.Parse(name, src)
	if err != nil {
		return err
	}
	rt := osiris.NewRuntime()
	frames, err := rt.Trace(ctx, prog)
	if err != nil && len(frames) == 0 {
		return err
	}
	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(frames)
	}
	for i, frame := range frames {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		fmt.Fprintf(stdout, "#%d %s\n", frame.Index+1, frame.Statement)
		if len(frame.Outputs) > 0 {
			fmt.Fprintf(stdout, "  outputs: %s\n", strings.Join(frame.Outputs, ", "))
		}
		if len(frame.Scope) > 0 {
			fmt.Fprintln(stdout, "  scope:")
			for _, bind := range frame.Scope {
				fmt.Fprintf(stdout, "    %s = %s\n", bind.Name, bind.Value)
			}
		}
		if frame.Error != "" {
			fmt.Fprintf(stdout, "  error: %s\n", frame.Error)
		}
	}
	return err
}

func readSource(path string) (string, string, error) {
	if path == "" || path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", "<stdin>", err
		}
		return string(data), "<stdin>", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	return string(data), path, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "osiris: a concurrent sigil engine")
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  osiris run [-f file] [file]")
	fmt.Fprintln(w, "  osiris vm [-f file] [file]")
	fmt.Fprintln(w, "  osiris eval <expression>")
	fmt.Fprintln(w, "  osiris demo")
	fmt.Fprintln(w, "  osiris fmt [-w] [file]")
	fmt.Fprintln(w, "  osiris viz [-imports] [file]")
	fmt.Fprintln(w, "  osiris wasm [-binary] [-o out.wat|out.wasm] [file]")
	fmt.Fprintln(w, "  osiris lsp")
	fmt.Fprintln(w, "  osiris map [file]")
	fmt.Fprintln(w, "  osiris deps [-json] [file]")
	fmt.Fprintln(w, "  osiris heap [-json] [-query expr] [file]")
	fmt.Fprintln(w, "  osiris trace [-json] [file]")
	fmt.Fprintln(w, "  osiris debug [-json] [-break line[:col]] [file]")
	fmt.Fprintln(w, "  osiris repl")
}
