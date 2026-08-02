package osiris

// This file is the capability-bounded std/os surface.  The host adapter uses
// the platform's os package; NewTestRuntime installs the in-memory adapter in
// the same call path so language tests do not need to provide Go mocks.

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const stdOSMaxRead = 1 << 20

type stdOSFileEntry struct {
	data    []byte
	mode    os.FileMode
	modTime time.Time
	isDir   bool
	target  string
}

type stdOSTestState struct {
	mu       sync.RWMutex
	files    map[string]*stdOSFileEntry
	env      map[string]string
	args     []string
	nextPID  int
	hostname string
	identity map[string]Value
	platform map[string]Value
}

type stdOSNoopCloser struct{}

func (stdOSNoopCloser) Close() error { return nil }

func stdOSFakeProcessProvider(rt *Runtime) osProcessProvider {
	return func(_ context.Context, _ string, _ []string, _ Value) (*osProcess, error) {
		rt.stdOSTest.mu.Lock()
		pid := rt.stdOSTest.nextPID
		rt.stdOSTest.nextPID++
		rt.stdOSTest.mu.Unlock()
		process := &osProcess{
			pidValue: pid,
			exitCode: 0,
			exitOK:   true,
			done:     make(chan struct{}),
			waitFn:   func() error { return nil },
			killFn:   func() error { return nil },
		}
		process.stdin = &streamValue{id: fmt.Sprintf("test-process:%d:stdin", pid), mode: "process_stdin", writer: bufio.NewWriter(io.Discard), closer: stdOSNoopCloser{}}
		process.stdout = &streamValue{id: fmt.Sprintf("test-process:%d:stdout", pid), mode: "process_stdout", reader: bufio.NewReader(strings.NewReader("")), closer: stdOSNoopCloser{}}
		process.stderr = &streamValue{id: fmt.Sprintf("test-process:%d:stderr", pid), mode: "process_stderr", reader: bufio.NewReader(strings.NewReader("")), closer: stdOSNoopCloser{}}
		return process, nil
	}
}

func newStdOSTestState() *stdOSTestState {
	s := &stdOSTestState{
		files:    make(map[string]*stdOSFileEntry),
		env:      make(map[string]string),
		nextPID:  1000,
		hostname: "osiris-test",
		identity: make(map[string]Value),
		platform: make(map[string]Value),
	}
	s.files["/"] = &stdOSFileEntry{mode: os.ModeDir | 0o755, isDir: true}
	s.platform["os"] = stringValue(runtime.GOOS)
	s.platform["arch"] = stringValue(runtime.GOARCH)
	return s
}

// SetTestIdentity replaces deterministic identity fields for a test runtime.
func (r *Runtime) SetTestIdentity(values map[string]Value) {
	if r == nil || r.stdOSTest == nil {
		return
	}
	r.stdOSTest.mu.Lock()
	r.stdOSTest.identity = make(map[string]Value, len(values))
	for key, value := range values {
		r.stdOSTest.identity[key] = value
	}
	r.stdOSTest.mu.Unlock()
}

// SetTestPlatform replaces deterministic platform report fields for a test runtime.
func (r *Runtime) SetTestPlatform(values map[string]Value) {
	if r == nil || r.stdOSTest == nil {
		return
	}
	r.stdOSTest.mu.Lock()
	r.stdOSTest.platform = make(map[string]Value, len(values))
	for key, value := range values {
		r.stdOSTest.platform[key] = value
	}
	r.stdOSTest.mu.Unlock()
}

func stdOSPlatform(rt *Runtime, args []Value) (Value, error) {
	if len(args) != 0 {
		return nullValue(), fmt.Errorf("platform expects no arguments")
	}
	if err := requireStdOSCapability(rt, "os.system_identity", "os.platform"); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		rt.stdOSTest.mu.RLock()
		keys := make([]string, 0, len(rt.stdOSTest.platform))
		for key := range rt.stdOSTest.platform {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		entries := make([]mapEntry, 0, len(keys))
		for _, key := range keys {
			entries = append(entries, mapEntry{key: stringValue(key), value: rt.stdOSTest.platform[key]})
		}
		rt.stdOSTest.mu.RUnlock()
		return mapValueOf(entries), nil
	}
	return mapValueOf([]mapEntry{{key: stringValue("os"), value: stringValue(runtime.GOOS)}, {key: stringValue("arch"), value: stringValue(runtime.GOARCH)}}), nil
}

func stdOSGroups(rt *Runtime, args []Value) (Value, error) {
	if len(args) != 0 {
		return nullValue(), fmt.Errorf("getgroups expects no arguments")
	}
	if err := requireStdOSCapability(rt, "os.system_identity", "os.getgroups"); err != nil {
		return nullValue(), err
	}
	groups, err := stdOSGetgroups()
	if err != nil {
		return nullValue(), filesystemHostFailure("os.getgroups", "", err)
	}
	out := make([]Value, len(groups))
	for i, group := range groups {
		out[i] = intValue(bigIntFromInt64(int64(group)), 64)
	}
	return listValue(out), nil
}

func stdOSCapabilities() []string {
	return []string{
		"os.file_read", "os.file_write", "os.file_metadata", "os.file_mutate",
		"os.directory_read", "os.directory_mutate", "os.symlink", "os.root",
		"os.environment", "os.environment_mutate", "os.system_identity",
		"os.process_inspect", "os.process_spawn", "os.process_signal",
		"os.process_kill", "os.process_raw_handle", "os.process", "os.signal", "os.watch",
	}
}

func requireStdOSCapability(rt *Runtime, capability, operation string) error {
	if rt != nil && rt.stdOSTest != nil {
		return nil
	}
	if rt != nil && rt.hasCapability(capability) {
		return nil
	}
	// Preserve the task-119 umbrella capabilities for existing applications.
	aliases := map[string]string{
		"os.file_read": "os.filesystem", "os.file_write": "os.filesystem",
		"os.file_metadata": "os.filesystem", "os.file_mutate": "os.filesystem",
		"os.directory_read": "os.filesystem", "os.directory_mutate": "os.filesystem",
		"os.symlink": "os.filesystem", "os.root": "os.filesystem",
		"os.process_inspect": "os.process", "os.process_spawn": "os.process",
		"os.process_signal": "os.process", "os.process_kill": "os.process",
	}
	if alias := aliases[capability]; alias != "" && rt != nil && rt.hasCapability(alias) {
		return nil
	}
	return osFailure(ErrorKindPermissionDenied, operation, fmt.Errorf("capability %q is not granted", capability))
}

func stdOSPath(value Value, operation string) (string, error) {
	path, err := filesystemPathArgument(value, operation)
	if err != nil {
		return "", err
	}
	return path, nil
}

func stdOSFakePath(path string) string {
	if path == "" {
		return "/"
	}
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func stdOSFakeParent(path string) string {
	parent := filepath.ToSlash(filepath.Dir(path))
	if parent == "." {
		return "/"
	}
	return parent
}

type stdOSFileHandle struct {
	mu       sync.Mutex
	file     *os.File
	test     *stdOSTestState
	path     string
	offset   int64
	writable bool
	append   bool
	borrowed bool
	closed   bool
}

func (h *stdOSFileHandle) Close() error {
	if h == nil {
		return os.ErrInvalid
	}
	h.mu.Lock()
	if h.closed {
		if h.borrowed {
			h.mu.Unlock()
			return nil
		}
		h.mu.Unlock()
		return os.ErrClosed
	}
	h.closed = true
	if h.borrowed {
		h.mu.Unlock()
		return nil
	}
	f := h.file
	h.mu.Unlock()
	if f != nil {
		return f.Close()
	}
	return nil
}

func (h *stdOSFileHandle) check(wantWrite bool) error {
	if h == nil {
		return os.ErrInvalid
	}
	if h.closed {
		return os.ErrClosed
	}
	if wantWrite && !h.writable {
		return os.ErrPermission
	}
	return nil
}

func (h *stdOSFileHandle) Read(p []byte) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.check(false); err != nil {
		return 0, err
	}
	if h.file != nil {
		return h.file.Read(p)
	}
	h.test.mu.RLock()
	entry := h.test.files[h.path]
	if entry == nil || entry.isDir {
		h.test.mu.RUnlock()
		return 0, os.ErrNotExist
	}
	if h.offset >= int64(len(entry.data)) {
		h.test.mu.RUnlock()
		return 0, io.EOF
	}
	n := copy(p, entry.data[h.offset:])
	h.offset += int64(n)
	h.test.mu.RUnlock()
	return n, nil
}

func (h *stdOSFileHandle) Write(p []byte) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.check(true); err != nil {
		return 0, err
	}
	if h.file != nil {
		return h.file.Write(p)
	}
	h.test.mu.Lock()
	entry := h.test.files[h.path]
	if entry == nil || entry.isDir {
		h.test.mu.Unlock()
		return 0, os.ErrNotExist
	}
	if h.append {
		h.offset = int64(len(entry.data))
	}
	end := h.offset + int64(len(p))
	if end > int64(len(entry.data)) {
		entry.data = append(entry.data, make([]byte, end-int64(len(entry.data)))...)
	}
	copy(entry.data[h.offset:end], p)
	h.offset = end
	entry.modTime = time.Unix(0, 0).UTC()
	h.test.mu.Unlock()
	return len(p), nil
}

func (h *stdOSFileHandle) ReadAt(p []byte, off int64) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.check(false); err != nil {
		return 0, err
	}
	if h.file != nil {
		return h.file.ReadAt(p, off)
	}
	if off < 0 {
		return 0, os.ErrInvalid
	}
	h.test.mu.RLock()
	entry := h.test.files[h.path]
	if entry == nil || entry.isDir {
		h.test.mu.RUnlock()
		return 0, os.ErrNotExist
	}
	if off >= int64(len(entry.data)) {
		h.test.mu.RUnlock()
		return 0, io.EOF
	}
	n := copy(p, entry.data[off:])
	h.test.mu.RUnlock()
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (h *stdOSFileHandle) WriteAt(p []byte, off int64) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.check(true); err != nil {
		return 0, err
	}
	if h.file != nil {
		return h.file.WriteAt(p, off)
	}
	if off < 0 {
		return 0, os.ErrInvalid
	}
	h.test.mu.Lock()
	entry := h.test.files[h.path]
	if entry == nil || entry.isDir {
		h.test.mu.Unlock()
		return 0, os.ErrNotExist
	}
	end := off + int64(len(p))
	if end > int64(len(entry.data)) {
		entry.data = append(entry.data, make([]byte, end-int64(len(entry.data)))...)
	}
	copy(entry.data[off:end], p)
	h.test.mu.Unlock()
	return len(p), nil
}

func (h *stdOSFileHandle) Seek(offset int64, whence int) (int64, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.check(false); err != nil {
		return 0, err
	}
	if h.file != nil {
		return h.file.Seek(offset, whence)
	}
	base := int64(0)
	if whence == io.SeekCurrent {
		base = h.offset
	} else if whence == io.SeekEnd {
		h.test.mu.RLock()
		if entry := h.test.files[h.path]; entry != nil {
			base = int64(len(entry.data))
		}
		h.test.mu.RUnlock()
	} else if whence != io.SeekStart {
		return 0, os.ErrInvalid
	}
	if base+offset < 0 {
		return 0, os.ErrInvalid
	}
	h.offset = base + offset
	return h.offset, nil
}

func (h *stdOSFileHandle) Sync() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.check(false); err != nil {
		return err
	}
	if h.file != nil {
		return h.file.Sync()
	}
	return nil
}

func (h *stdOSFileHandle) Truncate(size int64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.check(true); err != nil {
		return err
	}
	if size < 0 {
		return os.ErrInvalid
	}
	if h.file != nil {
		return h.file.Truncate(size)
	}
	h.test.mu.Lock()
	entry := h.test.files[h.path]
	if entry == nil || entry.isDir {
		h.test.mu.Unlock()
		return os.ErrNotExist
	}
	if size < int64(len(entry.data)) {
		entry.data = entry.data[:size]
	} else if size > int64(len(entry.data)) {
		entry.data = append(entry.data, make([]byte, size-int64(len(entry.data)))...)
	}
	h.test.mu.Unlock()
	return nil
}

func (h *stdOSFileHandle) Stat() (os.FileInfo, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.check(false); err != nil {
		return nil, err
	}
	if h.file != nil {
		return h.file.Stat()
	}
	h.test.mu.RLock()
	entry := h.test.files[h.path]
	if entry == nil {
		h.test.mu.RUnlock()
		return nil, os.ErrNotExist
	}
	info := stdOSFakeInfo(filepath.Base(h.path), entry)
	h.test.mu.RUnlock()
	return info, nil
}

type stdOSFakeFileInfo struct {
	name  string
	entry *stdOSFileEntry
}

func stdOSFakeInfo(name string, entry *stdOSFileEntry) os.FileInfo {
	return stdOSFakeFileInfo{name: name, entry: &stdOSFileEntry{data: append([]byte(nil), entry.data...), mode: entry.mode, modTime: entry.modTime, isDir: entry.isDir, target: entry.target}}
}
func (i stdOSFakeFileInfo) Name() string       { return i.name }
func (i stdOSFakeFileInfo) Size() int64        { return int64(len(i.entry.data)) }
func (i stdOSFakeFileInfo) Mode() os.FileMode  { return i.entry.mode }
func (i stdOSFakeFileInfo) ModTime() time.Time { return i.entry.modTime }
func (i stdOSFakeFileInfo) IsDir() bool        { return i.entry.isDir }
func (i stdOSFakeFileInfo) Sys() any           { return nil }

func stdOSHandleValue(handle *stdOSFileHandle, mode string) Value {
	return streamValueOf(&streamValue{id: handle.path, path: handle.path, mode: mode, closer: handle, stdFile: handle})
}

func stdOSHandle(args []Value, operation string) (*stdOSFileHandle, error) {
	if len(args) == 0 || args[0].kind != kindStream || args[0].stream == nil || args[0].stream.stdFile == nil {
		return nil, filesystemFailure(ErrorKindType, operation, "", operation+" expects an os file handle", nil)
	}
	return args[0].stream.stdFile, nil
}

func stdOSCallContext(ctx context.Context, operation string, fn func() (Value, error)) (Value, error) {
	if err := filesystemContext(ctx, operation, ""); err != nil {
		return nullValue(), err
	}
	return fn()
}

func stdOSOpenContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, "os.open", "", "open expects a path", nil)
	}
	if err := requireStdOSCapability(rt, "os.file_read", "os.open"); err != nil {
		return nullValue(), err
	}
	path, err := stdOSPath(args[0], "os.open")
	if err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.open", func() (Value, error) { return stdOSOpen(rt, path, os.O_RDONLY, 0) })
}

func stdOSOpen(rt *Runtime, path string, flag int, perm os.FileMode) (Value, error) {
	if rt != nil && rt.stdOSTest != nil {
		return stdOSFakeOpen(rt.stdOSTest, path, flag, perm)
	}
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nullValue(), filesystemHostFailure("os.open", path, err)
	}
	h := &stdOSFileHandle{file: f, path: path, writable: flag&(os.O_WRONLY|os.O_RDWR) != 0}
	return stdOSHandleValue(h, "os_file"), nil
}

func stdOSCreateContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, "os.create", "", "create expects a path", nil)
	}
	if err := requireStdOSCapability(rt, "os.file_write", "os.create"); err != nil {
		return nullValue(), err
	}
	path, err := stdOSPath(args[0], "os.create")
	if err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.create", func() (Value, error) { return stdOSOpen(rt, path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644) })
}

func stdOSOpenFileContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, "os.open_file", "", "open_file expects path, flags, and mode", nil)
	}
	if err := requireStdOSCapability(rt, "os.file_read", "os.open_file"); err != nil {
		return nullValue(), err
	}
	path, err := stdOSPath(args[0], "os.open_file")
	if err != nil {
		return nullValue(), err
	}
	flags, err := filesystemIntegerArgument(args[1], "os.open_file", "flags")
	if err != nil {
		return nullValue(), err
	}
	mode, err := filesystemIntegerArgument(args[2], "os.open_file", "mode")
	if err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.open_file", func() (Value, error) { return stdOSOpen(rt, path, int(flags), os.FileMode(mode)) })
}

func stdOSFakeOpen(state *stdOSTestState, raw string, flag int, perm os.FileMode) (Value, error) {
	path := stdOSFakePath(raw)
	state.mu.Lock()
	defer state.mu.Unlock()
	entry := state.files[path]
	if entry == nil {
		if flag&os.O_CREATE == 0 {
			return nullValue(), filesystemHostFailure("os.open", raw, os.ErrNotExist)
		}
		if parent := state.files[stdOSFakeParent(path)]; parent == nil || !parent.isDir {
			return nullValue(), filesystemHostFailure("os.open", raw, os.ErrNotExist)
		}
		entry = &stdOSFileEntry{mode: perm & os.ModePerm, modTime: time.Unix(0, 0).UTC()}
		state.files[path] = entry
	} else if flag&os.O_CREATE != 0 && flag&os.O_EXCL != 0 {
		return nullValue(), filesystemHostFailure("os.open", raw, os.ErrExist)
	}
	if entry.isDir {
		return nullValue(), filesystemHostFailure("os.open", raw, syscall.EISDIR)
	}
	if flag&os.O_TRUNC != 0 {
		entry.data = nil
	}
	h := &stdOSFileHandle{test: state, path: path, writable: flag&(os.O_WRONLY|os.O_RDWR) != 0, append: flag&os.O_APPEND != 0}
	if h.append {
		h.offset = int64(len(entry.data))
	}
	return stdOSHandleValue(h, "os_file"), nil
}

func stdOSReadFileContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, "os.read_file", "", "read_file expects a path", nil)
	}
	if err := requireStdOSCapability(rt, "os.file_read", "os.read_file"); err != nil {
		return nullValue(), err
	}
	path, err := stdOSPath(args[0], "os.read_file")
	if err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.read_file", func() (Value, error) {
		if rt != nil && rt.stdOSTest != nil {
			return stdOSFakeRead(rt.stdOSTest, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nullValue(), filesystemHostFailure("os.read_file", path, err)
		}
		if len(data) > stdOSMaxRead {
			return nullValue(), filesystemFailure(ErrorKindResourceExhausted, "os.read_file", path, "read exceeds the bounded limit", nil)
		}
		return bytesListValue(data), nil
	})
}

func stdOSFakeRead(state *stdOSTestState, raw string) (Value, error) {
	state.mu.RLock()
	entry := state.files[stdOSFakePath(raw)]
	if entry == nil || entry.isDir {
		state.mu.RUnlock()
		return nullValue(), filesystemHostFailure("os.read_file", raw, os.ErrNotExist)
	}
	data := append([]byte(nil), entry.data...)
	state.mu.RUnlock()
	return bytesListValue(data), nil
}

func stdOSWriteFileContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, "os.write_file", "", "write_file expects path, data, and mode", nil)
	}
	if err := requireStdOSCapability(rt, "os.file_write", "os.write_file"); err != nil {
		return nullValue(), err
	}
	path, err := stdOSPath(args[0], "os.write_file")
	if err != nil {
		return nullValue(), err
	}
	data, err := bytesFromValue(args[1])
	if err != nil {
		return nullValue(), filesystemFailure(ErrorKindType, "os.write_file", path, "data is not byte-compatible", nil)
	}
	mode, err := filesystemIntegerArgument(args[2], "os.write_file", "mode")
	if err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.write_file", func() (Value, error) {
		if rt != nil && rt.stdOSTest != nil {
			h, e := stdOSFakeOpen(rt.stdOSTest, path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(mode))
			if e != nil {
				return nullValue(), e
			}
			_, e = h.stream.stdFile.Write(data)
			_ = h.stream.stdFile.Close()
			if e != nil {
				return nullValue(), filesystemHostFailure("os.write_file", path, e)
			}
			return boolValue(true), nil
		}
		if e := os.WriteFile(path, data, os.FileMode(mode)); e != nil {
			return nullValue(), filesystemHostFailure("os.write_file", path, e)
		}
		return boolValue(true), nil
	})
}

func stdOSFileReadContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	h, err := stdOSHandle(args, "os.file_read")
	if err != nil {
		return nullValue(), err
	}
	if err = requireStdOSCapability(rt, "os.file_read", "os.file_read"); err != nil {
		return nullValue(), err
	}
	if len(args) != 2 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, "os.file_read", "", "file_read expects handle and size", nil)
	}
	n, err := filesystemNonnegativeArgument(args[1], "os.file_read", "size", stdOSMaxRead)
	if err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.file_read", func() (Value, error) {
		buf := make([]byte, n)
		count, e := h.Read(buf)
		if e != nil && e != io.EOF {
			return nullValue(), filesystemHostFailure("os.file_read", h.path, e)
		}
		return bytesListValue(buf[:count]), nil
	})
}

func stdOSFileWriteContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, "os.file_write", "", "file_write expects handle and data", nil)
	}
	h, err := stdOSHandle(args, "os.file_write")
	if err != nil {
		return nullValue(), err
	}
	if err = requireStdOSCapability(rt, "os.file_write", "os.file_write"); err != nil {
		return nullValue(), err
	}
	data, err := bytesFromValue(args[1])
	if err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.file_write", func() (Value, error) {
		n, e := h.Write(data)
		if e != nil {
			return nullValue(), filesystemHostFailure("os.file_write", h.path, e)
		}
		return uintValue(uint64(n), 64), nil
	})
}

func stdOSFileReadAtContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("read_at expects handle, size, and offset")
	}
	h, err := stdOSHandle(args, "os.read_at")
	if err != nil {
		return nullValue(), err
	}
	n, err := filesystemNonnegativeArgument(args[1], "os.read_at", "size", stdOSMaxRead)
	if err != nil {
		return nullValue(), err
	}
	off, err := filesystemIntegerArgument(args[2], "os.read_at", "offset")
	if err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.read_at", func() (Value, error) {
		buf := make([]byte, n)
		count, e := h.ReadAt(buf, off)
		if e != nil && e != io.EOF {
			return nullValue(), filesystemHostFailure("os.read_at", h.path, e)
		}
		return bytesListValue(buf[:count]), nil
	})
}
func stdOSFileWriteAtContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("write_at expects handle, data, and offset")
	}
	h, err := stdOSHandle(args, "os.write_at")
	if err != nil {
		return nullValue(), err
	}
	data, err := bytesFromValue(args[1])
	if err != nil {
		return nullValue(), err
	}
	off, err := filesystemIntegerArgument(args[2], "os.write_at", "offset")
	if err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.write_at", func() (Value, error) {
		n, e := h.WriteAt(data, off)
		if e != nil {
			return nullValue(), filesystemHostFailure("os.write_at", h.path, e)
		}
		return uintValue(uint64(n), 64), nil
	})
}
func stdOSSeekContext(_ *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("seek expects handle, offset, and whence")
	}
	h, err := stdOSHandle(args, "os.seek")
	if err != nil {
		return nullValue(), err
	}
	off, err := filesystemIntegerArgument(args[1], "os.seek", "offset")
	if err != nil {
		return nullValue(), err
	}
	whence, err := args[2].Int()
	if err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.seek", func() (Value, error) {
		pos, e := h.Seek(off, whence)
		if e != nil {
			return nullValue(), filesystemHostFailure("os.seek", h.path, e)
		}
		return intValue(bigIntFromInt64(pos), 64), nil
	})
}

func bigIntFromInt64(n int64) *big.Int { return big.NewInt(n) }

func stdOSStatValue(info os.FileInfo) Value {
	return mapValueOf([]mapEntry{{stringValue("name"), stringValue(info.Name())}, {stringValue("size"), intValue(bigIntFromInt64(info.Size()), 64)}, {stringValue("mode"), uintValue(uint64(info.Mode()), 32)}, {stringValue("dir"), boolValue(info.IsDir())}, {stringValue("mod_time"), stringValue(info.ModTime().UTC().Format(time.RFC3339Nano))}})
}

func stdOSStatContext(rt *Runtime, ctx context.Context, args []Value, follow bool) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("stat expects a path")
	}
	if err := requireStdOSCapability(rt, "os.file_metadata", "os.stat"); err != nil {
		return nullValue(), err
	}
	path, err := stdOSPath(args[0], "os.stat")
	if err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.stat", func() (Value, error) {
		var info os.FileInfo
		var e error
		if rt != nil && rt.stdOSTest != nil {
			rt.stdOSTest.mu.RLock()
			entry := rt.stdOSTest.files[stdOSFakePath(path)]
			if entry == nil {
				rt.stdOSTest.mu.RUnlock()
				return nullValue(), filesystemHostFailure("os.stat", path, os.ErrNotExist)
			}
			info = stdOSFakeInfo(filepath.Base(path), entry)
			rt.stdOSTest.mu.RUnlock()
		} else if follow {
			info, e = os.Stat(path)
		} else {
			info, e = os.Lstat(path)
		}
		if e != nil {
			return nullValue(), filesystemHostFailure("os.stat", path, e)
		}
		return stdOSStatValue(info), nil
	})
}

func stdOSMutatePath(rt *Runtime, operation, path string, fn func() error) (Value, error) {
	if err := requireStdOSCapability(rt, "os.file_mutate", operation); err != nil {
		return nullValue(), err
	}
	if err := fn(); err != nil {
		return nullValue(), filesystemHostFailure(operation, path, err)
	}
	return boolValue(true), nil
}

func stdOSMkdirContext(rt *Runtime, ctx context.Context, args []Value, all bool) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("mkdir expects path and mode")
	}
	path, err := stdOSPath(args[0], "os.mkdir")
	if err != nil {
		return nullValue(), err
	}
	mode, err := filesystemIntegerArgument(args[1], "os.mkdir", "mode")
	if err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.mkdir", func() (Value, error) {
		if rt != nil && rt.stdOSTest != nil {
			return stdOSFakeMkdir(rt.stdOSTest, path, os.FileMode(mode), all)
		}
		return stdOSMutatePath(rt, "os.mkdir", path, func() error {
			if all {
				return os.MkdirAll(path, os.FileMode(mode))
			}
			return os.Mkdir(path, os.FileMode(mode))
		})
	})
}

func stdOSFakeMkdir(s *stdOSTestState, raw string, mode os.FileMode, all bool) (Value, error) {
	path := stdOSFakePath(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	current := "/"
	for _, part := range parts {
		if part == "" {
			continue
		}
		current = strings.TrimSuffix(current, "/") + "/" + part
		if entry := s.files[current]; entry != nil {
			if !entry.isDir {
				return nullValue(), filesystemHostFailure("os.mkdir", raw, syscall.ENOTDIR)
			}
			continue
		}
		if !all && current != path {
			return nullValue(), filesystemHostFailure("os.mkdir", raw, os.ErrNotExist)
		}
		s.files[current] = &stdOSFileEntry{mode: os.ModeDir | mode, isDir: true, modTime: time.Unix(0, 0).UTC()}
	}
	return boolValue(true), nil
}

func stdOSRemoveContext(rt *Runtime, ctx context.Context, args []Value, all bool) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("remove expects path")
	}
	path, err := stdOSPath(args[0], "os.remove")
	if err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.remove", func() (Value, error) {
		if rt != nil && rt.stdOSTest != nil {
			return stdOSFakeRemove(rt.stdOSTest, path, all)
		}
		return stdOSMutatePath(rt, "os.remove", path, func() error {
			if all {
				return os.RemoveAll(path)
			}
			return os.Remove(path)
		})
	})
}
func stdOSFakeRemove(s *stdOSTestState, raw string, all bool) (Value, error) {
	path := stdOSFakePath(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.files[path] == nil {
		if all {
			return boolValue(true), nil
		}
		return nullValue(), filesystemHostFailure("os.remove", raw, os.ErrNotExist)
	}
	if all {
		for name := range s.files {
			if name == path || strings.HasPrefix(name, path+"/") {
				delete(s.files, name)
			}
		}
		return boolValue(true), nil
	}
	delete(s.files, path)
	return boolValue(true), nil
}

func stdOSReadDirContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("read_dir expects path")
	}
	if err := requireStdOSCapability(rt, "os.directory_read", "os.read_dir"); err != nil {
		return nullValue(), err
	}
	path, err := stdOSPath(args[0], "os.read_dir")
	if err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.read_dir", func() (Value, error) {
		names := []string{}
		if rt != nil && rt.stdOSTest != nil {
			root := stdOSFakePath(path)
			prefix := strings.TrimSuffix(root, "/") + "/"
			rt.stdOSTest.mu.RLock()
			for name, entry := range rt.stdOSTest.files {
				if strings.HasPrefix(name, prefix) && !strings.Contains(strings.TrimPrefix(name, prefix), "/") && name != root && entry != nil {
					names = append(names, filepath.Base(name))
				}
			}
			rt.stdOSTest.mu.RUnlock()
		} else {
			entries, e := os.ReadDir(path)
			if e != nil {
				return nullValue(), filesystemHostFailure("os.read_dir", path, e)
			}
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
		}
		sort.Strings(names)
		out := make([]Value, len(names))
		for i, name := range names {
			out[i] = stringValue(name)
		}
		return listValue(out), nil
	})
}

func stdOSEnvValue(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindString {
		return nullValue(), fmt.Errorf("getenv expects a name")
	}
	if err := requireStdOSCapability(rt, "os.environment", "os.getenv"); err != nil {
		return nullValue(), err
	}
	key := args[0].str
	return stdOSCallContext(ctx, "os.getenv", func() (Value, error) {
		if rt != nil && rt.stdOSTest != nil {
			rt.stdOSTest.mu.RLock()
			v, ok := rt.stdOSTest.env[key]
			rt.stdOSTest.mu.RUnlock()
			if !ok {
				return optionNoneValue(), nil
			}
			return optionSomeValue(stringValue(v)), nil
		}
		v, ok := os.LookupEnv(key)
		if !ok {
			return optionNoneValue(), nil
		}
		return optionSomeValue(stringValue(v)), nil
	})
}
func stdOSEnviron(rt *Runtime, _ context.Context, args []Value) (Value, error) {
	if len(args) != 0 {
		return nullValue(), fmt.Errorf("environ expects no arguments")
	}
	if err := requireStdOSCapability(rt, "os.environment", "os.environ"); err != nil {
		return nullValue(), err
	}
	values := map[string]string{}
	if rt != nil && rt.stdOSTest != nil {
		rt.stdOSTest.mu.RLock()
		for k, v := range rt.stdOSTest.env {
			values[k] = v
		}
		rt.stdOSTest.mu.RUnlock()
	} else {
		for _, item := range os.Environ() {
			if k, v, ok := strings.Cut(item, "="); ok {
				values[k] = v
			}
		}
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Value, len(keys))
	for i, k := range keys {
		out[i] = stringValue(k + "=" + values[k])
	}
	return listValue(out), nil
}
func stdOSSetenv(rt *Runtime, _ context.Context, args []Value) (Value, error) {
	if len(args) != 2 || args[0].kind != kindString || args[1].kind != kindString {
		return nullValue(), fmt.Errorf("setenv expects name and value")
	}
	if err := requireStdOSCapability(rt, "os.environment_mutate", "os.setenv"); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		rt.stdOSTest.mu.Lock()
		rt.stdOSTest.env[args[0].str] = args[1].str
		rt.stdOSTest.mu.Unlock()
		return boolValue(true), nil
	}
	if err := os.Setenv(args[0].str, args[1].str); err != nil {
		return nullValue(), filesystemHostFailure("os.setenv", args[0].str, err)
	}
	return boolValue(true), nil
}
func stdOSUnsetenv(rt *Runtime, _ context.Context, args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindString {
		return nullValue(), fmt.Errorf("unsetenv expects name")
	}
	if err := requireStdOSCapability(rt, "os.environment_mutate", "os.unsetenv"); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		rt.stdOSTest.mu.Lock()
		delete(rt.stdOSTest.env, args[0].str)
		rt.stdOSTest.mu.Unlock()
		return boolValue(true), nil
	}
	if err := os.Unsetenv(args[0].str); err != nil {
		return nullValue(), filesystemHostFailure("os.unsetenv", args[0].str, err)
	}
	return boolValue(true), nil
}

func stdOSExpand(args []Value) (Value, error) {
	if len(args) != 2 || args[0].kind != kindString || args[1].kind != kindFunction {
		return nullValue(), fmt.Errorf("expand expects string and mapping function")
	}
	return nullValue(), fmt.Errorf("expand mapping callbacks are not available at the std/os boundary")
}
func stdOSExpandEnv(rt *Runtime, args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindString {
		return nullValue(), fmt.Errorf("expand_env expects a string")
	}
	if err := requireStdOSCapability(rt, "os.environment", "os.expand_env"); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		rt.stdOSTest.mu.RLock()
		out := os.Expand(args[0].str, func(k string) string { return rt.stdOSTest.env[k] })
		rt.stdOSTest.mu.RUnlock()
		return stringValue(out), nil
	}
	return stringValue(os.ExpandEnv(args[0].str)), nil
}

func stdOSIdentity(rt *Runtime, name string) (Value, error) {
	if err := requireStdOSCapability(rt, "os.system_identity", "os."+name); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		rt.stdOSTest.mu.RLock()
		if v, ok := rt.stdOSTest.identity[name]; ok {
			rt.stdOSTest.mu.RUnlock()
			return v, nil
		}
		rt.stdOSTest.mu.RUnlock()
		switch name {
		case "hostname":
			return stringValue(rt.stdOSTest.hostname), nil
		case "getpid":
			return intValue(bigIntFromInt64(1000), 64), nil
		default:
			return intValue(bigIntFromInt64(0), 64), nil
		}
	}
	switch name {
	case "hostname":
		v, e := os.Hostname()
		if e != nil {
			return nullValue(), filesystemHostFailure("os.hostname", "", e)
		}
		return stringValue(v), nil
	case "getpid":
		return intValue(bigIntFromInt64(int64(os.Getpid())), 64), nil
	case "getppid":
		return intValue(bigIntFromInt64(int64(os.Getppid())), 64), nil
	case "getuid":
		return intValue(bigIntFromInt64(int64(stdOSGetuid())), 64), nil
	case "geteuid":
		return intValue(bigIntFromInt64(int64(stdOSGeteuid())), 64), nil
	case "getgid":
		return intValue(bigIntFromInt64(int64(stdOSGetgid())), 64), nil
	case "getegid":
		return intValue(bigIntFromInt64(int64(stdOSGetegid())), 64), nil
	case "getpagesize":
		return intValue(bigIntFromInt64(int64(os.Getpagesize())), 64), nil
	}
	return nullValue(), fmt.Errorf("unsupported identity %s", name)
}

func stdOSModule(rt *Runtime) map[string]Value {
	fn := func(name string, f contextCallable) Value { return contextFunctionValueOf("std/os."+name, f) }
	mod := map[string]Value{
		"args": fn("args", func(ctx context.Context, a []Value) (Value, error) { return osArgsValue(rt, ctx, a) }), "env": fn("env", func(ctx context.Context, a []Value) (Value, error) { return osEnvValue(rt, ctx, a) }), "env_names": fn("env_names", func(ctx context.Context, a []Value) (Value, error) { return osEnvNamesValue(rt, ctx, a) }),
		"open": fn("open", func(c context.Context, a []Value) (Value, error) { return stdOSOpenContext(rt, c, a) }), "create": fn("create", func(c context.Context, a []Value) (Value, error) { return stdOSCreateContext(rt, c, a) }), "open_file": fn("open_file", func(c context.Context, a []Value) (Value, error) { return stdOSOpenFileContext(rt, c, a) }), "create_temp": fn("create_temp", func(c context.Context, a []Value) (Value, error) { return stdOSCreateTempContext(rt, c, a) }), "pipe": fn("pipe", func(c context.Context, a []Value) (Value, error) { return stdOSPipeContext(rt, c, a) }), "read_file": fn("read_file", func(c context.Context, a []Value) (Value, error) { return stdOSReadFileContext(rt, c, a) }), "write_file": fn("write_file", func(c context.Context, a []Value) (Value, error) { return stdOSWriteFileContext(rt, c, a) }),
		"read": fn("read", func(c context.Context, a []Value) (Value, error) { return stdOSFileReadContext(rt, c, a) }), "write": fn("write", func(c context.Context, a []Value) (Value, error) { return stdOSFileWriteContext(rt, c, a) }), "write_string": fn("write_string", func(c context.Context, a []Value) (Value, error) { return stdOSWriteStringContext(rt, c, a) }), "read_at": fn("read_at", func(c context.Context, a []Value) (Value, error) { return stdOSFileReadAtContext(rt, c, a) }), "write_at": fn("write_at", func(c context.Context, a []Value) (Value, error) { return stdOSFileWriteAtContext(rt, c, a) }), "seek_file": fn("seek_file", func(c context.Context, a []Value) (Value, error) { return stdOSSeekContext(rt, c, a) }), "sync": fn("sync", func(c context.Context, a []Value) (Value, error) { return stdOSSyncContext(rt, c, a) }),
		"close": functionValueOf("std/os.close", closeBuiltinValue), "read_dir": fn("read_dir", func(c context.Context, a []Value) (Value, error) { return stdOSReadDirContext(rt, c, a) }), "stat": fn("stat", func(c context.Context, a []Value) (Value, error) { return stdOSStatContext(rt, c, a, true) }), "lstat": fn("lstat", func(c context.Context, a []Value) (Value, error) { return stdOSStatContext(rt, c, a, false) }), "mkdir": fn("mkdir", func(c context.Context, a []Value) (Value, error) { return stdOSMkdirContext(rt, c, a, false) }), "mkdir_all": fn("mkdir_all", func(c context.Context, a []Value) (Value, error) { return stdOSMkdirContext(rt, c, a, true) }), "remove": fn("remove", func(c context.Context, a []Value) (Value, error) { return stdOSRemoveContext(rt, c, a, false) }), "remove_all": fn("remove_all", func(c context.Context, a []Value) (Value, error) { return stdOSRemoveContext(rt, c, a, true) }), "truncate": fn("truncate", func(c context.Context, a []Value) (Value, error) { return stdOSTruncatePathContext(rt, c, a) }), "link": fn("link", func(c context.Context, a []Value) (Value, error) { return stdOSLinkContext(rt, c, a, false) }), "symlink": fn("symlink", func(c context.Context, a []Value) (Value, error) { return stdOSLinkContext(rt, c, a, true) }), "readlink": fn("readlink", func(c context.Context, a []Value) (Value, error) { return stdOSReadlinkContext(rt, c, a) }),
		"rename": fn("rename", func(_ context.Context, a []Value) (Value, error) {
			if len(a) != 2 {
				return nullValue(), fmt.Errorf("rename expects two paths")
			}
			old, e := stdOSPath(a[0], "os.rename")
			if e != nil {
				return nullValue(), e
			}
			next, e := stdOSPath(a[1], "os.rename")
			if e != nil {
				return nullValue(), e
			}
			return stdOSMutatePath(rt, "os.rename", old, func() error {
				if rt != nil && rt.stdOSTest != nil {
					rt.stdOSTest.mu.Lock()
					defer rt.stdOSTest.mu.Unlock()
					entry := rt.stdOSTest.files[stdOSFakePath(old)]
					if entry == nil {
						return os.ErrNotExist
					}
					rt.stdOSTest.files[stdOSFakePath(next)] = entry
					delete(rt.stdOSTest.files, stdOSFakePath(old))
					return nil
				}
				return os.Rename(old, next)
			})
		}),
		"chmod": fn("chmod", func(c context.Context, a []Value) (Value, error) { return stdOSChmodContext(rt, c, a) }), "chown": fn("chown", func(c context.Context, a []Value) (Value, error) { return stdOSChownContext(rt, c, a, false) }), "lchown": fn("lchown", func(c context.Context, a []Value) (Value, error) { return stdOSChownContext(rt, c, a, true) }), "chtimes": fn("chtimes", func(c context.Context, a []Value) (Value, error) { return stdOSChtimesContext(rt, c, a) }),
		"getwd": functionValueOf("std/os.getwd", func(a []Value) (Value, error) { return stdOSWorkingDir(rt, "os.getwd", a, false) }), "chdir": functionValueOf("std/os.chdir", func(a []Value) (Value, error) { return stdOSWorkingDir(rt, "os.chdir", a, true) }), "user_home_dir": functionValueOf("std/os.user_home_dir", func([]Value) (Value, error) { return stdOSUserDir(rt, "user_home_dir") }), "user_config_dir": functionValueOf("std/os.user_config_dir", func([]Value) (Value, error) { return stdOSUserDir(rt, "user_config_dir") }), "user_cache_dir": functionValueOf("std/os.user_cache_dir", func([]Value) (Value, error) { return stdOSUserDir(rt, "user_cache_dir") }), "copy_fs": fn("copy_fs", func(c context.Context, a []Value) (Value, error) { return stdOSCopyFSContext(rt, c, a) }), "open_root": functionValueOf("std/os.open_root", func(a []Value) (Value, error) { return stdOSOpenRoot(rt, a) }),
		"getenv": fn("getenv", func(c context.Context, a []Value) (Value, error) { return stdOSEnvValue(rt, c, a) }), "lookup_env": fn("lookup_env", func(c context.Context, a []Value) (Value, error) { return stdOSEnvValue(rt, c, a) }), "environ": fn("environ", func(c context.Context, a []Value) (Value, error) { return stdOSEnviron(rt, c, a) }), "setenv": fn("setenv", func(c context.Context, a []Value) (Value, error) { return stdOSSetenv(rt, c, a) }), "unsetenv": fn("unsetenv", func(c context.Context, a []Value) (Value, error) { return stdOSUnsetenv(rt, c, a) }), "expand_env": functionValueOf("std/os.expand_env", func(a []Value) (Value, error) { return stdOSExpandEnv(rt, a) }),
		"hostname": functionValueOf("std/os.hostname", func([]Value) (Value, error) { return stdOSIdentity(rt, "hostname") }), "getpid": functionValueOf("std/os.getpid", func([]Value) (Value, error) { return stdOSIdentity(rt, "getpid") }), "getppid": functionValueOf("std/os.getppid", func([]Value) (Value, error) { return stdOSIdentity(rt, "getppid") }), "getuid": functionValueOf("std/os.getuid", func([]Value) (Value, error) { return stdOSIdentity(rt, "getuid") }), "geteuid": functionValueOf("std/os.geteuid", func([]Value) (Value, error) { return stdOSIdentity(rt, "geteuid") }), "getgid": functionValueOf("std/os.getgid", func([]Value) (Value, error) { return stdOSIdentity(rt, "getgid") }), "getegid": functionValueOf("std/os.getegid", func([]Value) (Value, error) { return stdOSIdentity(rt, "getegid") }), "getpagesize": functionValueOf("std/os.getpagesize", func([]Value) (Value, error) { return stdOSIdentity(rt, "getpagesize") }),
		"process_spawn": fn("process_spawn", func(c context.Context, a []Value) (Value, error) { return processSpawnContextValue(rt, c, a) }), "process_wait": fn("process_wait", func(c context.Context, a []Value) (Value, error) { return processWaitContextValue(rt, c, a) }), "process_kill": fn("process_kill", func(c context.Context, a []Value) (Value, error) { return processKillContextValue(rt, c, a) }), "find_process": fn("find_process", func(c context.Context, a []Value) (Value, error) { return stdOSFindProcessContext(rt, c, a) }), "process_release": fn("process_release", func(c context.Context, a []Value) (Value, error) { return stdOSProcessReleaseContext(rt, c, a) }), "process_signal": fn("process_signal", func(c context.Context, a []Value) (Value, error) { return stdOSSignalProcessContext(rt, c, a) }), "process_state": fn("process_state", func(c context.Context, a []Value) (Value, error) { return stdOSProcessStateContext(rt, c, a) }), "start_process": fn("start_process", func(c context.Context, a []Value) (Value, error) { return processSpawnContextValue(rt, c, a) }), "signal_subscribe": fn("signal_subscribe", func(c context.Context, a []Value) (Value, error) { return osSignalContextValue(rt, c, a) }), "watch": fn("watch", func(c context.Context, a []Value) (Value, error) { return osWatchContextValue(rt, c, a) }),
	}
	for name, value := range map[string]Value{"O_RDONLY": intValue(bigIntFromInt64(int64(os.O_RDONLY)), 32), "O_WRONLY": intValue(bigIntFromInt64(int64(os.O_WRONLY)), 32), "O_RDWR": intValue(bigIntFromInt64(int64(os.O_RDWR)), 32), "O_APPEND": intValue(bigIntFromInt64(int64(os.O_APPEND)), 32), "O_CREATE": intValue(bigIntFromInt64(int64(os.O_CREATE)), 32), "O_EXCL": intValue(bigIntFromInt64(int64(os.O_EXCL)), 32), "O_SYNC": intValue(bigIntFromInt64(int64(os.O_SYNC)), 32), "O_TRUNC": intValue(bigIntFromInt64(int64(os.O_TRUNC)), 32), "dev_null": stringValue(os.DevNull), "err_invalid": stringValue("invalid argument"), "err_permission": stringValue("permission denied"), "err_exist": stringValue("file already exists"), "err_not_exist": stringValue("file does not exist"), "err_closed": stringValue("file already closed"), "err_process_done": stringValue("process already finished"), "err_no_handle": stringValue("process handle unavailable")} {
		mod[name] = value
	}
	mod["read_dir_entries"] = fn("read_dir_entries", func(c context.Context, a []Value) (Value, error) { return stdOSReadDirEntriesContext(rt, c, a) })
	mod["mkdir_temp"] = fn("mkdir_temp", func(c context.Context, a []Value) (Value, error) { return stdOSMkdirTempContext(rt, c, a) })
	mod["clearenv"] = fn("clearenv", func(c context.Context, a []Value) (Value, error) { return stdOSClearenv(rt, c, a) })
	mod["temp_dir"] = functionValueOf("std/os.temp_dir", func(a []Value) (Value, error) { return stdOSPathUtility(rt, "temp_dir", a) })
	mod["executable"] = functionValueOf("std/os.executable", func(a []Value) (Value, error) { return stdOSPathUtility(rt, "executable", a) })
	mod["is_exist"] = functionValueOf("std/os.is_exist", func(a []Value) (Value, error) {
		return stdOSPathPredicate(rt, a, func(os.FileInfo) bool { return true })
	})
	mod["is_not_exist"] = functionValueOf("std/os.is_not_exist", func(a []Value) (Value, error) {
		return stdOSMissingPath(rt, a)
	})
	mod["platform"] = functionValueOf("std/os.platform", func(a []Value) (Value, error) { return stdOSPlatform(rt, a) })
	mod["getgroups"] = functionValueOf("std/os.getgroups", func(a []Value) (Value, error) { return stdOSGroups(rt, a) })
	mod["same_file"] = functionValueOf("std/os.same_file", func(a []Value) (Value, error) { return stdOSSameFile(rt, a) })
	mod["stdin"] = stdOSStandardStream(rt, "stdin")
	mod["stdout"] = stdOSStandardStream(rt, "stdout")
	mod["stderr"] = stdOSStandardStream(rt, "stderr")
	mod["fd"] = functionValueOf("std/os.fd", func(a []Value) (Value, error) { return stdOSFd(rt, a) })
	mod["read_from"] = contextFunctionValueOf("std/os.read_from", func(c context.Context, a []Value) (Value, error) { return stdOSTransferFileContext(rt, c, a, false) })
	mod["write_to"] = contextFunctionValueOf("std/os.write_to", func(c context.Context, a []Value) (Value, error) { return stdOSTransferFileContext(rt, c, a, true) })
	return mod
}

// Keep these imports and the contract visible to the compiler while the
// package grows the remaining typed process/root wrappers in follow-up work.
var _ = bytes.NewBuffer
var _ = errors.Is
var _ = user.Current
