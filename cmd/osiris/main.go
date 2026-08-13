package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/sphireinc/osiris/internal/osiris"
)

// Release metadata is injected by the release build with -ldflags. Development
// builds keep explicit, actionable defaults instead of pretending to be a
// tagged release.
var (
	version   = "0.1.0-dev"
	commit    = "unknown"
	buildDate = "unknown"
)

// main is the CLI entry point for the Osiris executable. It hands control to
// the command dispatcher, routes the selected subcommand, and exits with the
// resulting status code so shell callers can treat the program like a normal
// command-line tool.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func main() {
	if code := execute(context.Background(), os.Args[1:], os.Stdout, os.Stderr); code != cliExitSuccess {
		os.Exit(code)
	}
}

const (
	cliExitSuccess  = 0
	cliExitUsage    = 64
	cliExitCompile  = 65
	cliExitInput    = 66
	cliExitRuntime  = 70
	cliExitInternal = 71
)

// cliDiagnostic is the versioned failure envelope written by
// --diagnostics=json. Successful command payloads retain their command-owned
// stdout schema; diagnostics always go to stderr.
type cliDiagnostic struct {
	SchemaVersion int    `json:"schema_version"`
	Command       string `json:"command,omitempty"`
	Category      string `json:"category"`
	ExitCode      int    `json:"exit_code"`
	Message       string `json:"message"`
}

type cliPhaseError struct {
	exitCode int
	err      error
}

func (e *cliPhaseError) Error() string { return e.err.Error() }
func (e *cliPhaseError) Unwrap() error { return e.err }

func cliCompileError(err error) error {
	if err == nil {
		return nil
	}
	return &cliPhaseError{exitCode: cliExitCompile, err: err}
}

// execute is the process boundary for the CLI. It owns exit-code selection and
// diagnostic rendering while run remains directly testable by command tests.
func execute(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	diagnosticsJSON, args, optionErr := consumeDiagnosticOption(args)
	command := ""
	if len(args) > 0 {
		command = args[0]
	}
	if optionErr != nil {
		writeCLIDiagnostic(stderr, command, cliExitUsage, optionErr, diagnosticsJSON)
		return cliExitUsage
	}
	if err := run(ctx, args, stdout, stderr); err != nil {
		code := classifyCLIExit(command, err)
		writeCLIDiagnostic(stderr, command, code, err, diagnosticsJSON)
		return code
	}
	return cliExitSuccess
}

func consumeDiagnosticOption(args []string) (bool, []string, error) {
	if len(args) == 0 || !strings.HasPrefix(args[0], "--diagnostics") {
		return false, args, nil
	}
	if args[0] != "--diagnostics=json" {
		return false, args, fmt.Errorf("--diagnostics only supports json")
	}
	return true, args[1:], nil
}

func classifyCLIExit(command string, err error) int {
	var phaseErr *cliPhaseError
	if errors.As(err, &phaseErr) {
		return phaseErr.exitCode
	}
	if errors.Is(err, flag.ErrHelp) || strings.HasPrefix(err.Error(), "flag provided but not defined") || strings.HasPrefix(err.Error(), "invalid value") || strings.Contains(err.Error(), " expects ") || strings.Contains(err.Error(), " is required") || strings.Contains(err.Error(), "cannot be combined") || strings.HasPrefix(err.Error(), "unknown command") {
		return cliExitUsage
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return cliExitInput
	}
	if strings.Contains(err.Error(), "error[parse_error]") || strings.Contains(err.Error(), "error[type_error]") || strings.Contains(err.Error(), "native C backend") || strings.Contains(err.Error(), "unsupported lowering") {
		return cliExitCompile
	}
	switch command {
	case "run", "vm", "test", "eval", "demo", "repl", "debug", "trace":
		return cliExitRuntime
	case "fmt", "opt", "ir", "ssa", "cfg", "dom", "loop", "c", "obj", "build", "compile", "infer", "alias", "pure", "closure", "viz", "wasm", "gpu", "map", "deps", "get", "mod", "heap", "lsp":
		return cliExitCompile
	default:
		return cliExitInternal
	}
}

func writeCLIDiagnostic(w io.Writer, command string, code int, err error, asJSON bool) {
	category := map[int]string{
		cliExitUsage: "usage", cliExitCompile: "compile", cliExitInput: "input", cliExitRuntime: "runtime", cliExitInternal: "internal",
	}[code]
	if asJSON {
		_ = json.NewEncoder(w).Encode(cliDiagnostic{SchemaVersion: 1, Command: command, Category: category, ExitCode: code, Message: err.Error()})
		return
	}
	fmt.Fprintf(w, "osiris: %s\n", err)
	if code == cliExitUsage {
		fmt.Fprintln(w)
		printUsage(w)
	}
}

// run handles the run command path for the CLI. It parses the command-specific
// flags and arguments, performs the requested Osiris operation, and writes the
// user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return nil
	}

	switch args[0] {
	case "version":
		return runVersion(stdout)
	case "run":
		return runProgram(ctx, args[1:], stdout)
	case "test":
		return runTests(ctx, args[1:], stdout)
	case "vm":
		return runProgramVM(ctx, args[1:], stdout)
	case "eval":
		return runEval(ctx, args[1:], stdout)
	case "demo":
		return runDemo(ctx, stdout)
	case "fmt":
		return runFmt(ctx, args[1:], stdout)
	case "opt":
		return runOpt(ctx, args[1:], stdout)
	case "ir":
		return runIR(ctx, args[1:], stdout)
	case "ssa":
		return runSSA(ctx, args[1:], stdout)
	case "cfg":
		return runCFG(ctx, args[1:], stdout)
	case "dom":
		return runDom(ctx, args[1:], stdout)
	case "loop":
		return runLoop(ctx, args[1:], stdout)
	case "c":
		return runC(ctx, args[1:], stdout)
	case "obj":
		return runObj(ctx, args[1:], stdout)
	case "build":
		return runBuild(ctx, args[1:], stdout)
	case "compile":
		return runCompile(ctx, args[1:], stdout)
	case "infer":
		return runInfer(ctx, args[1:], stdout)
	case "alias":
		return runAlias(ctx, args[1:], stdout)
	case "pure":
		return runPurity(ctx, args[1:], stdout)
	case "closure":
		return runClosure(ctx, args[1:], stdout)
	case "viz":
		return runViz(ctx, args[1:], stdout)
	case "wasm":
		return runWASM(ctx, args[1:], stdout)
	case "gpu":
		return runGPU(ctx, args[1:], stdout)
	case "lsp":
		return runLSP(ctx, stdout, stderr)
	case "map":
		return runMap(ctx, args[1:], stdout)
	case "deps":
		return runDeps(ctx, args[1:], stdout)
	case "get":
		return runGet(ctx, args[1:], stdout)
	case "mod":
		return runMod(ctx, args[1:], stdout)
	case "heap":
		return runHeap(ctx, args[1:], stdout, os.Stdin)
	case "trace":
		return runTrace(ctx, args[1:], stdout)
	case "debug":
		return runDebug(ctx, args[1:], stdout, os.Stdin)
	case "repl":
		return runRepl(ctx, stdout)
	case "help", "-h", "--help":
		if len(args) > 1 {
			if printCommandHelp(stdout, args[1]) {
				return nil
			}
			return fmt.Errorf("unknown command %q", args[1])
		}
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

type testEvent struct {
	SchemaVersion   int                                  `json:"schema_version"`
	Event           string                               `json:"event"`
	Name            string                               `json:"name"`
	File            string                               `json:"file"`
	Backend         string                               `json:"backend"`
	Status          string                               `json:"status"`
	Passed          bool                                 `json:"passed"`
	Skipped         bool                                 `json:"skipped,omitempty"`
	ExpectedFailure bool                                 `json:"expected_failure,omitempty"`
	UnexpectedPass  bool                                 `json:"unexpected_pass,omitempty"`
	DurationNS      int64                                `json:"duration_ns"`
	Outputs         []string                             `json:"outputs,omitempty"`
	Assertions      []osiris.LanguageTestAssertionResult `json:"assertions,omitempty"`
	Logs            []string                             `json:"logs,omitempty"`
	Location        osiris.LanguageTestLocation          `json:"location"`
	Error           string                               `json:"error,omitempty"`
	Coverage        *osiris.LanguageCoverageReport       `json:"coverage,omitempty"`
	Benchmark       *osiris.LanguageBenchmarkReport      `json:"benchmark,omitempty"`
	Artifacts       []osiris.LanguageTestArtifact        `json:"artifacts,omitempty"`
}

type filterValue struct{ value *string }

func (f filterValue) String() string {
	if f.value == nil {
		return ""
	}
	return *f.value
}

func (f filterValue) Set(value string) error {
	if f.value != nil {
		*f.value = value
	}
	return nil
}

func runTests(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	backend := fs.String("backend", "interpreter", "interpreter, vm, or native")
	jsonOut := fs.Bool("json", false, "write one stable JSON event per test")
	integration := fs.Bool("integration", false, "opt into host providers instead of deterministic fakes")
	capabilityText := fs.String("capabilities", "", "comma-separated host capabilities for integration tests")
	timeout := fs.Duration("timeout", 0, "per-test timeout")
	filter := fs.String("run", "", "run names matching a glob or substring")
	fs.Var(filterValue{value: filter}, "filter", "alias for -run")
	parallel := fs.Int("parallel", 1, "maximum concurrent test cases")
	deterministic := fs.Bool("deterministic", true, "retain discovery and output order")
	race := fs.Bool("race", false, "enable bounded concurrent execution checks for interpreter or VM")
	list := fs.Bool("list", false, "list the stable test inventory without executing cases")
	coverage := fs.String("coverage", "", "write statement/branch/function coverage JSON")
	artifacts := fs.String("artifacts", ".osiris/test-artifacts", "directory for fuzz and replay artifacts")
	bench := fs.Bool("bench", false, "run benchmark declarations")
	benchJSON := fs.String("bench-json", "", "write benchmark JSON to this path")
	fuzz := fs.Bool("fuzz", false, "run bounded fuzz declarations")
	fuzzRuns := fs.Int("fuzz-runs", 32, "maximum fuzz inputs per declaration")
	fuzzSeed := fs.Int64("fuzz-seed", 1, "default reproducible fuzz seed")
	fuzzCorpus := fs.String("fuzz-corpus", "", "default fuzz corpus file or directory")
	update := fs.Bool("update", false, "update golden files")
	cacheDir := fs.String("cache-dir", ".osiris/test-cache", "inventory cache directory")
	noCache := fs.Bool("no-cache", false, "disable inventory cache writes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	capabilities := []string{}
	if *capabilityText != "" {
		for _, capability := range strings.Split(*capabilityText, ",") {
			capability = strings.TrimSpace(capability)
			if capability != "" {
				capabilities = append(capabilities, capability)
			}
		}
	}
	options := osiris.LanguageTestOptions{Backend: *backend, Integration: *integration, Capabilities: capabilities, Timeout: *timeout, Filter: *filter, Parallel: *parallel, Deterministic: *deterministic, Race: *race, CoveragePath: *coverage, ArtifactsDir: *artifacts, Benchmark: *bench, BenchmarkJSON: *benchJSON, Fuzz: *fuzz, FuzzRuns: *fuzzRuns, FuzzSeed: *fuzzSeed, FuzzCorpus: *fuzzCorpus, UpdateGolden: *update, CacheDir: *cacheDir, NoCache: *noCache}
	if *list {
		inventory, err := osiris.DiscoverLanguageTestInventory(fs.Args(), *filter)
		if err != nil {
			return err
		}
		if *jsonOut {
			return json.NewEncoder(stdout).Encode(inventory)
		}
		for _, entry := range inventory.Tests {
			fmt.Fprintf(stdout, "%s %s:%d:%d\n", entry.Name, entry.File, entry.Location.Start.Line, entry.Location.Start.Column)
		}
		return nil
	}
	results, err := osiris.RunLanguageTests(ctx, fs.Args(), options)
	if err != nil {
		return err
	}
	failed := 0
	compileFailed := false
	for _, result := range results {
		if !result.Passed {
			failed++
		}
		if result.Status == "compile_error" {
			compileFailed = true
		}
		status := "ok"
		if result.Status == "skip" {
			status = "SKIP"
		} else if result.Status == "expected_failure" {
			status = "XFAIL"
		} else if !result.Passed {
			status = "FAIL"
		}
		message := result.ErrorText
		if *jsonOut {
			_ = json.NewEncoder(stdout).Encode(testEvent{SchemaVersion: 1, Event: "test", Name: result.Name, File: result.File, Backend: result.Backend, Status: status, Passed: result.Passed, Skipped: result.Skipped, ExpectedFailure: result.ExpectedFailure, UnexpectedPass: result.UnexpectedPass, DurationNS: result.DurationNS, Outputs: result.OutputStrings, Assertions: result.Assertions, Logs: result.Logs, Location: result.Location, Error: message, Coverage: result.Coverage, Benchmark: result.Benchmark, Artifacts: result.Artifacts})
		} else if message == "" {
			fmt.Fprintf(stdout, "%s %s\n", status, result.Name)
		} else {
			fmt.Fprintf(stdout, "%s %s: %s\n", status, result.Name, message)
		}
	}
	if len(results) == 0 {
		return fmt.Errorf("no Osiris tests found")
	}
	if compileFailed {
		return cliCompileError(fmt.Errorf("one or more Osiris test files failed to compile"))
	}
	if failed > 0 {
		return fmt.Errorf("%d Osiris test(s) failed", failed)
	}
	return nil
}

func runVersion(stdout io.Writer) error {
	_, err := fmt.Fprintf(stdout, "osiris %s\nsemantics: %s\ncommit: %s\nbuilt: %s\ngo: %s\ntarget: %s/%s\n", version, osiris.CoreSemanticsVersion, commit, buildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	return err
}

// runProgram handles the run program command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
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
		return cliCompileError(err)
	}

	rt := osiris.NewRuntime()
	outputs, err := rt.Run(ctx, prog)
	writeProgramOutputs(stdout, outputs)
	if err != nil {
		return err
	}
	return nil
}

// runProgramVM handles the run program vm command path for the CLI. It parses
// the command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
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
		return cliCompileError(err)
	}

	rt := osiris.NewRuntime()
	outputs, err := rt.RunVM(ctx, prog)
	writeProgramOutputs(stdout, outputs)
	if err != nil {
		return err
	}
	return nil
}

func writeProgramOutputs(stdout io.Writer, outputs []osiris.Value) {
	for i, v := range outputs {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		fmt.Fprintln(stdout, v.String())
	}
}

// runEval handles the run eval command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runEval(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("eval expects an expression")
	}
	src := "emit " + strings.Join(args, " ")
	prog, err := osiris.Parse("<eval>", src)
	if err != nil {
		return cliCompileError(err)
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

// runDemo handles the run demo command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
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

// runFmt handles the run fmt command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
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
		return cliCompileError(err)
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

// runOpt handles the run opt command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runOpt(ctx context.Context, args []string, stdout io.Writer) error {
	_ = ctx
	fs := flag.NewFlagSet("opt", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var write bool
	var report bool
	var jsonReport bool
	fs.BoolVar(&write, "w", false, "write result back to file")
	fs.BoolVar(&report, "report", false, "write a deterministic optimization report")
	fs.BoolVar(&jsonReport, "json", false, "write the optimization report as JSON")
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
		return cliCompileError(err)
	}
	optProg, optReport := osiris.OptimizeProgramWithReport(prog)
	formatted := osiris.FormatProgram(optProg)
	if report || jsonReport {
		if write {
			return fmt.Errorf("-w cannot be combined with -report or -json")
		}
		if jsonReport {
			return json.NewEncoder(stdout).Encode(optReport)
		}
		for _, pass := range optReport.Passes {
			if _, err := fmt.Fprintf(stdout, "%s: %s (%d -> %d statements)", pass.Name, pass.Status, pass.BeforeStatements, pass.AfterStatements); err != nil {
				return err
			}
			if pass.Reason != "" {
				if _, err := fmt.Fprintf(stdout, "; %s", pass.Reason); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(stdout); err != nil {
				return err
			}
		}
		return nil
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

// runIR handles the run ir command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runIR(ctx context.Context, args []string, stdout io.Writer) error {
	_ = ctx
	fs := flag.NewFlagSet("ir", flag.ContinueOnError)
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
	rt := osiris.NewRuntime()
	ir, err := rt.BuildIR(ctx, prog)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, osiris.FormatIRProgram(ir))
	return err
}

// runSSA handles the run ssa command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runSSA(ctx context.Context, args []string, stdout io.Writer) error {
	_ = ctx
	fs := flag.NewFlagSet("ssa", flag.ContinueOnError)
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
	rt := osiris.NewRuntime()
	ssaProg, err := rt.BuildSSA(ctx, prog)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, osiris.FormatSSAProgram(ssaProg))
	return err
}

// runCFG handles the run cfg command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runCFG(ctx context.Context, args []string, stdout io.Writer) error {
	_ = ctx
	fs := flag.NewFlagSet("cfg", flag.ContinueOnError)
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
	rt := osiris.NewRuntime()
	cfgProg, err := rt.BuildCFG(ctx, prog)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, osiris.FormatCFGProgram(cfgProg))
	return err
}

// runDom handles the run dom command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runDom(ctx context.Context, args []string, stdout io.Writer) error {
	_ = ctx
	fs := flag.NewFlagSet("dom", flag.ContinueOnError)
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
	rt := osiris.NewRuntime()
	domProg, err := rt.BuildDom(ctx, prog)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, osiris.FormatDomProgram(domProg))
	return err
}

// runLoop handles the run loop command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runLoop(ctx context.Context, args []string, stdout io.Writer) error {
	_ = ctx
	fs := flag.NewFlagSet("loop", flag.ContinueOnError)
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
	rt := osiris.NewRuntime()
	loopProg, err := rt.BuildLoopProgram(ctx, prog)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, osiris.FormatLoopProgram(loopProg))
	return err
}

// runC handles the run c command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runC(ctx context.Context, args []string, stdout io.Writer) error {
	return runCBackend(ctx, args, stdout, cBackendSource)
}

// runObj handles the run obj command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runObj(ctx context.Context, args []string, stdout io.Writer) error {
	return runCBackend(ctx, args, stdout, cBackendObject)
}

// runBuild handles the run build command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runBuild(ctx context.Context, args []string, stdout io.Writer) error {
	return runCBackend(ctx, args, stdout, cBackendExecutable)
}

// runCompile handles the run compile command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runCompile(ctx context.Context, args []string, stdout io.Writer) error {
	return runBuild(ctx, args, stdout)
}

type cBackendMode uint8

const (
	cBackendSource cBackendMode = iota
	cBackendObject
	cBackendExecutable
)

type stringListFlag []string

func (f *stringListFlag) String() string { return strings.Join(*f, " ") }

func (f *stringListFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

// runCBackend handles the run cbackend command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runCBackend(ctx context.Context, args []string, stdout io.Writer, mode cBackendMode) error {
	fs := flag.NewFlagSet("c", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var outputPath string
	var cacheDir string
	var linalgProvider string
	var linalgProviderFlags stringListFlag
	fs.StringVar(&outputPath, "o", "", "output path")
	fs.StringVar(&cacheDir, "cache-dir", "", "content-addressed executable cache directory")
	fs.StringVar(&linalgProvider, "linalg-provider", "", "C source implementing the Osiris numerical-provider ABI")
	fs.Var(&linalgProviderFlags, "linalg-provider-flag", "linker argument for the numerical provider (repeatable)")
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
	options := osiris.CCompileOptions{LinalgProviderSource: linalgProvider, LinalgProviderFlags: append([]string(nil), linalgProviderFlags...), PublicEmitSpacing: true}
	artifact, err := osiris.CompileCWithOptions(ctx, rt, prog, options)
	if err != nil {
		return cliCompileError(err)
	}
	switch mode {
	case cBackendSource:
		if outputPath != "" {
			return os.WriteFile(outputPath, []byte(artifact.Source), 0o644)
		}
		_, err = io.WriteString(stdout, artifact.Source)
		return err
	case cBackendObject:
		if outputPath == "" {
			return fmt.Errorf("-o is required for object-file output")
		}
		return osiris.CompileCObject(ctx, artifact.Source, outputPath)
	case cBackendExecutable:
		if outputPath == "" {
			return fmt.Errorf("-o is required for executable output")
		}
		if cacheDir != "" {
			_, err := osiris.CompileCExecutableCachedWithOptions(ctx, artifact.Source, outputPath, cacheDir, options)
			return err
		}
		return osiris.CompileCExecutableWithOptions(ctx, artifact.Source, outputPath, options)
	default:
		return fmt.Errorf("unsupported C backend mode")
	}
}

// runInfer handles the run infer command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runInfer(ctx context.Context, args []string, stdout io.Writer) error {
	_ = ctx
	fs := flag.NewFlagSet("infer", flag.ContinueOnError)
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
	rt := osiris.NewRuntime()
	inferProg, err := rt.InferProgram(ctx, prog)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, osiris.FormatInferProgram(inferProg))
	return err
}

// runAlias handles the run alias command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runAlias(ctx context.Context, args []string, stdout io.Writer) error {
	_ = ctx
	fs := flag.NewFlagSet("alias", flag.ContinueOnError)
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
	rt := osiris.NewRuntime()
	aliasProg, err := rt.AnalyzeAlias(ctx, prog)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, osiris.FormatAliasProgram(aliasProg))
	return err
}

// runPurity handles the run purity command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runPurity(ctx context.Context, args []string, stdout io.Writer) error {
	_ = ctx
	fs := flag.NewFlagSet("pure", flag.ContinueOnError)
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
	rt := osiris.NewRuntime()
	purityProg, err := rt.AnalyzePurity(ctx, prog)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, osiris.FormatPurityProgram(purityProg))
	return err
}

// runClosure handles the run closure command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runClosure(ctx context.Context, args []string, stdout io.Writer) error {
	_ = ctx
	fs := flag.NewFlagSet("closure", flag.ContinueOnError)
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
	rt := osiris.NewRuntime()
	closureProg, err := rt.AnalyzeClosure(context.Background(), prog)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, osiris.FormatClosureProgram(closureProg))
	return err
}

// runViz handles the run viz command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
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

// runWASM handles the run wasm command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
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

// runGPU handles the run gpu command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runGPU(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("gpu", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var outputPath string
	var jsonOut bool
	fs.StringVar(&outputPath, "o", "", "output path")
	fs.BoolVar(&jsonOut, "json", false, "write a JSON kernel plan instead of OpenCL source")
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
	artifact, err := osiris.CompileGPU(ctx, prog)
	if err != nil {
		return err
	}
	if outputPath != "" {
		if jsonOut || strings.HasSuffix(outputPath, ".json") {
			raw, err := osiris.EncodeGPUArtifactJSON(artifact)
			if err != nil {
				return err
			}
			return os.WriteFile(outputPath, raw, 0o644)
		}
		return os.WriteFile(outputPath, []byte(artifact.Source), 0o644)
	}
	if jsonOut {
		raw, err := osiris.EncodeGPUArtifactJSON(artifact)
		if err != nil {
			return err
		}
		_, err = stdout.Write(raw)
		return err
	}
	_, err = io.WriteString(stdout, artifact.Source)
	return err
}

// runRepl handles the run repl command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runRepl(ctx context.Context, stdout io.Writer) error {
	return runReplWithInput(ctx, os.Stdin, stdout)
}

// runReplWithInput is the testable REPL loop. It frames input only; parsing
// and execution remain owned by the language parser and Session APIs so the
// REPL cannot grow a second set of language rules.
func runReplWithInput(ctx context.Context, input io.Reader, stdout io.Writer) error {
	sess := osiris.NewSession(osiris.NewRuntime(), ctx, "<repl>")
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	var pending string
	var history string
	for {
		prompt := "osiris> "
		if pending != "" {
			prompt = "....> "
		}
		if _, err := io.WriteString(stdout, prompt); err != nil {
			return err
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			if strings.TrimSpace(pending) != "" {
				return errors.New("<repl>: incomplete input at end of input")
			}
			return nil
		}
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if pending == "" {
			switch trimmed {
			case "", ";":
				continue
			case "exit", "quit", ":q":
				return nil
			}
		}
		if pending == "" {
			pending = line
		} else {
			pending += "\n" + line
		}
		src, parseErr, complete := replSourceForInputWithHistory(history, pending)
		if !complete {
			continue
		}
		pending = ""
		if parseErr != nil {
			if _, writeErr := fmt.Fprintln(stdout, parseErr); writeErr != nil {
				return writeErr
			}
			continue
		}
		out, err := sess.ExecSource(src)
		if err != nil {
			if _, writeErr := fmt.Fprintln(stdout, err); writeErr != nil {
				return writeErr
			}
			continue
		}
		if history == "" {
			history = src
		} else {
			history += "\n" + src
		}
		for _, v := range out {
			if _, err := io.WriteString(stdout, v.String()+"\n"); err != nil {
				return err
			}
		}
	}
}

// replSourceForInput asks the parser whether input is a language source unit
// before trying the REPL's expression convenience. This makes declarations
// such as const/let/var and compound statements take exactly the same path as
// a source file while preserving the interactive `emit expression` shortcut.
func replSourceForInput(source string) (string, error, bool) {
	return replSourceForInputWithHistory("", source)
}

func replSourceForInputWithHistory(history, source string) (string, error, bool) {
	parseSource := source
	if history != "" {
		parseSource = history + "\n" + source
	}
	if _, err := osiris.Parse("<repl>", parseSource); err == nil {
		return source, nil, true
	} else if replInputIncomplete(source) {
		return "", nil, false
	} else {
		parseErr := err
		emitted := "emit " + source
		emittedSource := emitted
		if history != "" {
			emittedSource = history + "\n" + emitted
		}
		if _, err := osiris.Parse("<repl>", emittedSource); err == nil {
			return emitted, nil, true
		}
		return "", parseErr, true
	}
}

// replInputIncomplete performs only lexical input framing. It tracks paired
// delimiters and unterminated string/tag/comment state; it deliberately does
// not recognize declaration names or evaluator rules.
func replInputIncomplete(source string) bool {
	var stack []byte
	var quote byte
	escaped := false
	lineComment := false
	invalidDelimiter := false
	for i := 0; i < len(source); i++ {
		ch := source[i]
		if lineComment {
			if ch == '\n' {
				lineComment = false
			}
			continue
		}
		if quote != 0 {
			if quote == '"' && escaped {
				escaped = false
				continue
			}
			if quote == '"' && ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '/' && i+1 < len(source) && source[i+1] == '/' {
			lineComment = true
			i++
			continue
		}
		if ch == '#' {
			lineComment = true
			continue
		}
		switch ch {
		case '"', '`':
			quote = ch
		case '(', '[', '{':
			stack = append(stack, ch)
		case ')', ']', '}':
			if len(stack) > 0 && ((stack[len(stack)-1] == '(' && ch == ')') ||
				(stack[len(stack)-1] == '[' && ch == ']') ||
				(stack[len(stack)-1] == '{' && ch == '}')) {
				stack = stack[:len(stack)-1]
			} else {
				invalidDelimiter = true
			}
		}
	}
	if invalidDelimiter {
		return false
	}
	if quote != 0 || lineComment || len(stack) != 0 {
		return true
	}
	trimmed := strings.TrimSpace(source)
	for _, suffix := range []string{"=", "=>", ",", "+", "-", "*", "/", "%", "&&", "||", "|", "&", "?"} {
		if strings.HasSuffix(trimmed, suffix) {
			return true
		}
	}
	return false
}

// runLSP handles the run lsp command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
func runLSP(ctx context.Context, stdout, stderr io.Writer) error {
	_ = stderr
	server := osiris.NewLanguageServer(osiris.NewRuntime(), stdout, stderr)
	return server.Serve(ctx, os.Stdin)
}

// runMap handles the run map command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
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

// runDeps handles the run deps command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
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

// runMod owns generated module lock/cache lifecycle actions. It deliberately
// does not expose manual lockfile editing.
func runMod(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("mod expects init, tidy, download, verify, sync, clean, graph, list, audit, vendor, or workspace")
	}
	action := args[0]
	root := osiris.ModuleRootFor("")
	jsonOut := false
	for _, arg := range args[1:] {
		if arg == "-json" || arg == "--json" {
			jsonOut = true
		}
	}
	switch action {
	case "init":
		modulePath := ""
		if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
			modulePath = args[1]
		}
		if modulePath == "" {
			return fmt.Errorf("mod init requires a canonical module path")
		}
		manifest, err := osiris.ModuleInit(root, modulePath)
		if err != nil {
			return err
		}
		if jsonOut {
			return json.NewEncoder(stdout).Encode(manifest)
		}
		_, err = fmt.Fprintf(stdout, "created %s for %s\n", osiris.ModuleManifestFile(), manifest.Module)
		return err
	case "tidy":
		manifest, err := osiris.ModuleTidy(ctx, root)
		if err != nil {
			return err
		}
		if jsonOut {
			return json.NewEncoder(stdout).Encode(manifest)
		}
		_, err = fmt.Fprintf(stdout, "updated %s (%d requirements)\n", osiris.ModuleManifestFile(), len(manifest.Requirements))
		return err
	case "download":
		if err := osiris.ModuleDownload(ctx, root); err != nil {
			return err
		}
		if jsonOut {
			return json.NewEncoder(stdout).Encode(map[string]any{"action": "download", "root": root, "verified": true})
		}
		_, err := fmt.Fprintf(stdout, "downloaded verified modules for %s\n", root)
		return err
	case "verify":
		if err := osiris.ModuleVerify(root); err != nil {
			return err
		}
		if jsonOut {
			return json.NewEncoder(stdout).Encode(map[string]any{"action": "verify", "root": root, "verified": true})
		}
		_, err := fmt.Fprintf(stdout, "verified module cache for %s\n", root)
		return err
	case "graph":
		return runModGraph(ctx, root, stdout, jsonOut)
	case "list":
		return runModList(root, stdout, jsonOut)
	case "audit":
		report, err := osiris.AuditModule(root)
		if err != nil {
			return err
		}
		if jsonOut {
			data, err := osiris.EncodeModuleAudit(report)
			if err != nil {
				return err
			}
			_, err = stdout.Write(append(data, '\n'))
			return err
		}
		for _, finding := range report.Findings {
			fmt.Fprintf(stdout, "%s %s: %s\n", strings.ToUpper(finding.Severity), finding.Code, finding.Message)
		}
		if len(report.Findings) == 0 {
			_, err := fmt.Fprintln(stdout, "module audit: clean")
			return err
		}
		return nil
	case "vendor":
		destination := ""
		if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
			destination = args[1]
		}
		if err := osiris.ModuleVendor(root, destination); err != nil {
			return err
		}
		_, err := fmt.Fprintf(stdout, "vendored verified modules into %s\n", destination)
		return err
	case "workspace":
		return runModWorkspace(root, stdout, jsonOut)
	}
	if action == "sync" {
		return runDeps(ctx, args[1:], stdout)
	}
	if action != "clean" {
		return fmt.Errorf("unknown mod action %q", action)
	}
	if len(args) != 2 || args[1] == "-" {
		return fmt.Errorf("mod clean requires a source file path")
	}
	lockPath, cacheDir := osiris.ModuleStatePaths(args[1])
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.RemoveAll(cacheDir); err != nil {
		return err
	}
	_, err := fmt.Fprintf(stdout, "removed generated module state:\n  %s\n  %s\n", lockPath, cacheDir)
	return err
}

func runGet(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("get requires module[@selector]")
	}
	jsonOut := false
	spec := ""
	for _, arg := range args {
		if arg == "-json" || arg == "--json" {
			jsonOut = true
			continue
		}
		if spec != "" {
			return fmt.Errorf("get accepts one module selector")
		}
		spec = arg
	}
	path, selector := splitGetSpec(spec)
	root := osiris.ModuleRootFor("")
	manifest, err := osiris.ModuleGet(root, path, selector)
	if err != nil {
		return err
	}
	if selector != "none" {
		if err := osiris.ModuleDownload(ctx, root); err != nil {
			return err
		}
	}
	if jsonOut {
		return json.NewEncoder(stdout).Encode(manifest)
	}
	if selector == "none" {
		_, err = fmt.Fprintf(stdout, "removed %s from %s\n", path, osiris.ModuleManifestFile())
		return err
	}
	_, err = fmt.Fprintf(stdout, "resolved %s@%s\n", path, selector)
	return err
}

func splitGetSpec(spec string) (string, string) {
	if i := strings.LastIndex(spec, "@"); i > 0 && i < len(spec)-1 {
		return spec[:i], spec[i+1:]
	}
	return spec, "latest"
}

func runModList(root string, stdout io.Writer, jsonOut bool) error {
	lockPath, _ := osiris.ModuleStatePaths(root)
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return err
	}
	if jsonOut {
		_, err = stdout.Write(append(data, '\n'))
		return err
	}
	var lock struct {
		Modules map[string]struct{ Version, Commit, Provider string } `json:"modules"`
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		return err
	}
	paths := make([]string, 0, len(lock.Modules))
	for path := range lock.Modules {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		entry := lock.Modules[path]
		fmt.Fprintf(stdout, "%s %s %s %s\n", path, entry.Version, entry.Commit, entry.Provider)
	}
	return nil
}

func runModGraph(ctx context.Context, root string, stdout io.Writer, jsonOut bool) error {
	manifest, err := osiris.LoadModuleManifest(root)
	if err != nil {
		return err
	}
	source := filepath.Join(root, manifest.Source)
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	report, err := osiris.NewRuntime().BuildDependencyReport(source, string(data))
	if err != nil {
		return err
	}
	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	for _, ref := range report.Order {
		fmt.Fprintln(stdout, ref)
	}
	_ = ctx
	return nil
}

func runModWorkspace(root string, stdout io.Writer, jsonOut bool) error {
	modules, err := osiris.WorkspaceModules(root)
	if err != nil {
		return err
	}
	if jsonOut {
		return json.NewEncoder(stdout).Encode(modules)
	}
	for _, manifest := range modules {
		fmt.Fprintln(stdout, manifest.Module)
	}
	return nil
}

type debugBreakpoint struct {
	Line      int
	Column    int
	Condition string
}

type breakpointList []debugBreakpoint

// String operates on breakpointList instances in the command-line layer and
// keeps the CLI helper logic on the receiver so the rest of the package can
// stay focused on argument parsing and output formatting.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
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

// Set operates on breakpointList instances in the command-line layer and keeps
// the CLI helper logic on the receiver so the rest of the package can stay
// focused on argument parsing and output formatting.
//
// It returns an error when the operation cannot complete cleanly.
func (b *breakpointList) Set(v string) error {
	line, col, err := parseBreakpoint(v)
	if err != nil {
		return err
	}
	*b = append(*b, debugBreakpoint{Line: line, Column: col})
	return nil
}

// runDebug handles the run debug command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
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

// parseBreakpoint handles the command-line parse breakpoint input into a
// structured representation that the rest of the CLI can use without
// re-parsing raw text.
//
// It returns int, int, error so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
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

// parseDebugBreakpoint handles the command-line parse debug breakpoint input
// into a structured representation that the rest of the CLI can use without
// re-parsing raw text.
//
// It returns debugBreakpoint and an error. Callers should only trust the value
// when the error is nil.
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

// breakpointHit handles the breakpoint hit step in the surrounding package and
// returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
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

// printDebugFrame handles the CLI-facing print debug frame view. It turns
// internal command data into stable text that can be read directly from a
// terminal or redirected to a file.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
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

// printDebugScope handles the CLI-facing print debug scope view. It turns
// internal command data into stable text that can be read directly from a
// terminal or redirected to a file.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
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

// printDebugStack handles the CLI-facing print debug stack view. It turns
// internal command data into stable text that can be read directly from a
// terminal or redirected to a file.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
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

// runTrace handles the run trace command path for the CLI. It parses the
// command-specific flags and arguments, performs the requested Osiris
// operation, and writes the user-facing result to the provided stream.
//
// It returns an error when the operation cannot complete cleanly.
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

// readSource handles the read source step in the surrounding package and
// returns string, string, error.
//
// It returns string, string, error so callers can continue with the next stage
// of the pipeline without re-deriving the same information.
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

// printUsage handles the CLI-facing print usage view. It turns internal
// command data into stable text that can be read directly from a terminal or
// redirected to a file.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func printUsage(w io.Writer) {
	fmt.Fprintln(w, "osiris: a concurrent sigil engine")
	fmt.Fprintln(w, "usage:")
	for _, cmd := range commandHelpRows() {
		fmt.Fprintf(w, "  %-44s %s\n", cmd.usage, cmd.desc)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "use `osiris help <command>` for command-specific details")
	fmt.Fprintln(w, "use `osiris --diagnostics=json <command>` for JSON failure diagnostics")
}

type commandHelpRow struct {
	usage string
	desc  string
}

// commandHelpRows handles the command help rows step in the surrounding
// package and returns []commandHelpRow.
//
// It returns []commandHelpRow so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func commandHelpRows() []commandHelpRow {
	return []commandHelpRow{
		{"osiris version", "print release version, commit, build date, Go toolchain, and target"},
		{"osiris run [-f file] [file]", "run source in the interpreter"},
		{"osiris test [flags] [path...]", "discover and run source tests, benchmarks, coverage, fuzz targets, and goldens"},
		{"osiris vm [-f file] [file]", "run source through the bytecode VM"},
		{"osiris eval <expression>", "evaluate a single expression and emit the result"},
		{"osiris demo", "run the built-in demo program"},
		{"osiris fmt [-w] [file]", "format Osiris source code"},
		{"osiris opt [-w] [-report|-json] [file]", "rewrite source or emit a deterministic optimization report"},
		{"osiris ir [file]", "dump the lowered intermediate representation"},
		{"osiris ssa [file]", "dump SSA form with explicit defs and uses"},
		{"osiris cfg [file]", "dump the control-flow graph"},
		{"osiris dom [file]", "dump the dominator tree"},
		{"osiris loop [file]", "show canonical loop shapes from the IR"},
		{"osiris c [-o out.c] [file]", "lower the supported program subset to readable C"},
		{"osiris obj -o out.o [file]", "compile the C backend to an object file"},
		{"osiris build -o out [file]", "compile the C backend to a native executable (standard path)"},
		{"osiris compile -o out [file]", "alias for osiris build"},
		{"osiris pure [file]", "analyze purity and side effects"},
		{"osiris closure [file]", "show closure conversion and captured environments"},
		{"osiris infer [file]", "show inferred kinds, shapes, and units"},
		{"osiris alias [file]", "show alias classes and escape information"},
		{"osiris viz [-imports] [file]", "render the AST or import graph as DOT"},
		{"osiris wasm [-binary] [-o out.wat|out.wasm] [file]", "compile to WebAssembly text or binary"},
		{"osiris gpu [-json] [-o out.cl|out.json] [file]", "emit GPU kernel source or a kernel plan"},
		{"osiris lsp", "run the language server over stdio"},
		{"osiris map [file]", "emit source maps for debugging and stepping"},
		{"osiris deps [-json] [file]", "report module dependencies and cycles"},
		{"osiris get module[@selector]", "add, update, or remove a repository module requirement"},
		{"osiris mod init|tidy|download|verify|sync|clean|graph|list|audit|vendor|workspace", "manage manifests, workspaces, lockfiles, caches, and supply-chain state"},
		{"osiris heap [-json] [-query expr] [file]", "inspect live scopes, modules, and dependency state"},
		{"osiris trace [-json] [file]", "print execution trace frames"},
		{"osiris debug [-json] [-break line[:col]] [file]", "step through execution with breakpoints"},
		{"osiris repl", "start an interactive persistent REPL"},
	}
}

// printCommandHelp handles the CLI-facing print command help view. It turns
// internal command data into stable text that can be read directly from a
// terminal or redirected to a file.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func printCommandHelp(w io.Writer, name string) bool {
	for _, cmd := range commandHelpRows() {
		rest, ok := strings.CutPrefix(cmd.usage, "osiris ")
		if !ok {
			continue
		}
		parts := strings.Fields(rest)
		if len(parts) > 0 && parts[0] == name {
			fmt.Fprintf(w, "%s\n", cmd.usage)
			fmt.Fprintf(w, "%s\n", cmd.desc)
			return true
		}
	}
	return false
}
