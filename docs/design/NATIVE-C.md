# Native C backend contract

## Host capability contract

`CCompileOptions.NativeCapabilities` is the only native-build capability grant.
The list is normalized (empty names removed, duplicates removed, and sorted),
included in native executable cache identity, and used when validating
capability-gated FFI manifests. The compiler does not infer grants from the
build host.

The native C surface lowers a capability-gated POSIX subset of `std/os` and
`std/net`. The direct path helpers and bounded `read_file`/`write_file` calls
retain their owned 1 MiB byte contract. The integrated resource ABI additionally
covers `open`, `create`, `open_file`, `close`, `read`, `write`, `read_at`,
`write_at`, `seek_file`, `sync`, deadlines, `stat`, `lstat`, `read_dir`,
`open_root`, `root_open`, `root_close`, process handles and their streams,
TCP/UDP/DNS sockets, plain HTTP, provider-backed TLS, and snapshot watch
streams. Every admitted member has an explicit capability and direct C
runtime symbol; missing grants produce source-spanned compile diagnostics.

`std/os.getenv` is also lowered for the explicit `os.environment` capability.
It copies the requested key, never returns a borrowed C environment pointer,
and produces the generated Option ABI (`some(string)` or `none()`).

`std/os.env_names` and `std/os.environ` copy and key-sort bounded environment
snapshots. `std/os.setenv` and `std/os.unsetenv` are separate
`os.environment_mutate` operations; they mutate only the generated process and
are never inferred from the host capability set.

Native POSIX filesystem mutation also covers `mkdir`, `mkdir_all`, `remove`,
`remove_all`, `truncate`, `rename`, `link`, `symlink`, `chmod`, `readlink`, and
`write_string`,
with `os.file_mutate`, `os.symlink`, or `os.file_write` grants as documented in
the host-resource ABI. Rooted file creation, read/write, metadata, directory
mutation, rename, chmod, and close use the same confined descriptor model.

`std/os.hostname`, `std/os.getpid`, `std/os.getppid`, `std/os.getuid`,
`std/os.geteuid`, `std/os.getgid`, `std/os.getegid`, and
`std/os.getpagesize` require `os.system_identity`. Native tests assert only
portable predicates (positive IDs, non-empty hostname, and positive page
size), because process identity is intentionally host-specific.

Environment and identity operations remain capability-gated and host-specific.
Adding a capability name does not silently enable an operation whose native
lowering is absent. Process-group cancellation, deterministic environment
overlays, producer-aware generic resource authorization, native HTTP redirects,
generated closure handlers, and Linux generated-C/ASan evidence are covered by
the completed 148B contract. The remaining explicit boundaries are unsupported
flag-derived file grants and callback-backed raw-handle methods, Windows
execution evidence, and TLS when the OpenSSL provider is not available to the
build.

The native backend lowers the typed Osiris IR to generated C. It never invokes
the interpreter or VM as a fallback. Every IR operation that is outside the
generated-C ABI fails during compilation with the source file and span that
introduced it.

## Result and Option ABI

Generated values use a tagged `OValue`. `O_RESULT` and `O_OPTION` each own a
single heap-allocated payload box. The `fbits` field is the presence bit:

- `ok(value)` stores `value` in an `O_RESULT` box with `fbits = 1`.
- `err(error)` stores the structured error in an `O_RESULT` box with
  `fbits = 0`.
- `some(value)` stores `value` in an `O_OPTION` box with `fbits = 1`.
- `none()` stores a null sentinel in an `O_OPTION` box with `fbits = 0`.

The shared `oresultbind` helper handles both Result and Option. Successful
values are returned from the box; failed Results preserve their error record,
and empty Options produce a `not_found` error. The same ABI supports
`unwrap`, `expect`, `or_else`, `to_option`, `to_result`, `collect`,
`issome`, and `isnone` for the parity-tested forms.

## Errors and ownership

`OError` carries the public kind, message, user message, operation, resource,
source, location, cause, stack, metadata, retryability, severity, and redaction
fields. Payload boxes and aggregate buffers are registered with the generated
runtime allocation tracker. Cleanup paths release or invalidate owned handles
before returning through normal, error, cancellation, and task edges.

### Capability-gated native FFI lifecycle

The direct native FFI adapter has one admission boundary and one ownership
boundary:

- `LoadFFILibrary` decodes a strict manifest, requires an absolute library
  path, verifies the SHA-256 digest before `dlopen`, checks the host-granted
  capabilities, and resolves every declared function and handle-release
  symbol before returning a live library.
- `FFILibrary.CallContext` checks the library state, declaration, current
  capability grants, argument shape, and context cancellation before entering
  the foreign call. A canceled context prevents entry. A synchronous foreign
  call that has already entered C cannot be interrupted by Go cancellation;
  callback adapters observe cancellation and return a neutral callback result.
- `FFILibrary.Close` is idempotent but will not unload a library while a call
  or an owned opaque handle remains live. Capability revocation blocks future
  calls and constants, while cleanup of already-owned handles remains allowed
  so revocation cannot strand native resources. Close marks the library as
  closing before leaving the Go-side mutex; concurrent callers wait for that
  one unload attempt, and a native close error leaves the library retryable.
- Opaque handles are borrowed under a mutex for the complete synchronous
  foreign call. Close cannot release a borrowed value, and a losing close
  caller waits for the one release attempt and observes the same release
  error. Library-owned release callbacks cannot be replaced by a caller;
  release is attempted exactly once and active-handle accounting is retired
  even when the foreign release function reports an error.
- Native symbol caches are protected independently from the library lifecycle.
  The `FFILibrary` call count prevents `dlclose` while a cached symbol is in
  use; after close, the library's native adapter is cleared and calls fail
  before reaching the adapter.

These rules deliberately make cancellation cooperative at the FFI boundary:
Osiris does not spawn an untracked goroutine to make an arbitrary C function
appear cancellable, because unloading its library while that function runs
would be unsafe.

## Tasks

The supported native task contract is deterministic and cooperative:

- `spawn` creates an owned deferred thunk;
- the first `await` evaluates it and later awaits reuse the terminal result;
- `cancel` transitions a pending task to a structured `cancelled` failure;
- `parallel` preserves source order and stops at the first terminal failure;
- `try`/`catch` receives the structured error produced by the task boundary.

Host-backed asynchronous scheduler work, cancellation of an already-running
external syscall, and APIs without a native ABI remain compile-time capability
boundaries. Synchronous native file, socket, process, TLS/HTTP, and watch
operations use owned resources and bounded polling; they do not invoke the
interpreter or VM as a fallback.

## Modules, patterns, and diagnostics

Standard-library imports are resolved as static module identities before C
emission. Member calls are selected from the explicit native module table;
dynamic module values and unsupported members produce source-spanned native
diagnostics. Enum, list, type, relational, logical, grouped, guarded, and
repeated-binding patterns lower to generated control-flow tests and bindings.

The maintained native parity tests include generated-source ABI markers,
interpreter/VM/native differential fixtures, malformed and unsupported
diagnostics, cleanup edges, task failure/cancellation, and undefined-behavior
sanitizer runs.

## Linux generated-C evidence

Linux execution evidence is maintained separately from the macOS/POSIX test
run. `internal/osiris/task148b_linux_parity_test.go` compares deterministic
interpreter, VM, and generated-C results with fixed output contracts and runs
an AddressSanitizer-generated executable. It intentionally excludes host IDs,
clocks, environment state, network services, and host-dependent paths so the
same expected output is meaningful across Linux and macOS.

Run the evidence in a Linux kernel/userspace with:

```sh
scripts/test-native-linux.sh
```

The script uses `golang:1.26-bookworm` and `linux/arm64` by default, and can be
overridden with `OSIRIS_LINUX_IMAGE` and `OSIRIS_LINUX_PLATFORM`. It reports
the Go/compiler versions, runs deterministic generated-C parity, then runs
the same generated-C path with `OSIRIS_C_SANITIZERS=address`. A Docker daemon
that is unavailable or does not report a Linux engine is an environmental
blocker; no Linux execution claim is made from the macOS host-side skipped
tests. The 148B evidence was executed successfully in Colima with Go 1.26.5,
Debian GCC 12.2, deterministic interpreter/VM/native comparison, and the
generated-C AddressSanitizer fixture.
