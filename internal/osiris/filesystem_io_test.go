package osiris

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestFilesystemIOInterpreterVMParity(t *testing.T) {
	runners := []struct {
		name string
		run  func(context.Context, *Program) ([]Value, error)
	}{
		{name: "interpreter", run: NewRuntime().Run},
		{name: "vm", run: NewRuntime().RunVM},
	}
	for _, runner := range runners {
		t.Run(runner.name, func(t *testing.T) {
			root := t.TempDir()
			emptyDir := filepath.Join(root, "a-dir")
			nestedDir := filepath.Join(root, "b-dir", "inner")
			sourcePath := filepath.Join(root, "c-file.bin")
			copyPath := filepath.Join(root, "d-copy.bin")
			renamedPath := filepath.Join(root, "e-renamed.bin")
			source := `
emit mkdir(` + strconv.Quote(emptyDir) + `)
emit mkdir_all(` + strconv.Quote(nestedDir) + `)
emit write(` + strconv.Quote(sourcePath) + `, "abc")
emit append(` + strconv.Quote(sourcePath) + `, [0u8, 255u8])
emit exists(` + strconv.Quote(sourcePath) + `)
emit file_size(` + strconv.Quote(sourcePath) + `)
emit is_file(` + strconv.Quote(sourcePath) + `)
emit is_dir(` + strconv.Quote(sourcePath) + `)
emit is_dir(` + strconv.Quote(emptyDir) + `)
emit read_limit(` + strconv.Quote(sourcePath) + `, 4)
emit read_dir(` + strconv.Quote(root) + `, 16)
emit copy_file(` + strconv.Quote(sourcePath) + `, ` + strconv.Quote(copyPath) + `)
emit truncate_file(` + strconv.Quote(copyPath) + `, 3)
emit rename(` + strconv.Quote(copyPath) + `, ` + strconv.Quote(renamedPath) + `)
emit file_size(` + strconv.Quote(renamedPath) + `)
let output = openappend(` + strconv.Quote(sourcePath) + `)
emit tell(output)
emit writechunk(output, "Z")
emit flush(output)
emit tell(output)
emit truncate(output, 4)
emit tell(output)
emit seek(output, -1, "end")
emit close(output)
emit close(output)
let input = openread(` + strconv.Quote(renamedPath) + `)
emit tell(input)
emit seek(input, 1, "start")
emit readchunk(input, 1)
emit tell(input)
emit seek(input, 0, "current")
emit seek(input, -1, "end")
emit readchunk(input, 4)
emit readchunk(input, 1)
emit close(input)
emit remove(` + strconv.Quote(renamedPath) + `)
emit exists(` + strconv.Quote(renamedPath) + `)
emit remove(` + strconv.Quote(emptyDir) + `)
`
			prog, err := Parse("<filesystem-parity>", source)
			if err != nil {
				t.Fatal(err)
			}
			output, err := runner.run(context.Background(), prog)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, len(output))
			for index := range output {
				got[index] = output[index].String()
			}
			want := []string{
				"true", "true", "true", "2u64", "true", "5u64", "true", "false", "true",
				"[97u8, 98u8, 99u8, 0u8]",
				`["a-dir", "b-dir", "c-file.bin"]`,
				"5u64", "true", "true", "3u64",
				"5u64", "1u64", "true", "6u64", "true", "6u64", "3u64", "true", "false",
				"0u64", "1u64", "[98u8]", "2u64", "2u64", "2u64", "[99u8]", "null", "true",
				"true", "false", "true",
			}
			if len(got) != len(want) {
				t.Fatalf("outputs = %d, want %d\n%s", len(got), len(want), strings.Join(got, "\n"))
			}
			for index := range want {
				if got[index] != want[index] {
					t.Fatalf("output[%d] = %s, want %s\nall:\n%s", index, got[index], want[index], strings.Join(got, "\n"))
				}
			}
			data, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, []byte{'a', 'b', 'c', 0}) {
				t.Fatalf("source bytes = %v, want [97 98 99 0]", data)
			}
		})
	}
}

func TestFilesystemIOErrorsBoundsAndDeterminism(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "missing")
	for _, call := range []struct {
		name string
		fn   func() (Value, error)
	}{
		{name: "exists", fn: func() (Value, error) { return existsValue([]Value{stringValue(missing)}) }},
		{name: "is_file", fn: func() (Value, error) { return isFileValue([]Value{stringValue(missing)}) }},
		{name: "is_dir", fn: func() (Value, error) { return isDirValue([]Value{stringValue(missing)}) }},
	} {
		value, err := call.fn()
		if err != nil || value.Truthy() {
			t.Fatalf("%s(missing) = %s, %v; want false", call.name, value.String(), err)
		}
	}

	_, err := fileSizeValue([]Value{stringValue(missing)})
	assertFilesystemError(t, err, ErrorKindNotFound, "file_size", missing)
	if err == nil || strings.Contains(err.Error(), "no such file") {
		t.Fatalf("public error leaks host wording: %v", err)
	}
	var record *OsirisError
	if !errors.As(err, &record) || len(record.debugMetadata) == 0 {
		t.Fatalf("filesystem error debug metadata = %#v, want private native code", record)
	}
	redacted := record.Redacted()
	if len(redacted.debugMetadata) != 0 {
		t.Fatalf("redacted debug metadata = %#v, want empty", redacted.debugMetadata)
	}

	_, err = readLimitContextValue(context.Background(), []Value{stringValue(missing), uintValue(1, 64)})
	assertFilesystemError(t, err, ErrorKindNotFound, "read_limit", missing)
	_, err = readLimitContextValue(context.Background(), []Value{stringValue(missing), uintValue(filesystemMaxReadBytes+1, 64)})
	assertFilesystemError(t, err, ErrorKindResourceExhausted, "read_limit", missing)
	_, err = readDirContextValue(context.Background(), []Value{stringValue(root), uintValue(filesystemMaxDirectoryEntries+1, 64)})
	assertFilesystemError(t, err, ErrorKindResourceExhausted, "read_dir", root)

	for _, name := range []string{"zeta", "alpha", "middle"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	value, err := readDirContextValue(context.Background(), []Value{stringValue(root), uintValue(3, 64)})
	if err != nil || value.String() != `["alpha", "middle", "zeta"]` {
		t.Fatalf("read_dir = %s, %v", value.String(), err)
	}
	if err := os.WriteFile(filepath.Join(root, "fourth"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = readDirContextValue(context.Background(), []Value{stringValue(root), uintValue(3, 64)})
	assertFilesystemError(t, err, ErrorKindResourceExhausted, "read_dir", root)

	_, err = mkdirValue([]Value{stringValue(filepath.Join(root, "alpha"))})
	assertFilesystemError(t, err, ErrorKindAlreadyExists, "mkdir", filepath.Join(root, "alpha"))
	_, err = mkdirValue([]Value{stringValue(filepath.Join(root, "alpha", "child"))})
	assertFilesystemError(t, err, ErrorKindInvalidArgument, "mkdir", filepath.Join(root, "alpha", "child"))
}

func TestFilesystemIOSymlinkAndExplicitRemoval(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated Windows privileges")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	link := filepath.Join(root, "link.txt")
	if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	size, err := fileSizeValue([]Value{stringValue(link)})
	if err != nil || size.String() != "6u64" {
		t.Fatalf("file_size(symlink) = %s, %v", size.String(), err)
	}
	if _, err := truncateFileValue([]Value{stringValue(link), uintValue(3, 64)}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "tar" {
		t.Fatalf("target after truncate symlink = %q, %v", got, err)
	}
	if _, err := removeValue([]Value{stringValue(link)}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("link still exists: %v", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "tar" {
		t.Fatalf("remove followed symlink: target = %q, %v", got, err)
	}

	nonempty := filepath.Join(root, "nonempty")
	if err := os.Mkdir(nonempty, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nonempty, "kept"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := removeValue([]Value{stringValue(nonempty)}); err == nil {
		t.Fatal("remove recursively removed a non-empty directory")
	}
	if _, err := os.Stat(filepath.Join(nonempty, "kept")); err != nil {
		t.Fatalf("explicit child was unexpectedly removed: %v", err)
	}
}

func TestSeekTellOriginsEOFAndClosedStream(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seek.bin")
	if err := os.WriteFile(path, []byte("abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	stream, err := openReadStream(path)
	if err != nil {
		t.Fatal(err)
	}
	if value, err := stream.readChunk(2); err != nil || value.String() != "[97u8, 98u8]" {
		t.Fatalf("read = %s, %v", value.String(), err)
	}
	if position, err := stream.tell(); err != nil || position != 2 {
		t.Fatalf("tell = %d, %v; want 2", position, err)
	}
	for _, step := range []struct {
		offset int64
		origin string
		want   int64
	}{
		{offset: 1, origin: "current", want: 3},
		{offset: -1, origin: "end", want: 5},
		{offset: 0, origin: "start", want: 0},
		{offset: 3, origin: "end", want: 9},
	} {
		if got, err := stream.seek(step.offset, step.origin); err != nil || got != step.want {
			t.Fatalf("seek(%d, %q) = %d, %v; want %d", step.offset, step.origin, got, err, step.want)
		}
	}
	if value, err := stream.readChunk(1); !errors.Is(err, io.EOF) || value.kind != kindNull {
		t.Fatalf("read past EOF = %s, %v; want EOF", value.String(), err)
	}
	if _, err := stream.seek(-1, "start"); err == nil {
		t.Fatal("negative absolute seek unexpectedly succeeded")
	}
	if _, err := stream.seek(0, "middle"); err == nil {
		t.Fatal("unknown seek origin unexpectedly succeeded")
	}
	if _, err := stream.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.tell(); err == nil {
		t.Fatal("tell on a closed stream unexpectedly succeeded")
	}
	if _, err := stream.seek(0, "start"); err == nil {
		t.Fatal("seek on a closed stream unexpectedly succeeded")
	}
}

type shortWriter struct {
	max  int
	data bytes.Buffer
	zero bool
}

func (writer *shortWriter) Write(data []byte) (int, error) {
	if writer.zero {
		return 0, nil
	}
	if len(data) > writer.max {
		data = data[:writer.max]
	}
	return writer.data.Write(data)
}

type cancelAfterReader struct {
	reader io.Reader
	cancel context.CancelFunc
}

func (reader *cancelAfterReader) Read(data []byte) (int, error) {
	count, err := reader.reader.Read(data)
	reader.cancel()
	return count, err
}

func TestCopyFileLoopShortWritesBoundsAndCancellation(t *testing.T) {
	writer := &shortWriter{max: 2}
	count, err := copyFileContentsContext(context.Background(), bytes.NewBufferString("abcdef"), writer)
	if err != nil || count != 6 || writer.data.String() != "abcdef" {
		t.Fatalf("short-write copy = %d, %q, %v", count, writer.data.String(), err)
	}
	zero := &shortWriter{zero: true}
	if _, err := copyFileContentsContext(context.Background(), bytes.NewBufferString("a"), zero); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("zero write error = %v, want io.ErrShortWrite", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelAfterReader{reader: bytes.NewBuffer(bytes.Repeat([]byte{'x'}, filesystemCopyBufferBytes*2)), cancel: cancel}
	if _, err := copyFileContentsContext(ctx, reader, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled copy error = %v, want context.Canceled", err)
	}
}

func TestStreamCloseUnblocksAndConcurrentFlushIsRaceSafe(t *testing.T) {
	readFile, writeFile, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer writeFile.Close()
	stream := &streamValue{
		id:     "pipe",
		path:   "pipe",
		mode:   "read",
		file:   readFile,
		reader: bufio.NewReader(readFile),
	}
	started := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		close(started)
		_, err := stream.readChunk(1)
		finished <- err
	}()
	<-started
	if _, err := stream.close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("close did not unblock a pending file read")
	}

	path := filepath.Join(t.TempDir(), "race.txt")
	output, err := openWriteStream(path, false)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for index := 0; index < 8; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for attempt := 0; attempt < 32; attempt++ {
				_, _ = output.writeChunk(stringValue("x"))
				_, _ = output.flush()
			}
		}()
	}
	group.Wait()
	if _, err := output.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := output.close(); err != nil {
		t.Fatal(err)
	}
}

func TestFilesystemPermissionAndCrossDeviceErrorMapping(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "protected")
	if err := os.WriteFile(protected, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(protected, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(protected, 0o600) })
		if _, err := readLimitContextValue(context.Background(), []Value{stringValue(protected), uintValue(1, 64)}); err != nil {
			var record *OsirisError
			if !errors.As(err, &record) || record.kind != ErrorKindPermissionDenied {
				t.Fatalf("permission error = %T %v, want permission_denied", err, err)
			}
		}
	}
	if got := filesystemErrorKind(&os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.EXDEV}); got != ErrorKindIO {
		t.Fatalf("cross-device rename kind = %s, want %s", got, ErrorKindIO)
	}
}

func TestFilesystemNativeGateIsSourceAware(t *testing.T) {
	for _, name := range filesystemBuiltinNames() {
		source := "emit " + name + "(\"fixture\")\n"
		if name == "append" {
			source = `emit append("fixture", "x")` + "\n"
		}
		prog, err := Parse("<filesystem-native>", source)
		if err != nil {
			t.Fatal(err)
		}
		_, err = CompileC(context.Background(), NewRuntime(), prog)
		if err == nil || !strings.Contains(err.Error(), `filesystem builtin "`+name+`" is not supported`) {
			t.Fatalf("%s native error = %v, want explicit gate", name, err)
		}
		if !strings.Contains(err.Error(), "<filesystem-native>:1:6") {
			t.Fatalf("%s native error lacks source location: %v", name, err)
		}
	}
}

func TestFilesystemInferenceSurface(t *testing.T) {
	prog, err := Parse("<filesystem-infer>", `
let appended = append("file", "x")
let limited = read_limit("file", 1)
let copied = copy_file("file", "copy")
let path_truncated = truncate_file("file", 0)
let present = exists("file")
let size = file_size("file")
let regular = is_file("file")
let directory = is_dir("file")
let made = mkdir("directory")
let parents = mkdir_all("parent/child")
let entries = read_dir("directory")
let renamed = rename("old", "new")
let removed = remove("file")
let output = openappend("file")
let position = seek(output, 0, "start")
let observed = tell(output)
let stream_truncated = truncate(output, 0)
`)
	if err != nil {
		t.Fatal(err)
	}
	inferred, err := NewRuntime().InferProgram(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	report := FormatInferProgram(inferred)
	for _, want := range []string{
		"main.appended: uint64",
		"main.limited: list",
		"main.copied: uint64",
		"main.path_truncated: bool",
		"main.present: bool",
		"main.size: uint64",
		"main.regular: bool",
		"main.directory: bool",
		"main.made: bool",
		"main.parents: bool",
		"main.entries: list",
		"main.renamed: bool",
		"main.removed: bool",
		"main.output: stream",
		"main.position: uint64",
		"main.observed: uint64",
		"main.stream_truncated: bool",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("inference lacks %q:\n%s", want, report)
		}
	}
}

func assertFilesystemError(t *testing.T, err error, kind ErrorKind, operation, resource string) {
	t.Helper()
	var record *OsirisError
	if !errors.As(err, &record) || record == nil {
		t.Fatalf("error = %T %v, want structured filesystem error", err, err)
	}
	if record.kind != kind || record.operation != operation || record.resource != resource {
		t.Fatalf("error = kind=%s operation=%q resource=%q, want %s %q %q", record.kind, record.operation, record.resource, kind, operation, resource)
	}
}
