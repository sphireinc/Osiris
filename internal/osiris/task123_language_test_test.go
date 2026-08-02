package osiris

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTask123InventoryAndDeterministicParameterizedCases(t *testing.T) {
	root := filepath.Join("testdata", "task-123")
	inventory, err := DiscoverLanguageTestInventory([]string{filepath.Join(root, "basic_test.osr")}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Tests) != 5 || inventory.Tests[0].Name != "addition" || inventory.Tests[1].Name != "parameterized[one]" {
		t.Fatalf("inventory = %#v", inventory.Tests)
	}
	results, err := RunLanguageTests(context.Background(), []string{filepath.Join(root, "basic_test.osr")}, LanguageTestOptions{Parallel: 2, Deterministic: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 5 || results[0].Status != "ok" || results[1].Status != "ok" || results[2].Status != "ok" {
		t.Fatalf("results = %#v", results)
	}
	if !results[3].Skipped || results[3].Status != "skip" || !results[4].ExpectedFailure || results[4].Status != "expected_failure" {
		t.Fatalf("skip/xfail results = %#v", results[3:])
	}
	if len(results[0].Assertions) < 2 || results[0].Assertions[1].Location.Start.Line != 3 {
		t.Fatalf("assertion location = %#v", results[0].Assertions)
	}
}

func TestTask123FixtureGoldenCoverageAndVM(t *testing.T) {
	root := filepath.Join("testdata", "task-123")
	coverage := filepath.Join(t.TempDir(), "coverage.json")
	results, err := RunLanguageTests(context.Background(), []string{filepath.Join(root, "fixture_test.osr"), filepath.Join(root, "golden_test.osr")}, LanguageTestOptions{Backend: "interpreter", CoveragePath: coverage, Deterministic: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if !result.Passed {
			t.Fatalf("result = %#v, error = %v", result, result.Error)
		}
	}
	data, err := os.ReadFile(coverage)
	if err != nil {
		t.Fatal(err)
	}
	var report LanguageCoverageReport
	if err := json.Unmarshal(data, &report); err != nil || !report.Available || report.SchemaVersion != 1 || len(report.SourceMap) == 0 {
		t.Fatalf("coverage = %s, err = %v", data, err)
	}
	vmResults, err := RunLanguageTests(context.Background(), []string{filepath.Join(root, "golden_test.osr")}, LanguageTestOptions{Backend: "vm", Deterministic: true})
	if err != nil || len(vmResults) != 1 || !vmResults[0].Passed {
		t.Fatalf("vm results = %#v, err = %v", vmResults, err)
	}
}

func TestTask123BenchmarkFuzzAndTimeoutAreBounded(t *testing.T) {
	dir := t.TempDir()
	benchmark := filepath.Join(dir, "bench_test.osr")
	if err := os.WriteFile(benchmark, []byte("// osiris:test name=bench\n// osiris:benchmark warmup=0 samples=1\nemit 1 + 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := RunLanguageTests(context.Background(), []string{benchmark}, LanguageTestOptions{Benchmark: true, Deterministic: true})
	if err != nil || len(results) != 1 || !results[0].Passed || results[0].Benchmark == nil || len(results[0].Benchmark.CostsMS["interpreter"].Samples) != 1 {
		t.Fatalf("benchmark = %#v, err = %v", results, err)
	}
	fuzz := filepath.Join(dir, "fuzz_test.osr")
	if err := os.WriteFile(fuzz, []byte("// osiris:test name=fuzz\n// osiris:fuzz runs=2 max_size=4\nemit len({{input}})\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fuzzResults, err := RunLanguageTests(context.Background(), []string{fuzz}, LanguageTestOptions{Fuzz: true, FuzzRuns: 2, FuzzSeed: 9, ArtifactsDir: filepath.Join(dir, "artifacts"), Deterministic: true})
	if err != nil || len(fuzzResults) != 1 || !fuzzResults[0].Passed {
		t.Fatalf("fuzz = %#v, err = %v", fuzzResults, err)
	}
	timeout := filepath.Join(dir, "timeout_test.osr")
	if err := os.WriteFile(timeout, []byte("// osiris:test name=timeout\nwhile true { let x = 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	timeoutResults, err := RunLanguageTests(context.Background(), []string{timeout}, LanguageTestOptions{Timeout: time.Millisecond, Deterministic: true})
	if err != nil || len(timeoutResults) != 1 || timeoutResults[0].Passed {
		t.Fatalf("timeout = %#v, err = %v", timeoutResults, err)
	}
}
