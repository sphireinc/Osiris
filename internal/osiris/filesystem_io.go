package osiris

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"syscall"
)

const (
	// read_limit returns its payload as an in-memory Osiris list. Keep the
	// public ceiling explicit so a hostile size argument cannot request an
	// unbounded allocation.
	filesystemMaxReadBytes uint64 = 1 << 20

	// Directory reads fail rather than return an incomplete listing. The
	// default is intentionally modest, and callers can opt into the larger
	// documented hard ceiling when they know the directory cardinality.
	filesystemDefaultDirectoryEntries uint64 = 4 << 10
	filesystemMaxDirectoryEntries     uint64 = 64 << 10

	// Copies use one reusable buffer; the destination payload is never held in
	// memory by the runtime.
	filesystemCopyBufferBytes = 32 << 10
)

func filesystemFailure(kind ErrorKind, operation, resource, message string, host error) error {
	record := newOsirisError(kind, message).
		withMetadata("operation", stringValue(operation)).
		withMetadata("component", stringValue("filesystem"))
	if resource != "" {
		record = record.withMetadata("resource", stringValue(resource))
	}
	if host != nil {
		record = withNativeErrorCode(record, host).
			withDebugMetadata("host_error", stringValue(host.Error()))
	}
	return record
}

func filesystemHostFailure(operation, resource string, host error) error {
	return filesystemFailure(
		filesystemErrorKind(host),
		operation,
		resource,
		operation+" could not complete",
		host,
	)
}

func filesystemErrorKind(err error) ErrorKind {
	switch {
	case err == nil:
		return ErrorKindUnknown
	case errors.Is(err, context.Canceled):
		return ErrorKindCancelled
	case errors.Is(err, context.DeadlineExceeded):
		return ErrorKindTimeout
	case errors.Is(err, os.ErrNotExist):
		return ErrorKindNotFound
	case errors.Is(err, os.ErrPermission),
		errors.Is(err, syscall.EROFS):
		return ErrorKindPermissionDenied
	case errors.Is(err, os.ErrExist):
		return ErrorKindAlreadyExists
	case errors.Is(err, syscall.ENOTEMPTY):
		return ErrorKindConflict
	case errors.Is(err, syscall.ENOTDIR),
		errors.Is(err, syscall.EISDIR),
		errors.Is(err, syscall.EINVAL),
		errors.Is(err, syscall.ENAMETOOLONG),
		errors.Is(err, syscall.ELOOP),
		errors.Is(err, fs.ErrInvalid):
		return ErrorKindInvalidArgument
	case errors.Is(err, syscall.ENOSPC),
		errors.Is(err, syscall.EMFILE),
		errors.Is(err, syscall.ENFILE):
		return ErrorKindResourceExhausted
	case errors.Is(err, syscall.EXDEV),
		errors.Is(err, syscall.EBADF),
		errors.Is(err, syscall.EIO),
		errors.Is(err, os.ErrClosed):
		return ErrorKindIO
	default:
		return ErrorKindIO
	}
}

func filesystemPathArgument(value Value, operation string) (string, error) {
	if value.kind != kindString {
		return "", filesystemFailure(
			ErrorKindType,
			operation,
			"",
			operation+" expects a string path",
			nil,
		)
	}
	path := value.str
	if path == "" || strings.IndexByte(path, 0) >= 0 {
		return "", filesystemFailure(
			ErrorKindInvalidArgument,
			operation,
			path,
			operation+" expects a non-empty path without NUL bytes",
			nil,
		)
	}
	return path, nil
}

func filesystemIntegerArgument(value Value, operation, name string) (int64, error) {
	integer, err := exactInteger(value, name)
	if err != nil {
		return 0, filesystemFailure(
			ErrorKindType,
			operation,
			"",
			operation+" expects "+name+" to be an exact integer",
			nil,
		)
	}
	if !integer.IsInt64() {
		return 0, filesystemFailure(
			ErrorKindInvalidArgument,
			operation,
			"",
			operation+" "+name+" is outside the signed 64-bit range",
			nil,
		)
	}
	return integer.Int64(), nil
}

func filesystemNonnegativeArgument(value Value, operation, name string, maximum uint64) (uint64, error) {
	integer, err := exactInteger(value, name)
	if err != nil {
		return 0, filesystemFailure(
			ErrorKindType,
			operation,
			"",
			operation+" expects "+name+" to be an exact integer",
			nil,
		)
	}
	if integer.Sign() < 0 {
		return 0, filesystemFailure(
			ErrorKindInvalidArgument,
			operation,
			"",
			operation+" "+name+" must be non-negative",
			nil,
		)
	}
	if !integer.IsUint64() {
		return 0, filesystemFailure(
			ErrorKindInvalidArgument,
			operation,
			"",
			operation+" "+name+" is outside the unsigned 64-bit range",
			nil,
		)
	}
	number := integer.Uint64()
	if maximum != 0 && number > maximum {
		return 0, filesystemFailure(
			ErrorKindResourceExhausted,
			operation,
			"",
			fmt.Sprintf("%s %s exceeds the limit of %d", operation, name, maximum),
			nil,
		)
	}
	return number, nil
}

func filesystemContext(ctx context.Context, operation, resource string) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return filesystemHostFailure(operation, resource, err)
	}
	return nil
}

func filesystemContextCall(ctx context.Context, args []Value, operation string, call callable) (Value, error) {
	resource := ""
	if len(args) > 0 && args[0].kind == kindString {
		resource = args[0].str
	}
	if err := filesystemContext(ctx, operation, resource); err != nil {
		return nullValue(), err
	}
	return call(args)
}

func bytesListValue(data []byte) Value {
	output := make([]Value, len(data))
	for index, item := range data {
		output[index] = uintValue(uint64(item), 8)
	}
	return listValue(output)
}

func appendContextValue(ctx context.Context, args []Value) (Value, error) {
	const operation = "append"
	if len(args) != 2 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, operation, "", "append expects 2 arguments", nil)
	}
	path, err := filesystemPathArgument(args[0], operation)
	if err != nil {
		return nullValue(), err
	}
	data, err := bytesFromValue(args[1])
	if err != nil {
		return nullValue(), filesystemFailure(ErrorKindType, operation, path, "append expects string or byte-compatible data", nil)
	}
	if err := filesystemContext(ctx, operation, path); err != nil {
		return nullValue(), err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nullValue(), filesystemHostFailure(operation, path, err)
	}
	written, writeErr := writeAllContext(ctx, file, data)
	closeErr := file.Close()
	if writeErr != nil {
		return nullValue(), filesystemHostFailure(operation, path, writeErr)
	}
	if closeErr != nil {
		return nullValue(), filesystemHostFailure(operation, path, closeErr)
	}
	return uintValue(uint64(written), 64), nil
}

func appendValue(args []Value) (Value, error) {
	return appendContextValue(context.Background(), args)
}

func writeAllContext(ctx context.Context, writer io.Writer, data []byte) (int, error) {
	written := 0
	for written < len(data) {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return written, err
			}
		}
		end := minInt(len(data), written+filesystemCopyBufferBytes)
		count, err := writer.Write(data[written:end])
		written += count
		if err != nil {
			return written, err
		}
		if count == 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

func readLimitContextValue(ctx context.Context, args []Value) (Value, error) {
	const operation = "read_limit"
	if len(args) != 2 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, operation, "", "read_limit expects 2 arguments", nil)
	}
	path, err := filesystemPathArgument(args[0], operation)
	if err != nil {
		return nullValue(), err
	}
	limit, err := filesystemNonnegativeArgument(args[1], operation, "limit", filesystemMaxReadBytes)
	if err != nil {
		var record *OsirisError
		if errors.As(err, &record) && record != nil && record.resource == "" {
			err = record.withMetadata("resource", stringValue(path))
		}
		return nullValue(), err
	}
	if err := filesystemContext(ctx, operation, path); err != nil {
		return nullValue(), err
	}
	file, err := os.Open(path)
	if err != nil {
		return nullValue(), filesystemHostFailure(operation, path, err)
	}
	data := make([]byte, 0, minInt(int(limit), filesystemCopyBufferBytes))
	buffer := make([]byte, minInt(int(limit), filesystemCopyBufferBytes))
	var readErr error
	for uint64(len(data)) < limit {
		if err := filesystemContext(ctx, operation, path); err != nil {
			readErr = err
			break
		}
		want := len(buffer)
		if remaining := int(limit - uint64(len(data))); want > remaining {
			want = remaining
		}
		count, err := file.Read(buffer[:want])
		if count > 0 {
			data = append(data, buffer[:count]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			readErr = filesystemHostFailure(operation, path, err)
			break
		}
		if count == 0 {
			readErr = filesystemHostFailure(operation, path, io.ErrNoProgress)
			break
		}
	}
	closeErr := file.Close()
	if readErr != nil {
		return nullValue(), readErr
	}
	if closeErr != nil {
		return nullValue(), filesystemHostFailure(operation, path, closeErr)
	}
	return bytesListValue(data), nil
}

func readLimitValue(args []Value) (Value, error) {
	return readLimitContextValue(context.Background(), args)
}

func copyFileContentsContext(ctx context.Context, source io.Reader, destination io.Writer) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	buffer := make([]byte, filesystemCopyBufferBytes)
	var copied int64
	for {
		if err := ctx.Err(); err != nil {
			return copied, err
		}
		read, readErr := source.Read(buffer)
		if err := ctx.Err(); err != nil {
			return copied, err
		}
		if read > 0 {
			written, writeErr := writeAllContext(ctx, destination, buffer[:read])
			copied += int64(written)
			if writeErr != nil {
				return copied, writeErr
			}
		}
		if readErr == io.EOF {
			return copied, nil
		}
		if readErr != nil {
			return copied, readErr
		}
		if read == 0 {
			return copied, io.ErrNoProgress
		}
	}
}

func copyFileContextValue(ctx context.Context, args []Value) (Value, error) {
	const operation = "copy_file"
	if len(args) != 2 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, operation, "", "copy_file expects 2 arguments", nil)
	}
	sourcePath, err := filesystemPathArgument(args[0], operation)
	if err != nil {
		return nullValue(), err
	}
	destinationPath, err := filesystemPathArgument(args[1], operation)
	if err != nil {
		return nullValue(), err
	}
	if err := filesystemContext(ctx, operation, sourcePath); err != nil {
		return nullValue(), err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return nullValue(), filesystemHostFailure(operation, sourcePath, err)
	}
	sourceInfo, err := source.Stat()
	if err != nil {
		_ = source.Close()
		return nullValue(), filesystemHostFailure(operation, sourcePath, err)
	}
	if !sourceInfo.Mode().IsRegular() {
		_ = source.Close()
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, operation, sourcePath, "copy_file source must be a regular file", nil)
	}
	if destinationInfo, statErr := os.Stat(destinationPath); statErr == nil {
		if os.SameFile(sourceInfo, destinationInfo) {
			_ = source.Close()
			return nullValue(), filesystemFailure(ErrorKindInvalidArgument, operation, sourcePath, "copy_file source and destination must differ", nil).(*OsirisError).withMetadata("destination", stringValue(destinationPath))
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		_ = source.Close()
		return nullValue(), filesystemHostFailure(operation, destinationPath, statErr)
	}
	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		_ = source.Close()
		return nullValue(), filesystemHostFailure(operation, destinationPath, err)
	}
	copied, copyErr := copyFileContentsContext(ctx, source, destination)
	destinationCloseErr := destination.Close()
	sourceCloseErr := source.Close()
	if copyErr != nil {
		return nullValue(), filesystemHostFailure(operation, destinationPath, copyErr)
	}
	if destinationCloseErr != nil {
		return nullValue(), filesystemHostFailure(operation, destinationPath, destinationCloseErr)
	}
	if sourceCloseErr != nil {
		return nullValue(), filesystemHostFailure(operation, sourcePath, sourceCloseErr)
	}
	return uintValue(uint64(copied), 64), nil
}

func copyFileValue(args []Value) (Value, error) {
	return copyFileContextValue(context.Background(), args)
}

func truncateFileContextValue(ctx context.Context, args []Value) (Value, error) {
	const operation = "truncate_file"
	if len(args) != 2 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, operation, "", "truncate_file expects 2 arguments", nil)
	}
	path, err := filesystemPathArgument(args[0], operation)
	if err != nil {
		return nullValue(), err
	}
	size, err := filesystemNonnegativeArgument(args[1], operation, "size", uint64(^uint64(0)>>1))
	if err != nil {
		return nullValue(), attachFilesystemResource(err, path)
	}
	if err := filesystemContext(ctx, operation, path); err != nil {
		return nullValue(), err
	}
	if err := os.Truncate(path, int64(size)); err != nil {
		return nullValue(), filesystemHostFailure(operation, path, err)
	}
	return boolValue(true), nil
}

func truncateFileValue(args []Value) (Value, error) {
	return truncateFileContextValue(context.Background(), args)
}

func filesystemStat(args []Value, operation string) (os.FileInfo, string, bool, error) {
	if len(args) != 1 {
		return nil, "", false, filesystemFailure(ErrorKindInvalidArgument, operation, "", operation+" expects 1 argument", nil)
	}
	path, err := filesystemPathArgument(args[0], operation)
	if err != nil {
		return nil, "", false, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, path, false, nil
	}
	if err != nil {
		return nil, path, false, filesystemHostFailure(operation, path, err)
	}
	return info, path, true, nil
}

func existsValue(args []Value) (Value, error) {
	_, _, exists, err := filesystemStat(args, "exists")
	if err != nil {
		return nullValue(), err
	}
	return boolValue(exists), nil
}

func fileSizeValue(args []Value) (Value, error) {
	info, path, exists, err := filesystemStat(args, "file_size")
	if err != nil {
		return nullValue(), err
	}
	if !exists {
		return nullValue(), filesystemHostFailure("file_size", path, os.ErrNotExist)
	}
	if !info.Mode().IsRegular() {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, "file_size", path, "file_size expects a regular file", nil)
	}
	if info.Size() < 0 {
		return nullValue(), filesystemFailure(ErrorKindIO, "file_size", path, "file_size returned an invalid negative size", nil)
	}
	return uintValue(uint64(info.Size()), 64), nil
}

func isFileValue(args []Value) (Value, error) {
	info, _, exists, err := filesystemStat(args, "is_file")
	if err != nil {
		return nullValue(), err
	}
	return boolValue(exists && info.Mode().IsRegular()), nil
}

func isDirValue(args []Value) (Value, error) {
	info, _, exists, err := filesystemStat(args, "is_dir")
	if err != nil {
		return nullValue(), err
	}
	return boolValue(exists && info.IsDir()), nil
}

func mkdirContextValue(ctx context.Context, args []Value) (Value, error) {
	const operation = "mkdir"
	if len(args) != 1 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, operation, "", "mkdir expects 1 argument", nil)
	}
	path, err := filesystemPathArgument(args[0], operation)
	if err != nil {
		return nullValue(), err
	}
	if err := filesystemContext(ctx, operation, path); err != nil {
		return nullValue(), err
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		return nullValue(), filesystemHostFailure(operation, path, err)
	}
	return boolValue(true), nil
}

func mkdirValue(args []Value) (Value, error) {
	return mkdirContextValue(context.Background(), args)
}

func mkdirAllContextValue(ctx context.Context, args []Value) (Value, error) {
	const operation = "mkdir_all"
	if len(args) != 1 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, operation, "", "mkdir_all expects 1 argument", nil)
	}
	path, err := filesystemPathArgument(args[0], operation)
	if err != nil {
		return nullValue(), err
	}
	if err := filesystemContext(ctx, operation, path); err != nil {
		return nullValue(), err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nullValue(), filesystemHostFailure(operation, path, err)
	}
	return boolValue(true), nil
}

func mkdirAllValue(args []Value) (Value, error) {
	return mkdirAllContextValue(context.Background(), args)
}

func readDirContextValue(ctx context.Context, args []Value) (Value, error) {
	const operation = "read_dir"
	if len(args) < 1 || len(args) > 2 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, operation, "", "read_dir expects a path and optional limit", nil)
	}
	path, err := filesystemPathArgument(args[0], operation)
	if err != nil {
		return nullValue(), err
	}
	limit := filesystemDefaultDirectoryEntries
	if len(args) == 2 {
		limit, err = filesystemNonnegativeArgument(args[1], operation, "limit", filesystemMaxDirectoryEntries)
		if err != nil {
			return nullValue(), attachFilesystemResource(err, path)
		}
	}
	if err := filesystemContext(ctx, operation, path); err != nil {
		return nullValue(), err
	}
	directory, err := os.Open(path)
	if err != nil {
		return nullValue(), filesystemHostFailure(operation, path, err)
	}
	entries, readErr := directory.ReadDir(int(limit) + 1)
	closeErr := directory.Close()
	if readErr != nil && readErr != io.EOF {
		return nullValue(), filesystemHostFailure(operation, path, readErr)
	}
	if closeErr != nil {
		return nullValue(), filesystemHostFailure(operation, path, closeErr)
	}
	if uint64(len(entries)) > limit {
		return nullValue(), filesystemFailure(
			ErrorKindResourceExhausted,
			operation,
			path,
			fmt.Sprintf("read_dir contains more than %d entries", limit),
			nil,
		)
	}
	names := make([]string, len(entries))
	for index, entry := range entries {
		if err := filesystemContext(ctx, operation, path); err != nil {
			return nullValue(), err
		}
		names[index] = entry.Name()
	}
	sort.Strings(names)
	output := make([]Value, len(names))
	for index, name := range names {
		output[index] = stringValue(name)
	}
	return listValue(output), nil
}

func readDirValue(args []Value) (Value, error) {
	return readDirContextValue(context.Background(), args)
}

func renameContextValue(ctx context.Context, args []Value) (Value, error) {
	const operation = "rename"
	if len(args) != 2 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, operation, "", "rename expects 2 arguments", nil)
	}
	oldPath, err := filesystemPathArgument(args[0], operation)
	if err != nil {
		return nullValue(), err
	}
	newPath, err := filesystemPathArgument(args[1], operation)
	if err != nil {
		return nullValue(), err
	}
	if err := filesystemContext(ctx, operation, oldPath); err != nil {
		return nullValue(), err
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		record := filesystemHostFailure(operation, oldPath, err)
		var structured *OsirisError
		if errors.As(record, &structured) {
			record = structured.withMetadata("destination", stringValue(newPath))
		}
		return nullValue(), record
	}
	return boolValue(true), nil
}

func renameValue(args []Value) (Value, error) {
	return renameContextValue(context.Background(), args)
}

func removeContextValue(ctx context.Context, args []Value) (Value, error) {
	const operation = "remove"
	if len(args) != 1 {
		return nullValue(), filesystemFailure(ErrorKindInvalidArgument, operation, "", "remove expects 1 argument", nil)
	}
	path, err := filesystemPathArgument(args[0], operation)
	if err != nil {
		return nullValue(), err
	}
	if err := filesystemContext(ctx, operation, path); err != nil {
		return nullValue(), err
	}
	if err := os.Remove(path); err != nil {
		return nullValue(), filesystemHostFailure(operation, path, err)
	}
	return boolValue(true), nil
}

func removeValue(args []Value) (Value, error) {
	return removeContextValue(context.Background(), args)
}

func attachFilesystemResource(err error, resource string) error {
	var record *OsirisError
	if errors.As(err, &record) && record != nil && record.resource == "" {
		return record.withMetadata("resource", stringValue(resource))
	}
	return err
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func filesystemBuiltinNames() []string {
	return []string{
		"append",
		"copy_file",
		"exists",
		"file_size",
		"is_dir",
		"is_file",
		"mkdir",
		"mkdir_all",
		"openappend",
		"read_dir",
		"read_limit",
		"remove",
		"rename",
		"seek",
		"tell",
		"truncate",
		"truncate_file",
	}
}
