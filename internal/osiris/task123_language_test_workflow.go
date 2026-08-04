package osiris

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type languageTestCase struct {
	name            string
	file            string
	location        LanguageTestLocation
	body            string
	setup           string
	cleanup         string
	assertions      []languageTestAssertion
	expected        []string
	skipReason      string
	expectedFailure bool
	golden          *languageGoldenSpec
	benchmark       *languageBenchmarkSpec
	fuzz            *languageFuzzSpec
	parameters      []languageParameterCase
	parseErr        error
}

type languageTestAssertion struct {
	expression string
	line       int
}

type languageParameterCase struct {
	name   string
	values map[string]string
}

type languageGoldenSpec struct {
	path string
	line int
}

type languageBenchmarkSpec struct {
	warmup  int
	samples int
}

type languageFuzzSpec struct {
	seed    int64
	runs    int
	corpus  string
	maxSize int
}

type languageDirective struct {
	name string
	text string
	line int
}

var outputAssertionPattern = regexp.MustCompile(`^output\[([0-9]+)\]\s*==\s*(.+)$`)
var outputCountPattern = regexp.MustCompile(`^output_count\s*==\s*([0-9]+)$`)
var outputContainsPattern = regexp.MustCompile(`^output\s+contains\s+(.+)$`)

func parseLanguageTestFile(file, source string) ([]languageTestCase, error) {
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	base = strings.TrimSuffix(base, "_test")
	base = strings.TrimSuffix(base, ".test")
	current := &languageTestCase{name: base, file: file, location: languageTestLocation(file, 1, 1, 1, 1)}
	section := "body"
	seenHeader := false
	var cases []languageTestCase
	lines := strings.SplitAfter(source, "\n")
	for lineIndex, line := range lines {
		lineNumber := lineIndex + 1
		if directive, ok := parseLanguageDirective(line, lineNumber); ok {
			if directive.name == "test" {
				if seenHeader || current.body != "" || current.setup != "" || current.cleanup != "" || len(current.assertions) > 0 || len(current.expected) > 0 {
					cases = append(cases, expandLanguageTestCase(*current)...)
				}
				seenHeader = true
				current = &languageTestCase{name: base, file: file, location: languageTestLocation(file, lineNumber, 1, lineNumber, maxDirectiveColumn(line))}
				section = "body"
				if err := applyTestHeader(current, directive.text); err != nil {
					return nil, fmt.Errorf("%s:%d: %w", file, lineNumber, err)
				}
				continue
			}
			if err := applyLanguageDirective(current, directive); err != nil {
				return nil, fmt.Errorf("%s:%d: %w", file, lineNumber, err)
			}
			if directive.name == "fixture" {
				words := splitDirectiveWords(directive.text)
				if len(words) > 0 {
					section = words[0]
				}
			}
			continue
		}
		switch section {
		case "setup":
			current.setup += line
		case "cleanup":
			current.cleanup += line
		default:
			current.body += line
		}
	}
	if seenHeader || current.body != "" || current.setup != "" || current.cleanup != "" || len(current.assertions) > 0 || len(current.expected) > 0 || len(cases) == 0 {
		cases = append(cases, expandLanguageTestCase(*current)...)
	}
	return cases, nil
}

func maxDirectiveColumn(line string) int {
	if len(line) == 0 {
		return 1
	}
	return len([]rune(strings.TrimRight(line, "\r\n"))) + 1
}

func parseLanguageDirective(line string, lineNumber int) (languageDirective, bool) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"))
	if !strings.HasPrefix(trimmed, "// osiris:") {
		return languageDirective{}, false
	}
	text := strings.TrimSpace(strings.TrimPrefix(trimmed, "// osiris:"))
	if text == "" {
		return languageDirective{}, false
	}
	words := splitDirectiveWords(text)
	if len(words) == 0 {
		return languageDirective{}, false
	}
	name := words[0]
	rest := strings.TrimSpace(strings.TrimPrefix(text, name))
	return languageDirective{name: name, text: rest, line: lineNumber}, true
}

func splitDirectiveWords(text string) []string {
	var words []string
	var current strings.Builder
	quote := rune(0)
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			words = append(words, current.String())
			current.Reset()
		}
	}
	for _, r := range text {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if quote != 0 {
			if r == '\\' {
				escaped = true
			} else if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case ' ', '\t':
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return words
}

func directiveAttributes(text string) (map[string]string, []string, error) {
	attrs := make(map[string]string)
	var positional []string
	for _, word := range splitDirectiveWords(text) {
		key, value, ok := strings.Cut(word, "=")
		if !ok {
			positional = append(positional, word)
			continue
		}
		if key == "" {
			return nil, nil, fmt.Errorf("empty directive attribute")
		}
		attrs[key] = value
	}
	return attrs, positional, nil
}

func applyTestHeader(item *languageTestCase, text string) error {
	attrs, positional, err := directiveAttributes(text)
	if err != nil {
		return err
	}
	if name := attrs["name"]; name != "" {
		item.name = name
	} else if len(positional) > 0 {
		item.name = positional[0]
	}
	if item.name == "" {
		return fmt.Errorf("test declaration requires name=... or a positional name")
	}
	if value := attrs["skip"]; value != "" {
		item.skipReason = value
	}
	if value := attrs["xfail"]; value != "" {
		item.expectedFailure = true
	}
	return nil
}

func applyLanguageDirective(item *languageTestCase, directive languageDirective) error {
	if directive.name == "assert" {
		expression := strings.TrimSpace(directive.text)
		if expression == "" {
			return fmt.Errorf("assert requires an assertion expression")
		}
		item.assertions = append(item.assertions, languageTestAssertion{expression: expression, line: directive.line})
		return nil
	}
	attrs, positional, err := directiveAttributes(directive.text)
	if err != nil {
		return err
	}
	switch directive.name {
	case "expect":
		if values := attrs["outputs"]; values != "" {
			var decoded []string
			if err := json.Unmarshal([]byte(values), &decoded); err != nil {
				return fmt.Errorf("expect outputs must be a JSON string array: %w", err)
			}
			item.expected = append(item.expected, decoded...)
		} else if len(positional) > 0 {
			item.expected = append(item.expected, positional[0])
		} else {
			return fmt.Errorf("expect requires a quoted value or outputs=[...]")
		}
	case "skip":
		item.skipReason = firstDirectiveValue(attrs, positional, "skipped")
	case "xfail":
		item.expectedFailure = true
	case "golden":
		path := attrs["path"]
		if path == "" && len(positional) > 0 {
			path = positional[0]
		}
		if path == "" {
			return fmt.Errorf("golden requires path=...")
		}
		item.golden = &languageGoldenSpec{path: path, line: directive.line}
	case "benchmark":
		item.benchmark = &languageBenchmarkSpec{warmup: directiveInt(attrs, "warmup", 2), samples: directiveInt(attrs, "samples", 15)}
		if item.benchmark.warmup < 0 || item.benchmark.samples < 1 {
			return fmt.Errorf("benchmark warmup must be non-negative and samples must be positive")
		}
	case "fuzz":
		item.fuzz = &languageFuzzSpec{seed: directiveInt64(attrs, "seed", 1), runs: directiveInt(attrs, "runs", 32), corpus: attrs["corpus"], maxSize: directiveInt(attrs, "max_size", 32)}
		if item.fuzz.runs < 1 || item.fuzz.maxSize < 1 {
			return fmt.Errorf("fuzz runs and max_size must be positive")
		}
	case "case":
		name := attrs["name"]
		if name == "" && len(positional) > 0 {
			name = positional[0]
		}
		if name == "" {
			return fmt.Errorf("case requires name=...")
		}
		values := make(map[string]string)
		for key, value := range attrs {
			if key != "name" {
				values[key] = value
			}
		}
		item.parameters = append(item.parameters, languageParameterCase{name: name, values: values})
	case "fixture":
		if len(positional) != 1 || (positional[0] != "setup" && positional[0] != "body" && positional[0] != "cleanup") {
			return fmt.Errorf("fixture syntax is fixture setup, fixture body, or fixture cleanup")
		}
	default:
		return fmt.Errorf("unknown osiris directive %q", directive.name)
	}
	return nil
}

func firstDirectiveValue(attrs map[string]string, positional []string, fallback string) string {
	for _, key := range []string{"reason", "name", "skip", "xfail"} {
		if attrs[key] != "" {
			return attrs[key]
		}
	}
	if len(positional) > 0 {
		return positional[0]
	}
	return fallback
}

func directiveInt(attrs map[string]string, key string, fallback int) int {
	value, err := strconv.Atoi(attrs[key])
	if attrs[key] == "" || err != nil {
		return fallback
	}
	return value
}

func directiveInt64(attrs map[string]string, key string, fallback int64) int64 {
	value, err := strconv.ParseInt(attrs[key], 10, 64)
	if attrs[key] == "" || err != nil {
		return fallback
	}
	return value
}

func expandLanguageTestCase(item languageTestCase) []languageTestCase {
	if len(item.parameters) == 0 {
		return []languageTestCase{item}
	}
	items := make([]languageTestCase, 0, len(item.parameters))
	for _, parameter := range item.parameters {
		copy := item
		copy.name = item.name + "[" + parameter.name + "]"
		copy.body = renderLanguageTemplate(item.body, parameter.values)
		copy.setup = renderLanguageTemplate(item.setup, parameter.values)
		copy.cleanup = renderLanguageTemplate(item.cleanup, parameter.values)
		copy.expected = make([]string, len(item.expected))
		for index, expected := range item.expected {
			copy.expected[index] = renderLanguageTemplate(expected, parameter.values)
		}
		copy.assertions = make([]languageTestAssertion, len(item.assertions))
		for index, assertion := range item.assertions {
			copy.assertions[index] = assertion
			copy.assertions[index].expression = renderLanguageTemplate(assertion.expression, parameter.values)
		}
		copy.parameters = nil
		items = append(items, copy)
	}
	return items
}

func renderLanguageTemplate(source string, values map[string]string) string {
	for key, value := range values {
		source = strings.ReplaceAll(source, "{{"+key+"}}", value)
	}
	return source
}

func runLanguageTestCase(ctx context.Context, item *languageTestCase, options LanguageTestOptions, result *LanguageTestResult) {
	if options.Race && normalizedTestBackend(options.Backend) == "native" {
		setLanguageTestFailure(result, fmt.Errorf("-race is supported only by interpreter and vm backends"), item.location)
		return
	}
	caseCtx := ctx
	if options.Timeout > 0 {
		var cancel context.CancelFunc
		caseCtx, cancel = context.WithTimeout(ctx, options.Timeout)
		defer cancel()
	}
	backend := normalizedTestBackend(options.Backend)
	rt := NewTestRuntime()
	if options.Integration {
		rt = NewRuntime()
		for _, capability := range options.Capabilities {
			rt.GrantCapability(capability)
		}
	}
	rt.languageTest = newLanguageTestState(item.name)
	runFragment := func(source string, collectCoverage bool) languageExecution {
		return executeLanguageFragment(caseCtx, item.file, source, backend, rt, collectCoverage)
	}
	if item.setup != "" {
		if execution := runFragment(item.setup, false); execution.err != nil {
			setLanguageTestFailure(result, execution.err, item.location)
			return
		}
	}
	var cleanupExecution languageExecution
	if item.cleanup != "" {
		defer func() {
			cleanupExecution = runFragment(item.cleanup, false)
			if cleanupExecution.err != nil && result.Passed {
				setLanguageTestFailure(result, fmt.Errorf("fixture cleanup: %w", cleanupExecution.err), item.location)
			}
		}()
	}
	if item.fuzz != nil && options.Fuzz {
		if err := runLanguageFuzz(caseCtx, item, options, result); err != nil {
			setLanguageTestFailure(result, err, item.location)
			return
		}
		result.Passed = true
		result.Status = "ok"
		return
	}
	execution := runFragment(item.body, options.CoveragePath != "")
	result.Outputs = execution.outputs
	result.OutputStrings = execution.outputStrings
	failed, skipped, control, controlMessage, logs, testAssertions := rt.languageTest.snapshot()
	result.Logs = logs
	for i := range testAssertions {
		if testAssertions[i].Location.Source == "" {
			testAssertions[i].Location = item.location
		}
	}
	result.Assertions = append(result.Assertions, testAssertions...)
	if control == "skip" || skipped {
		result.Passed = true
		result.Skipped = true
		result.Status = "skip"
		result.ErrorText = controlMessage
		return
	}
	if control == "fatal" {
		if controlMessage == "" {
			controlMessage = "test stopped by fatal"
		}
		setLanguageTestFailure(result, fmt.Errorf("%s", controlMessage), item.location)
		return
	}
	if execution.coverage != nil {
		result.Coverage = execution.coverage
		if options.CoveragePath != "" {
			path, err := writeLanguageCoverage(options.CoveragePath, execution.coverage)
			if err != nil {
				setLanguageTestFailure(result, err, item.location)
				return
			}
			result.Artifacts = append(result.Artifacts, LanguageTestArtifact{Kind: "coverage", Path: path})
		}
	}
	if execution.err != nil {
		setLanguageTestFailure(result, execution.err, locationFromExecution(execution, item.location))
		if result.ExpectedFailure {
			result.Passed = true
			result.Status = "expected_failure"
		}
		return
	}
	if item.expectedFailure {
		result.UnexpectedPass = true
		result.Status = "unexpected_pass"
		result.Passed = false
		result.ErrorText = "expected failure passed"
		return
	}
	if failed {
		setLanguageTestFailure(result, fmt.Errorf("test %s reported a failure", item.name), item.location)
		return
	}
	if err := checkLanguageAssertions(item, result.OutputStrings, result); err != nil {
		setLanguageTestFailure(result, err, result.Location)
		return
	}
	if item.golden != nil {
		if err := checkLanguageGolden(item, result, options); err != nil {
			setLanguageTestFailure(result, err, languageTestLocation(item.file, item.golden.line, 1, item.golden.line, 1))
			return
		}
	}
	if item.benchmark != nil && options.Benchmark {
		report, err := runLanguageBenchmark(caseCtx, item, options)
		if err != nil {
			setLanguageTestFailure(result, err, item.location)
			return
		}
		result.Benchmark = report
		if options.BenchmarkJSON != "" {
			path, err := writeLanguageBenchmark(options.BenchmarkJSON, report)
			if err != nil {
				setLanguageTestFailure(result, err, item.location)
				return
			}
			result.Artifacts = append(result.Artifacts, LanguageTestArtifact{Kind: "benchmark", Path: path})
		}
	}
	result.Passed = true
	result.Status = "ok"
}

type languageExecution struct {
	outputs       []Value
	outputStrings []string
	coverage      *LanguageCoverageReport
	location      LanguageTestLocation
	err           error
}

func executeLanguageFragment(ctx context.Context, sourceName, source, backend string, rt *Runtime, coverage bool) languageExecution {
	if ctx == nil {
		ctx = context.Background()
	}
	prog, err := Parse(sourceName, source)
	if err != nil {
		return languageExecution{err: err, location: languageLocationFromError(err, sourceName)}
	}
	if err := ctx.Err(); err != nil {
		return languageExecution{err: err, location: languageTestLocation(sourceName, 1, 1, 1, 1)}
	}
	switch backend {
	case "interpreter":
		if coverage {
			frames, traceErr := rt.Trace(ctx, prog)
			outputs := flattenTraceOutputs(frames)
			return languageExecution{outputStrings: outputs, coverage: buildLanguageCoverage(prog, backend, frames), err: traceErr, location: languageLocationFromError(traceErr, sourceName)}
		}
		outputs, runErr := rt.Run(ctx, prog)
		return languageExecution{outputs: outputs, outputStrings: valuesToStrings(outputs), err: runErr, location: languageLocationFromError(runErr, sourceName)}
	case "vm":
		outputs, runErr := rt.RunVM(ctx, prog)
		return languageExecution{outputs: outputs, outputStrings: valuesToStrings(outputs), coverage: buildLanguageCoverage(prog, backend, nil), err: runErr, location: languageLocationFromError(runErr, sourceName)}
	case "native":
		return executeLanguageNative(ctx, prog, rt)
	default:
		return languageExecution{err: fmt.Errorf("test backend %q is unsupported; choose interpreter, vm, or native", backend), location: languageTestLocation(sourceName, 1, 1, 1, 1)}
	}
}

func executeLanguageNative(ctx context.Context, prog *Program, rt *Runtime) languageExecution {
	artifact, err := CompileC(ctx, rt, prog)
	if err != nil {
		return languageExecution{err: err, location: languageLocationFromError(err, prog.SourceName)}
	}
	dir, err := os.MkdirTemp("", "osiris-task123-native-")
	if err != nil {
		return languageExecution{err: err, location: languageTestLocation(prog.SourceName, 1, 1, 1, 1)}
	}
	defer os.RemoveAll(dir)
	executable := filepath.Join(dir, "test")
	if err := CompileCExecutable(ctx, artifact.Source, executable); err != nil {
		return languageExecution{err: err, location: languageLocationFromError(err, prog.SourceName)}
	}
	command := exec.CommandContext(ctx, executable)
	output, err := command.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return languageExecution{outputStrings: splitNativeOutput(string(output)), err: fmt.Errorf("native test execution: %w: %s", err, strings.TrimSpace(string(output))), location: languageLocationFromError(err, prog.SourceName)}
	}
	return languageExecution{outputStrings: splitNativeOutput(string(output)), coverage: buildLanguageCoverage(prog, "native", nil)}
}

func flattenTraceOutputs(frames []TraceFrame) []string {
	var outputs []string
	for _, frame := range frames {
		outputs = append(outputs, frame.Outputs...)
	}
	return outputs
}

func splitNativeOutput(output string) []string {
	output = strings.TrimSuffix(output, "\n")
	if output == "" {
		return nil
	}
	return strings.Split(output, "\n")
}

func locationFromExecution(execution languageExecution, fallback LanguageTestLocation) LanguageTestLocation {
	if execution.location.Source != "" || execution.location.Start.Line != 0 {
		return execution.location
	}
	return fallback
}

func setLanguageTestFailure(result *LanguageTestResult, err error, location LanguageTestLocation) {
	result.Passed = false
	result.Status = "fail"
	result.Error = err
	if err != nil {
		result.ErrorText = err.Error()
	}
	if location.Source != "" {
		result.Location = location
	}
}

func checkLanguageAssertions(item *languageTestCase, outputs []string, result *LanguageTestResult) error {
	if len(item.expected) > 0 {
		assertion := LanguageTestAssertionResult{Expression: "expect outputs", Passed: equalStringSlices(outputs, item.expected), Location: languageTestLocation(item.file, item.location.Start.Line, 1, item.location.Start.Line, 1)}
		if !assertion.Passed {
			assertion.Message = fmt.Sprintf("outputs = %s, want %s", formatStringSlice(outputs), formatStringSlice(item.expected))
		}
		result.Assertions = append(result.Assertions, assertion)
		if !assertion.Passed {
			return fmt.Errorf("output assertion failed: %s", assertion.Message)
		}
	}
	for _, itemAssertion := range item.assertions {
		passed, message := evaluateLanguageAssertion(itemAssertion.expression, outputs)
		assertion := LanguageTestAssertionResult{Expression: itemAssertion.expression, Passed: passed, Location: languageTestLocation(item.file, itemAssertion.line, 1, itemAssertion.line, 1), Message: message}
		result.Assertions = append(result.Assertions, assertion)
		if !passed {
			return fmt.Errorf("assertion at %s failed: %s", formatLanguageLocation(assertion.Location), message)
		}
	}
	return nil
}

func evaluateLanguageAssertion(expression string, outputs []string) (bool, string) {
	if match := outputAssertionPattern.FindStringSubmatch(expression); match != nil {
		index, _ := strconv.Atoi(match[1])
		want := decodeAssertionValue(match[2])
		if index >= len(outputs) {
			return false, fmt.Sprintf("output[%d] is missing", index)
		}
		if outputs[index] != want {
			return false, fmt.Sprintf("output[%d] = %q, want %q", index, outputs[index], want)
		}
		return true, ""
	}
	if match := outputCountPattern.FindStringSubmatch(expression); match != nil {
		want, _ := strconv.Atoi(match[1])
		if len(outputs) != want {
			return false, fmt.Sprintf("output_count = %d, want %d", len(outputs), want)
		}
		return true, ""
	}
	if match := outputContainsPattern.FindStringSubmatch(expression); match != nil {
		want := decodeAssertionValue(match[1])
		for _, output := range outputs {
			if output == want {
				return true, ""
			}
		}
		return false, fmt.Sprintf("outputs do not contain %q", want)
	}
	return false, "supported assertions are output[index] == value, output_count == n, and output contains value"
}

func decodeAssertionValue(value string) string {
	value = strings.TrimSpace(value)
	if decoded, err := strconv.Unquote(value); err == nil {
		return decoded
	}
	return value
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func formatStringSlice(values []string) string {
	data, _ := json.Marshal(values)
	return string(data)
}

func formatLanguageLocation(location LanguageTestLocation) string {
	return fmt.Sprintf("%s:%d:%d", location.Source, location.Start.Line, location.Start.Column)
}

func checkLanguageGolden(item *languageTestCase, result *LanguageTestResult, options LanguageTestOptions) error {
	path := item.golden.path
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(item.file), path)
	}
	document := struct {
		SchemaVersion int      `json:"schema_version"`
		Name          string   `json:"name"`
		Backend       string   `json:"backend"`
		Outputs       []string `json:"outputs"`
	}{1, item.name, result.Backend, result.OutputStrings}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	actual, readErr := os.ReadFile(path)
	if options.UpdateGolden {
		if err := os.WriteFile(path, encoded, 0o644); err != nil {
			return err
		}
		result.Artifacts = append(result.Artifacts, LanguageTestArtifact{Kind: "golden", Path: path})
		return nil
	}
	if readErr != nil {
		return fmt.Errorf("golden %q is missing; rerun with -update", path)
	}
	var want struct {
		SchemaVersion int      `json:"schema_version"`
		Name          string   `json:"name"`
		Outputs       []string `json:"outputs"`
	}
	if err := json.Unmarshal(actual, &want); err != nil {
		return fmt.Errorf("golden %q is invalid JSON: %w", path, err)
	}
	var got struct {
		SchemaVersion int      `json:"schema_version"`
		Name          string   `json:"name"`
		Outputs       []string `json:"outputs"`
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		return err
	}
	if want.SchemaVersion != got.SchemaVersion || want.Name != got.Name || !equalStringSlices(want.Outputs, got.Outputs) {
		return fmt.Errorf("golden %q differs; rerun with -update", path)
	}
	result.Artifacts = append(result.Artifacts, LanguageTestArtifact{Kind: "golden", Path: path})
	return nil
}

func buildLanguageCoverage(prog *Program, backend string, frames []TraceFrame) *LanguageCoverageReport {
	mapData := BuildSourceMap(prog)
	coveredLines := make(map[int]struct{})
	for _, frame := range frames {
		coveredLines[frame.Range.Start.Line] = struct{}{}
	}
	statements := CoverageCounts{Total: len(mapData.Entries)}
	functions := CoverageCounts{}
	for _, entry := range mapData.Entries {
		if _, ok := coveredLines[entry.Range.Start.Line]; ok {
			statements.Covered++
		}
		if entry.Kind == "func" {
			functions.Total++
			if _, ok := coveredLines[entry.Range.Start.Line]; ok {
				functions.Covered++
			}
		}
	}
	statements.Missed = statements.Total - statements.Covered
	functions.Missed = functions.Total - functions.Covered
	branches := CoverageCounts{}
	for _, line := range strings.Split(prog.SourceName+"\n", "\n") {
		_ = line
	}
	// BuildSourceMap deliberately preserves statement spans but expression
	// branches are not executable entries. Count branch declarations from the
	// source map's stable statement text and report them as static until the
	// selected backend exposes branch probes.
	for _, entry := range mapData.Entries {
		if strings.Contains(entry.Statement, "while ") || strings.Contains(entry.Statement, "for ") || strings.Contains(entry.Statement, "match ") {
			branches.Total++
			if _, ok := coveredLines[entry.Range.Start.Line]; ok {
				branches.Covered++
			}
		}
	}
	branches.Missed = branches.Total - branches.Covered
	available := backend == "interpreter" && frames != nil
	report := &LanguageCoverageReport{SchemaVersion: 1, Backend: backend, Available: available, Statements: statements, Branches: branches, Functions: functions, SourceMap: mapData.Entries}
	if !available {
		report.Reason = "selected backend does not publish execution trace probes"
	}
	return report
}

func writeLanguageCoverage(path string, report *LanguageCoverageReport) (string, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func runLanguageBenchmark(ctx context.Context, item *languageTestCase, options LanguageTestOptions) (*LanguageBenchmarkReport, error) {
	spec := item.benchmark
	if spec == nil {
		return nil, nil
	}
	backend := normalizedTestBackend(options.Backend)
	run := func() (time.Duration, error) {
		started := time.Now()
		execution := executeLanguageFragment(ctx, item.file, item.body, backend, NewTestRuntime(), false)
		if execution.err != nil {
			return 0, execution.err
		}
		return time.Since(started), nil
	}
	for i := 0; i < spec.warmup; i++ {
		if _, err := run(); err != nil {
			return nil, err
		}
	}
	samples := make([]float64, 0, spec.samples)
	var total float64
	for i := 0; i < spec.samples; i++ {
		duration, err := run()
		if err != nil {
			return nil, err
		}
		ms := float64(duration.Nanoseconds()) / 1e6
		ms = float64(int64(ms*1000+0.5)) / 1000
		samples = append(samples, ms)
		total += ms
	}
	return &LanguageBenchmarkReport{SchemaVersion: 1, Workload: item.name, Backend: backend, IterationsPerRun: 1, WarmupRuns: spec.warmup, MeasuredRuns: spec.samples, Environment: LanguageBenchmarkEnvironment{OS: runtime.GOOS, Arch: runtime.GOARCH, Go: runtime.Version()}, CostsMS: map[string]LanguageBenchmarkCost{backend: {Samples: samples, Average: float64(int64((total/float64(len(samples)))*1000+0.5)) / 1000}}, Caveats: []string{"samples include source parse, backend startup, and process startup for native execution"}}, nil
}

func writeLanguageBenchmark(path string, report *LanguageBenchmarkReport) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return "", err
	}
	data, err := json.MarshalIndent(report, "", "")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func runLanguageFuzz(ctx context.Context, item *languageTestCase, options LanguageTestOptions, result *LanguageTestResult) error {
	spec := item.fuzz
	seed := spec.seed
	if seed == 0 {
		seed = options.FuzzSeed
	}
	runs := spec.runs
	if options.FuzzRuns > 0 {
		runs = options.FuzzRuns
	}
	corpus := spec.corpus
	if corpus == "" {
		corpus = options.FuzzCorpus
	}
	inputs, err := languageFuzzCorpus(corpus, item.file)
	if err != nil {
		return err
	}
	rng := rand.New(rand.NewSource(seed))
	for len(inputs) < runs {
		length := 1 + rng.Intn(spec.maxSize)
		var input strings.Builder
		for i := 0; i < length; i++ {
			input.WriteByte("abcdefghijklmnopqrstuvwxyz0123456789"[rng.Intn(36)])
		}
		inputs = append(inputs, input.String())
	}
	if len(inputs) > runs {
		inputs = inputs[:runs]
	}
	fails := func(input string) (languageExecution, string) {
		source := strings.ReplaceAll(item.body, "{{input}}", strconv.Quote(input))
		execution := executeLanguageFragment(ctx, item.file, source, normalizedTestBackend(options.Backend), NewTestRuntime(), false)
		return execution, source
	}
	for index, input := range inputs {
		execution, _ := fails(input)
		if execution.err == nil {
			continue
		}
		shrunk := input
		attempts := 0
		for len(shrunk) > 0 && attempts < 64 {
			changed := false
			for cut := 0; cut < len(shrunk); cut++ {
				candidate := shrunk[:cut] + shrunk[cut+1:]
				candidateExecution, _ := fails(candidate)
				attempts++
				if candidateExecution.err != nil {
					shrunk = candidate
					changed = true
					break
				}
				if attempts >= 64 {
					break
				}
			}
			if !changed {
				break
			}
		}
		_, replaySource := fails(shrunk)
		artifactDir := filepath.Join(options.ArtifactsDir, "fuzz")
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			return err
		}
		digest := sha256.Sum256([]byte(item.name + "\x00" + strconv.FormatInt(seed, 10) + "\x00" + shrunk))
		stem := filepath.Join(artifactDir, hex.EncodeToString(digest[:]))
		artifact := struct {
			SchemaVersion int    `json:"schema_version"`
			Name          string `json:"name"`
			Seed          int64  `json:"seed"`
			Run           int    `json:"run"`
			Input         string `json:"input"`
			Shrunk        string `json:"shrunk"`
			Error         string `json:"error"`
			Replay        string `json:"replay"`
		}{1, item.name, seed, index, input, shrunk, execution.err.Error(), stem + ".osr"}
		data, _ := json.MarshalIndent(artifact, "", "  ")
		data = append(data, '\n')
		if err := os.WriteFile(stem+".json", data, 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(stem+".osr", []byte(replaySource), 0o644); err != nil {
			return err
		}
		result.Artifacts = append(result.Artifacts, LanguageTestArtifact{Kind: "fuzz-failure", Path: stem + ".json"}, LanguageTestArtifact{Kind: "fuzz-replay", Path: stem + ".osr"})
		return fmt.Errorf("fuzz input failed after %d shrink attempts: %s", attempts, execution.err)
	}
	return nil
}

func languageFuzzCorpus(path, sourceFile string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(sourceFile), path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		data, err := os.ReadFile(path)
		return []string{string(data)}, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	inputs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(path, entry.Name()))
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, string(data))
	}
	return inputs, nil
}

func languageLocationFromError(err error, source string) LanguageTestLocation {
	if err == nil {
		return LanguageTestLocation{}
	}
	text := err.Error()
	parts := strings.Split(text, ":")
	for i := len(parts) - 3; i >= 0; i-- {
		line, lineErr := strconv.Atoi(parts[i+1])
		col, colErr := strconv.Atoi(strings.TrimSpace(parts[i+2]))
		if lineErr == nil && colErr == nil && line > 0 && col > 0 {
			return languageTestLocation(source, line, col, line, col)
		}
	}
	return languageTestLocation(source, 1, 1, 1, 1)
}
