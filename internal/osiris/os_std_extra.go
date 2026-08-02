package osiris

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

type stdOSRootHandle struct {
	mu     sync.Mutex
	closed bool
}

func (r *stdOSRootHandle) Close() error {
	if r == nil {
		return os.ErrInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return os.ErrClosed
	}
	r.closed = true
	return nil
}
func (r *stdOSRootHandle) isClosed() bool {
	if r == nil {
		return true
	}
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	return closed
}

func stdOSRootHandleFromValue(value Value) (*stdOSRootHandle, error) {
	handle, ok := mapFieldValue(value, "__root_handle")
	if !ok || handle.kind != kindStream || handle.stream == nil || handle.stream.closer == nil {
		return nil, fmt.Errorf("value is not an open root")
	}
	root, ok := handle.stream.closer.(*stdOSRootHandle)
	if !ok {
		return nil, fmt.Errorf("value is not an open root")
	}
	if root.isClosed() {
		return nil, os.ErrClosed
	}
	return root, nil
}

func stdOSCreateTempContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nullValue(), fmt.Errorf("create_temp expects directory and optional pattern")
	}
	dir, err := stdOSPath(args[0], "os.create_temp")
	if err != nil {
		return nullValue(), err
	}
	pattern := "osiris-*"
	if len(args) == 2 {
		if args[1].kind != kindString {
			return nullValue(), fmt.Errorf("create_temp pattern must be a string")
		}
		pattern = args[1].str
	}
	if err := requireStdOSCapability(rt, "os.file_write", "os.create_temp"); err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.create_temp", func() (Value, error) {
		if rt != nil && rt.stdOSTest != nil {
			rt.stdOSTest.mu.Lock()
			name := filepath.Join(stdOSFakePath(dir), strings.ReplaceAll(pattern, "*", "tmp"))
			for i := 0; rt.stdOSTest.files[name] != nil; i++ {
				name = filepath.Join(stdOSFakePath(dir), fmt.Sprintf("%s%d", strings.ReplaceAll(pattern, "*", "tmp"), i+1))
			}
			rt.stdOSTest.files[name] = &stdOSFileEntry{mode: 0o600, modTime: time.Unix(0, 0).UTC()}
			rt.stdOSTest.mu.Unlock()
			value, err := stdOSOpen(rt, name, os.O_RDWR, 0o600)
			stdOSSetHandlePolicy(value, rt, "os.file_write")
			return value, err
		}
		f, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nullValue(), filesystemHostFailure("os.create_temp", dir, err)
		}
		value := stdOSHandleValue(&stdOSFileHandle{file: f, path: f.Name(), writable: true}, "os_file")
		stdOSSetHandlePolicy(value, rt, "os.file_write")
		return value, nil
	})
}

func stdOSPipeContext(rt *Runtime, ctx context.Context, _ []Value) (Value, error) {
	if err := requireStdOSCapability(rt, "os.file_read", "os.pipe"); err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.pipe", func() (Value, error) {
		if rt != nil && rt.stdOSTest != nil {
			path := "/.pipe"
			rt.stdOSTest.mu.Lock()
			rt.stdOSTest.files[path] = &stdOSFileEntry{mode: os.ModeNamedPipe | 0o600, modTime: time.Unix(0, 0).UTC()}
			rt.stdOSTest.mu.Unlock()
			r := &stdOSFileHandle{test: rt.stdOSTest, path: path}
			w := &stdOSFileHandle{test: rt.stdOSTest, path: path, writable: true}
			readValue := stdOSHandleValue(r, "os_pipe_read")
			writeValue := stdOSHandleValue(w, "os_pipe_write")
			stdOSSetHandlePolicy(readValue, rt, "os.file_read")
			stdOSSetHandlePolicy(writeValue, rt, "os.file_write")
			return tupleValue([]Value{readValue, writeValue}), nil
		}
		r, w, err := os.Pipe()
		if err != nil {
			return nullValue(), filesystemHostFailure("os.pipe", "", err)
		}
		readValue := stdOSHandleValue(&stdOSFileHandle{file: r, path: "pipe:read"}, "os_pipe_read")
		writeValue := stdOSHandleValue(&stdOSFileHandle{file: w, path: "pipe:write", writable: true}, "os_pipe_write")
		stdOSSetHandlePolicy(readValue, rt, "os.file_read")
		stdOSSetHandlePolicy(writeValue, rt, "os.file_write")
		return tupleValue([]Value{readValue, writeValue}), nil
	})
}

func stdOSWriteStringContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 2 || args[1].kind != kindString {
		return nullValue(), fmt.Errorf("write_string expects handle and string")
	}
	return stdOSFileWriteContext(rt, ctx, []Value{args[0], stringValue(args[1].str)})
}

func stdOSSyncContext(rt *Runtime, _ context.Context, args []Value) (Value, error) {
	if err := requireStdOSCapability(rt, "os.file_write", "os.sync"); err != nil {
		return nullValue(), err
	}
	h, err := stdOSHandle(args, "os.sync")
	if err != nil {
		return nullValue(), err
	}
	if err := h.Sync(); err != nil {
		return nullValue(), filesystemHostFailure("os.sync", h.path, err)
	}
	return boolValue(true), nil
}

func stdOSTruncatePathContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("truncate expects path and size")
	}
	path, err := stdOSPath(args[0], "os.truncate")
	if err != nil {
		return nullValue(), err
	}
	size, err := filesystemIntegerArgument(args[1], "os.truncate", "size")
	if err != nil {
		return nullValue(), err
	}
	if size < 0 {
		return nullValue(), fmt.Errorf("truncate size must be non-negative")
	}
	return stdOSCallContext(ctx, "os.truncate", func() (Value, error) {
		if rt != nil && rt.stdOSTest != nil {
			rt.stdOSTest.mu.Lock()
			entry := rt.stdOSTest.files[stdOSFakePath(path)]
			if entry == nil || entry.isDir {
				rt.stdOSTest.mu.Unlock()
				return nullValue(), filesystemHostFailure("os.truncate", path, os.ErrNotExist)
			}
			if int64(len(entry.data)) > size {
				entry.data = entry.data[:size]
			} else {
				entry.data = append(entry.data, make([]byte, size-int64(len(entry.data)))...)
			}
			rt.stdOSTest.mu.Unlock()
			return boolValue(true), nil
		}
		return stdOSMutatePath(rt, "os.truncate", path, func() error { return os.Truncate(path, size) })
	})
}

func stdOSLinkContext(rt *Runtime, _ context.Context, args []Value, symbolic bool) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("link expects old and new paths")
	}
	old, err := stdOSPath(args[0], "os.link")
	if err != nil {
		return nullValue(), err
	}
	next, err := stdOSPath(args[1], "os.link")
	if err != nil {
		return nullValue(), err
	}
	capability := "os.file_mutate"
	if symbolic {
		capability = "os.symlink"
	}
	if err := requireStdOSCapability(rt, capability, "os.link"); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		rt.stdOSTest.mu.Lock()
		defer rt.stdOSTest.mu.Unlock()
		if symbolic {
			rt.stdOSTest.files[stdOSFakePath(next)] = &stdOSFileEntry{mode: os.ModeSymlink | 0o777, target: old, modTime: time.Unix(0, 0).UTC()}
			return boolValue(true), nil
		}
		entry := rt.stdOSTest.files[stdOSFakePath(old)]
		if entry == nil {
			return nullValue(), filesystemHostFailure("os.link", old, os.ErrNotExist)
		}
		rt.stdOSTest.files[stdOSFakePath(next)] = entry
		return boolValue(true), nil
	}
	var hostErr error
	if symbolic {
		hostErr = os.Symlink(old, next)
	} else {
		hostErr = os.Link(old, next)
	}
	if hostErr != nil {
		return nullValue(), filesystemHostFailure("os.link", next, hostErr)
	}
	return boolValue(true), nil
}

func stdOSReadlinkContext(rt *Runtime, _ context.Context, args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("readlink expects path")
	}
	path, err := stdOSPath(args[0], "os.readlink")
	if err != nil {
		return nullValue(), err
	}
	if err := requireStdOSCapability(rt, "os.symlink", "os.readlink"); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		rt.stdOSTest.mu.RLock()
		entry := rt.stdOSTest.files[stdOSFakePath(path)]
		rt.stdOSTest.mu.RUnlock()
		if entry == nil || entry.target == "" {
			return nullValue(), filesystemHostFailure("os.readlink", path, os.ErrInvalid)
		}
		return stringValue(entry.target), nil
	}
	target, err := os.Readlink(path)
	if err != nil {
		return nullValue(), filesystemHostFailure("os.readlink", path, err)
	}
	return stringValue(target), nil
}

func stdOSWorkingDir(rt *Runtime, operation string, args []Value, change bool) (Value, error) {
	if change {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("chdir expects path")
		}
		path, err := stdOSPath(args[0], operation)
		if err != nil {
			return nullValue(), err
		}
		if err := requireStdOSCapability(rt, "os.directory_mutate", operation); err != nil {
			return nullValue(), err
		}
		if rt != nil && rt.stdOSTest != nil {
			return boolValue(true), nil
		}
		if err := os.Chdir(path); err != nil {
			return nullValue(), filesystemHostFailure(operation, path, err)
		}
		return boolValue(true), nil
	}
	if len(args) != 0 {
		return nullValue(), fmt.Errorf("getwd expects no arguments")
	}
	if err := requireStdOSCapability(rt, "os.system_identity", operation); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		return stringValue("/"), nil
	}
	path, err := os.Getwd()
	if err != nil {
		return nullValue(), filesystemHostFailure(operation, "", err)
	}
	return stringValue(path), nil
}

func stdOSUserDir(rt *Runtime, name string) (Value, error) {
	if err := requireStdOSCapability(rt, "os.system_identity", "os."+name); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		return stringValue("/home/test"), nil
	}
	var path string
	var err error
	switch name {
	case "user_home_dir":
		path, err = os.UserHomeDir()
	case "user_config_dir":
		path, err = os.UserConfigDir()
	case "user_cache_dir":
		path, err = os.UserCacheDir()
	}
	if err != nil {
		return nullValue(), filesystemHostFailure("os."+name, "", err)
	}
	return stringValue(path), nil
}

func stdOSCopyFSContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("copy_fs expects destination and source directory")
	}
	destination, err := stdOSPath(args[0], "os.copy_fs")
	if err != nil {
		return nullValue(), err
	}
	source, err := stdOSPath(args[1], "os.copy_fs")
	if err != nil {
		return nullValue(), err
	}
	if err := requireStdOSCapability(rt, "os.file_read", "os.copy_fs"); err != nil {
		return nullValue(), err
	}
	if err := requireStdOSCapability(rt, "os.directory_mutate", "os.copy_fs"); err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.copy_fs", func() (Value, error) {
		if rt != nil && rt.stdOSTest != nil {
			state := rt.stdOSTest
			sourcePath := stdOSFakePath(source)
			destinationPath := stdOSFakePath(destination)
			state.mu.Lock()
			defer state.mu.Unlock()
			sourceEntry := state.files[sourcePath]
			if sourceEntry == nil || !sourceEntry.isDir {
				return nullValue(), filesystemHostFailure("os.copy_fs", source, os.ErrNotExist)
			}
			state.files[destinationPath] = &stdOSFileEntry{mode: os.ModeDir | 0o755, isDir: true, modTime: time.Unix(0, 0).UTC()}
			paths := make([]string, 0)
			for path := range state.files {
				if path != sourcePath && strings.HasPrefix(path, strings.TrimSuffix(sourcePath, "/")+"/") {
					paths = append(paths, path)
				}
			}
			if len(paths) > 4096 {
				return nullValue(), filesystemFailure(ErrorKindResourceExhausted, "os.copy_fs", source, "copy exceeds the entry bound", nil)
			}
			sort.Strings(paths)
			var total int64
			for _, path := range paths {
				rel, _ := filepath.Rel(sourcePath, path)
				target := filepath.Join(destinationPath, rel)
				entry := state.files[path]
				if entry.isDir {
					state.files[target] = &stdOSFileEntry{mode: entry.mode, isDir: true, modTime: time.Unix(0, 0).UTC()}
					continue
				}
				total += int64(len(entry.data))
				if total > 64<<20 {
					return nullValue(), filesystemFailure(ErrorKindResourceExhausted, "os.copy_fs", source, "copy exceeds the byte bound", nil)
				}
				state.files[target] = &stdOSFileEntry{data: append([]byte(nil), entry.data...), mode: entry.mode, modTime: time.Unix(0, 0).UTC(), target: entry.target}
			}
			return boolValue(true), nil
		}
		if err := os.MkdirAll(destination, 0o755); err != nil {
			return nullValue(), filesystemHostFailure("os.copy_fs", destination, err)
		}
		var total int64
		var entries int
		err := filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			entries++
			if entries > 4096 {
				return fmt.Errorf("copy exceeds the entry bound")
			}
			rel, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			target := filepath.Join(destination, rel)
			if info.IsDir() {
				return os.MkdirAll(target, info.Mode().Perm())
			}
			if info.Size() > stdOSMaxRead || total+info.Size() > 64<<20 {
				return fmt.Errorf("copy exceeds the byte bound")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			total += int64(len(data))
			return os.WriteFile(target, data, info.Mode().Perm())
		})
		if err != nil {
			return nullValue(), filesystemHostFailure("os.copy_fs", source, err)
		}
		return boolValue(true), nil
	})
}

func stdOSOpenRoot(rt *Runtime, args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("open_root expects path")
	}
	path, err := stdOSPath(args[0], "os.open_root")
	if err != nil {
		return nullValue(), err
	}
	if err := requireStdOSCapability(rt, "os.root", "os.open_root"); err != nil {
		return nullValue(), err
	}
	rootHandle := &stdOSRootHandle{}
	if rt != nil && rt.stdOSTest != nil {
		rootPath := stdOSFakePath(path)
		rt.stdOSTest.mu.RLock()
		entry := rt.stdOSTest.files[rootPath]
		rt.stdOSTest.mu.RUnlock()
		if entry == nil || !entry.isDir {
			return nullValue(), filesystemHostFailure("os.open_root", path, os.ErrNotExist)
		}
		return mapValueOf([]mapEntry{{stringValue("root"), stringValue(rootPath)}, {stringValue("__root_handle"), streamValueOf(&streamValue{id: "root:" + rootPath, path: rootPath, mode: "os_root", closer: rootHandle})}}), nil
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nullValue(), filesystemHostFailure("os.open_root", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nullValue(), filesystemHostFailure("os.open_root", path, err)
	}
	if !info.IsDir() {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, "os.open_root", path, "root must be a directory", nil)
	}
	return mapValueOf([]mapEntry{{stringValue("root"), stringValue(filepath.Clean(path))}, {stringValue("__root_handle"), streamValueOf(&streamValue{id: "root:" + path, path: path, mode: "os_root", closer: rootHandle})}}), nil
}

func stdOSReadDirEntriesContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("read_dir_entries expects path")
	}
	path, err := stdOSPath(args[0], "os.read_dir_entries")
	if err != nil {
		return nullValue(), err
	}
	if err := requireStdOSCapability(rt, "os.directory_read", "os.read_dir_entries"); err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.read_dir_entries", func() (Value, error) {
		entries := make([]Value, 0)
		if rt != nil && rt.stdOSTest != nil {
			root := stdOSFakePath(path)
			prefix := strings.TrimSuffix(root, "/") + "/"
			rt.stdOSTest.mu.RLock()
			for name, entry := range rt.stdOSTest.files {
				if strings.HasPrefix(name, prefix) && !strings.Contains(strings.TrimPrefix(name, prefix), "/") && name != root {
					info := stdOSFakeInfo(filepath.Base(name), entry)
					entries = append(entries, stdOSStatValue(info))
				}
			}
			rt.stdOSTest.mu.RUnlock()
		} else {
			hostEntries, e := os.ReadDir(path)
			if e != nil {
				return nullValue(), filesystemHostFailure("os.read_dir_entries", path, e)
			}
			for _, entry := range hostEntries {
				info, e := entry.Info()
				if e != nil {
					return nullValue(), filesystemHostFailure("os.read_dir_entries", path, e)
				}
				entries = append(entries, stdOSStatValue(info))
			}
		}
		sort.Slice(entries, func(i, j int) bool {
			a, _ := mapFieldValue(entries[i], "name")
			b, _ := mapFieldValue(entries[j], "name")
			return a.str < b.str
		})
		return listValue(entries), nil
	})
}

func stdOSMkdirTempContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nullValue(), fmt.Errorf("mkdir_temp expects directory and optional pattern")
	}
	dir, err := stdOSPath(args[0], "os.mkdir_temp")
	if err != nil {
		return nullValue(), err
	}
	pattern := "osiris-*"
	if len(args) == 2 {
		if args[1].kind != kindString {
			return nullValue(), fmt.Errorf("mkdir_temp pattern must be a string")
		}
		pattern = args[1].str
	}
	if err := requireStdOSCapability(rt, "os.directory_mutate", "os.mkdir_temp"); err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.mkdir_temp", func() (Value, error) {
		if rt != nil && rt.stdOSTest != nil {
			path := filepath.Join(stdOSFakePath(dir), strings.ReplaceAll(pattern, "*", "tmp"))
			if _, e := stdOSFakeMkdir(rt.stdOSTest, path, 0o700, false); e != nil {
				return nullValue(), e
			}
			return stringValue(path), nil
		}
		path, e := os.MkdirTemp(dir, pattern)
		if e != nil {
			return nullValue(), filesystemHostFailure("os.mkdir_temp", dir, e)
		}
		return stringValue(path), nil
	})
}

func stdOSClearenv(rt *Runtime, _ context.Context, args []Value) (Value, error) {
	if len(args) != 0 {
		return nullValue(), fmt.Errorf("clearenv expects no arguments")
	}
	if err := requireStdOSCapability(rt, "os.environment_mutate", "os.clearenv"); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		rt.stdOSTest.mu.Lock()
		clear(rt.stdOSTest.env)
		rt.stdOSTest.mu.Unlock()
		return boolValue(true), nil
	}
	os.Clearenv()
	return boolValue(true), nil
}

func stdOSPathUtility(rt *Runtime, name string, args []Value) (Value, error) {
	if len(args) != 0 {
		return nullValue(), fmt.Errorf("%s expects no arguments", name)
	}
	if err := requireStdOSCapability(rt, "os.system_identity", "os."+name); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		if name == "temp_dir" {
			return stringValue("/tmp"), nil
		}
		return stringValue("/usr/bin/osiris-test"), nil
	}
	if name == "temp_dir" {
		return stringValue(os.TempDir()), nil
	}
	path, err := os.Executable()
	if err != nil {
		return nullValue(), filesystemHostFailure("os.executable", "", err)
	}
	return stringValue(path), nil
}

func stdOSPathPredicate(rt *Runtime, args []Value, predicate func(os.FileInfo) bool) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("path predicate expects path")
	}
	path, err := stdOSPath(args[0], "os.path_predicate")
	if err != nil {
		return nullValue(), err
	}
	if err := requireStdOSCapability(rt, "os.file_metadata", "os.path_predicate"); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		rt.stdOSTest.mu.RLock()
		entry := rt.stdOSTest.files[stdOSFakePath(path)]
		rt.stdOSTest.mu.RUnlock()
		if entry == nil {
			return boolValue(false), nil
		}
		return boolValue(predicate(stdOSFakeInfo(filepath.Base(path), entry))), nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return boolValue(false), nil
		}
		return nullValue(), filesystemHostFailure("os.path_predicate", path, err)
	}
	return boolValue(predicate(info)), nil
}

func stdOSMissingPath(rt *Runtime, args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("is_not_exist expects path")
	}
	path, err := stdOSPath(args[0], "os.is_not_exist")
	if err != nil {
		return nullValue(), err
	}
	if err := requireStdOSCapability(rt, "os.file_metadata", "os.is_not_exist"); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		rt.stdOSTest.mu.RLock()
		_, ok := rt.stdOSTest.files[stdOSFakePath(path)]
		rt.stdOSTest.mu.RUnlock()
		return boolValue(!ok), nil
	}
	_, err = os.Stat(path)
	return boolValue(errors.Is(err, os.ErrNotExist)), nil
}

func stdOSSameFile(rt *Runtime, args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("same_file expects two paths")
	}
	left, err := stdOSPath(args[0], "os.same_file")
	if err != nil {
		return nullValue(), err
	}
	right, err := stdOSPath(args[1], "os.same_file")
	if err != nil {
		return nullValue(), err
	}
	if err := requireStdOSCapability(rt, "os.file_metadata", "os.same_file"); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		rt.stdOSTest.mu.RLock()
		a := rt.stdOSTest.files[stdOSFakePath(left)]
		b := rt.stdOSTest.files[stdOSFakePath(right)]
		rt.stdOSTest.mu.RUnlock()
		return boolValue(a != nil && a == b), nil
	}
	a, err := os.Stat(left)
	if err != nil {
		return nullValue(), filesystemHostFailure("os.same_file", left, err)
	}
	b, err := os.Stat(right)
	if err != nil {
		return nullValue(), filesystemHostFailure("os.same_file", right, err)
	}
	return boolValue(os.SameFile(a, b)), nil
}

func stdOSStandardStream(rt *Runtime, name string) Value {
	if rt != nil && rt.stdOSTest != nil {
		path := "/dev/" + name
		rt.stdOSTest.mu.Lock()
		if rt.stdOSTest.files[path] == nil {
			rt.stdOSTest.files[path] = &stdOSFileEntry{mode: 0o600, modTime: time.Unix(0, 0).UTC()}
		}
		rt.stdOSTest.mu.Unlock()
		capability := "os.file_write"
		if name == "stdin" {
			capability = "os.file_read"
		}
		return stdOSHandleValue(&stdOSFileHandle{test: rt.stdOSTest, path: path, writable: name != "stdin", borrowed: true, runtime: rt, capability: capability}, "os_standard")
	}
	file := os.Stdin
	if name == "stdout" {
		file = os.Stdout
	}
	if name == "stderr" {
		file = os.Stderr
	}
	capability := "os.file_write"
	if name == "stdin" {
		capability = "os.file_read"
	}
	return stdOSHandleValue(&stdOSFileHandle{file: file, path: "/dev/" + name, writable: name != "stdin", borrowed: true, runtime: rt, capability: capability}, "os_standard")
}

func stdOSFd(rt *Runtime, args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("fd expects a file handle")
	}
	h, err := stdOSHandle(args, "os.fd")
	if err != nil {
		return nullValue(), err
	}
	if err := requireStdOSCapability(rt, "os.process_raw_handle", "os.fd"); err != nil {
		return nullValue(), err
	}
	if h.file == nil {
		return uintValue(0, 64), nil
	}
	return uintValue(uint64(h.file.Fd()), 64), nil
}

func stdOSFileName(rt *Runtime, args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("name expects a file handle")
	}
	h, err := stdOSHandle(args, "os.name")
	if err != nil {
		return nullValue(), err
	}
	if err := h.check(false); err != nil {
		return nullValue(), filesystemHostFailure("os.name", h.path, err)
	}
	return stringValue(h.path), nil
}

// stdOSUnsupportedOperation is used for Go os interfaces that cannot be
// represented safely as Osiris values. It still performs the capability check
// first, so a denied operation never reveals whether the host supports it.
func stdOSUnsupportedOperation(rt *Runtime, operation, capability string, args []Value) (Value, error) {
	if err := requireStdOSCapability(rt, capability, operation); err != nil {
		return nullValue(), err
	}
	return nullValue(), osFailure(ErrorKindUnsupported, operation, fmt.Errorf("%s is intentionally unavailable in the portable std/os boundary", operation))
}

func stdOSDirFS(rt *Runtime, args []Value) (Value, error) {
	return stdOSOpenRoot(rt, args)
}

func stdOSIsPathSeparator(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("is_path_separator expects one byte value")
	}
	value, err := filesystemIntegerArgument(args[0], "os.is_path_separator", "value")
	if err != nil {
		return nullValue(), err
	}
	return boolValue(value == int64(filepath.Separator) || value == '/' || value == '\\'), nil
}

func stdOSTransferFileContext(rt *Runtime, ctx context.Context, args []Value, writeTo bool) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("file transfer expects two handles")
	}
	if err := requireStdOSCapability(rt, "os.file_read", "os.file_transfer"); err != nil {
		return nullValue(), err
	}
	if writeTo {
		if err := requireStdOSCapability(rt, "os.file_write", "os.write_to"); err != nil {
			return nullValue(), err
		}
	} else if err := requireStdOSCapability(rt, "os.file_write", "os.read_from"); err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.file_transfer", func() (Value, error) {
		var source *stdOSFileHandle
		var destination *stdOSFileHandle
		if writeTo {
			source, _ = stdOSHandle(args, "os.write_to")
			destination, _ = stdOSHandle([]Value{args[1]}, "os.write_to")
		} else {
			source, _ = stdOSHandle([]Value{args[1]}, "os.read_from")
			destination, _ = stdOSHandle(args, "os.read_from")
		}
		if source == nil || destination == nil {
			return nullValue(), fmt.Errorf("file transfer expects os file handles")
		}
		buffer := make([]byte, 32<<10)
		total := 0
		for total < stdOSMaxRead {
			n, e := source.Read(buffer)
			if n > 0 {
				written, we := destination.Write(buffer[:n])
				total += written
				if we != nil {
					return nullValue(), filesystemHostFailure("os.file_transfer", destination.path, we)
				}
				if written != n {
					return nullValue(), io.ErrShortWrite
				}
			}
			if e == io.EOF {
				break
			}
			if e != nil {
				return nullValue(), filesystemHostFailure("os.file_transfer", source.path, e)
			}
		}
		return uintValue(uint64(total), 64), nil
	})
}

func stdOSChmodContext(rt *Runtime, _ context.Context, args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("chmod expects path and mode")
	}
	path, err := stdOSPath(args[0], "os.chmod")
	if err != nil {
		return nullValue(), err
	}
	mode, err := filesystemIntegerArgument(args[1], "os.chmod", "mode")
	if err != nil {
		return nullValue(), err
	}
	if err := requireStdOSCapability(rt, "os.file_mutate", "os.chmod"); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		rt.stdOSTest.mu.Lock()
		entry := rt.stdOSTest.files[stdOSFakePath(path)]
		if entry == nil {
			rt.stdOSTest.mu.Unlock()
			return nullValue(), os.ErrNotExist
		}
		entry.mode = (entry.mode &^ os.ModePerm) | os.FileMode(mode)&os.ModePerm
		rt.stdOSTest.mu.Unlock()
		return boolValue(true), nil
	}
	if err := os.Chmod(path, os.FileMode(mode)); err != nil {
		return nullValue(), filesystemHostFailure("os.chmod", path, err)
	}
	return boolValue(true), nil
}

func stdOSChownContext(rt *Runtime, _ context.Context, args []Value, link bool) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("chown expects path, uid, and gid")
	}
	path, err := stdOSPath(args[0], "os.chown")
	if err != nil {
		return nullValue(), err
	}
	uid, err := filesystemIntegerArgument(args[1], "os.chown", "uid")
	if err != nil {
		return nullValue(), err
	}
	gid, err := filesystemIntegerArgument(args[2], "os.chown", "gid")
	if err != nil {
		return nullValue(), err
	}
	if err := requireStdOSCapability(rt, "os.file_mutate", "os.chown"); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		return boolValue(true), nil
	}
	err = stdOSChown(path, int(uid), int(gid), link)
	if err != nil {
		return nullValue(), filesystemHostFailure("os.chown", path, err)
	}
	return boolValue(true), nil
}

func stdOSChtimesContext(rt *Runtime, _ context.Context, args []Value) (Value, error) {
	if len(args) != 3 || args[1].kind != kindString || args[2].kind != kindString {
		return nullValue(), fmt.Errorf("chtimes expects path and RFC3339 times")
	}
	path, err := stdOSPath(args[0], "os.chtimes")
	if err != nil {
		return nullValue(), err
	}
	atime, err := time.Parse(time.RFC3339Nano, args[1].str)
	if err != nil {
		return nullValue(), err
	}
	mtime, err := time.Parse(time.RFC3339Nano, args[2].str)
	if err != nil {
		return nullValue(), err
	}
	if err := requireStdOSCapability(rt, "os.file_mutate", "os.chtimes"); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		rt.stdOSTest.mu.Lock()
		if entry := rt.stdOSTest.files[stdOSFakePath(path)]; entry != nil {
			entry.modTime = mtime
			_ = atime
		}
		rt.stdOSTest.mu.Unlock()
		return boolValue(true), nil
	}
	if err := os.Chtimes(path, atime, mtime); err != nil {
		return nullValue(), filesystemHostFailure("os.chtimes", path, err)
	}
	return boolValue(true), nil
}

func stdOSProcessReleaseContext(rt *Runtime, _ context.Context, args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindOSProcess || args[0].process == nil {
		return nullValue(), fmt.Errorf("process_release expects a process")
	}
	if err := requireStdOSCapability(rt, "os.process_inspect", "os.process_release"); err != nil {
		return nullValue(), err
	}
	if err := args[0].process.close(); err != nil {
		return nullValue(), osFailure(ErrorKindIO, "os.process_release", err)
	}
	return boolValue(true), nil
}

func stdOSProcessStateContext(rt *Runtime, _ context.Context, args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindOSProcess || args[0].process == nil {
		return nullValue(), fmt.Errorf("process_state expects a process")
	}
	if err := requireStdOSCapability(rt, "os.process_inspect", "os.process_state"); err != nil {
		return nullValue(), err
	}
	return processResultValue(args[0].process), nil
}

func stdOSFindProcessContext(rt *Runtime, _ context.Context, args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("find_process expects pid")
	}
	pid, err := filesystemIntegerArgument(args[0], "os.find_process", "pid")
	if err != nil {
		return nullValue(), err
	}
	if err := requireStdOSCapability(rt, "os.process_inspect", "os.find_process"); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		return osProcessValueOf(&osProcess{pidValue: int(pid), done: make(chan struct{})}), nil
	}
	p, err := os.FindProcess(int(pid))
	if err != nil {
		return nullValue(), osFailure(ErrorKindNotFound, "os.find_process", err)
	}
	return osProcessValueOf(&osProcess{command: &exec.Cmd{Process: p}, pidValue: int(pid), done: make(chan struct{})}), nil
}

func stdOSSignalProcessContext(rt *Runtime, _ context.Context, args []Value) (Value, error) {
	if len(args) != 2 || args[0].kind != kindOSProcess {
		return nullValue(), fmt.Errorf("process_signal expects process and signal")
	}
	if err := requireStdOSCapability(rt, "os.process_signal", "os.process_signal"); err != nil {
		return nullValue(), err
	}
	if rt != nil && rt.stdOSTest != nil {
		return boolValue(true), nil
	}
	signalNumber, err := args[1].Int()
	if err != nil {
		return nullValue(), err
	}
	if args[0].process.command == nil || args[0].process.command.Process == nil {
		return nullValue(), osFailure(ErrorKindUnavailable, "os.process_signal", syscall.EBADF)
	}
	if err := args[0].process.command.Process.Signal(syscall.Signal(signalNumber)); err != nil {
		return nullValue(), osFailure(ErrorKindIO, "os.process_signal", err)
	}
	return boolValue(true), nil
}

func stdOSRootPath(root Value, name Value, operation string) (string, error) {
	if _, err := stdOSRootHandleFromValue(root); err != nil {
		return "", filesystemFailure(ErrorKindIO, operation, "", "root is closed", err)
	}
	base, ok := mapFieldValue(root, "root")
	if !ok || base.kind != kindString {
		return "", fmt.Errorf("%s expects a root handle", operation)
	}
	rel, err := stdOSPath(name, operation)
	if err != nil {
		return "", err
	}
	rel = filepath.Clean(rel)
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", filesystemFailure(ErrorKindPermissionDenied, operation, rel, "root path must be relative and confined", nil)
	}
	path := filepath.Join(base.str, rel)
	cleanBase := filepath.Clean(base.str)
	cleanPath := filepath.Clean(path)
	if cleanPath != cleanBase && !strings.HasPrefix(cleanPath, cleanBase+string(filepath.Separator)) {
		return "", filesystemFailure(ErrorKindPermissionDenied, operation, rel, "root path escapes its root", nil)
	}
	return cleanPath, nil
}

func stdOSRootOpenContext(rt *Runtime, ctx context.Context, args []Value, create bool) (Value, error) {
	if len(args) != 2 {
		if create {
			return nullValue(), fmt.Errorf("root_create expects root and path")
		}
		return nullValue(), fmt.Errorf("root_open expects root and path")
	}
	root, err := stdOSRootHandleFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	path, err := stdOSRootPath(args[0], args[1], "os.root_open")
	if err != nil {
		return nullValue(), err
	}
	capability := "os.file_read"
	flags := os.O_RDONLY
	perm := os.FileMode(0)
	if create {
		capability = "os.file_write"
		flags = os.O_CREATE | os.O_RDWR | os.O_TRUNC
		perm = 0o644
	}
	if err := requireStdOSCapability(rt, capability, "os.root_open"); err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.root_open", func() (Value, error) {
		value, e := stdOSOpen(rt, path, flags, perm)
		if e == nil && value.stream != nil && value.stream.stdFile != nil {
			value.stream.stdFile.root = root
			value.stream.stdFile.runtime = rt
			value.stream.stdFile.capability = capability
		}
		return value, e
	})
}

func stdOSRootOpenFileContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 4 {
		return nullValue(), fmt.Errorf("root_open_file expects root, path, flags, and mode")
	}
	flags, err := filesystemIntegerArgument(args[2], "os.root_open_file", "flags")
	if err != nil {
		return nullValue(), err
	}
	if err := stdOSValidateFlags(flags); err != nil {
		return nullValue(), err
	}
	mode, err := filesystemIntegerArgument(args[3], "os.root_open_file", "mode")
	if err != nil {
		return nullValue(), err
	}
	root, err := stdOSRootHandleFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	path, err := stdOSRootPath(args[0], args[1], "os.root_open_file")
	if err != nil {
		return nullValue(), err
	}
	capability := "os.file_read"
	if flags&(int64(os.O_WRONLY)|int64(os.O_RDWR)) != 0 {
		capability = "os.file_write"
	}
	if err := requireStdOSCapability(rt, capability, "os.root_open_file"); err != nil {
		return nullValue(), err
	}
	return stdOSCallContext(ctx, "os.root_open_file", func() (Value, error) {
		value, e := stdOSOpen(rt, path, int(flags), os.FileMode(mode))
		if e == nil && value.stream != nil && value.stream.stdFile != nil {
			value.stream.stdFile.root = root
			value.stream.stdFile.runtime = rt
			value.stream.stdFile.capability = capability
		}
		return value, e
	})
}

func stdOSRootReadFileContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("root_read_file expects root and path")
	}
	path, err := stdOSRootPath(args[0], args[1], "os.root_read_file")
	if err != nil {
		return nullValue(), err
	}
	return stdOSReadFileContext(rt, ctx, []Value{stringValue(path)})
}
func stdOSRootWriteFileContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 4 {
		return nullValue(), fmt.Errorf("root_write_file expects root, path, data, and mode")
	}
	path, err := stdOSRootPath(args[0], args[1], "os.root_write_file")
	if err != nil {
		return nullValue(), err
	}
	return stdOSWriteFileContext(rt, ctx, []Value{stringValue(path), args[2], args[3]})
}
func stdOSRootStatContext(rt *Runtime, ctx context.Context, args []Value, follow bool) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("root_stat expects root and path")
	}
	path, err := stdOSRootPath(args[0], args[1], "os.root_stat")
	if err != nil {
		return nullValue(), err
	}
	return stdOSStatContext(rt, ctx, []Value{stringValue(path)}, follow)
}
func stdOSRootReadlinkContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("root_readlink expects root and path")
	}
	path, err := stdOSRootPath(args[0], args[1], "os.root_readlink")
	if err != nil {
		return nullValue(), err
	}
	return stdOSReadlinkContext(rt, ctx, []Value{stringValue(path)})
}
func stdOSRootMkdirContext(rt *Runtime, ctx context.Context, args []Value, all bool) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("root_mkdir expects root, path, and mode")
	}
	path, err := stdOSRootPath(args[0], args[1], "os.root_mkdir")
	if err != nil {
		return nullValue(), err
	}
	return stdOSMkdirContext(rt, ctx, []Value{stringValue(path), args[2]}, all)
}
func stdOSRootRemoveContext(rt *Runtime, ctx context.Context, args []Value, all bool) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("root_remove expects root and path")
	}
	path, err := stdOSRootPath(args[0], args[1], "os.root_remove")
	if err != nil {
		return nullValue(), err
	}
	return stdOSRemoveContext(rt, ctx, []Value{stringValue(path)}, all)
}
func stdOSRootClose(_ *Runtime, _ context.Context, args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("root_close expects root")
	}
	root, err := stdOSRootHandleFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	if err := root.Close(); err != nil {
		return boolValue(false), nil
	}
	return boolValue(true), nil
}
func stdOSRootFS(root Value) (Value, error) {
	base, ok := mapFieldValue(root, "root")
	if !ok {
		return nullValue(), fmt.Errorf("root_fs expects a root")
	}
	if _, err := stdOSRootHandleFromValue(root); err != nil {
		return nullValue(), err
	}
	return mapValueOf([]mapEntry{{key: stringValue("root"), value: base}}), nil
}

func stdOSRootTwoPathContext(rt *Runtime, ctx context.Context, args []Value, operation string, symbolic bool) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("%s expects root, old path, and new path", operation)
	}
	old, err := stdOSRootPath(args[0], args[1], operation)
	if err != nil {
		return nullValue(), err
	}
	next, err := stdOSRootPath(args[0], args[2], operation)
	if err != nil {
		return nullValue(), err
	}
	return stdOSLinkContext(rt, ctx, []Value{stringValue(old), stringValue(next)}, symbolic)
}
func stdOSRootRenameContext(rt *Runtime, _ context.Context, args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("root_rename expects root, old path, and new path")
	}
	old, err := stdOSRootPath(args[0], args[1], "os.root_rename")
	if err != nil {
		return nullValue(), err
	}
	next, err := stdOSRootPath(args[0], args[2], "os.root_rename")
	if err != nil {
		return nullValue(), err
	}
	return stdOSMutatePath(rt, "os.root_rename", old, func() error {
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
}
func stdOSRootChmodContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("root_chmod expects root, path, and mode")
	}
	path, err := stdOSRootPath(args[0], args[1], "os.root_chmod")
	if err != nil {
		return nullValue(), err
	}
	return stdOSChmodContext(rt, ctx, []Value{stringValue(path), args[2]})
}
func stdOSRootChownContext(rt *Runtime, ctx context.Context, args []Value, link bool) (Value, error) {
	if len(args) != 4 {
		return nullValue(), fmt.Errorf("root_chown expects root, path, uid, and gid")
	}
	path, err := stdOSRootPath(args[0], args[1], "os.root_chown")
	if err != nil {
		return nullValue(), err
	}
	return stdOSChownContext(rt, ctx, []Value{stringValue(path), args[2], args[3]}, link)
}
func stdOSRootChtimesContext(rt *Runtime, ctx context.Context, args []Value) (Value, error) {
	if len(args) != 4 {
		return nullValue(), fmt.Errorf("root_chtimes expects root, path, access time, and modification time")
	}
	path, err := stdOSRootPath(args[0], args[1], "os.root_chtimes")
	if err != nil {
		return nullValue(), err
	}
	return stdOSChtimesContext(rt, ctx, []Value{stringValue(path), args[2], args[3]})
}

// Keep the imported package visible on platforms where the host adapter is
// built without a user database lookup in the selected test path.
var _ = io.EOF
