package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

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
