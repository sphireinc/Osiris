package osiris

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type CArtifact struct {
	Source string
}

// CompileC compiles the c into the requested backend artifact so the caller
// can execute or inspect the program without keeping the source interpreter in
// the hot path.
//
// It returns *CArtifact and an error. Callers should only trust the value when
// the error is nil.
func CompileC(ctx context.Context, rt *Runtime, prog *Program) (*CArtifact, error) {
	if rt == nil {
		rt = NewRuntime()
	}
	sess := NewSession(rt, ctx, prog.SourceName)
	outputs, err := sess.ExecProgram(prog, false, nil)
	if err != nil {
		return nil, err
	}
	rendered := make([]string, len(outputs))
	for i, v := range outputs {
		rendered[i] = v.String()
	}
	return &CArtifact{Source: buildCSnapshot(prog.SourceName, rendered)}, nil
}

// buildCSnapshot handles the build csnapshot step in the surrounding package
// and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func buildCSnapshot(sourceName string, outputs []string) string {
	var b strings.Builder
	b.WriteString("#include <stdio.h>\n\n")
	b.WriteString("// Osiris C snapshot from ")
	b.WriteString(strings.ReplaceAll(strings.TrimSpace(sourceName), "\n", " "))
	b.WriteByte('\n')
	b.WriteString("static const char *osiris_outputs[] = {\n")
	if len(outputs) == 0 {
		b.WriteString("  \"\",\n")
	} else {
		for _, out := range outputs {
			b.WriteString("  \"")
			b.WriteString(cEscapeString(out))
			b.WriteString("\",\n")
		}
	}
	b.WriteString("};\n")
	b.WriteString("static const int osiris_output_count = ")
	b.WriteString(strconv.Itoa(len(outputs)))
	b.WriteString(";\n\n")
	b.WriteString("int main(void) {\n")
	b.WriteString("  for (int i = 0; i < osiris_output_count; ++i) {\n")
	b.WriteString("    puts(osiris_outputs[i]);\n")
	b.WriteString("  }\n")
	b.WriteString("  return 0;\n")
	b.WriteString("}\n")
	return b.String()
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
	return compileCArtifact(ctx, source, outputPath, true)
}

// CompileCExecutable compiles the cexecutable into the requested backend
// artifact so the caller can execute or inspect the program without keeping
// the source interpreter in the hot path.
//
// It returns an error when the operation cannot complete cleanly.
func CompileCExecutable(ctx context.Context, source string, outputPath string) error {
	return compileCArtifact(ctx, source, outputPath, false)
}

// compileCArtifact handles the compile cartifact step in the surrounding
// package and returns error.
//
// It returns an error when the operation cannot complete cleanly.
func compileCArtifact(ctx context.Context, source, outputPath string, object bool) error {
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
