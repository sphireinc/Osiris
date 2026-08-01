package osiris

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCBackendMissionControlShowcaseParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	sourcePath := filepath.Join("..", "..", "benchmarks", "aegis-mission-control.osr")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	program, err := Parse(sourcePath, string(source))
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewRuntime()
	interpreter, err := runtime.Run(context.Background(), program)
	if err != nil {
		t.Fatalf("interpreter: %v", err)
	}
	vm, err := runtime.RunVM(context.Background(), program)
	if err != nil {
		t.Fatalf("VM: %v", err)
	}
	if got, want := valuesOutput(vm), valuesOutput(interpreter); got != want {
		t.Fatalf("VM output differs from interpreter\nwant:\n%s\ngot:\n%s", want, got)
	}

	artifact, err := CompileCWithOptions(context.Background(), NewRuntime(), program, CCompileOptions{PublicEmitSpacing: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"static OValue osiris_fn_",
		"otask(",
		"oawait(",
		"oparallel(",
		"otry(",
		"oresultbind(",
		"ocrypto_hash(",
		"ostd_time_date(",
		"ostd_signal_convolve(",
		"omatmul(",
		"odecimal(",
		"obigint(",
		"oquantityunit(",
		"oencode(",
		"odecode(",
		"oerror(",
	} {
		if !strings.Contains(artifact.Source, marker) {
			t.Fatalf("generated C omitted %q", marker)
		}
	}
	const reportDigest = "a1eafc9b8493791c78c1c94d5a4f98211e714f35616bbfea184f199508068966"
	if strings.Contains(artifact.Source, reportDigest) {
		t.Fatal("generated C embeds the precomputed mission digest")
	}
	for _, forbidden := range []string{
		"Session.ExecProgram",
		"RunVM",
		"osiris run",
		"osiris vm",
		"popen(",
		"system(",
		"fopen(",
	} {
		if strings.Contains(artifact.Source, forbidden) {
			t.Fatalf("generated C contains forbidden evaluator/subprocess/source marker %q", forbidden)
		}
	}

	binaryDir := t.TempDir()
	executable := filepath.Join(binaryDir, "mission-control-showcase")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	var first []byte
	for run := 0; run < 3; run++ {
		var stdout, stderr bytes.Buffer
		command := exec.Command(executable)
		command.Dir = runDir
		command.Stdout = &stdout
		command.Stderr = &stderr
		if err := command.Run(); err != nil {
			t.Fatalf("native executable run %d: %v\nstderr: %s", run+1, err, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("native executable run %d wrote stderr: %q", run+1, stderr.String())
		}
		if run == 0 {
			first = append([]byte(nil), stdout.Bytes()...)
		} else if !bytes.Equal(stdout.Bytes(), first) {
			t.Fatalf("native executable run %d is nondeterministic", run+1)
		}
	}
	const wantSHA256 = "16d0a61eb6aa77a9a5a0ed1735678bf5eb152962e43d38170c88cbc8ec4771c2"
	if got := fmt.Sprintf("%x", sha256.Sum256(first)); got != wantSHA256 {
		if os.Getenv("OSIRIS_C_SANITIZERS") != "" {
			t.Logf("sanitized native output SHA-256 = %s; exact production fingerprint is checked without instrumentation", got)
			return
		}
		t.Fatalf("native output SHA-256 = %s, want %s\n%s", got, wantSHA256, first)
	}
}

func TestCBackendCodecCrossDirectionParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	const declarations = `import "std/time" as time
let archive = ["archive", time.date(2032, 2, 29, 12, 30, 0, 0, time.utc_location), [7u32, 1.25f64, complex(2.5f64, -3.0f64)]]
`
	encoder, err := Parse("<native-codec-encoder>", declarations+"emit encode(archive)\n")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := NewRuntime().Run(context.Background(), encoder)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != 1 || encoded[0].kind != kindString {
		t.Fatalf("interpreter encode result = %#v", encoded)
	}
	encodedBytes := nativeBytesExpr([]byte(encoded[0].str))
	source := declarations +
		"emit bytes(encode(archive)) == " + encodedBytes + "\n" +
		"emit bytes(encode(decode(" + encodedBytes + "))) == " + encodedBytes + "\n"
	program, err := Parse("<native-codec-cross-direction>", source)
	if err != nil {
		t.Fatal(err)
	}
	want, err := NewRuntime().Run(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "codec-cross-direction")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatalf("compile executable: %v\n%s", err, artifact.Source)
	}
	output, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native codec executable: %v\n%s", err, output)
	}
	if got := string(output); got != valuesOutput(want) {
		t.Fatalf("native codec output = %q, want %q", got, valuesOutput(want))
	}
}

func TestCBackendCodecRejectsMalformedCorpus(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	corpus := [][]byte{
		{},
		[]byte("OSR"),
		{'O', 'S', 'R', '1', 0xff},
		{'O', 'S', 'R', '1', 0x05, 0x80, 0x00},
		{'O', 'S', 'R', '1', 0x00, 0x00},
	}
	for tag := byte(0x20); tag < 0x40; tag++ {
		corpus = append(corpus, []byte{'O', 'S', 'R', '1', tag})
	}
	var source strings.Builder
	for _, raw := range corpus {
		fmt.Fprintf(&source, "emit try { decode(%s) } catch err { \"caught\" }\n", nativeBytesExpr(raw))
	}
	program, err := Parse("<native-codec-malformed>", source.String())
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "codec-malformed")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatalf("compile executable: %v\n%s", err, artifact.Source)
	}
	output, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native malformed corpus: %v\n%s", err, output)
	}
	if got, want := string(output), strings.Repeat("\"caught\"\n", len(corpus)); got != want {
		t.Fatalf("native malformed corpus output = %q, want %q", got, want)
	}
}

func TestCBackendDeferredTaskAndFailureParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	const source = `let captured = 41
let delayed = spawn({ uint("not-a-number") })
let answer = spawn({ captured + 1 })
emit "tasks-created"
emit await(answer)
emit await(answer)
emit try { await(delayed) } catch err { cat("caught/", err) }
emit try { await(delayed) } catch err { cat("caught-again/", err) }
emit parallel({ 3 }, { 2 }, { 1 })
`
	program, err := Parse("<native-task-parity>", source)
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewRuntime()
	interpreter, err := runtime.Run(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := runtime.RunVM(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := valuesOutput(vm), valuesOutput(interpreter); got != want {
		t.Fatalf("VM task output = %q, want %q", got, want)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(artifact.Source, "otask(") > strings.Index(artifact.Source, "oawait(") {
		t.Fatal("generated task is not created before its first await")
	}
	executable := filepath.Join(t.TempDir(), "task-parity")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatalf("compile executable: %v\n%s", err, artifact.Source)
	}
	output, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native task executable: %v\n%s", err, output)
	}
	if got := string(output); got != valuesOutput(interpreter) {
		t.Fatalf("native task output = %q, want %q", got, valuesOutput(interpreter))
	}
}

func TestCBackendStructuredResultFailureParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	const source = `emit try { let? value = ok(41); value + 1 } catch caught { caught.kind }
emit try { let? value = err(error(InvalidArgument, "bad result")); value } catch caught { cat(caught.kind, "/", caught.message) }
let delayed = spawn({ let? value = err(error(NotFound, "missing task")); value })
emit try { await(delayed) } catch caught { cat(caught.kind, "/", caught.message) }
emit try { await(delayed) } catch caught { caught.kind }
let cancelled = spawn({ while true { }; 99 })
emit cancel(cancelled)
emit try { await(cancelled) } catch caught { cat(caught.kind, "/", caught.message) }
let completed = spawn({ 7 })
emit await(completed)
emit cancel(completed)
`
	program, err := Parse("<native-structured-result>", source)
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewRuntime()
	interpreter, err := runtime.Run(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := runtime.RunVM(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := valuesOutput(vm), valuesOutput(interpreter); got != want {
		t.Fatalf("VM structured result output = %q, want %q", got, want)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"oerror(", "oresultfail(", "oresultbind(", "omember(", "ocancel("} {
		if !strings.Contains(artifact.Source, marker) {
			t.Fatalf("generated C omitted %q", marker)
		}
	}
	executable := filepath.Join(t.TempDir(), "structured-result")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatalf("compile executable: %v\n%s", err, artifact.Source)
	}
	output, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native structured result executable: %v\n%s", err, output)
	}
	if got := string(output); got != valuesOutput(interpreter) {
		t.Fatalf("native structured result output = %q, want %q", got, valuesOutput(interpreter))
	}
}

func TestCBackendRequiredTextUnicodeParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	const source = `import "std/text" as text
emit text.upper("café déjà vu")
emit text.fields("alpha βeta gamma")
emit text.slug("Crème Brûlée / Δelta")
`
	program, err := Parse("<native-text-unicode>", source)
	if err != nil {
		t.Fatal(err)
	}
	want, err := NewRuntime().Run(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "text-unicode")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatalf("compile executable: %v\n%s", err, artifact.Source)
	}
	output, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native text executable: %v\n%s", err, output)
	}
	if got := string(output); got != valuesOutput(want) {
		t.Fatalf("native Unicode text output = %q, want %q", got, valuesOutput(want))
	}
}

func TestCBackendRequiredTimeBoundaryParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	const source = `import "std/time" as time
let leap = time.date(2024, 2, 29, 23, 59, 59, 0, time.utc_location)
emit time.format(leap, time.rfc3339)
emit time.format(time.add(leap, time.parse_duration("2s")), time.rfc3339)
emit time.format(time.add_date(time.date(2024, 1, 31, 0, 0, 0, 0, time.utc_location), 0, 1, 0), time.rfc3339)
emit time.duration_string(time.sub(time.add(leap, time.parse_duration("49h2m3s")), leap))
`
	program, err := Parse("<native-time-boundaries>", source)
	if err != nil {
		t.Fatal(err)
	}
	want, err := NewRuntime().Run(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "time-boundaries")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatalf("compile executable: %v\n%s", err, artifact.Source)
	}
	output, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native time executable: %v\n%s", err, output)
	}
	if got := string(output); got != valuesOutput(want) {
		t.Fatalf("native time output = %q, want %q", got, valuesOutput(want))
	}
}

func TestCBackendRequiredModuleErrorParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	const source = `import "std/crypto" as crypto
import "std/list" as list
import "std/math" as math
import "std/signal" as signal
import "std/text" as text
import "std/time" as time
emit try { crypto.random("seed", -1) } catch err { "caught" }
emit try { list.range(-1) } catch err { "caught" }
emit try { math.mean([]) } catch err { "caught" }
emit try { math.std([]) } catch err { "caught" }
emit try { math.quantile([1], 2) } catch err { "caught" }
emit try { math.phase("bad") } catch err { "caught" }
emit try { signal.hann(-1) } catch err { "caught" }
emit try { signal.convolve([1], ["bad"]) } catch err { "caught" }
emit try { text.slug(1) } catch err { "caught" }
emit try { text.fields(1) } catch err { "caught" }
emit try { text.upper(1) } catch err { "caught" }
emit try { text.join(1, [1]) } catch err { "caught" }
emit try { time.date(2023, 2, 29, 0, 0, 0, 0, time.utc_location) } catch err { "caught" }
emit try { time.parse_duration("999999999999999999999h") } catch err { "caught" }
`
	program, err := Parse("<native-module-errors>", source)
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewRuntime()
	want, err := runtime.Run(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := runtime.RunVM(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if got := valuesOutput(vm); got != valuesOutput(want) {
		t.Fatalf("VM error output = %q, want %q", got, valuesOutput(want))
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "module-errors")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatalf("compile executable: %v\n%s", err, artifact.Source)
	}
	output, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native module-error executable: %v\n%s", err, output)
	}
	if got := string(output); got != valuesOutput(want) {
		t.Fatalf("native module error output = %q, want %q", got, valuesOutput(want))
	}
}

func TestCBackendCryptoVectorsParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	const source = `import "std/crypto" as crypto
emit crypto.random("seed", 0)
emit crypto.random(42, 3)
emit crypto.random([1u8, "x"], 4)
emit crypto.hash("seed", 42u32, [true, complex(2, -3)])
`
	program, err := Parse("<native-crypto-vectors>", source)
	if err != nil {
		t.Fatal(err)
	}
	want, err := NewRuntime().Run(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "crypto-vectors")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatalf("compile executable: %v\n%s", err, artifact.Source)
	}
	output, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native crypto executable: %v\n%s", err, output)
	}
	if got := string(output); got != valuesOutput(want) {
		t.Fatalf("native crypto vectors = %q, want %q", got, valuesOutput(want))
	}
}

func TestCBackendRequiredNativeSurfaceParity(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	const source = `import "std/crypto" as crypto
import "std/list" as list
import "std/math" as math
import "std/signal" as signal
import "std/text" as text
import "std/time" as time
emit crypto.random("surface", 2)
emit crypto.hash("surface", [1u8, 2u16, 3u32])
emit list.range(4)
emit [math.mean([1, 2, 3]), math.std([1, 2, 3]), math.quantile([1, 2, 3, 4], 0.25), math.phase(complex(0, 1))]
emit signal.hann(4)
emit signal.convolve([1, 2, 3], [1, 1])
emit [text.slug("Orbital Mission"), text.fields("a β c"), text.upper("café"), text.join("/", [1u8, "x"])]
let epoch = time.date(2032, 2, 29, 1, 2, 3, 4, time.utc_location)
let shifted = time.add_date(time.add(epoch, time.parse_duration("2h3m4s")), 1, 1, 1)
emit [time.format(shifted, time.rfc3339), time.duration_string(time.sub(shifted, epoch))]
emit cat("value/", 7u32, "/", complex(2, -3))
emit [rotl(1u8, 2u8), bxor(15u16, 3u16), abs(-4i32), uint(5), u64(6), f64(7)]
emit [dec("1845000000.25"), bigint("340282366920938463463374607431768211507")]
let? extracted = ok(9)
emit extracted
let archive = ["surface", epoch, [7u32, 1.25f64, complex(2.5f64, -3.0f64)]]
emit decode(encode(archive))
emit [len(archive), at(archive, 0)]
let left = matrix([[1, 2], [3, 4]])
let right = transpose(left)
emit [matmul(left, right), trace(left), det(left), norm(left)]
let delayed = spawn({ kilo * meter })
emit await(delayed)
emit parallel({ mega }, { 2 })
`
	program, err := Parse("<native-required-surface>", source)
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewRuntime()
	interpreter, err := runtime.Run(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := runtime.RunVM(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := valuesOutput(vm), valuesOutput(interpreter); got != want {
		t.Fatalf("VM required-surface output = %q, want %q", got, want)
	}
	artifact, err := CompileC(context.Background(), NewRuntime(), program)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "required-native-surface")
	if err := CompileCExecutable(context.Background(), artifact.Source, executable); err != nil {
		t.Fatalf("compile executable: %v\n%s", err, artifact.Source)
	}
	output, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native required-surface executable: %v\n%s", err, output)
	}
	if got := string(output); got != valuesOutput(interpreter) {
		t.Fatalf("native required-surface output = %q, want %q", got, valuesOutput(interpreter))
	}
}

func nativeBytesExpr(raw []byte) string {
	var out strings.Builder
	out.WriteString("bytes([")
	for i, item := range raw {
		if i > 0 {
			out.WriteString(", ")
		}
		fmt.Fprintf(&out, "%du8", item)
	}
	out.WriteString("])")
	return out.String()
}
