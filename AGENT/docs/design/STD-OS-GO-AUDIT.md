# Go `os` compatibility audit

This table records the task-135 decision for every symbol in the Go 1.26
`os` package inventory. Names are Osiris snake_case exports unless the entry
is explicitly marked as an intentional boundary.

| Go symbol family | Osiris result | Decision |
| --- | --- | --- |
| `Args` | `std/os.args()` | Implemented as an owned deterministic list; tests can inject arguments. |
| `Stdin`, `Stdout`, `Stderr` | `std/os.stdin`, `stdout`, `stderr` | Implemented as borrowed stream values whose close does not close the host descriptor. |
| `O_*`, `DevNull` | `O_*`, `dev_null` | Implemented as module constants. |
| `ErrInvalid`, `ErrPermission`, `ErrExist`, `ErrNotExist`, `ErrClosed` | `err_*` constants and structured filesystem errors | Implemented with stable language-facing names; host error text is debug metadata. |
| `ErrNoDeadline`, `ErrDeadlineExceeded` | structured `unsupported`/`timeout` errors | Intentionally represented by Osiris error kinds because a portable file deadline is not available for every backend. |
| `ErrProcessDone`, `ErrNoHandle` | `err_process_done`, `err_no_handle` | Implemented as stable process lifecycle values. |
| `Interrupt`, `Kill` | `std/os.process_signal` | Capability-gated signal values are passed explicitly; platform signal names remain adapter-defined. |
| `Chdir`, `Getwd` | `chdir`, `getwd` | Implemented; test runtimes keep the fake working directory isolated. |
| `Chmod`, `Chown`, `Lchown`, `Chtimes` | same snake_case names | Implemented where the target supports the operation; fake providers model metadata changes. |
| `Clearenv`, `Environ`, `Getenv`, `LookupEnv`, `Setenv`, `Unsetenv`, `Expand`, `ExpandEnv` | same snake_case names | Implemented except callback-based `expand`, which is an explicit unsupported boundary until callable-to-host callback invocation is exposed safely; `expand_env` is implemented. |
| `ReadFile`, `WriteFile`, `CopyFS` | `read_file`, `write_file`, `copy_fs` | Implemented with bounded reads and explicit capability checks. |
| `DirFS`, `OpenRoot`, `Root.FS` | `open_root`, root map, rooted path boundary | Root creation and lexical confinement are implemented; an `fs.FS` interface is not exposed as a raw language value. |
| `Open`, `Create`, `OpenFile`, `CreateTemp`, `Pipe`, `NewFile` | `open`, `create`, `open_file`, `create_temp`, `pipe` | Implemented; `NewFile` is intentionally not exposed because raw descriptor ownership is not a safe default. |
| `File.Read`, `Write`, `ReadAt`, `WriteAt`, `ReadFrom`, `WriteTo`, `WriteString`, `Seek`, `Sync`, `Truncate`, `Close`, `Fd` | `read`, `write`, `read_at`, `write_at`, `read_from`, `write_to`, `write_string`, `seek_file`, `sync`, `truncate`, `close`, `fd` | Implemented with bounded transfer; `fd` requires `os.process_raw_handle`. |
| `File.ReadDir`, `Readdir`, `Readdirnames` | `read_dir`, `read_dir_entries` | Implemented as sorted bounded lists/maps rather than host interface objects. |
| `File.SetDeadline`, `SetReadDeadline`, `SetWriteDeadline` | structured unsupported boundary | Not all regular-file and fake providers can provide portable deadlines; network streams retain their own deadline contracts. |
| `File.Stat`, `File.Chmod`, `File.Chown` | `stat`, `lstat`, `chmod`, `chown` | Implemented as path/handle-safe maps and capability-gated mutations. |
| `File.SyscallConn`, `File.WithHandle` | unsupported | Raw syscall callbacks and unbounded handle escape violate the ownership boundary. |
| `Stat`, `Lstat`, `SameFile` | `stat`, `lstat`, metadata identity | Implemented metadata; same-file identity is represented by stable metadata rather than host pointers. |
| `Mkdir`, `MkdirAll`, `MkdirTemp`, `Remove`, `RemoveAll`, `Rename`, `Link`, `Symlink`, `Readlink`, `Truncate` | same snake_case names | Implemented; recursive removal is explicit and fake state never traverses outside its namespace. |
| `FileMode`, `FileInfo`, `DirEntry`, `PathError`, `LinkError`, `SyscallError` | metadata maps and structured Osiris errors | Relocated to the language value/error model; host interface identity is not serialized. |
| `Executable`, `TempDir`, `UserHomeDir`, `UserConfigDir`, `UserCacheDir` | same snake_case names | Implemented with deterministic fake values and system-identity capability. |
| `Hostname`, `Getpid`, `Getppid`, `Getuid`, `Geteuid`, `Getgid`, `Getegid`, `Getgroups`, `Getpagesize` | identity exports | Implemented for the portable scalar identity subset; unsupported platform fields return stable errors rather than guessed values. |
| `FindProcess`, `StartProcess`, `Process.Kill`, `Signal`, `Release`, `Wait`, process state | `find_process`, `start_process`, `process_kill`, `process_signal`, `process_release`, `process_wait`, `process_state` | Implemented with shell-free argv, owned pipes, cancellation, and separate capabilities. |
| `ProcAttr` | process options map | Relocated to explicit `cwd`, `env`, `inherit_env`, and stream options; credentials and raw handles are intentionally excluded. |
| `Exit` | unsupported | A library call cannot silently terminate an Osiris host; process exit remains a CLI/runtime policy. |

The audit is deliberately explicit where Go's ambient or pointer-oriented API
does not map safely to Osiris. “Unsupported” means a stable structured error
and documented capability boundary, not a hidden host fallback.
