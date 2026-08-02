# Task 135: Expand `std/os` to full filesystem and system integration

**Priority:** P1
**Dependencies:** 102, 116, 119, 123
**Status:** Complete

## Objective

Expand `std/os` from its current narrow hosted boundary into a comprehensive,
capability-gated operating-system standard library. The completed surface
should cover the portable functionality users reasonably expect from Go's
`os` package while preserving Osiris-specific safety, deterministic testing,
structured errors, bounded I/O, explicit ownership, cancellation, and backend
capability rules.

The Go `os` package is the compatibility reference for the inventory. It
includes file handles and streaming I/O, file metadata and modes, directory
operations, path mutation, environment and system identity, process handles
and process state, rooted filesystem access, error classification, open flags,
standard streams, and platform-specific capability reporting. See the current
Go reference at <https://pkg.go.dev/os>.

This task must produce an Osiris-shaped API, not a line-for-line copy of Go's
raw syscall-oriented API. Operations that mutate global state, expose a raw
handle, bypass a configured root, terminate the runtime, or depend on a
platform-specific privilege must require a distinct capability and have a
documented portable fallback or denial behavior.

## Current baseline and gap

The existing `std/os` module exposes only:

- `args`
- `env`
- `env_names`
- `process_spawn`
- `process_wait`
- `process_kill`
- `signal_subscribe`
- `watch`

Task 135 adds the remaining filesystem, file-handle, metadata, environment,
system identity, rooted-access, error, and process-state surfaces described
below. Existing APIs must remain source-compatible unless a security or
ownership defect requires a documented breaking change.

## Required API inventory

Names below are the required semantic inventory. The final Osiris spelling may
use the repository's snake-case convention, but every item must have either a
public `std/os` export or an explicit documented decision that it belongs in a
more appropriate module such as `std/path`, `std/fs`, or `std/time`.

### 1. Open flags, seek modes, errors, and standard streams

Add stable constants and structured error classification for:

- Open access modes: `O_RDONLY`, `O_WRONLY`, `O_RDWR`.
- Open modifiers: `O_APPEND`, `O_CREATE`, `O_EXCL`, `O_SYNC`, `O_TRUNC`.
- Seek modes: `SEEK_SET`, `SEEK_CUR`, `SEEK_END` or Osiris names mapped to the
  canonical `std/io` seek constants.
- Invalid argument, permission, exists, not-found, closed, no-deadline,
  deadline-exceeded, process-done, and no-handle conditions.
- Error predicates equivalent to `IsExist`, `IsNotExist`, `IsPermission`, and
  `IsTimeout`, operating on structured Osiris errors and wrapped host causes.
- Structured path, link, and syscall error records with operation, path,
  source, target, host code, public metadata, and debug-only detail.
- Explicit `stdin`, `stdout`, and `stderr` handles. They must be injected or
  capability-granted resources, never silently opened from ambient descriptors.

The package-variable compatibility audit must explicitly map these Go
`os`-level variables:

| Go variable | Required Osiris decision |
| --- | --- |
| `Args` | Preserve the existing `args()` application-argument contract for source compatibility, and add a full-argv form such as `args_all()` that includes the executable/program name. Both forms must be deterministic under host injection. |
| `Stdin`, `Stdout`, `Stderr` | Provide explicit standard-stream resources with ownership, capability, close, and host-injection rules. They must not be ambient mutable globals in deterministic or sandboxed runtimes. |
| `ErrInvalid`, `ErrPermission`, `ErrExist`, `ErrNotExist`, `ErrClosed` | Provide stable Osiris error-kind values or equivalent matchable sentinels. `errors.Is`-style classification must work through path/link/syscall wrappers. |
| `ErrNoDeadline`, `ErrDeadlineExceeded` | Provide stable deadline error kinds and preserve cancellation/timeout distinction in structured errors. |
| `ErrProcessDone`, `ErrNoHandle` | Provide stable process lifecycle error kinds for wait, release, signal, kill, and raw-handle operations. |
| `Interrupt`, `Kill` | Add named signal values or an equivalent typed signal namespace. `Kill` must remain distinct from ordinary cancellable process-tree cleanup. |

The compatibility audit must also cover the package constant `DevNull`, with a
portable null-device abstraction rather than assuming `/dev/null` on every
target. These mappings are required acceptance evidence; “covered by the
general error or process sections” is not sufficient.

The API must document which flags are portable, which are advisory, and which
are unsupported on a target platform. Unknown or contradictory flag
combinations must fail before a host syscall.

### 2. File handles and bounded streaming I/O

Add an owned `os_file` resource, integrated with existing stream ownership and
scope-bound cleanup:

- `open`, `create`, `open_file`, `create_temp`, and an explicit pipe operation.
- `close`, `name`, `fd`/raw-handle access only under a separate unsafe handle
  capability, `stat`, `sync`, `seek`, and `truncate`.
- `read`, `write`, `read_at`, `write_at`, and `write_string` with explicit byte
  limits, partial-progress reporting, and cancellation-aware variants.
- `read_from` and `write_to`/copy operations that preserve backpressure and do
  not allocate an entire source or destination in memory.
- `read_file` and `write_file` convenience operations with mandatory or
  host-configured maximum byte bounds.
- `read_dir`, `readdir`, and `readdir_names` with bounded counts, deterministic
  ordering, and explicit handling for directories changing during iteration.
- Read, write, and combined deadline operations where the platform supports
  them; unsupported deadlines return a structured `no_deadline` error.
- `chmod`, `chown`, `lchown`, `chtimes`, `chdir`, and file-level operations with
  separate metadata/mutation capabilities.

File methods must be safe for concurrent use where the host permits it, must
serialize offset-mutating operations when required, and must not expose a
closed descriptor through a stale `Value`. Every returned handle must have a
single documented owner and work with `defer`/scope cleanup.

### 3. File information, modes, and directory entries

Add typed or structured values equivalent to:

- `FileInfo`: name, size, mode, modification time, directory bit, and stable
  type information; document what is unavailable on each platform.
- `FileMode`: regular file, directory, symlink, device, named pipe, socket,
  setuid/setgid/sticky, and permission bits where meaningful.
- `DirEntry`: name, directory/type bits, and lazy bounded `info` lookup.
- `stat`, `lstat`, and `same_file`, with symlink-following behavior explicit.

Serialization must be deterministic and must not leak host-specific raw
structures, usernames, device numbers, or debug metadata without permission.

### 4. Path and filesystem mutation

Add capability-gated operations equivalent to:

- `mkdir`, `mkdir_all`, `mkdir_temp`.
- `remove`, `remove_all`, `rename`, `link`, and `symlink`.
- `readlink` and symlink-aware `stat`/`lstat`.
- `truncate` and path-level `chmod`, `chown`, `lchown`, and `chtimes`.
- `getwd`, `chdir`, `executable`, and `temp_dir`.
- `user_home_dir`, `user_config_dir`, and `user_cache_dir`, returning
  capability-aware `Option`/`Result` values rather than assuming a desktop
  layout.
- `copy_fs` or an Osiris-equivalent bounded tree copy operation.
- `dir_fs` or an explicit bridge to the repository's filesystem abstraction.

All recursive operations require an explicit maximum depth, entry count, and
total byte budget. `remove_all` must not follow directory symlinks by default.
Rename, link, and symlink operations must report cross-device and unsupported
platform errors without silently falling back to copy/delete behavior.

### 5. Rooted and traversal-safe filesystem access

Add a rooted resource equivalent to Go's `OpenRoot`/`Root` API:

- `open_root(path)` creates an owned root capability.
- Root methods include `open`, `open_file`, `create`, `read_file`, `write_file`,
  `readlink`, `stat`, `lstat`, `mkdir`, `mkdir_all`, `remove`, `remove_all`,
  `rename`, `link`, `symlink`, `chmod`, `chown`, `chtimes`, and `fs`.
- Relative names must reject absolute paths, NUL bytes, invalid components,
  and traversal outside the root.
- Symlink escape policy must be explicit and tested. Where the host cannot
  guarantee kernel-enforced confinement, the operation must deny unsafe paths
  rather than claim equivalent security.
- Closing a root invalidates every derived handle that depends on it, or the
  ownership model must explicitly retain an independent safe descriptor.

This surface is required for safe package/resource extraction, configuration
loading, archive expansion, and untrusted workspace handling.

### 6. Environment and process-global state

Extend the current read-only environment model with explicit, isolated
operations:

- `getenv`, `lookup_env`, and `environ` aliases/structured forms.
- `expand_env` and deterministic `expand` with an injected mapping.
- `setenv`, `unsetenv`, and `clearenv` only against a runtime-owned child/
  process environment overlay by default. Mutating the host process must
  require a separate `os.environment_mutate` capability and be opt-in.
- `hostname`, `getwd`, `executable`, and `temp_dir`.
- `getpid`, `getppid`, `getuid`, `geteuid`, `getgid`, `getegid`, `getgroups`,
  and `getpagesize`, with platform-availability results and explicit privacy
  policy.
- A capability/platform report describing supported file flags, deadlines,
  signals, identity fields, raw handles, and privilege-dependent operations.

No operation may clear or mutate ambient environment state merely because a
program imported `std/os`. Global mutation must be serializable, auditable,
and disabled in deterministic or sandboxed hosts.

### 7. Process handles and process state

Expand the current child-process API with a typed process/state model:

- `find_process(pid)` with explicit inspect capability and safe behavior for
  inaccessible or already-exited processes.
- `start_process(name, argv, attributes)` as a lower-level process operation,
  while retaining `process_spawn` as the safe pipe-oriented API.
- Process attributes for working directory, environment overlay, standard
  stream wiring, credentials only when explicitly supported, and process-group
  policy.
- `process_signal`, `process_release`, `process_wait`, `process_kill`, and
  cancellation-aware variants with idempotent ownership rules.
- Process state fields/methods equivalent to `exit_code`, `exited`, `pid`,
  `success`, `signal`, `user_time`, `system_time`, and a redacted portable
  representation of host usage data.
- Explicit `process_done` and `no_handle` errors.
- An unsafe, separately gated raw-handle callback only if the runtime can
  guarantee no handle escapes its callback lifetime. Otherwise document the
  operation as intentionally unsupported.
- `exit(code)` only as an explicitly gated terminal operation that runs scope
  cleanup according to the language contract, emits no misleading success
  result, and is unavailable to ordinary library code.

Process tree cleanup, cancellation, output bounds, and reaping must remain
stronger than the compatibility reference where required by Osiris task and
resource semantics.

### 8. Additional platform operations and explicit decisions

Audit every Go `os` symbol not listed above and record one of:

1. implemented in `std/os`;
2. moved to a more coherent Osiris module with a cross-reference;
3. capability-gated and implemented only on supported platforms;
4. intentionally unsupported with a stable structured error and rationale.

The audit must explicitly cover `NewFile`, `File.Fd`, `File.SyscallConn`,
`Process.WithHandle`, `ProcAttr`, `SyscallError`, `CopyFS`, `DirFS`, `Root.FS`,
and all user-directory, identity, link, deadline, and file-copy functions.

## Capability model

Define separate capabilities rather than one broad `os` permission:

- `os.file_read`
- `os.file_write`
- `os.file_metadata`
- `os.file_mutate`
- `os.directory_read`
- `os.directory_mutate`
- `os.symlink`
- `os.root`
- `os.environment`
- `os.environment_mutate`
- `os.system_identity`
- `os.process_inspect`
- `os.process_spawn`
- `os.process_signal`
- `os.process_kill`
- `os.process_raw_handle`
- `os.signal`
- `os.watch`
- `os.exit`

Capabilities must be visible in manifests/host configuration, included in
structured denial diagnostics without secret values, and enforced consistently
by interpreter, VM, task helpers, CLI execution, and deterministic providers.

## Application test behavior

When an Osiris application test runs through the language-native test harness,
the harness must install the deterministic fake provider bundle by default.
Application tests should be able to call ordinary `std/os` APIs without
implementing Go-side or runtime-side fake providers themselves.

The test author supplies only language-level fixtures and expectations, such
as an in-memory directory tree, an injected environment map, a simulated child
process result, or a platform capability profile. The harness routes the
ordinary `std/os` calls through the fake filesystem, file-handle, environment,
process, identity, clock, and platform providers while preserving the same
capability checks, limits, errors, ownership, cancellation, and cleanup rules
used by production providers.

Real host access must be opt-in. A test that intentionally exercises the host
filesystem, launches a real process, reads host identity, or observes real OS
signals must be marked/configured as an integration test and request the
corresponding capability explicitly. Ordinary application tests must not
silently escape into the developer's filesystem, environment, processes, or
credentials.

This means a TODO-list test can write normal Osiris code such as:

```osiris
# app/todo_test.osr (discovered by `osiris test app`)
import "std/os" as os
let wrote = os.write_file("/todos/today.json", "[]", 420)
let contents = os.read_file("/todos/today.json")
emit [wrote, contents]
```

The current fixture is the ordinary `std/os` API itself: the harness owns
provider lifecycle and resets fake state between files. Task 123 may add
assertion/declaration syntax and scoped fixtures, but a test must never need to
construct or register a provider implementation.

## Implementation requirements

- Preserve explicit argument vectors and shell-free process execution.
- Keep all file and process output bounded; no convenience API may hide an
  unbounded allocation behind `read_file`, directory traversal, or copy.
- Integrate every handle with streams, task cancellation, scope-bound cleanup,
  and close-on-error behavior.
- Use build-tagged adapters for Unix, Windows, and unsupported targets.
- Prefer kernel-enforced root confinement and document weaker fallbacks.
- Use deterministic sorting for directory entries, environment names, map-like
  metadata, and serialized error fields.
- Preserve public/debug metadata separation and redact secrets, credentials,
  environment values, raw paths, and host handles at trust boundaries.
- Keep native C, WASM, and GPU behavior explicit. Hosted-only operations must
  fail at source span with a stable capability diagnostic; no hidden host-I/O
  delegation is allowed.
- Add CLI JSON diagnostics and support-matrix entries for capability denials,
  unsupported platform features, and resource-bound failures.
- Add LSP/reference metadata for every public symbol and every deliberate
  non-goal.

## Tests and validation

### Functional and security tests

- Open flag validation, create/exclusive/truncate/append/sync behavior.
- File read/write/seek/read-at/write-at, partial progress, sync, truncate,
  write-string, bounded copy, and pipe backpressure.
- Directory ordering, bounded iteration, changing-directory behavior, file
  metadata, modes, symlink-following, `stat`/`lstat`, and same-file identity.
- Recursive mkdir/remove/copy limits; rename/link/symlink behavior; no
  symlink traversal in destructive recursion.
- Root confinement, `..` rejection, absolute-path rejection, symlink escape,
  root close, and concurrent derived-handle behavior.
- Temporary files/directories, user directory lookup, executable lookup, and
  missing-platform identity fields.
- Environment overlay isolation, expansion, mutation denial, and no ambient
  host mutation in tests.
- Process lookup/start/signal/release/wait/state mapping, process-group
  cleanup, timeout/cancellation, reaping, stream ownership, and raw-handle
  denial.
- Error wrapping/classification, path/link/syscall metadata, secret redaction,
  and stable CLI JSON diagnostics.

### Determinism and portability

- In-memory fake filesystem, file handles, environment, identity provider,
  process provider, clock, and platform capability provider.
- Race tests for concurrent file operations, close/read/write, root lifetime,
  process wait/kill, and watcher cancellation.
- Linux, macOS, Windows, and unsupported-target compile checks; platform tests
  must skip only when the capability is genuinely unavailable and must assert
  the documented fallback.
- Native C rejection tests with source spans for every hosted-only operation.
- Full interpreter/VM parity, CLI examples, docs/reference checks, `go vet`,
  normal tests, race tests, sanitizer tests, and repository builds.

## Documentation requirements

- Expand the `std/os` reference page with the complete API inventory,
  signatures, capabilities, limits, ownership, cancellation, platform matrix,
  and examples.
- Add a technical design document covering file handles, rooted access,
  capability boundaries, process state, error taxonomy, and platform adapters.
- Add a Go-compatibility audit table mapping every current Go `os` symbol to an
  Osiris symbol, another module, or an explicit unsupported decision.
- Document why Osiris intentionally differs from Go for ambient environment
  mutation, raw descriptors, `Exit`, privileged identity changes, unbounded
  reads, and process-handle escape.
- Update standard-library navigation, runtime-kind/type metadata, support
  matrix, CLI JSON diagnostics, and generated examples.

## Acceptance criteria

- [x] The Go `os` inventory is audited completely and every symbol has an
  implemented, relocated, capability-gated, or explicitly unsupported result.
- [x] Files, directories, metadata, path mutation, temporary resources, and
  rooted access are available with bounded ownership-safe semantics.
- [x] Process handles/state and environment/system identity cover the approved
  portable surface without ambient mutation or hidden privilege escalation.
- [x] Capability declarations and denials are consistent across interpreter,
  VM, CLI, deterministic providers, and native/backend gates.
- [x] File/process resources participate correctly in cancellation,
  scope-bound cleanup, backpressure, and error paths.
- [x] Deterministic fake providers cover filesystem, environment, identity,
  process, clock, and platform capability behavior.
- [x] Normal, race, sanitizer, cross-platform compile, native diagnostic, and
  documentation validation passes with evidence recorded in this task.
- [x] This task is moved to `AGENT/tasks/completed/` only after every required
      checkbox and the compatibility audit are complete.

## Current implementation evidence

The hosted std/os expansion is implemented in `internal/osiris/os_std.go` and
`internal/osiris/os_std_extra.go`. It includes capability-gated owned file
handles, bounded whole-file and transfer APIs, sorted directory metadata,
temporary files/directories, path mutation and links, environment overlays,
identity/platform reporting, rooted path handles, process state helpers, and
borrowed standard streams. `NewTestRuntime` installs the in-memory filesystem,
file handles, environment, identity, platform, fixed clock, and simulated
process providers automatically; `os_std_test.go` proves both direct and
ordinary language-level std/os calls use those providers without Go-side fake
registration.

Documentation is recorded in `AGENT/docs/design/STD-OS.md`,
`AGENT/docs/design/STD-OS-GO-AUDIT.md`, the public `std/os` reference group,
and the support matrix. Unix/Windows identity adapter compilation was added;
Linux and Windows package test binaries compile from this macOS host.

The compatibility boundaries are explicit rather than hidden gaps: callback
based `os.expand`, raw Go interface/descriptor callbacks, portable file
deadlines, and ambient `os.exit` return stable structured unsupported errors;
task 123 may add richer test declarations without replacing the implemented
`osiris test` provider contract. Validation evidence for this task is recorded
below.

Validation evidence:

- `GOTOOLCHAIN=local go test ./... -count=1` passed on Go 1.26.1 darwin/arm64.
- `go test -race ./internal/osiris -run 'TestStdOS|TestOS' -count=1` passed.
- LLVM AddressSanitizer native parity passed with
  `OSIRIS_C_SANITIZERS=address` and Homebrew LLVM clang.
- Linux/amd64 and Windows/amd64 package and CLI test binaries cross-compiled
  successfully with `go test -c`.
- Native hosted-OS rejection tests passed with source-spanned diagnostics.
- `go vet ./...`, `go build ./...`, `jq` validation for all changed JSON, and
  the docs reference/navigation checks under Node 24.12.0 passed.
- The CLI test runner and language test runner passed with both interpreter
  and VM backends; each ordinary test received fresh deterministic providers.

Documentation correction after completion:

- The public `std/os` reference now contains 159 individual export entries;
  no category entry such as `filesystem` is used as a substitute for
  signatures and behavior.
- The technical reference indexes the same 159 names, and the reference
  checker extracts `stdOSModule` to reject missing, stale, or undocumented
  exports and to require per-entry syntax, parameters, returns, values,
  details, and examples.

## Non-goals and safety boundaries

- No implicit shell, unrestricted raw syscall API, or unbounded file/tree read.
- No default mutation of the host working directory or ambient environment.
- No privilege escalation, credential-changing APIs, or raw process handles
  without a separately reviewed capability and lifetime-safe design.
- No promise that platform-specific Go behavior exists on every target; each
  exception must be explicit, testable, and documented.
