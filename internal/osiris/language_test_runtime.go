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

// LanguageTestOptions controls source-test execution. Normal tests receive a
// fresh deterministic NewTestRuntime for each case.
type LanguageTestOptions struct {
	Backend      string
	Timeout      time.Duration
	Integration  bool
	Capabilities []string
}

type LanguageTestResult struct {
	Name     string
	Passed   bool
	Outputs  []Value
	Error    error
	Duration time.Duration
}

// DiscoverLanguageTests returns deterministic *_test.osr files from files or
// directories. A direct file is accepted regardless of suffix.
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
			seen[filepath.Clean(path)] = struct{}{}
			continue
		}
		err = filepath.WalkDir(raw, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
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
		})
		if err != nil {
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

// RunLanguageTestSource executes one source test with an isolated provider
// bundle. Integration mode is explicit and must provide its capabilities.
func RunLanguageTestSource(ctx context.Context, name, source string, options LanguageTestOptions) LanguageTestResult {
	started := time.Now()
	result := LanguageTestResult{Name: name}
	if ctx == nil {
		ctx = context.Background()
	}
	if options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, options.Timeout)
		defer cancel()
	}
	program, err := Parse(name, source)
	if err != nil {
		result.Error = err
		result.Duration = time.Since(started)
		return result
	}
	rt := NewTestRuntime()
	if options.Integration {
		rt = NewRuntime()
		for _, capability := range options.Capabilities {
			rt.GrantCapability(capability)
		}
	}
	switch options.Backend {
	case "", "interpreter":
		result.Outputs, result.Error = rt.Run(ctx, program)
	case "vm":
		result.Outputs, result.Error = rt.RunVM(ctx, program)
	default:
		result.Error = fmt.Errorf("test backend %q is unsupported; choose interpreter or vm", options.Backend)
	}
	result.Passed = result.Error == nil
	result.Duration = time.Since(started)
	return result
}

// RunLanguageTests executes discovered tests in deterministic order and
// resets fake provider state for every test case.
func RunLanguageTests(ctx context.Context, paths []string, options LanguageTestOptions) ([]LanguageTestResult, error) {
	files, err := DiscoverLanguageTests(paths)
	if err != nil {
		return nil, err
	}
	results := make([]LanguageTestResult, 0, len(files))
	for _, path := range files {
		source, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		results = append(results, RunLanguageTestSource(ctx, path, string(source), options))
	}
	return results, nil
}
