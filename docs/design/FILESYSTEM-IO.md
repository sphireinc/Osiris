# Filesystem and Stream I/O

## Status

This document defines the hosted filesystem contract for the interpreter and
bytecode VM. The native C backend does not implement this contract and rejects
the filesystem built-ins at their source locations.

Osiris is not a filesystem sandbox. A program has the filesystem authority of
the process that runs it. Every operation below acts on exactly the path passed
by the program; the runtime does not search parent directories, expand shell
syntax, follow an environment-controlled fallback, or recursively remove
content.

## Paths and data

A path is a non-empty Osiris `string` without a NUL byte. It is passed to the
host operating system without lexical cleaning or canonicalization. Relative
paths are relative to the process working directory. Platform path separators,
case rules, reserved names, and maximum lengths therefore remain host rules.

`write`, `append`, and `writechunk` accept the existing byte-compatible value
families:

- a `string`, copied as its exact bytes;
- a `list<uint8>` or byte-compatible tensor;
- an unsigned or exact non-negative numeric value supported by the existing
  byte encoder.

`read` returns the file bytes in a `string` without adding text encoding or
newline normalization. `readbytes`, `read_limit`, and `readchunk` return exact
`uint8` values. `lines`, `csv`, and `jsonparse` retain their existing
text/structured-data contracts and do not change filesystem ownership.

## Whole-file operations

| Operation | Contract |
| --- | --- |
| `read(path)` | Reads the complete file into a `string`. |
| `readbytes(path)` | Reads the complete file into `list<uint8>`. |
| `read_limit(path, limit)` | Reads at most `limit` bytes into `list<uint8>`. `limit` is an exact non-negative integer no greater than 1,048,576. Reaching the limit is a successful prefix read, not an error. |
| `write(path, data)` | Creates or truncates the destination and returns `true`. |
| `append(path, data)` | Creates the destination if absent, appends without truncating, and returns the exact `uint64` byte count. |
| `copy_file(source, destination)` | Copies a regular source file, creates or truncates the destination, and returns the exact `uint64` byte count. Source and destination must not resolve to the same file. |
| `truncate_file(path, size)` | Resizes a file to an exact non-negative signed-64-bit size and returns `true`. Growing a file exposes zero bytes according to the host filesystem and may create a sparse file. |

New regular files use mode `0644` before the process umask. Existing
destination permissions are not replaced by `write`, `append`, `openwrite`,
`openappend`, or `copy_file`. None of these calls performs an implicit
`fsync`; success means the hosted write and close boundary succeeded, not that
storage hardware has made the data durable.

`copy_file` uses one 32 KiB buffer and loops over short reads and short writes.
It is not an atomic replace operation. A failure can leave a partial
destination, and the runtime does not silently retry by writing a temporary
file or renaming it.

The `read_limit` ceiling accounts for the fact that every returned byte is a
full width-preserving Osiris `Value`, not only one byte of Go backing storage.
The result allocation is therefore larger than the raw file prefix but remains
bounded by the documented element ceiling.

## Metadata and directories

`exists`, `is_file`, and `is_dir` return `false` for a missing path. Other host
errors, including permission failures, remain errors. The predicates use the
resolved target: a symlink to a regular file is a file, a symlink to a
directory is a directory, and a broken symlink is missing.

`file_size` accepts a regular file and returns its non-negative byte size as
`uint64`. It follows a final symlink. Directories and other non-regular file
kinds are rejected instead of assigning a host-specific size meaning.

`mkdir(path)` creates one directory with mode `0755` before umask. Its parent
must already exist, and an existing path is `already_exists`.
`mkdir_all(path)` creates missing parent directories, uses the same mode for
each new directory, and succeeds when the full path is already a directory.

`read_dir(path[, limit])` returns a `list<string>` of entry base names in
lexical byte order. A string name is the complete first-release public entry
representation; no host `FileInfo` or `dirent` fields leak into the language.
The default limit is 4,096 entries and the maximum accepted limit is 65,536.
If the directory contains more entries than the selected limit, the operation
fails with `resource_exhausted` rather than returning an incomplete listing.
The implementation retains at most `limit + 1` host entries before sorting.

## Path mutation

`rename(old_path, new_path)` delegates one rename to the host and returns
`true`. It does not fall back to copy-and-remove. Replacement rules for an
existing destination, directory replacement, and case-only renames remain
platform rules. The API makes no portable atomicity guarantee. A
cross-filesystem rename fails as `io_error` and leaves both explicit paths to
the host operation; Osiris does not manufacture cross-filesystem success.

`remove(path)` removes one explicit file, symlink, or empty directory and
returns `true`. It never traverses a directory. Removing a symlink removes the
link itself, not its target. A non-empty directory is a `conflict`.

## Owned file streams

`openread`, `openwrite`, and `openappend` each return an owned `stream`.
`openwrite` creates or truncates a file. `openappend` creates or preserves a
file, opens it with host append semantics, and starts its logical cursor at the
current end. New files use mode `0644` before umask.

The program must call `close` on every owned stream:

- the first close marks the handle terminal, flushes a writable buffer, closes
  the descriptor, and returns `true`;
- a later close returns `false`;
- the terminal state is retained even if the first flush or close reports an
  error;
- reads, writes, seeks, tells, truncation, and flushes on a closed stream fail
  with `io_error`.

Repeated `flush` calls on an open stream are safe and return `true`. Flushing a
read-only stream is an open-stream no-op. Flush moves bytes through the
runtime's user-space buffer to the hosted descriptor; it is not a durability
or remote-receipt guarantee.

This release does not expose standard input, output, or error as stream values.
They cannot yet be represented as ordinary borrowed handles whose ownership
and close behavior are unambiguous, so no exceptional global-stream rules are
introduced.

## Seeking and stream truncation

`seek(stream, offset, origin)` accepts an exact signed-64-bit byte offset and
one of the strings `start`, `current`, or `end`. It flushes buffered writes and
accounts for reader look-ahead before moving the hosted file cursor. It
returns the new non-negative byte position as `uint64`.

File contents occupy the half-open byte range `[0, file_size)`. The byte
boundaries from `0` through `file_size` are valid positions, and hosted regular
files may also seek beyond end-of-file. A later read there returns end-of-file;
a later write can create a gap under the host filesystem's ordinary rules. A
seek that would produce a negative position is `invalid_argument`.

`tell(stream)` performs the same buffered-cursor accounting and returns the
logical position as `uint64`. `truncate(stream, size)` accepts an owned
writable file stream, flushes it, resizes the file, returns `true`, and leaves
the cursor unchanged even when it is now beyond the new end. TCP and other
non-file streams reject `seek`, `tell`, and `truncate` as `unsupported`.

## Cancellation and concurrency

The new filesystem operations are registered as context-aware built-ins.
Interpreter calls, VM calls, and calls evaluated inside `spawn`, `timeout`,
`race`, or `select` receive the current session/task context. Operations check
that context before entering the host operation. `read_limit`, `append`,
`copy_file`, and `read_dir` also check between bounded chunks or entries.

Portable regular-file APIs cannot interrupt every host syscall after it has
entered the kernel. Cancellation is therefore cooperative: it is prompt
between chunks but does not claim a platform-independent deadline for one
stalled filesystem syscall. Closing a file stream marks it terminal and closes
its descriptor without waiting for the read lifecycle lock, allowing the host
close operation to unblock a pending stream read. Copy helpers own their local
descriptors until the call returns and do not expose them to `close`.

Each stream serializes reads and writes separately. Cursor-changing operations
take both lifecycle locks, so `seek`, `tell`, and `truncate` cannot race the
stream's buffered reader or writer. `close` is safe to call concurrently and
only one caller performs the hosted close.

## Symlinks

Whole-file reads, writes, append, copy sources, truncation, file metadata, and
file opens follow the host's normal final-symlink behavior. `truncate_file`
therefore truncates a symlink target. `read_dir` returns the link's entry name
without resolving it for the public representation. `rename` and `remove`
operate on the named directory entry, so a final symlink is renamed or removed
without renaming or removing its target.

The runtime does not attempt to make a check followed by a mutation
race-free. Programs that require confinement must arrange it outside this API;
this contract does not claim resistance to symlink swaps by a concurrent
process.

## Structured failures

Filesystem failures are `OsirisError` records with stable `kind`,
`operation`, `component = "filesystem"`, and, when known, `resource`.
Two-path operations also include `destination`. Host error prose and numeric
codes are debug metadata only and disappear from redacted output.

| Host condition | Stable kind |
| --- | --- |
| cancelled context | `cancelled` |
| expired context | `timeout` |
| missing path | `not_found` |
| permission denied or read-only filesystem | `permission_denied` |
| path already exists | `already_exists` |
| non-empty directory | `conflict` |
| malformed path, non-directory component, directory used as a file, invalid seek | `invalid_argument` |
| descriptor, device, cross-filesystem rename, or other filesystem I/O failure | `io_error` |
| file-descriptor, storage-space, or documented listing/read limit exhaustion | `resource_exhausted` |

Argument kind failures are `type_error`. Normal error rendering never includes
host error strings or debug-only metadata. Source locations are attached by the
interpreter and VM at the call expression.
