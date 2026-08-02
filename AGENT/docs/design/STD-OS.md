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

## Per-symbol documentation contract

The lists above describe design areas only; they are not the public API
reference. Every exported name below is a separate `std/os` reference entry.
Each entry has its own syntax, accepted parameters, return type, returned
value shape, errors, capability requirements, ownership/cancellation rules,
backend status, and example. No slash-separated family entry is allowed to
stand in for those individual contracts. The technical reference also indexes
the same names, and `docs/scripts/check-reference.mjs` compares this index with
the actual `stdOSModule` export table.

```text
std/os.args
std/os.args_all
std/os.chdir
std/os.chmod
std/os.chown
std/os.chtimes
std/os.clearenv
std/os.close
std/os.copy_fs
std/os.create
std/os.create_temp
std/os.dir_fs
std/os.env
std/os.env_names
std/os.environ
std/os.executable
std/os.exit
std/os.expand
std/os.expand_env
std/os.fd
std/os.find_process
std/os.getegid
std/os.getenv
std/os.geteuid
std/os.getgid
std/os.getgroups
std/os.getpagesize
std/os.getpid
std/os.getppid
std/os.getuid
std/os.getwd
std/os.hostname
std/os.is_exist
std/os.is_not_exist
std/os.is_path_separator
std/os.is_permission
std/os.is_timeout
std/os.lchown
std/os.link
std/os.lookup_env
std/os.lstat
std/os.mkdir
std/os.mkdir_all
std/os.mkdir_temp
std/os.name
std/os.new_file
std/os.new_syscall_error
std/os.open
std/os.open_file
std/os.open_root
std/os.path_exists
std/os.path_not_exists
std/os.pipe
std/os.platform
std/os.process_kill
std/os.process_release
std/os.process_signal
std/os.process_spawn
std/os.process_state
std/os.process_wait
std/os.read
std/os.read_at
std/os.read_dir
std/os.read_dir_entries
std/os.read_file
std/os.read_from
std/os.readlink
std/os.remove
std/os.remove_all
std/os.rename
std/os.root_chmod
std/os.root_chown
std/os.root_chtimes
std/os.root_close
std/os.root_create
std/os.root_fs
std/os.root_lchown
std/os.root_link
std/os.root_lstat
std/os.root_mkdir
std/os.root_mkdir_all
std/os.root_open
std/os.root_open_file
std/os.root_read_file
std/os.root_readlink
std/os.root_remove
std/os.root_remove_all
std/os.root_rename
std/os.root_stat
std/os.root_symlink
std/os.root_write_file
std/os.same_file
std/os.seek_file
std/os.set_deadline
std/os.set_read_deadline
std/os.set_write_deadline
std/os.setenv
std/os.signal_subscribe
std/os.start_process
std/os.stat
std/os.stdin
std/os.stdout
std/os.stderr
std/os.symlink
std/os.sync
std/os.syscall_conn
std/os.temp_dir
std/os.truncate
std/os.unsetenv
std/os.user_cache_dir
std/os.user_config_dir
std/os.user_home_dir
std/os.watch
std/os.with_handle
std/os.write
std/os.write_at
std/os.write_file
std/os.write_string
std/os.write_to
std/os.O_APPEND
std/os.O_CREATE
std/os.O_EXCL
std/os.O_RDONLY
std/os.O_RDWR
std/os.O_SYNC
std/os.O_TRUNC
std/os.O_WRONLY
std/os.SEEK_CUR
std/os.SEEK_END
std/os.SEEK_SET
std/os.dev_null
std/os.err_closed
std/os.err_deadline_exceeded
std/os.err_exist
std/os.err_invalid
std/os.err_no_deadline
std/os.err_no_handle
std/os.err_not_exist
std/os.err_permission
std/os.err_process_done
std/os.interrupt
std/os.kill
std/os.mode_append
std/os.mode_char_device
std/os.mode_device
std/os.mode_dir
std/os.mode_exclusive
std/os.mode_irregular
std/os.mode_named_pipe
std/os.mode_perm
std/os.mode_setgid
std/os.mode_setuid
std/os.mode_socket
std/os.mode_sticky
std/os.mode_symlink
std/os.mode_temporary
std/os.mode_type
std/os.path_list_separator
std/os.path_separator
```

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
