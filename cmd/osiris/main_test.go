package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sphireinc/osiris/internal/osiris"
)

func TestVersionReportsActionableProvenance(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := execute(context.Background(), []string{"version"}, &stdout, &stderr); code != cliExitSuccess {
		t.Fatalf("version exit = %d, stderr = %s", code, stderr.String())
	}
	for _, field := range []string{"osiris ", "commit:", "built:", "go:", "target:"} {
		if !strings.Contains(stdout.String(), field) {
			t.Fatalf("version output lacks %q: %s", field, stdout.String())
		}
	}
}

func TestRunOptJSONReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.osr")
	if err := os.WriteFile(path, []byte("let dead = 1 + 2\nemit 2 * (3 + 4)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := run(context.Background(), []string{"opt", "-json", path}, &stdout, io.Discard); err != nil {
		t.Fatalf("opt -json failed: %v", err)
	}
	var report struct {
		Version int `json:"version"`
		Passes  []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"passes"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("report is not JSON: %v\n%s", err, stdout.String())
	}
	if report.Version != 1 || len(report.Passes) != 6 || report.Passes[0].Name != "simplify-and-propagate" || report.Passes[0].Status != "applied" {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestExecuteDiagnosticContract(t *testing.T) {
	valid := filepath.Join(t.TempDir(), "valid.osr")
	runtimeFailure := filepath.Join(t.TempDir(), "runtime.osr")
	compileFailure := filepath.Join(t.TempDir(), "compile.osr")
	if err := os.WriteFile(valid, []byte("emit 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtimeFailure, []byte("emit missing_name\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(compileFailure, []byte("let = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantClass  string
	}{
		{name: "success", args: []string{"run", valid}, wantCode: cliExitSuccess, wantStdout: "42u64\n"},
		{name: "usage", args: []string{"run", "--missing"}, wantCode: cliExitUsage, wantClass: "usage"},
		{name: "input", args: []string{"run", "does-not-exist.osr"}, wantCode: cliExitInput, wantClass: "input"},
		{name: "compile", args: []string{"run", compileFailure}, wantCode: cliExitCompile, wantClass: "compile"},
		{name: "runtime", args: []string{"run", runtimeFailure}, wantCode: cliExitRuntime, wantClass: "runtime"},
		{name: "unknown", args: []string{"unknown"}, wantCode: cliExitUsage, wantClass: "usage"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append([]string{"--diagnostics=json"}, tc.args...)
			if got := execute(context.Background(), args, &stdout, &stderr); got != tc.wantCode {
				t.Fatalf("exit code = %d, want %d; stderr=%s", got, tc.wantCode, stderr.String())
			}
			if got := stdout.String(); got != tc.wantStdout {
				t.Fatalf("stdout = %q, want %q", got, tc.wantStdout)
			}
			if tc.wantClass == "" {
				if stderr.Len() != 0 {
					t.Fatalf("successful command wrote stderr: %q", stderr.String())
				}
				return
			}
			var diagnostic cliDiagnostic
			if err := json.Unmarshal(stderr.Bytes(), &diagnostic); err != nil {
				t.Fatalf("stderr is not a JSON diagnostic: %v\n%s", err, stderr.String())
			}
			if diagnostic.SchemaVersion != 1 || diagnostic.Category != tc.wantClass || diagnostic.ExitCode != tc.wantCode || diagnostic.Command != tc.args[0] {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
		})
	}
}

func TestExecuteHumanUsageWritesOnlyStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := execute(context.Background(), []string{"help", "missing"}, &stdout, &stderr); code != cliExitUsage {
		t.Fatalf("exit code = %d, want %d", code, cliExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("invalid help wrote stdout: %q", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "unknown command") || !strings.Contains(got, "usage:") || !strings.Contains(got, "--diagnostics=json") {
		t.Fatalf("stderr lacks stable usage diagnostic: %q", got)
	}
}

func TestExecutableAppliesDiagnosticExitCode(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "osiris")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Env = append(os.Environ(), "GOCACHE="+filepath.Join(t.TempDir(), "gocache"))
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	cmd := exec.Command(bin, "--diagnostics=json", "run", "missing.osr")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("missing input unexpectedly succeeded")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != cliExitInput {
		t.Fatalf("exit error = %v, want code %d", err, cliExitInput)
	}
	var diagnostic cliDiagnostic
	if err := json.Unmarshal(output, &diagnostic); err != nil {
		t.Fatalf("executable diagnostic is not JSON: %v\n%s", err, output)
	}
	if diagnostic.Category != "input" || diagnostic.ExitCode != cliExitInput {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestRunModCleanRemovesOnlyGeneratedModuleState(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "main.osr")
	if err := os.WriteFile(source, []byte("emit 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lockPath, cacheDir := osiris.ModuleStatePaths(source)
	if err := os.WriteFile(lockPath, []byte("generated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "module.osr"), []byte("generated"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := run(context.Background(), []string{"mod", "clean", source}, &stdout, io.Discard); err != nil {
		t.Fatalf("mod clean: %v", err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock remains: %v", err)
	}
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Fatalf("cache remains: %v", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source was affected: %v", err)
	}
}

func TestRunBuildWithCache(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "program.osr")
	if err := os.WriteFile(sourcePath, []byte("emit 6 * 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(dir, "cache")
	outputPath := filepath.Join(dir, "program")
	var stdout, stderr bytes.Buffer
	args := []string{"build", "-o", outputPath, "-cache-dir", cacheDir, sourcePath}
	if err := run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("cached build failed: %v\nstderr: %s", err, stderr.String())
	}
	out, err := exec.Command(outputPath).CombinedOutput()
	if err != nil {
		t.Fatalf("cached executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "42u64\n"; got != want {
		t.Fatalf("cached executable output = %q, want %q", got, want)
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil || len(entries) < 2 {
		t.Fatalf("cache manifest/artifact missing: entries=%v err=%v", entries, err)
	}
}

func TestRunBuildWithLinalgProviderSource(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "program.osr")
	providerPath := filepath.Join(dir, "provider.c")
	outputPath := filepath.Join(dir, "program")
	if err := os.WriteFile(sourcePath, []byte("emit 6 * 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(providerPath, []byte("int osiris_provider_marker(void) { return 0; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := []string{"build", "-o", outputPath, "-linalg-provider", providerPath, sourcePath}
	if err := run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("provider build failed: %v\nstderr: %s", err, stderr.String())
	}
	out, err := exec.Command(outputPath).CombinedOutput()
	if err != nil {
		t.Fatalf("provider executable failed: %v\n%s", err, out)
	}
	if got, want := string(out), "42u64\n"; got != want {
		t.Fatalf("provider executable output = %q, want %q", got, want)
	}
}
