# Process, Environment, Signals, and Filesystem Watching

`std/os` is the hosted application boundary for operating-system integration.
It is deliberately capability-gated and is not an ambient wrapper around the
host process. The interpreter and VM expose the surface; native C, WASM, and
GPU backends report a source-spanned unsupported diagnostic because they do not
own a portable host-I/O ABI.

## Capability surface

The runtime recognizes these independent capabilities:

| Capability | Operations |
| --- | --- |
| `os.environment` | `env(name)`, `env_names()` |
| `os.process` | `process_spawn`, `process_wait`, `process_kill` |
| `os.signal` | `signal_subscribe(name)` |
| `os.watch` | `watch(path[, recursive])` |

`args()` is read-only and does not require a capability. It returns arguments
after the executable name. Hosts can inject both arguments and environment
maps through `Runtime.SetCommandLineArgs` and `Runtime.SetEnvironment`; an
injected empty value is authoritative and never falls back to the host.

Runtime tests can replace the process and snapshot providers with deterministic
in-memory fakes. This keeps language-level cancellation, stream ownership,
event normalization, and overflow tests independent of ambient executables or
filesystem timing while leaving production runtimes on the platform adapters.

Environment reads return `Option<string>`. Names are sorted, while values are
never copied into permission-denied errors or public diagnostic metadata.
There is no API for mutating the ambient environment.

## Child processes

`process_spawn(command, arguments[, options])` always passes an executable and
an argument vector to `exec.Cmd`. It never concatenates arguments, invokes a
shell, or interprets quoting. Arguments, working directories, environment
keys, and environment values reject NUL bytes. Inherited environment state is
opt-in with `inherit_env: true`; an explicit `env` map is sorted before it is
handed to the host.

The returned process map owns an `os_process` plus stdin, stdout, and stderr
streams. Pipe reads and writes remain streaming and therefore do not create an
unbounded captured-output buffer. Callers must close the streams and process,
normally with scope-bound cleanup. `process_wait` returns `{status, success,
signal}` for normal exits. Timeout or task cancellation terminates the process
group on Unix-like systems; Windows and other platforms use the documented
direct-process fallback.

The runtime does not use `exec.CommandContext` for cancellation because that
primitive only guarantees direct-child termination. Instead, a context watcher
calls the platform process-tree adapter and then lets the single wait owner
reap the child. This makes cancellation, timeout, and explicit close converge
on one cleanup path.

## Signals

`signal_subscribe` is separate from the numerical DSP functions in
`std/signal`. Names are explicit and platform adapters accept the portable
set (`SIGINT`, `SIGTERM`, `SIGHUP`, `SIGUSR1`, and `SIGUSR2` where available).
Notifications are delivered on a bounded queue. Closing the returned event
stream unregisters the host subscription; dropped notifications do not grow
runtime memory without limit.

## Filesystem watching

`watch` uses a portable snapshot adapter. Paths are made absolute, cleaned,
and serialized with slash separators. Event records are JSON lines with
`create`, `modify`, `remove`, and `overflow` categories plus `is_dir` where
known. The watcher queue is bounded. When it fills, it reserves a slot for an
overflow marker and drops delta history; the caller must rescan the path
before applying later events. Close and context cancellation stop the polling
loop and release its queue.

The current portable adapter watches regular files and uses a 50 ms polling
interval. Platform-native notification backends can replace the snapshot
provider later without changing the language contract.

## Diagnostics and backend policy

Host failures are converted to structured Osiris errors with an operation and
`component: "os"`. Host error text is retained only as debug metadata. Native
compilation rejects `std/os` members at their source span rather than emitting
an executable with a hidden host dependency.

Tests cover capability denial and secret redaction, deterministic empty and
non-empty injection, shell-free argument passing, isolated child environments,
pipe ownership, timeout cleanup, signal delivery, normalized watch events,
overflow, cancellation, and native diagnostics.
