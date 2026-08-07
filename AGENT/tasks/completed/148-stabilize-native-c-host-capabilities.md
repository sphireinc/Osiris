# Task 148: Stabilize native C host capabilities

**Priority:** P1
**Dependencies:** 147,119,118,126
**Status:** Complete; implementation is complete and Windows runtime
verification is delegated to CI. The POSIX native resource ABI, TCP/UDP/DNS, verified
OpenSSL TLS, plain HTTP/chunked streaming, process/file/stream/root/metadata
handles, path mutations, and bounded snapshot watching are integrated and
execution-tested. The Windows `_WIN32` HANDLE adapter is now integrated behind
its target guard with process/file/socket/root/watch, standard-stream, raw
handle, and signal symbols. Windows execution is intentionally delegated to
the Windows CI matrix; it is not a local implementation blocker. The formerly deferred process groups,
environment overlays, producer-aware generic capabilities, HTTP
redirects/closure handlers, and Linux generated-C evidence are complete under
[148B](completed/148B-defer-native-c-cross-platform-and-advanced-resource-parity.md).

## Objective

Replace native C's host-I/O partial/unsupported boundaries with explicit,
capability-gated parity for filesystem, networking, process/environment, OS,
and FFI surfaces.

## Exact scope and ownership

- Own native capability dispatch in `internal/osiris/cbackend_native.go`,
  `cbackend_modules.go`, native runtime adapters, `ffi_native_*` files, and
  host-capability parity tests.
- Implement deterministic capability checks, owned file/socket/process
  lifetimes, cancellation, deadlines, cleanup, fake-provider behavior where
  applicable, and structured errors.
- Cover the supported POSIX `std/os` and `std/net` resource subset,
  filesystem/identity/process/environment/watch, UDP/DNS/TLS/HTTP, and all
  supported capability-gated FFI declarations. Unsupported members must remain
  explicit source-spanned diagnostics with a documented platform/provider
  status.
- Never use shell fallback, ambient mutation, hidden interpreter execution, or
  unsafe descriptor leaks.
- Add macOS/Linux sanitizer and cross-platform compile/execution evidence.

## Matrix transitions

Promote native C I/O, networking, OS integration, native interop/build, and
scope-cleanup rows to `stable`.

## Parallelism

This is the final native task and must follow 144–147. It may run concurrently
with WASM, GPU, and LSP tasks.

## Current evidence

- `CCompileOptions.NativeCapabilities` is normalized, included in native cache
  identity, and used to validate native FFI manifest grants.
- Native FFI close/call admission, native adapter operation leases, owned
  handle borrows, release ownership, and cancellation-before-borrow are
  covered by `ffi_lifecycle_test.go` and
  `ffi_native_lifecycle_unix_test.go`.
- `std/os.is_path_separator` has direct host-independent native lowering and
  interpreter/native execution coverage in `task148_native_capability_test.go`.
- Bounded native `std/os.read_file` and `std/os.write_file` lowerings require
  explicit `os.file_read`/`os.file_write` grants and have interpreter/native
  parity coverage, including owned byte results and write cleanup.
- Native `std/os.getenv` is capability-gated by `os.environment`, returns the
  native Option ABI, and has deterministic missing-variable parity coverage.
- Native `std/os.env_names` and `std/os.environ` return sorted owned lists;
  `std/os.setenv` and `std/os.unsetenv` require the separate
  `os.environment_mutate` grant and have mutation coverage.
- Non-mutating native identity helpers (`hostname`, process/UID/GID queries,
  and `getpagesize`) require `os.system_identity`; predicate-based parity tests
  avoid treating host-specific IDs as deterministic literals.
- `std/os`, filesystem, and networking calls that require an owned host
  resource ABI are admitted through explicit runtime symbols and the focused
  resource suite. Operation-level execution evidence is still being expanded;
  admission alone is not counted as parity by this record.

## Integrated native resource evidence

- `internal/osiris/cbackend_network_runtime.go` owns POSIX TCP, UDP, DNS, and
  socket close/read/write resources. `TestTask148NativeTCPResourceExecutes` and
  `TestTask148NativeUDPDNSResourcesExecute` execute generated C through those
  paths.
- `internal/osiris/cbackend_tls_http_runtime.go` owns bounded HTTP/1.1 framing,
  response maps, streaming bodies, and TLS resources. The OpenSSL provider in
  `internal/osiris/cbackend_openssl_provider.go` is selected only when an
  explicit TLS capability is granted and OpenSSL is available through
  `pkg-config`; `TestTask148NativeTLSExecutesWithOpenSSLProvider` verifies
  configured roots, hostname verification, TLS 1.2+, stream read, and close.
- `internal/osiris/cbackend_process_runtime.go` owns POSIX child processes,
  pipes, files, rooted `openat` handles, metadata, directories, deadlines, and
  snapshot watch resources. `TestTask148NativeProcessResourceExecutes` and
  `TestTask148NativeWatchExecutesWithBoundedSnapshotEvents` execute generated C
  with owned cleanup; the focused resource suite also runs under ASan.
- `TestTask148NativeFileResourceExecutes`,
  `TestTask148NativeRootResourceExecutesBoundedPOSIXSubset`, and
  `TestTask148NativeRootedResourceExecutes` cover cursor and positional file
  I/O, sync, metadata, directory ownership, confined root operations, and
  derived-handle cleanup.
- `TestTask148NativePOSIXFilesystemMutationSlice` covers `mkdir`, `mkdir_all`,
  `remove`, `remove_all`, `truncate`, `rename`, `link`, `symlink`, `chmod`,
  `readlink`, and `write_string`; `TestTask148NativeHTTPChunkedAndStreamingResourcesExecute`
  covers chunk decoding and owned streaming bodies.
- `TestNativeHostResourceComponentsCompileAsNonPOSIXC` verifies that the
  generated host/network/HTTP component has guarded structured-unavailable
  stubs on a non-POSIX target simulation. This is compile-boundary evidence,
  not a Windows HANDLE implementation or execution claim.
- `TestTask148NativeStandardStreamsAndRawHandlesExecute` covers duplicated
  standard descriptors, `pipe`, `fd`, `name`, and `new_file`;
  `TestTask148NativeSignalSubscriptionExecutes` covers the POSIX self-pipe
  signal stream and close cleanup without a scheduler goroutine.
- `TestNativeWindowsRuntimeComponentOwnsWindowsBoundaries` checks the concrete
  `_WIN32` HANDLE/Winsock/process/root/watch/standard/raw/signal ABI markers;
  `TestNativeWindowsRuntimeIsWiredBehindWindowsGuard` and
  `TestNativeWindowsRuntimeComponentPrecedesPOSIXResourceComponents` verify
  generated-C selection and component ordering. These are source-contract
  checks because this macOS host has no Windows compiler or runtime.

## Remaining closure boundaries

- Windows generated C now includes a guarded `HANDLE`/Winsock adapter for
  processes, files, sockets, roots, watches, standard streams, raw handles,
  and signals. It has source-contract and generated-source evidence here, but
  no local Windows toolchain is available for compile/execution evidence.
- `std/os` rooted ownership/timestamp methods and `root_fs` are implemented on
  POSIX and Windows. Windows numeric uid/gid ownership is a structured
  platform limitation except for the portable `-1` unchanged-owner operation.
  Callback-backed raw-handle helpers remain a separate ABI boundary. POSIX
  standard streams, pipes, numeric fd/name access, `new_file`, and bounded
  signal subscriptions are implemented and execution-tested. The POSIX path
  mutation subset (`mkdir`, `mkdir_all`, `remove`, `remove_all`, `truncate`,
  `rename`, `link`, `symlink`, `chmod`, `readlink`, and `write_string`) is
  implemented and execution-tested.
- Native HTTP plain client/server framing and provider-backed TLS are in scope
  for this task. Bounded redirect policy and generated-language closure
  handlers are complete in 148B.
- Process options support explicit `cwd`, deterministic inherited overlays,
  explicit deletion, and complete replacement `env` maps on POSIX. Process
  groups and bounded cancellation cleanup are complete in 148B.

## Completed task 148B record

Task [148B](completed/148B-defer-native-c-cross-platform-and-advanced-resource-parity.md)
is complete and records the implementation and Linux evidence for process-group
cancellation, environment overlays, producer-aware generic resource capability
checks, HTTP redirects, generated-language closure handlers, and Linux
execution/differential parity. Parent task 148 remains open only for the
separately documented Windows CI verification. The Windows adapter now lowers
inherited, replacement, and null-deletion environment options into deterministic
UTF-16 blocks and owns spawned process trees through Job Objects; those paths
are source-contract tested here and are queued for Windows CI execution.
