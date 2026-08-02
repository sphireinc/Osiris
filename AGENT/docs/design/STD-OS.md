# `std/os` system integration

Task 135 expands `std/os` to a capability-bounded system boundary. The
interpreter and VM share the same module exports and receive the current task
context at every potentially blocking operation. Native C reports these
exports as hosted boundaries; it does not silently fall back to host calls.

## Public surface

The module exposes the existing `args`, `env`, `env_names`, process, signal,
and watch operations plus:

- flags and stable error names (`O_RDONLY`, `O_WRONLY`, `O_RDWR`, `O_APPEND`,
  `O_CREATE`, `O_EXCL`, `O_SYNC`, `O_TRUNC`, `dev_null`, and error values);
- `open`, `create`, `open_file`, `create_temp`, `pipe`, `read`, `write`,
  `write_string`, `read_at`, `write_at`, `seek_file`, `sync`, and `close`;
- bounded `read_file` and `write_file`, `read_dir`, `stat`, and `lstat`;
- `mkdir`, `mkdir_all`, `remove`, `remove_all`, `rename`, `truncate`,
  `chmod`, `chown`, `lchown`, `chtimes`, `link`, `symlink`, and `readlink`;
- `getwd`, `chdir`, `copy_fs`, `dir_fs`, `open_root`, the rooted method family,
  and the user-directory helpers;
- `getenv`, `lookup_env`, `environ`, `setenv`, `unsetenv`, and `expand_env`;
- host identity (`hostname`, `getpid`, `getppid`, `getuid`, `geteuid`, `getgid`,
  `getegid`, and `getpagesize`); and
- `find_process`, `start_process`, `process_signal`, `process_release`, and
  `process_state`, alongside the original safe pipe-oriented process API.
- `args_all`, error predicates, `name`, `fd`, `same_file`, `getgroups`,
  `stdin`, `stdout`, `stderr`, and explicit structured unsupported boundaries
  for callback expansion, deadlines, raw descriptor callbacks, and `exit`.

Osiris deliberately returns bounded lists/maps instead of exposing Go
interfaces. File handles are owned `stream` values and participate in the
existing `close` and `defer` lifecycle. Metadata maps contain stable fields
(`name`, `size`, `mode`, `dir`, and `mod_time`); host-specific `FileInfo.Sys`
values and raw descriptors are not serialized into language values.

## Capabilities and safety

Host runtimes must grant the narrow capability required by an operation. The
filesystem capabilities are `os.file_read`, `os.file_write`,
`os.file_metadata`, `os.file_mutate`, `os.directory_read`,
`os.directory_mutate`, `os.symlink`, and `os.root`. Environment mutation is
separate from `os.environment`; identity and process inspection are separate
from process creation, signals, killing, and raw handles. The historical
`os.process` and `os.filesystem` umbrellas remain accepted for compatibility.

Whole-file reads are capped at 1 MiB. Directory results are sorted. Process
arguments remain an explicit vector and are never passed through a shell.
Cancellation is checked before each operation and the normal stream cleanup
path closes descriptors. `chdir`, ambient environment mutation, privilege
changes, and process exit are never performed merely by importing the module.

Root handles are path-confined and reject lexical traversal outside the root;
applications requiring race-resistant descriptor-relative confinement must use
an OS-specific deployment boundary because a portable path API cannot claim
symlink-swap resistance.

## Deterministic test providers

`NewTestRuntime()` installs an in-memory filesystem and file handles, isolated
environment overlay, deterministic identity, fixed clock, platform values,
and a simulated child-process provider. `SetEnvironment` and
`SetCommandLineArgs` configure those fixtures. Application code continues to
call ordinary `std/os` functions, so test authors do not implement a Go fake
provider and a test cannot accidentally read or mutate the developer's host
filesystem.

`osiris test` discovers `*_test.osr` and `.test.osr` files, creates a fresh
test runtime for every case, and emits stable text or JSON events. `-backend
vm` checks the same source through the VM; `-integration` is explicit and
requires requested capabilities. Task 123 may add declaration syntax,
coverage, fuzzing, and golden-file features, but it must retain this provider
and capability contract.

## Intentional differences from Go `os`

The inventory follows Go's package-level filesystem, environment, identity,
and process concepts, but it does not expose unbounded `ReadFile`, ambient
`Exit`, privileged credential mutation, unrestricted `SyscallConn`, or raw
descriptor ownership. Those omissions are safety and determinism boundaries,
not missing implicit fallbacks.
