package osiris

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestOSArgsAndEnvironmentUseDeterministicInjectionAndCapabilities(t *testing.T) {
	rt := NewRuntime()
	rt.SetCommandLineArgs([]string{"first", "second"})
	rt.SetEnvironment(map[string]string{"TOKEN": "redacted-value", "A": "1"})
	args, err := osArgsValue(rt, context.Background(), nil)
	if err != nil || args.String() != `["first", "second"]` {
		t.Fatalf("args = %s, err = %v", args.String(), err)
	}
	if _, err := osEnvValue(rt, context.Background(), []Value{stringValue("TOKEN")}); err == nil {
		t.Fatal("environment access succeeded without capability")
	}
	rt.GrantCapability("os.environment")
	value, err := osEnvValue(rt, context.Background(), []Value{stringValue("TOKEN")})
	if err != nil || value.kind != kindOption || value.opt == nil || !value.opt.present || value.opt.val.str != "redacted-value" {
		t.Fatalf("env value = %s, err = %v", value.String(), err)
	}
	names, err := osEnvNamesValue(rt, context.Background(), nil)
	if err != nil || names.String() != `["A", "TOKEN"]` {
		t.Fatalf("env names = %s, err = %v", names.String(), err)
	}
}

func TestOSInjectedEmptyInputsDoNotFallBackToHostState(t *testing.T) {
	rt := NewRuntime()
	rt.SetCommandLineArgs([]string{})
	rt.SetEnvironment(map[string]string{})
	rt.GrantCapability("os.environment")
	args, err := osArgsValue(rt, context.Background(), nil)
	if err != nil || args.String() != "[]" {
		t.Fatalf("empty args = %s, err = %v", args.String(), err)
	}
	names, err := osEnvNamesValue(rt, context.Background(), nil)
	if err != nil || names.String() != "[]" {
		t.Fatalf("empty environment names = %s, err = %v", names.String(), err)
	}
	value, err := osEnvValue(rt, context.Background(), []Value{stringValue("PATH")})
	if err != nil || value.String() != "none" {
		t.Fatalf("empty environment PATH = %s, err = %v", value.String(), err)
	}
}

func TestOSProcessIsShellFreeBoundedAndCancellable(t *testing.T) {
	rt := NewRuntime()
	rt.GrantCapability("os.process")
	started, err := processSpawnContextValue(rt, context.Background(), []Value{
		stringValue("/bin/echo"),
		listValue([]Value{stringValue("hello")}),
		mapValueOf([]mapEntry{{key: stringValue("env"), value: mapValueOf([]mapEntry{{key: stringValue("ONLY"), value: stringValue("yes")}})}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	processValue, err := mapGetValue([]Value{started, stringValue("process")})
	if err != nil {
		t.Fatal(err)
	}
	processValue, err = unwrapOption(processValue)
	if err != nil {
		t.Fatal(err)
	}
	stdoutValue, err := mapGetValue([]Value{started, stringValue("stdout")})
	if err != nil {
		t.Fatal(err)
	}
	stdoutValue, err = unwrapOption(stdoutValue)
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := stdoutValue.AsStream()
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := stdout.readChunk(64)
	if err != nil {
		t.Fatal(err)
	}
	data, err := bytesFromValue(chunk)
	if err != nil || string(data) != "hello\n" {
		t.Fatalf("stdout = %q, err = %v", data, err)
	}
	result, err := processWaitContextValue(rt, context.Background(), []Value{processValue, uintValue(1000, 64)})
	if err != nil {
		t.Fatal(err)
	}
	status, err := mapGetValue([]Value{result, stringValue("status")})
	statusInt, statusErr := status.Int()
	if err != nil {
		t.Fatal(err)
	}
	status, err = unwrapOption(status)
	if err != nil {
		t.Fatal(err)
	}
	statusInt, statusErr = status.Int()
	if statusErr != nil || statusInt != 0 {
		t.Fatalf("status = %s, err = %v", status.String(), err)
	}
	_, _ = stdout.close()
	_ = processValue.process.close()

	started, err = processSpawnContextValue(rt, context.Background(), []Value{
		stringValue("/bin/sh"), listValue([]Value{stringValue("-c"), stringValue("sleep 2")}), nullValue(),
	})
	if err != nil {
		t.Fatal(err)
	}
	processValue, err = mapGetValue([]Value{started, stringValue("process")})
	if err != nil {
		t.Fatal(err)
	}
	processValue, err = unwrapOption(processValue)
	if err != nil {
		t.Fatal(err)
	}
	_, err = processWaitContextValue(rt, context.Background(), []Value{processValue, uintValue(20, 64)})
	var record *OsirisError
	if !errors.As(err, &record) || record.kind != ErrorKindTimeout {
		t.Fatalf("timeout error = %T %v", err, err)
	}
}

func TestOSDeterministicFakeProcessAndWatchProviders(t *testing.T) {
	rt := NewRuntime()
	rt.GrantCapability("os.process")
	rt.GrantCapability("os.watch")
	var commandName string
	rt.setOSProcessProvider(func(_ context.Context, command string, args []string, _ Value) (*osProcess, error) {
		commandName = command + " " + strings.Join(args, " ")
		process := &osProcess{pidValue: 4242, done: make(chan struct{})}
		process.stdin = &streamValue{id: "fake-stdin", mode: "process_stdin", writer: bufio.NewWriter(io.Discard), closer: io.NopCloser(strings.NewReader(""))}
		process.stdout = &streamValue{id: "fake-stdout", mode: "process_stdout", reader: bufio.NewReader(strings.NewReader("fake output\n")), closer: io.NopCloser(strings.NewReader(""))}
		process.stderr = &streamValue{id: "fake-stderr", mode: "process_stderr", reader: bufio.NewReader(strings.NewReader("")), closer: io.NopCloser(strings.NewReader(""))}
		process.waitFn = func() error {
			process.exitCode = 0
			process.exitOK = true
			return nil
		}
		return process, nil
	})
	started, err := processSpawnContextValue(rt, context.Background(), []Value{stringValue("fake-command"), listValue([]Value{stringValue("one"), stringValue("two")})})
	if err != nil {
		t.Fatal(err)
	}
	if commandName != "fake-command one two" {
		t.Fatalf("provider received command %q", commandName)
	}
	stdout, err := mapGetValue([]Value{started, stringValue("stdout")})
	if err != nil {
		t.Fatal(err)
	}
	stdout, err = unwrapOption(stdout)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := stdout.AsStream()
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := stream.readChunk(64)
	if err != nil {
		t.Fatal(err)
	}
	if data, err := bytesFromValue(chunk); err != nil || string(data) != "fake output\n" {
		t.Fatalf("fake stdout = %q, err = %v", data, err)
	}
	processValue, err := mapGetValue([]Value{started, stringValue("process")})
	if err != nil {
		t.Fatal(err)
	}
	processValue, err = unwrapOption(processValue)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := processWaitContextValue(rt, context.Background(), []Value{processValue}); err != nil {
		t.Fatal(err)
	}

	directory := t.TempDir()
	path := filepath.Join(directory, "fake.txt")
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	_ = info
	var scans int
	rt.setOSSnapshotProvider(func(root string, recursive bool) (fileSnapshot, error) {
		scans++
		if scans == 1 {
			return fileSnapshot{}, nil
		}
		return fileSnapshot{path: info}, nil
	})
	watchValue, err := osWatchContextValue(rt, context.Background(), []Value{stringValue(directory), boolValue(false)})
	if err != nil {
		t.Fatal(err)
	}
	watch, err := watchValue.AsStream()
	if err != nil {
		t.Fatal(err)
	}
	defer watch.close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	line, err := watch.readLineContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line.str, `"kind":"create"`) || !strings.Contains(line.str, "fake.txt") {
		t.Fatalf("fake watch event = %q", line.str)
	}
}

func TestOSSignalSubscriptionIsCapabilityGatedAndBounded(t *testing.T) {
	rt := NewRuntime()
	if _, err := osSignalContextValue(rt, context.Background(), []Value{stringValue("SIGINT")}); err == nil || !strings.Contains(err.Error(), "permission_denied") {
		t.Fatalf("signal denial = %v", err)
	}
	rt.GrantCapability("os.signal")
	if _, err := osSignalContextValue(rt, context.Background(), []Value{stringValue("SIGUNKNOWN")}); err == nil || !strings.Contains(err.Error(), "invalid_argument") {
		t.Fatalf("invalid signal = %v", err)
	}
	streamValue, err := osSignalContextValue(rt, context.Background(), []Value{stringValue("SIGINT")})
	if err != nil {
		t.Skipf("SIGINT is unavailable on this platform: %v", err)
	}
	stream, err := streamValue.AsStream()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.close(); err != nil {
		t.Fatal(err)
	}
}

func TestOSWatchNormalizesEventsAndCloses(t *testing.T) {
	rt := NewRuntime()
	rt.GrantCapability("os.watch")
	directory := t.TempDir()
	watchValue, err := osWatchContextValue(rt, context.Background(), []Value{stringValue(directory), boolValue(false)})
	if err != nil {
		t.Fatal(err)
	}
	watch, err := watchValue.AsStream()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "fixture.txt")
	if err := os.WriteFile(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	line, err := watch.readLineContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line.str, `"kind":"create"`) || !strings.Contains(line.str, "fixture.txt") || strings.Contains(line.str, "\\") {
		t.Fatalf("watch event = %q, want normalized create path", line.str)
	}
	if _, err := watch.close(); err != nil {
		t.Fatal(err)
	}
}

func TestOSWatchReportsOverflowForAnUnconsumedBurst(t *testing.T) {
	rt := NewRuntime()
	rt.GrantCapability("os.watch")
	directory := t.TempDir()
	watchValue, err := osWatchContextValue(rt, context.Background(), []Value{stringValue(directory), boolValue(false)})
	if err != nil {
		t.Fatal(err)
	}
	watch, err := watchValue.AsStream()
	if err != nil {
		t.Fatal(err)
	}
	defer watch.close()
	for i := 0; i < osWatchQueueLimit*2; i++ {
		path := filepath.Join(directory, "burst-"+strconv.Itoa(i)+".txt")
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for {
		line, readErr := watch.readLineContext(ctx)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(line.str, `"kind":"overflow"`) {
			return
		}
	}
}

func TestOSCapabilityDenialDoesNotExposeSecrets(t *testing.T) {
	rt := NewRuntime()
	rt.SetEnvironment(map[string]string{"SECRET": "do-not-print"})
	_, err := osEnvValue(rt, context.Background(), []Value{stringValue("SECRET")})
	if err == nil || strings.Contains(err.Error(), "do-not-print") || !strings.Contains(err.Error(), "permission_denied") {
		t.Fatalf("denial = %v, want redacted permission error", err)
	}
	_, err = processSpawnContextValue(rt, context.Background(), []Value{stringValue("/bin/echo"), listValue(nil)})
	if err == nil || !strings.Contains(err.Error(), "permission_denied") {
		t.Fatalf("process denial = %v", err)
	}
}

func TestNativeRejectsHostedOSWithSourceSpan(t *testing.T) {
	program, err := Parse("os.osr", "import \"std/os\" as os\nemit os.args()\n")
	if err != nil {
		t.Fatal(err)
	}
	_, err = CompileC(context.Background(), NewRuntime(), program)
	if err == nil || !strings.Contains(err.Error(), "os.osr:2:9: native C backend: static module member") {
		t.Fatalf("error = %v, want source-spanned native std/os diagnostic", err)
	}
}
