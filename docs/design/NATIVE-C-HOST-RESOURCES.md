# Native C host-resource ABI

This document specifies the integrated POSIX and Windows native host-resource
components for task 148. The capability table, direct dispatch lowering, owned
cleanup, and platform policy are implemented together in
`internal/osiris/cbackend_process_runtime.go` and
`internal/osiris/cbackend_native.go`; the guarded Windows HANDLE/Winsock
implementation is in `internal/osiris/cbackend_windows_runtime.go`.

## Resource representation

The component represents an owned native resource as an `O_RESOURCE` value
whose payload points to an `OHostResource`. The payload is not a dynamic FFI library:
the `OSIRIS_HOST_RESOURCE_MAGIC` value and the resource kind are checked at
every operation boundary.

The resource kinds are:

- `OSIRIS_HOST_PROCESS`: a child PID and three owned pipe descriptors.
- `OSIRIS_HOST_FILE`: one owned file descriptor.
- `OSIRIS_HOST_ROOT`: an owned directory descriptor or directory HANDLE used by
  rooted-file access.
- `OSIRIS_HOST_STREAM`: one descriptor detached from a process pipe.
- `OSIRIS_HOST_WATCH`: a bounded snapshot watcher with owned pending JSON event
  strings.
- `OSIRIS_HOST_SIGNAL`: a bounded signal subscription with owned notification
  state.

On POSIX, file/root/stream resources own descriptors. On Windows, they own
`HANDLE` values and socket resources own Winsock `SOCKET` values; no Windows
handle is cast to a POSIX integer. The generated source emits the Windows
component before the POSIX/network components, and the latter are suppressed
by `OSIRIS_WINDOWS_COMPONENT` on `_WIN32`.

Every resource is registered in the generated allocation lifetime. `close`
closes descriptors immediately and marks the resource closed. The process-exit
cleanup list closes any resource that was not explicitly released. A closed
resource cannot be borrowed again, and a value of the wrong kind is rejected
before a host syscall.

## Process operations

| Symbol | Signature | Capability | Result and ownership |
| --- | --- | --- | --- |
| `std/os.process_spawn` | `process_spawn(command: string, arguments: list<string>[, options: map])` | `os.process_spawn` | A map containing owned `process`, `pid`, `stdin`, `stdout`, and `stderr` values. POSIX options support `cwd`, `env`, and `inherit_env`. `argv[0]` is the command and the caller's arguments follow it. |
| `std/os.process_wait` | `process_wait(process, timeout_or_null)` | `os.process` | A map containing `status`, `success`, and `signal`. A timeout sends `SIGTERM`, waits for a bounded grace period, sends `SIGKILL` to the owned process group, reaps the leader, and returns a timeout error. |
| `std/os.process_kill` | `process_kill(process, signal_or_null)` | `os.process_kill` | `true` after a signal is sent to the owned process group. The default signal is `SIGKILL`; `process_signal` can select another validated signal. |
| `std/os.process_signal` | `process_signal(process, signal)` | `os.process_signal` | `true` after a validated signal is sent. |
| `std/os.process_release` | `process_release(process)` | `os.process_inspect` | `true` after the child has been waited. A running child returns `busy` rather than becoming an unreaped zombie. |
| `std/os.process_stdin` | `process_stdin(process)` | `os.process` | An owned writable stream detached from the process handle. |
| `std/os.process_stdout` | `process_stdout(process)` | `os.process` | An owned readable stream detached from the process handle. |
| `std/os.process_stderr` | `process_stderr(process)` | `os.process` | An owned readable stream detached from the process handle. |

Spawning is shell-free. The command and every argument are copied into a
NUL-terminated `argv` array and passed to `execvp`; no shell parser, command
string concatenation, or hidden backend is involved. The child closes the
parent-only pipe ends before `execvp`. If setup fails, all pipe ends are closed
and the child is synchronously reaped.

The optional `options` map accepts only these native fields: `cwd: string`,
`env: map<string,string|null>`, and `inherit_env: bool`. An omitted `cwd`
inherits the parent directory. `inherit_env` defaults to `true`. When it is
true, the child starts with a copy of the parent environment and each `env`
entry replaces the inherited value; a `null` entry deletes that key. When it
is false, the child starts empty and the explicit map becomes the complete
environment. Environment entries are sorted by key before `execve`, so the
same options map produces deterministic `envp` ordering. Keys and values must
be bounded NUL-free strings; an invalid entry fails before `fork`.

`process_wait` uses `waitpid(..., WNOHANG)` and a bounded one-millisecond poll
interval. It does not create a scheduler goroutine or leak a cancellation
watcher. Each POSIX child calls `setsid` before `execve`, making its PID the
owned process-group and session identifier. `process_kill`, timeout cleanup,
explicit close, and generated-process exit operate on that group. Cleanup is
TERM, a 100 ms bounded grace period, KILL, then blocking reap; it is performed
before the process resource is marked closed. `process_release` refuses to
discard an un-waited process handle. A detached process stream remains an
independent owned resource and does not keep the process group alive after its
owner is closed.

The Windows process adapter uses `CreateProcessW`, three owned `CreatePipe`
connections, `WaitForSingleObject`, `GetExitCodeProcess`, and
`TerminateProcess`. It accepts `cwd`; Windows environment overlay execution
and process-group/job-object behavior remain parent-task platform evidence
because this macOS host has no Windows runtime. A Windows process resource
cannot be mistaken for another resource kind because every HANDLE wrapper
carries magic, kind, and closed-state checks.

## File and stream operations

| Symbol | Signature | Capability | Result and ownership |
| --- | --- | --- | --- |
| `std/os.open` | `open(path)` | `os.file_read` | Owned read-only file handle. Use `open_file(path, flags, mode)` for explicit flags and mode; flag-derived write admission remains a documented follow-up. |
| `std/os.create` | `create(path)` | `os.file_write` | Owned read/write file handle created with `O_CREAT|O_TRUNC` and the hosted default mode. |
| `std/os.open_file` | `open_file(path, flags, mode)` | `os.file_read` | Owned file handle. The current native gate is coarse; contradictory flags still fail before `open(2)`. |
| `std/os.stat` | `stat(path)` | `os.file_metadata` | Owned map `{name: string, size: i64, mode: u64, dir: bool, mod_time: string}` from `stat(2)`. |
| `std/os.lstat` | `lstat(path)` | `os.file_metadata` | The same owned metadata map, using `lstat(2)` so a symlink is not followed. |
| `std/os.read_dir` | `read_dir(path)` | `os.directory_read` | Sorted owned `list<string>` of entry names, excluding `.` and `..`. |
| `std/os.close` | `close(file_or_stream)` | `os.file_read` | Closes an admitted host resource and invalidates it. A later host close is an error; network close returns `false` for a repeated close. |
| `std/os.read` | `read(file, size, timeout_or_null)` | `os.file_read` | A bounded `list<u8>` containing the bytes read. Maximum one MiB per call. |
| `std/os.write` | `write(file, bytes, timeout_or_null)` | `os.file_write` | Number of bytes written. Input is copied into owned temporary storage. |
| `std/os.read_at` | `read_at(file, size, offset, timeout_or_null)` | `os.file_read` | Bounded `list<u8>` from `pread(2)` without changing the shared cursor. |
| `std/os.write_at` | `write_at(file, bytes, offset, timeout_or_null)` | `os.file_write` | Number of bytes written by `pwrite(2)` without changing the shared cursor. |
| `std/os.seek_file` | `seek_file(file, offset, whence)` | `os.file_read` | New absolute cursor from `lseek(2)`. |
| `std/os.sync` | `sync(file)` | `os.file_write` | `null` after `fsync(2)`. |
| `std/os.set_deadline` | `set_deadline(resource, timeout_or_null)` | `os.file_metadata` | `null`; stores a monotonic deadline on the resource. |
| `std/os.set_read_deadline` | `set_read_deadline(resource, timeout_or_null)` | `os.file_metadata` | `null`; the current component applies the same deadline to bounded I/O. |
| `std/os.set_write_deadline` | `set_write_deadline(resource, timeout_or_null)` | `os.file_metadata` | `null`; the current component applies the same deadline to bounded I/O. |

## Filesystem mutations

| Symbol | Signature | Capability | Result and ownership |
| --- | --- | --- | --- |
| `std/os.mkdir` | `mkdir(path, mode)` | `os.file_mutate` | `true` after `mkdir(2)` creates one directory. |
| `std/os.mkdir_all` | `mkdir_all(path, mode)` | `os.file_mutate` | `true` after bounded recursive directory creation. |
| `std/os.remove` | `remove(path)` | `os.file_mutate` | `true` after removing one file or empty directory. |
| `std/os.remove_all` | `remove_all(path)` | `os.file_mutate` | `true` after bounded recursive removal; symlinks are removed as entries. |
| `std/os.truncate` | `truncate(path, size)` | `os.file_mutate` | `true` after changing the path length. |
| `std/os.rename` | `rename(old_path, new_path)` | `os.file_mutate` | `true` after one POSIX `rename(2)`. |
| `std/os.chmod` | `chmod(path, mode)` | `os.file_mutate` | `true` after `chmod(2)`. |
| `std/os.readlink` | `readlink(path)` | `os.symlink` | An owned, bounded target string from `readlink(2)`. |
| `std/os.link` | `link(old_path, new_path)` | `os.file_mutate` | `true` after a POSIX hard-link operation. |
| `std/os.symlink` | `symlink(old_path, new_path)` | `os.symlink` | `true` after a POSIX symbolic-link operation. |
| `std/os.write_string` | `write_string(file, string, timeout_or_null)` | `os.file_write` | Number of bytes written through an owned file or stream handle. |

Read and write first use `poll(2)` with the effective operation timeout. The
effective timeout is the earlier of the call timeout and the resource deadline.
An expired operation returns a structured timeout error. The component does
not spawn a goroutine to interrupt a blocked syscall. Every temporary byte
buffer is freed on success, error, and invalid-element paths.

## Standard streams and raw handles

| Symbol | Signature | Capability | Result and ownership |
| --- | --- | --- | --- |
| `std/os.stdin` | borrowed module value `File` | `os.file_read` | An owned duplicate of descriptor 0, exposed as a readable stream resource. Closing it does not close the process's original standard descriptor. |
| `std/os.stdout` | borrowed module value `File` | `os.file_write` | An owned duplicate of descriptor 1, exposed as a writable stream resource. |
| `std/os.stderr` | borrowed module value `File` | `os.file_write` | An owned duplicate of descriptor 2, exposed as a writable stream resource. |
| `std/os.pipe` | `pipe()` | `os.file_read` plus `os.file_write` for the returned pair | A two-element owned stream list containing the read and write ends of a POSIX pipe. |
| `std/os.fd` | `fd(file_or_stream)` | `os.process_raw_handle` | The borrowed numeric descriptor as `u64`; the returned number does not transfer ownership. |
| `std/os.name` | `name(file_or_stream)` | `os.file_metadata` | An owned string containing the resource name. |
| `std/os.new_file` | `new_file(fd: u64, name: string)` | `os.process_raw_handle` | An owned duplicate of the supplied descriptor with the supplied name. |

Native `with_handle` and `syscall_conn` remain explicit rejection boundaries
because they require a generated-language callback/raw-connection ABI rather
than just a descriptor value.

## Signal subscriptions

| Symbol | Signature | Capability | Result and ownership |
| --- | --- | --- | --- |
| `std/os.signal_subscribe` | `signal_subscribe(name: string)` | `os.signal` | An owned signal stream resource. POSIX supports `SIGINT`, `SIGTERM`, `SIGUSR1`, `SIGUSR2`, and `SIGHUP`; Windows supports `SIGINT`, `SIGTERM`, and `SIGBREAK`. `readchunk(subscription, size, timeout_or_null)` returns bounded signal bytes. |

The POSIX signal adapter uses an owned non-blocking self-pipe and an
async-signal-safe one-byte handler. The Windows adapter uses an owned HANDLE
pipe and the CRT console-signal hook. Neither adapter creates a scheduler
goroutine. A single native signal subscription is allowed per generated
process; closing it restores the previous signal action and releases both pipe
ends.

## Rooted resources and watching

`std/os.open_root(path)` returns an `OSIRIS_HOST_ROOT` directory resource
backed by a POSIX directory descriptor or a Windows directory HANDLE.
`std/os.root_open(root, relative_path)` opens a read-only child, while
`std/os.root_open_file(root, relative_path, flags, mode)` resolves an arbitrary
child with
`openat(2)` on POSIX or a confined, reparse-checked Win32 path on Windows.
Absolute paths, empty paths, `..` components, use-after-close, and path
components that resolve through an escape symlink/reparse point are rejected.
The Windows adapter rechecks opened handles with
`GetFinalPathNameByHandleW`; mutations that race a validation are rejected
after the handle is opened and are never exposed as a usable resource.

The integrated root subset exposes `root_create`, `root_read_file`,
`root_write_file`, `root_stat`, `root_lstat`, `root_readlink`, `root_mkdir`,
`root_mkdir_all`, `root_remove`, `root_remove_all`, `root_rename`, `root_link`,
`root_symlink`, `root_chmod`, `root_chown`, `root_lchown`, `root_chtimes`,
`root_fs`, and `root_close`. POSIX uses descriptor-relative operations and
`fchownat`/`futimens`; Windows uses confined HANDLE operations, Win32 file
times, and returns a structured platform error for non-`-1` POSIX ownership
IDs because Windows has no numeric uid/gid ownership model. `-1`/`-1` is the
portable unchanged-owner operation. `root_lstat` and `root_readlink` inspect
the final reparse point without following it. `root_mkdir_all` and
`root_remove_all` reject reparse-point traversal.

`std/os.watch` is integrated as a bounded POSIX snapshot adapter. The resource
maintains a deterministic file snapshot, emits newline-delimited JSON
`create`, `modify`, and `remove` events, polls in bounded 50 ms intervals, and
returns owned event strings. Timeout and close release pending event storage.
The Windows adapter uses `ReadDirectoryChangesW` with an owned overlapped
HANDLE and JSON-escapes UTF-8 paths before queuing the same event shape. The
portable POSIX implementation is deliberately a snapshot implementation rather
than a claim of inotify/FSEvents queue equivalence; native overflow events
remain platform work.

| Symbol | Signature | Capability | Result and ownership |
| --- | --- | --- | --- |
| `std/os.watch` | `watch(path, recursive: bool \| null)` | `os.watch` | Owned watch resource. Omitted or null recursion selects recursive snapshots; `false` watches only the named directory. |
| watch `readchunk` | `readchunk(watch, size, timeout_or_null)` | `os.watch` | Owned bounded string containing newline-delimited JSON events in deterministic order. |

Generic operations are producer-aware. The native compiler tracks the resource
producer through SSA copies, process result members, and HTTP `body_stream`
members. The capability required by each operation is:

| Producer | `readchunk` | `writechunk` | `close` |
| --- | --- | --- | --- |
| `std/os.open`, `open_file`, `stdin` | `os.file_read` | `os.file_write` | `os.file_read` |
| `std/os.create`, `stdout`, `stderr` | `os.file_read` | `os.file_write` | `os.file_write` |
| `std/os.process_spawn` and process streams | `os.process` | `os.process` | `os.process` |
| `std/os.watch` | `os.watch` | `os.watch` | `os.watch` |
| `std/os.signal_subscribe` | `os.signal` | `os.signal` | `os.signal` |
| TCP/UDP sockets | `net.tcp_read` | `net.tcp_write` | `net.close` |
| TLS streams | the granted TLS producer capability | the granted TLS producer capability | the granted TLS producer capability |
| HTTP body streams | `net.http_stream` | `net.http_stream` | `net.http_stream` |

An unknown producer is rejected at native compilation with a source-spanned
diagnostic; it is never granted the old ambient network capability. This keeps
capability denial deterministic and prevents a file or process stream from
silently depending on an unrelated socket grant.

The focused coverage is in
`internal/osiris/task148b_native_capability_test.go` and includes producer
aliases, process members, watches, signals, sockets, TLS, HTTP bodies, and
denial diagnostics.

## HTTP redirects and generated handlers

Native HTTP follows at most ten redirects for status 301, 302, 303, 307, and
308. `Location` may be absolute, scheme-relative, root-relative, or
path-relative. A 301/302/303 converts a non-GET request to GET and removes its
body and content-length; 307/308 preserve method and body. Cross-authority
redirects remove `Authorization`, `Proxy-Authorization`, and `Cookie`. Each
intermediate response is drained and its provider stream is closed before the
next hop. Cancellation and malformed locations abort the chain and close the
current stream. Exceeding the limit returns a structured redirect error.

`http_serve(listener, handler, status_or_null)` accepts a generated `O_CLOSURE`
directly through the native `ocall` ABI. The handler receives a request map and
returns a response map with optional `status`, `body`, and `headers` fields.
Status and body/header sizes are validated against the same native limits as
static responses. Handler errors, invalid response values, cancellation, and
transport failures close the accepted connection and propagate a structured
error; the closure and its captures are borrowed only for the synchronous call
and are never retained by the provider.

## Integration and remaining scope

The emitter and capability integration is complete for the symbols listed in
the tables. Windows `HANDLE` ownership, standard streams, raw handles, socket
resources, rooted operations, and watches are selected behind the
`_WIN32`/`OSIRIS_WINDOWS_COMPONENT` guard; POSIX keeps its descriptor adapter.
Callback-backed raw-handle methods and Windows execution evidence remain
outside 148B. Process groups, environment overlays, redirects, generated
closure handlers, producer-aware generic capabilities, and Linux execution
evidence are complete for the POSIX/native-C contract.

The focused checks are in
`internal/osiris/cbackend_process_runtime_test.go`. They verify the explicit
capability manifest, ownership/cancellation markers, and C syntax against the
current generated prelude without mutating shared emitter files.
