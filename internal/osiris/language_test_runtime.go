package osiris

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LanguageTestOptions controls the source-test workflow. A zero value selects
// the interpreter, one worker, deterministic output, and no optional reports.
// Test execution never reuses a completed result from disk; CacheDir only
// stores an inventory manifest and is therefore safe for source-changing runs.
type LanguageTestOptions struct {
	Backend       string
	Timeout       time.Duration
	Integration   bool
	Capabilities  []string
	Filter        string
	Parallel      int
	Deterministic bool
	Race          bool
	CoveragePath  string
	ArtifactsDir  string
	Benchmark     bool
	BenchmarkJSON string
	Fuzz          bool
	FuzzRuns      int
	FuzzSeed      int64
	FuzzCorpus    string
	UpdateGolden  bool
	CacheDir      string
	NoCache       bool
}

// LanguageTestLocation is the source range attached to a declaration,
// assertion, failure, or artifact. Lines and columns are one-based.
type LanguageTestLocation struct {
	Source string         `json:"source"`
	Start  SourcePosition `json:"start"`
	End    SourcePosition `json:"end"`
}

// LanguageTestAssertionResult records one source-level output assertion.
type LanguageTestAssertionResult struct {
	Expression string               `json:"expression"`
	Passed     bool                 `json:"passed"`
	Location   LanguageTestLocation `json:"location"`
	Message    string               `json:"message,omitempty"`
}

// LanguageCoverageReport is a stable source-map report. Interpreter coverage
// uses the public Trace API; VM and native runs expose the static map but mark
// execution coverage unavailable until those backends publish probes.
type LanguageCoverageReport struct {
	SchemaVersion int              `json:"schema_version"`
	Backend       string           `json:"backend"`
	Available     bool             `json:"available"`
	Reason        string           `json:"reason,omitempty"`
	Statements    CoverageCounts   `json:"statements"`
	Branches      CoverageCounts   `json:"branches"`
	Functions     CoverageCounts   `json:"functions"`
	SourceMap     []SourceMapEntry `json:"source_map"`
}

// CoverageCounts is the total/covered/missed triplet used by each coverage
// dimension.
type CoverageCounts struct {
	Total   int `json:"total"`
	Covered int `json:"covered"`
	Missed  int `json:"missed"`
}

// LanguageBenchmarkReport follows the repository's schema-versioned benchmark
// shape: raw samples, warmups, measured runs, environment, and caveats are
// retained rather than reducing a run to one unstable average.
type LanguageBenchmarkReport struct {
	SchemaVersion    int                              `json:"schema_version"`
	Workload         string                           `json:"workload"`
	Backend          string                           `json:"backend"`
	IterationsPerRun int                              `json:"iterations_per_run"`
	WarmupRuns       int                              `json:"warmup_runs"`
	MeasuredRuns     int                              `json:"measured_runs"`
	Environment      LanguageBenchmarkEnvironment     `json:"environment"`
	CostsMS          map[string]LanguageBenchmarkCost `json:"costs_ms"`
	Caveats          []string                         `json:"caveats"`
}

// LanguageBenchmarkEnvironment identifies the host that produced samples.
type LanguageBenchmarkEnvironment struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
	Go   string `json:"go"`
}

// LanguageBenchmarkCost stores raw milliseconds and a rounded average.
type LanguageBenchmarkCost struct {
	Samples []float64 `json:"samples"`
	Average float64   `json:"average"`
}

// LanguageTestArtifact identifies a reproducible file written by fuzz,
// golden, coverage, or benchmark processing.
type LanguageTestArtifact struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

// LanguageTestResult is the result of one expanded source declaration.
type LanguageTestResult struct {
	Name            string                        `json:"name"`
	File            string                        `json:"file"`
	Backend         string                        `json:"backend"`
	Status          string                        `json:"status"`
	Passed          bool                          `json:"passed"`
	Skipped         bool                          `json:"skipped,omitempty"`
	ExpectedFailure bool                          `json:"expected_failure,omitempty"`
	UnexpectedPass  bool                          `json:"unexpected_pass,omitempty"`
	Outputs         []Value                       `json:"-"`
	OutputStrings   []string                      `json:"outputs,omitempty"`
	Assertions      []LanguageTestAssertionResult `json:"assertions,omitempty"`
	Logs            []string                      `json:"logs,omitempty"`
	Location        LanguageTestLocation          `json:"location"`
	Error           error                         `json:"-"`
	ErrorText       string                        `json:"error,omitempty"`
	Duration        time.Duration                 `json:"-"`
	DurationNS      int64                         `json:"duration_ns"`
	Coverage        *LanguageCoverageReport       `json:"coverage,omitempty"`
	Benchmark       *LanguageBenchmarkReport      `json:"benchmark,omitempty"`
	Artifacts       []LanguageTestArtifact        `json:"artifacts,omitempty"`
}

// LanguageTestInventory is the stable internal discovery surface intended for
// future LSP/code-lens integration.
type LanguageTestInventory struct {
	SchemaVersion int                 `json:"schema_version"`
	Files         []string            `json:"files"`
	Tests         []LanguageTestEntry `json:"tests"`
	Package       string              `json:"package,omitempty"`
	Module        string              `json:"module,omitempty"`
}

// LanguageTestEntry describes one source declaration without executing it.
type LanguageTestEntry struct {
	Name            string               `json:"name"`
	File            string               `json:"file"`
	Location        LanguageTestLocation `json:"location"`
	Skip            bool                 `json:"skip,omitempty"`
	ExpectedFailure bool                 `json:"expected_failure,omitempty"`
	Benchmark       bool                 `json:"benchmark,omitempty"`
	Fuzz            bool                 `json:"fuzz,omitempty"`
}

// DiscoverLanguageTests returns deterministic *_test.osr and *.test.osr files
// from files or directories. A direct file is accepted regardless of suffix.
func DiscoverLanguageTests(paths []string) ([]string, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	seen := make(map[string]struct{})
	for _, raw := range paths {
		info, err := os.Stat(raw)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			path, err := filepath.Abs(raw)
			if err != nil {
				return nil, err
			}
			path = filepath.Clean(path)
			if !isLanguageTestFile(path) {
				if companion := colocatedTestFile(path); companion != "" {
					seen[companion] = struct{}{}
					continue
				}
			}
			seen[path] = struct{}{}
			continue
		}
		if err := filepath.WalkDir(raw, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if path != raw && (entry.Name() == ".git" || entry.Name() == ".osiris" || strings.HasPrefix(entry.Name(), ".")) {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(entry.Name(), "_test.osr") || strings.HasSuffix(entry.Name(), ".test.osr") {
				absolute, err := filepath.Abs(path)
				if err != nil {
					return err
				}
				seen[filepath.Clean(absolute)] = struct{}{}
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	files := make([]string, 0, len(seen))
	for path := range seen {
		files = append(files, path)
	}
	sort.Strings(files)
	return files, nil
}

func isLanguageTestFile(path string) bool {
	name := filepath.Base(path)
	return strings.HasSuffix(name, "_test.osr") || strings.HasSuffix(name, ".test.osr")
}

// colocatedTestFile maps a production source file to its canonical sidecar,
// retaining the *_test.osr spelling as a compatibility fallback. The returned
// path is absolute and exists on disk.
func colocatedTestFile(sourcePath string) string {
	base := strings.TrimSuffix(sourcePath, filepath.Ext(sourcePath))
	candidates := []string{base + ".test.osr", base + "_test.osr"}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			absolute, err := filepath.Abs(candidate)
			if err == nil {
				return filepath.Clean(absolute)
			}
		}
	}
	return ""
}

func colocatedSourceFile(testPath string) string {
	base := testPath
	if strings.HasSuffix(base, ".test.osr") {
		base = strings.TrimSuffix(base, ".test.osr")
	} else if strings.HasSuffix(base, "_test.osr") {
		base = strings.TrimSuffix(base, "_test.osr")
	} else {
		return ""
	}
	source := base + ".osr"
	if info, err := os.Stat(source); err == nil && !info.IsDir() {
		return source
	}
	return ""
}

// DiscoverLanguageTestInventory parses declarations and returns their stable
// source order. Malformed files return an error because inventory is a tooling
// contract; normal execution turns the same error into a compile result.
func DiscoverLanguageTestInventory(paths []string, filter string) (LanguageTestInventory, error) {
	files, err := DiscoverLanguageTests(paths)
	if err != nil {
		return LanguageTestInventory{}, err
	}
	entries := make([]LanguageTestEntry, 0)
	for _, file := range files {
		source, err := os.ReadFile(file)
		if err != nil {
			return LanguageTestInventory{}, err
		}
		cases, err := parseLanguageTestFile(file, string(source))
		if err != nil {
			return LanguageTestInventory{}, err
		}
		for _, item := range cases {
			if filter != "" && !languageTestFilterMatch(filter, item.name) {
				continue
			}
			entries = append(entries, LanguageTestEntry{Name: item.name, File: file, Location: item.location, Skip: item.skipReason != "", ExpectedFailure: item.expectedFailure, Benchmark: item.benchmark != nil, Fuzz: item.fuzz != nil})
		}
	}
	return LanguageTestInventory{SchemaVersion: 1, Files: files, Tests: entries}, nil
}

// RunLanguageTestSource executes one source program through the requested
// public backend. It is retained as the small library entry point used by
// existing callers; declaration-aware runs use RunLanguageTests.
func RunLanguageTestSource(ctx context.Context, name, source string, options LanguageTestOptions) LanguageTestResult {
	started := time.Now()
	result := LanguageTestResult{Name: name, File: name, Backend: normalizedTestBackend(options.Backend), Status: "ok", Passed: false, Location: languageTestLocation(name, 1, 1, 1, 1)}
	if ctx == nil {
		ctx = context.Background()
	}
	caseDef := languageTestCase{name: name, file: name, body: source, location: result.Location}
	runLanguageTestCase(ctx, &caseDef, options, &result)
	result.Duration = time.Since(started)
	result.DurationNS = result.Duration.Nanoseconds()
	return result
}

// RunLanguageTests discovers, expands, filters, and executes source tests. It
// preserves discovery order in its returned slice even when Parallel is set.
// Input/path failures are returned as errors; source and runtime failures are
// represented as results so JSON mode can report every case.
func RunLanguageTests(ctx context.Context, paths []string, options LanguageTestOptions) ([]LanguageTestResult, error) {
	files, err := DiscoverLanguageTests(paths)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if options.Parallel < 1 {
		options.Parallel = 1
	}
	if options.FuzzRuns < 1 {
		options.FuzzRuns = 32
	}
	if options.FuzzSeed == 0 {
		options.FuzzSeed = 1
	}
	if options.ArtifactsDir == "" {
		options.ArtifactsDir = filepath.Join(".osiris", "test-artifacts")
	}
	if options.CacheDir != "" && !options.NoCache {
		if err := writeLanguageTestInventoryCache(options.CacheDir, files); err != nil {
			return nil, err
		}
	}
	all := make([]languageTestCase, 0)
	for _, file := range files {
		source, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		cases, parseErr := parseLanguageTestFile(file, string(source))
		if parseErr != nil {
			all = append(all, languageTestCase{name: filepath.Base(file), file: file, body: string(source), parseErr: parseErr, location: languageTestLocation(file, 1, 1, 1, 1)})
			continue
		}
		if sourceFile := colocatedSourceFile(file); sourceFile != "" {
			production, err := os.ReadFile(sourceFile)
			if err != nil {
				return nil, err
			}
			prefix := string(production) + "\n"
			for i := range cases {
				cases[i].body = prefix + cases[i].body
			}
		}
		for _, item := range cases {
			if options.Filter != "" && !languageTestFilterMatch(options.Filter, item.name) {
				continue
			}
			all = append(all, item)
		}
	}
	results := make([]LanguageTestResult, len(all))
	if options.Parallel == 1 || len(all) < 2 {
		for i := range all {
			results[i] = runIndexedLanguageTest(ctx, &all[i], options)
		}
		return results, nil
	}
	type job struct{ index int }
	jobs := make(chan job)
	workers := options.Parallel
	if workers > len(all) {
		workers = len(all)
	}
	finished := make(chan struct{})
	for worker := 0; worker < workers; worker++ {
		go func() {
			for item := range jobs {
				results[item.index] = runIndexedLanguageTest(ctx, &all[item.index], options)
			}
			finished <- struct{}{}
		}()
	}
	for i := range all {
		select {
		case jobs <- job{index: i}:
		case <-ctx.Done():
			results[i] = canceledLanguageTestResult(all[i], ctx.Err())
		}
	}
	close(jobs)
	for worker := 0; worker < workers; worker++ {
		<-finished
	}
	return results, nil
}

func runIndexedLanguageTest(ctx context.Context, item *languageTestCase, options LanguageTestOptions) LanguageTestResult {
	started := time.Now()
	result := LanguageTestResult{Name: item.name, File: item.file, Backend: normalizedTestBackend(options.Backend), Status: "fail", Location: item.location}
	if item.parseErr != nil {
		result.Error = item.parseErr
		result.ErrorText = item.parseErr.Error()
		result.Status = "compile_error"
		result.Duration = time.Since(started)
		result.DurationNS = result.Duration.Nanoseconds()
		return result
	}
	if item.skipReason != "" {
		result.Status, result.Passed, result.Skipped = "skip", true, true
		result.ErrorText = item.skipReason
		result.Duration = time.Since(started)
		result.DurationNS = result.Duration.Nanoseconds()
		return result
	}
	if item.expectedFailure {
		result.ExpectedFailure = true
	}
	runLanguageTestCase(ctx, item, options, &result)
	result.Duration = time.Since(started)
	result.DurationNS = result.Duration.Nanoseconds()
	if options.Deterministic {
		result.DurationNS = 0
	}
	return result
}

func canceledLanguageTestResult(item languageTestCase, err error) LanguageTestResult {
	return LanguageTestResult{Name: item.name, File: item.file, Backend: "interpreter", Status: "cancel", Passed: false, Error: err, ErrorText: err.Error(), Location: item.location}
}

func normalizedTestBackend(backend string) string {
	if backend == "" {
		return "interpreter"
	}
	return backend
}

func languageTestLocation(source string, line, col, endLine, endCol int) LanguageTestLocation {
	return LanguageTestLocation{Source: source, Start: SourcePosition{Line: line, Column: col}, End: SourcePosition{Line: endLine, Column: endCol}}
}

func languageTestFilterMatch(filter, name string) bool {
	if filter == name || filter == "" {
		return true
	}
	matched, err := filepath.Match(filter, name)
	if err == nil && matched {
		return true
	}
	return strings.Contains(name, filter)
}

func writeLanguageTestInventoryCache(dir string, files []string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "inventory-v1.json")
	data := []byte("{\"schema_version\":1,\"files\":[")
	for i, file := range files {
		if i > 0 {
			data = append(data, ',')
		}
		data = append(data, []byte(fmt.Sprintf("%q", file))...)
	}
	data = append(data, []byte("]}\n")...)
	return os.WriteFile(path, data, 0o644)
}
