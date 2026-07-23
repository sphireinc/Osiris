package osiris

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type CArtifact struct {
	Source string
}

// CCompileOptions selects optional native link-time facilities.
type CCompileOptions struct {
	LinalgProviderSource string
}

// CCompileManifest describes an addressable native executable artifact. Cache
// keys include every input that can affect machine code generation.
type CCompileManifest struct {
	Key                  string   `json:"key"`
	SourceSHA256         string   `json:"source_sha256"`
	Compiler             string   `json:"compiler"`
	CompilerIdentity     string   `json:"compiler_identity"`
	Flags                []string `json:"flags"`
	LinalgProviderSHA256 string   `json:"linalg_provider_sha256,omitempty"`
	Target               string   `json:"target"`
	CacheHit             bool     `json:"-"`
}

// CompileC compiles the c into the requested backend artifact so the caller
// can execute or inspect the program without keeping the source interpreter in
// the hot path.
//
// It returns *CArtifact and an error. Callers should only trust the value when
// the error is nil.
func CompileC(ctx context.Context, rt *Runtime, prog *Program) (*CArtifact, error) {
	return CompileCWithOptions(ctx, rt, prog, CCompileOptions{})
}

// CompileCWithOptions lowers a program with explicit native backend options.
func CompileCWithOptions(ctx context.Context, rt *Runtime, prog *Program, options CCompileOptions) (*CArtifact, error) {
	if rt == nil {
		rt = NewRuntime()
	}
	ir, err := rt.BuildIR(ctx, prog)
	if err != nil {
		return nil, err
	}
	source, err := emitCIRWithOptions(ir, options)
	if err != nil {
		return nil, err
	}
	return &CArtifact{Source: source}, nil
}

// cEscapeString handles the c escape string step in the surrounding package
// and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func cEscapeString(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c >= 0x20 && c <= 0x7e {
				b.WriteByte(c)
				continue
			}
			b.WriteString(fmt.Sprintf(`\x%02x`, c))
		}
	}
	return b.String()
}

// CompileCObject compiles the cobject into the requested backend artifact so
// the caller can execute or inspect the program without keeping the source
// interpreter in the hot path.
//
// It returns an error when the operation cannot complete cleanly.
func CompileCObject(ctx context.Context, source string, outputPath string) error {
	return compileCArtifact(ctx, source, outputPath, true, CCompileOptions{})
}

// CompileCExecutable compiles the cexecutable into the requested backend
// artifact so the caller can execute or inspect the program without keeping
// the source interpreter in the hot path.
//
// It returns an error when the operation cannot complete cleanly.
func CompileCExecutable(ctx context.Context, source string, outputPath string) error {
	return CompileCExecutableWithOptions(ctx, source, outputPath, CCompileOptions{})
}

// CompileCExecutableWithOptions compiles generated C and optional provider C.
func CompileCExecutableWithOptions(ctx context.Context, source, outputPath string, options CCompileOptions) error {
	return compileCArtifact(ctx, source, outputPath, false, options)
}

// CompileCExecutableCached compiles source into outputPath, reusing a
// content-addressed executable in cacheDir when the generated C, compiler, and
// compilation flags are identical. cacheDir is explicit so builds stay
// reproducible and callers control cache lifetime.
func CompileCExecutableCached(ctx context.Context, source, outputPath, cacheDir string) (*CCompileManifest, error) {
	return CompileCExecutableCachedWithOptions(ctx, source, outputPath, cacheDir, CCompileOptions{})
}

func CompileCExecutableCachedWithOptions(ctx context.Context, source, outputPath, cacheDir string, options CCompileOptions) (*CCompileManifest, error) {
	compiler := os.Getenv("CC")
	if compiler == "" {
		compiler = "cc"
	}
	compilerPath, err := exec.LookPath(compiler)
	if err != nil {
		return nil, fmt.Errorf("%s not found: %w", compiler, err)
	}
	identityOut, err := exec.CommandContext(ctx, compilerPath, "--version").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("identify C compiler %s: %w", compilerPath, err)
	}
	flags := []string{"-std=c11", "-O3", "-march=native", "-DNDEBUG", "-flto", "-lm"}
	sourceHash := sha256.Sum256([]byte(source))
	providerHash := ""
	if options.LinalgProviderSource != "" {
		data, err := os.ReadFile(options.LinalgProviderSource)
		if err != nil {
			return nil, fmt.Errorf("native numerical provider %q: %w", options.LinalgProviderSource, err)
		}
		hash := sha256.Sum256(data)
		providerHash = hex.EncodeToString(hash[:])
	}
	keyInput := strings.Join([]string{hex.EncodeToString(sourceHash[:]), providerHash, compilerPath, string(identityOut), strings.Join(flags, "\x00"), "executable"}, "\x00")
	keyHash := sha256.Sum256([]byte(keyInput))
	manifest := &CCompileManifest{
		Key:                  hex.EncodeToString(keyHash[:]),
		SourceSHA256:         hex.EncodeToString(sourceHash[:]),
		LinalgProviderSHA256: providerHash,
		Compiler:             compilerPath,
		CompilerIdentity:     strings.TrimSpace(string(identityOut)),
		Flags:                flags,
		Target:               "executable",
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, err
	}
	cachePath := filepath.Join(cacheDir, manifest.Key)
	if info, err := os.Stat(cachePath); err == nil && !info.IsDir() {
		if err := copyExecutable(cachePath, outputPath); err != nil {
			return nil, err
		}
		manifest.CacheHit = true
		return manifest, nil
	}
	tempPath := cachePath + ".tmp"
	if err := compileCArtifact(ctx, source, tempPath, false, options); err != nil {
		return nil, err
	}
	if err := os.Rename(tempPath, cachePath); err != nil {
		_ = os.Remove(tempPath)
		return nil, err
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(cachePath+".json", append(encoded, '\n'), 0o644); err != nil {
		return nil, err
	}
	if err := copyExecutable(cachePath, outputPath); err != nil {
		return nil, err
	}
	return manifest, nil
}

func copyExecutable(sourcePath, outputPath string) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, data, 0o755); err != nil {
		return err
	}
	return os.Chmod(outputPath, 0o755)
}

// compileCArtifact handles the compile cartifact step in the surrounding
// package and returns error.
//
// It returns an error when the operation cannot complete cleanly.
func compileCArtifact(ctx context.Context, source, outputPath string, object bool, options CCompileOptions) error {
	compiler := os.Getenv("CC")
	if compiler == "" {
		compiler = "cc"
	}
	if _, err := exec.LookPath(compiler); err != nil {
		return fmt.Errorf("%s not found: %w", compiler, err)
	}
	dir, err := os.MkdirTemp("", "osiris-c-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	srcPath := filepath.Join(dir, "snapshot.c")
	if err := os.WriteFile(srcPath, []byte(source), 0o644); err != nil {
		return err
	}

	args := []string{"-std=c11", "-O3", "-march=native", "-DNDEBUG"}
	if object {
		args = append(args, "-c", srcPath, "-o", outputPath)
	} else {
		args = append(args, "-flto", srcPath, "-o", outputPath)
		if options.LinalgProviderSource != "" {
			if _, err := os.Stat(options.LinalgProviderSource); err != nil {
				return fmt.Errorf("native numerical provider %q: %w", options.LinalgProviderSource, err)
			}
			args = append(args, options.LinalgProviderSource)
		}
	}
	cmd := exec.CommandContext(ctx, compiler, args...)
	if !object {
		cmd.Args = append(cmd.Args, "-lm")
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s failed: %w\n%s", compiler, err, strings.TrimSpace(string(output)))
	}
	return nil
}
