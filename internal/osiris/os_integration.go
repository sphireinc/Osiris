package osiris

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	osWatchQueueLimit = 64
)

type osProcessProvider func(context.Context, string, []string, Value) (*osProcess, error)
type osSnapshotProvider func(string, bool) (fileSnapshot, error)

type osProcess struct {
	mu         sync.Mutex
	command    *exec.Cmd
	stdin      *streamValue
	stdout     *streamValue
	stderr     *streamValue
	done       chan struct{}
	waitOnce   sync.Once
	waitErr    error
	closed     bool
	waitFn     func() error
	killFn     func() error
	pidValue   int
	exitCode   int
	exitOK     bool
	exitSignal string
}

func (p *osProcess) pid() int {
	if p == nil {
		return 0
	}
	if p.command == nil || p.command.Process == nil {
		return p.pidValue
	}
	return p.command.Process.Pid
}

func (p *osProcess) isClosed() bool {
	if p == nil {
		return true
	}
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	return closed
}

func (p *osProcess) waitDone() {
	p.waitOnce.Do(func() {
		if p.waitFn != nil {
			p.waitErr = p.waitFn()
		} else if p.command != nil {
			p.waitErr = p.command.Wait()
		}
		close(p.done)
	})
}

func (p *osProcess) kill() error {
	if p == nil {
		return fmt.Errorf("process is not running")
	}
	select {
	case <-p.done:
		return nil
	default:
	}
	if p.killFn != nil {
		return p.killFn()
	}
	if p.command == nil || p.command.Process == nil {
		return fmt.Errorf("process is not running")
	}
	return terminateProcessTree(p.command)
}

func (p *osProcess) close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()
	select {
	case <-p.done:
	default:
		_ = p.kill()
	}
	if p.stdin != nil {
		_, _ = p.stdin.close()
	}
	if p.stdout != nil {
		_, _ = p.stdout.close()
	}
	if p.stderr != nil {
		_, _ = p.stderr.close()
	}
	return nil
}

func osFailure(kind ErrorKind, operation string, host error) error {
	record := newOsirisError(kind, operation+" failed").
		withMetadata("operation", stringValue(operation)).
		withMetadata("component", stringValue("os"))
	if host != nil {
		record = withNativeErrorCode(record, host).
			withDebugMetadata("host_error", stringValue(host.Error()))
	}
	return record
}

func osContextError(ctx context.Context, operation string) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		kind := ErrorKindCancelled
		if errors.Is(err, context.DeadlineExceeded) {
			kind = ErrorKindTimeout
		}
		return osFailure(kind, operation, err)
	}
	return nil
}

func requireOSCapability(rt *Runtime, capability, operation string) error {
	if rt == nil || !rt.hasCapability(capability) {
		return osFailure(ErrorKindPermissionDenied, operation, fmt.Errorf("capability %q is not granted", capability))
	}
	return nil
}

func (r *Runtime) SetCommandLineArgs(args []string) {
	if r == nil {
		return
	}
	r.osMu.Lock()
	r.injectedArgs = make([]string, len(args))
	copy(r.injectedArgs, args)
	r.osMu.Unlock()
	if r.stdOSTest != nil {
		r.stdOSTest.mu.Lock()
		r.stdOSTest.args = append([]string(nil), args...)
		r.stdOSTest.mu.Unlock()
	}
}

func (r *Runtime) SetEnvironment(values map[string]string) {
	if r == nil {
		return
	}
	r.osMu.Lock()
	r.injectedEnv = make(map[string]string, len(values))
	for key, value := range values {
		r.injectedEnv[key] = value
	}
	r.environmentSet = true
	r.osMu.Unlock()
	if r.stdOSTest != nil {
		r.stdOSTest.mu.Lock()
		r.stdOSTest.env = make(map[string]string, len(values))
		for key, value := range values {
			r.stdOSTest.env[key] = value
		}
		r.stdOSTest.mu.Unlock()
	}
}

// setOSProcessProvider and setOSSnapshotProvider are deterministic host-test
// seams. Production runtimes leave them nil and use the platform adapters;
// tests can supply in-memory process and filesystem state without relying on
// ambient executables or filesystem timing.
func (r *Runtime) setOSProcessProvider(provider osProcessProvider) {
	if r == nil {
		return
	}
	r.osMu.Lock()
	r.processProvider = provider
	r.osMu.Unlock()
}

func (r *Runtime) setOSSnapshotProvider(provider osSnapshotProvider) {
	if r == nil {
		return
	}
	r.osMu.Lock()
	r.snapshotProvider = provider
	r.osMu.Unlock()
}

func (r *Runtime) osProcessProvider() osProcessProvider {
	if r == nil {
		return nil
	}
	r.osMu.RLock()
	provider := r.processProvider
	r.osMu.RUnlock()
	return provider
}

func (r *Runtime) osSnapshotProvider() osSnapshotProvider {
	if r == nil {
		return nil
	}
	r.osMu.RLock()
	provider := r.snapshotProvider
	r.osMu.RUnlock()
	return provider
}

func osArgsValue(rt *Runtime, _ context.Context, args []Value) (Value, error) {
	if len(args) != 0 {
		return nullValue(), osFailure(ErrorKindInvalidArgument, "os.args", fmt.Errorf("args expects no arguments"))
	}
	if rt == nil {
		return stringListValue(os.Args[1:]), nil
	}
	if rt.stdOSTest != nil {
		rt.osMu.RLock()
		values := append([]string(nil), rt.injectedArgs...)
		rt.osMu.RUnlock()
		return stringListValue(values), nil
	}
	rt.osMu.RLock()
	values := append([]string(nil), rt.injectedArgs...)
	injected := rt.injectedArgs != nil
	rt.osMu.RUnlock()
	if !injected {
		values = os.Args[1:]
	}
	return stringListValue(values), nil
}

func stringListValue(values []string) Value {
	out := make([]Value, len(values))
	for i, value := range values {
		out[i] = stringValue(value)
	}
	return listValue(out)
}

func osEnvValue(rt *Runtime, _ context.Context, args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindString || args[0].str == "" || strings.ContainsAny(args[0].str, "=\x00") {
		return nullValue(), osFailure(ErrorKindInvalidArgument, "os.env", fmt.Errorf("env expects one valid variable name"))
	}
	if err := requireOSCapability(rt, "os.environment", "os.env"); err != nil {
		return nullValue(), err
	}
	name := args[0].str
	if rt != nil && rt.stdOSTest != nil {
		rt.stdOSTest.mu.RLock()
		value, ok := rt.stdOSTest.env[name]
		rt.stdOSTest.mu.RUnlock()
		if !ok {
			return noneBuiltin(nil)
		}
		return someBuiltin([]Value{stringValue(value)})
	}
	var value string
	var ok bool
	injected := false
	if rt != nil {
		rt.osMu.RLock()
		if rt.environmentSet {
			injected = true
			value, ok = rt.injectedEnv[name]
		}
		rt.osMu.RUnlock()
	}
	if !injected && !ok {
		value, ok = os.LookupEnv(name)
	}
	if !ok {
		return noneBuiltin(nil)
	}
	return someBuiltin([]Value{stringValue(value)})
}

func osEnvNamesValue(rt *Runtime, _ context.Context, args []Value) (Value, error) {
	if len(args) != 0 {
		return nullValue(), osFailure(ErrorKindInvalidArgument, "os.env_names", fmt.Errorf("env_names expects no arguments"))
	}
	if err := requireOSCapability(rt, "os.environment", "os.env_names"); err != nil {
		return nullValue(), err
	}
	values := map[string]string{}
	if rt != nil && rt.stdOSTest != nil {
		rt.stdOSTest.mu.RLock()
		for key := range rt.stdOSTest.env {
			values[key] = ""
		}
		rt.stdOSTest.mu.RUnlock()
		names := make([]string, 0, len(values))
		for key := range values {
			names = append(names, key)
		}
		sort.Strings(names)
		return stringListValue(names), nil
	}
	if rt != nil {
		rt.osMu.RLock()
		if rt.environmentSet {
			for key, value := range rt.injectedEnv {
				values[key] = value
			}
		}
		rt.osMu.RUnlock()
	}
	if rt == nil || !rt.environmentIsInjected() {
		for _, entry := range os.Environ() {
			if key, _, found := strings.Cut(entry, "="); found {
				values[key] = ""
			}
		}
	}
	names := make([]string, 0, len(values))
	for key := range values {
		names = append(names, key)
	}
	sort.Strings(names)
	return stringListValue(names), nil
}

func (r *Runtime) environmentIsInjected() bool {
	if r == nil {
		return false
	}
	r.osMu.RLock()
	injected := r.environmentSet
	r.osMu.RUnlock()
	return injected
}

func mapFieldValue(value Value, name string) (Value, bool) {
	if value.kind != kindMap || value.mapValue == nil {
		return nullValue(), false
	}
	for _, entry := range value.mapValue.entries {
		if entry.key.kind == kindString && entry.key.str == name {
			return entry.value, true
		}
	}
	return nullValue(), false
}

func stringArgumentList(value Value, operation string) ([]string, error) {
	if value.kind != kindList {
		return nil, osFailure(ErrorKindType, operation, fmt.Errorf("arguments must be a list of strings"))
	}
	values := make([]string, len(value.list))
	for i, item := range value.list {
		if item.kind != kindString || strings.IndexByte(item.str, 0) >= 0 {
			return nil, osFailure(ErrorKindInvalidArgument, operation, fmt.Errorf("arguments must contain only NUL-free strings"))
		}
		values[i] = item.str
	}
	return values, nil
}

func processOptions(value Value, operation string) (cwd string, env []string, inherit bool, err error) {
	if value.kind == kindNull {
		return "", nil, false, nil
	}
	if value.kind != kindMap {
		return "", nil, false, osFailure(ErrorKindType, operation, fmt.Errorf("options must be a map"))
	}
	if field, ok := mapFieldValue(value, "cwd"); ok {
		if field.kind != kindString || field.str == "" || strings.IndexByte(field.str, 0) >= 0 {
			return "", nil, false, osFailure(ErrorKindInvalidArgument, operation, fmt.Errorf("cwd must be a non-empty NUL-free path"))
		}
		cwd = field.str
	}
	if field, ok := mapFieldValue(value, "inherit_env"); ok {
		if field.kind != kindBool {
			return "", nil, false, osFailure(ErrorKindType, operation, fmt.Errorf("inherit_env must be bool"))
		}
		inherit = field.boo
	}
	if field, ok := mapFieldValue(value, "env"); ok {
		if field.kind != kindMap || field.mapValue == nil {
			return "", nil, false, osFailure(ErrorKindType, operation, fmt.Errorf("env must be a map"))
		}
		env = make([]string, 0, len(field.mapValue.entries))
		keys := make([]string, 0, len(field.mapValue.entries))
		values := make(map[string]string, len(field.mapValue.entries))
		for _, entry := range field.mapValue.entries {
			if entry.key.kind != kindString || entry.value.kind != kindString || strings.ContainsAny(entry.key.str, "=\x00") || strings.IndexByte(entry.value.str, 0) >= 0 {
				return "", nil, false, osFailure(ErrorKindInvalidArgument, operation, fmt.Errorf("env keys and values must be valid strings"))
			}
			keys = append(keys, entry.key.str)
			values[entry.key.str] = entry.value.str
		}
		sort.Strings(keys)
		for _, key := range keys {
			env = append(env, key+"="+values[key])
		}
	}
	return cwd, env, inherit, nil
}

func processSpawnContextValue(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(args) < 2 || len(args) > 3 {
		return nullValue(), osFailure(ErrorKindInvalidArgument, "os.process_spawn", fmt.Errorf("process_spawn expects command, arguments, and optional options"))
	}
	if err := requireOSCapability(rt, "os.process", "os.process_spawn"); err != nil {
		return nullValue(), err
	}
	if err := osContextError(ctx, "os.process_spawn"); err != nil {
		return nullValue(), err
	}
	if args[0].kind != kindString || args[0].str == "" || strings.IndexByte(args[0].str, 0) >= 0 {
		return nullValue(), osFailure(ErrorKindInvalidArgument, "os.process_spawn", fmt.Errorf("command must be a non-empty NUL-free string"))
	}
	commandArgs, err := stringArgumentList(args[1], "os.process_spawn")
	if err != nil {
		return nullValue(), err
	}
	var options Value
	if len(args) == 3 {
		options = args[2]
	} else {
		options = nullValue()
	}
	cwd, env, inherit, err := processOptions(options, "os.process_spawn")
	if err != nil {
		return nullValue(), err
	}
	if provider := rt.osProcessProvider(); provider != nil {
		process, providerErr := provider(ctx, args[0].str, commandArgs, options)
		if providerErr != nil {
			return nullValue(), osFailure(ErrorKindUnavailable, "os.process_spawn", providerErr)
		}
		if process == nil {
			return nullValue(), osFailure(ErrorKindUnavailable, "os.process_spawn", fmt.Errorf("process provider returned nil"))
		}
		if process.done == nil {
			process.done = make(chan struct{})
		}
		go process.waitDone()
		go process.cancelOnContext(ctx)
		return processSpawnValue(process), nil
	}
	// CommandContext only terminates the direct child on cancellation. The
	// process watcher below uses the platform adapter so cancellation also
	// reaches the supported process group.
	command := exec.Command(args[0].str, commandArgs...)
	command.Stdin = nil
	stdin, err := command.StdinPipe()
	if err != nil {
		return nullValue(), osFailure(ErrorKindIO, "os.process_spawn", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nullValue(), osFailure(ErrorKindIO, "os.process_spawn", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nullValue(), osFailure(ErrorKindIO, "os.process_spawn", err)
	}
	command.Dir = cwd
	if env != nil || !inherit {
		command.Env = env
	}
	configureProcessTree(command)
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nullValue(), osFailure(ErrorKindUnavailable, "os.process_spawn", err)
	}
	process := &osProcess{command: command, done: make(chan struct{})}
	process.stdin = &streamValue{id: fmt.Sprintf("process:%d:stdin", command.Process.Pid), mode: "process_stdin", writer: bufio.NewWriter(stdin), closer: stdin}
	process.stdout = &streamValue{id: fmt.Sprintf("process:%d:stdout", command.Process.Pid), mode: "process_stdout", reader: bufio.NewReader(stdout), closer: stdout}
	process.stderr = &streamValue{id: fmt.Sprintf("process:%d:stderr", command.Process.Pid), mode: "process_stderr", reader: bufio.NewReader(stderr), closer: stderr}
	go process.waitDone()
	go process.cancelOnContext(ctx)
	return processSpawnValue(process), nil
}

func processSpawnValue(process *osProcess) Value {
	return mapValueOf([]mapEntry{
		{key: stringValue("process"), value: osProcessValueOf(process)},
		{key: stringValue("pid"), value: intValue(big.NewInt(int64(process.pid())), 64)},
		{key: stringValue("stdin"), value: streamValueOf(process.stdin)},
		{key: stringValue("stdout"), value: streamValueOf(process.stdout)},
		{key: stringValue("stderr"), value: streamValueOf(process.stderr)},
	})
}

func (p *osProcess) cancelOnContext(ctx context.Context) {
	if ctx == nil {
		return
	}
	select {
	case <-ctx.Done():
		select {
		case <-p.done:
		default:
			_ = p.kill()
		}
	case <-p.done:
	}
}

func processResultValue(process *osProcess) Value {
	status := process.exitCode
	success := process.exitOK
	signalName := process.exitSignal
	if process.command != nil && process.command.ProcessState != nil {
		status = process.command.ProcessState.ExitCode()
		success = process.command.ProcessState.Success()
		signalName = processSignalName(process.waitErr)
	}
	return mapValueOf([]mapEntry{
		{key: stringValue("status"), value: intValue(big.NewInt(int64(status)), 64)},
		{key: stringValue("success"), value: boolValue(success)},
		{key: stringValue("signal"), value: stringValue(signalName)},
	})
}

func processWaitContextValue(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(args) < 1 || len(args) > 2 || args[0].kind != kindOSProcess || args[0].process == nil {
		return nullValue(), osFailure(ErrorKindType, "os.process_wait", fmt.Errorf("process_wait expects a process and optional timeout"))
	}
	if err := requireOSCapability(rt, "os.process", "os.process_wait"); err != nil {
		return nullValue(), err
	}
	process := args[0].process
	waitCtx := ctx
	var cancel context.CancelFunc
	if len(args) == 2 {
		timeout, err := filesystemNonnegativeArgument(args[1], "os.process_wait", "timeout_ms", uint64(^uint(0)>>1))
		if err != nil {
			return nullValue(), err
		}
		if timeout > 0 {
			waitCtx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
			defer cancel()
		}
	}
	select {
	case <-process.done:
		return processResultValue(process), nil
	case <-waitCtx.Done():
		_ = process.kill()
		<-process.done
		if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
			return nullValue(), osFailure(ErrorKindTimeout, "os.process_wait", waitCtx.Err())
		}
		return nullValue(), osFailure(ErrorKindCancelled, "os.process_wait", waitCtx.Err())
	}
}

func processKillContextValue(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindOSProcess || args[0].process == nil {
		return nullValue(), osFailure(ErrorKindType, "os.process_kill", fmt.Errorf("process_kill expects a process"))
	}
	if err := requireOSCapability(rt, "os.process", "os.process_kill"); err != nil {
		return nullValue(), err
	}
	if err := osContextError(ctx, "os.process_kill"); err != nil {
		return nullValue(), err
	}
	if err := args[0].process.kill(); err != nil {
		return nullValue(), osFailure(ErrorKindIO, "os.process_kill", err)
	}
	return boolValue(true), nil
}

type osEventReader struct {
	mu     sync.Mutex
	queue  <-chan string
	closer func()
	closed bool
	buffer []byte
}

func (r *osEventReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	if len(r.buffer) > 0 {
		n := copy(p, r.buffer)
		r.buffer = r.buffer[n:]
		r.mu.Unlock()
		return n, nil
	}
	if r.closed {
		r.mu.Unlock()
		return 0, io.EOF
	}
	r.mu.Unlock()
	line, ok := <-r.queue
	if !ok {
		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()
		return 0, io.EOF
	}
	data := []byte(line + "\n")
	n := copy(p, data)
	if n < len(data) {
		r.mu.Lock()
		r.buffer = append(r.buffer, data[n:]...)
		r.mu.Unlock()
	}
	return n, nil
}

func eventStreamValue(id string, events <-chan string, closeFn func()) Value {
	reader := &osEventReader{queue: events, closer: closeFn}
	return streamValueOf(&streamValue{id: id, path: id, mode: "os_event", reader: bufio.NewReader(reader), closer: reader})
}

func (r *osEventReader) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.mu.Unlock()
	if r.closer != nil {
		r.closer()
	}
	return nil
}

func osSignalContextValue(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(args) != 1 || args[0].kind != kindString {
		return nullValue(), osFailure(ErrorKindInvalidArgument, "os.signal_subscribe", fmt.Errorf("signal_subscribe expects a signal name"))
	}
	if err := requireOSCapability(rt, "os.signal", "os.signal_subscribe"); err != nil {
		return nullValue(), err
	}
	sig, name, err := signalForName(args[0].str)
	if err != nil {
		return nullValue(), osFailure(ErrorKindInvalidArgument, "os.signal_subscribe", err)
	}
	if err := osContextError(ctx, "os.signal_subscribe"); err != nil {
		return nullValue(), err
	}
	queue := make(chan string, 8)
	stop := make(chan struct{})
	var once sync.Once
	signals := make(chan os.Signal, 8)
	signal.Notify(signals, sig)
	go func() {
		defer close(queue)
		defer signal.Stop(signals)
		for {
			select {
			case received := <-signals:
				select {
				case queue <- name:
				default:
				}
				_ = received
			case <-ctx.Done():
				return
			case <-stop:
				return
			}
		}
	}()
	return eventStreamValue("signal:"+name, queue, func() { once.Do(func() { close(stop) }) }), nil
}

type fileSnapshot map[string]os.FileInfo

func snapshotPath(root string, recursive bool) (fileSnapshot, error) {
	result := fileSnapshot{}
	info, err := os.Stat(root)
	if err != nil {
		return result, err
	}
	if !info.IsDir() {
		result[root] = info
		return result, nil
	}
	if !recursive {
		entries, readErr := os.ReadDir(root)
		if readErr != nil {
			return result, readErr
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			path := filepath.Clean(filepath.Join(root, entry.Name()))
			entryInfo, infoErr := entry.Info()
			if infoErr != nil {
				return result, infoErr
			}
			result[path] = entryInfo
		}
		return result, nil
	}
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || !info.Mode().IsRegular() {
			return nil
		}
		result[filepath.Clean(path)] = info
		return nil
	})
	return result, err
}

func snapshotPathForRuntime(rt *Runtime, root string, recursive bool) (fileSnapshot, error) {
	if provider := rt.osSnapshotProvider(); provider != nil {
		return provider(root, recursive)
	}
	return snapshotPath(root, recursive)
}

func watchEvent(kind, path string, info os.FileInfo) string {
	event := map[string]any{"kind": kind, "path": filepath.ToSlash(filepath.Clean(path))}
	if info != nil {
		event["is_dir"] = info.IsDir()
	}
	data, _ := json.Marshal(event)
	return string(data)
}

func enqueueWatchEvent(queue chan string, overflowed *bool, event, overflowEvent string) {
	select {
	case queue <- event:
		return
	default:
	}
	if *overflowed {
		return
	}
	// Preserve one slot for the rescan marker. Dropping an arbitrary queued
	// event is intentional: once overflow is reported, the consumer must
	// rescan the watched path instead of treating the remaining delta as a
	// complete history.
	select {
	case <-queue:
	default:
	}
	*overflowed = true
	select {
	case queue <- overflowEvent:
	default:
	}
}

func osWatchContextValue(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(args) < 1 || len(args) > 2 || args[0].kind != kindString || args[0].str == "" {
		return nullValue(), osFailure(ErrorKindInvalidArgument, "os.watch", fmt.Errorf("watch expects path and optional recursive bool"))
	}
	if err := requireOSCapability(rt, "os.watch", "os.watch"); err != nil {
		return nullValue(), err
	}
	recursive := true
	if len(args) == 2 {
		if args[1].kind != kindBool {
			return nullValue(), osFailure(ErrorKindType, "os.watch", fmt.Errorf("recursive must be bool"))
		}
		recursive = args[1].boo
	}
	root, err := filepath.Abs(filepath.Clean(args[0].str))
	if err != nil {
		return nullValue(), osFailure(ErrorKindInvalidArgument, "os.watch", err)
	}
	initial, err := snapshotPathForRuntime(rt, root, recursive)
	if err != nil {
		return nullValue(), osFailure(filesystemErrorKind(err), "os.watch", err)
	}
	queue := make(chan string, osWatchQueueLimit)
	stop := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(queue)
		previous := initial
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		overflowed := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				current, scanErr := snapshotPathForRuntime(rt, root, recursive)
				if scanErr != nil {
					enqueueWatchEvent(queue, &overflowed, watchEvent("overflow", root, nil), watchEvent("overflow", root, nil))
					continue
				}
				for path, info := range current {
					if old, ok := previous[path]; !ok {
						enqueueWatchEvent(queue, &overflowed, watchEvent("create", path, info), watchEvent("overflow", root, nil))
					} else if old.ModTime() != info.ModTime() || old.Size() != info.Size() || old.Mode() != info.Mode() {
						enqueueWatchEvent(queue, &overflowed, watchEvent("modify", path, info), watchEvent("overflow", root, nil))
					}
				}
				for path, info := range previous {
					if _, ok := current[path]; !ok {
						enqueueWatchEvent(queue, &overflowed, watchEvent("remove", path, info), watchEvent("overflow", root, nil))
					}
				}
				previous = current
			}
		}
	}()
	return eventStreamValue("watch:"+root, queue, func() { once.Do(func() { close(stop) }) }), nil
}
