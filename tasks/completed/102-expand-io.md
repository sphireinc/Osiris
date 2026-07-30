# Task 102: Expand file and stream I/O safely

**Priority:** P2  
**Dependencies:** 014, 025, 039, 040, 045, 046  
**Status:** Complete  

## Objective

Expand the existing Osiris I/O helpers into a coherent file, directory, and
seekable-stream API. Use the C I/O reference as a capability inventory, not as
an instruction to expose `FILE*`, global streams, host `errno`, or unsafe
format-string behavior.

Reference inventory: <https://cppreference.com/c/io>

## Existing surface to preserve

Audit the implementation and canonical I/O reference first. Preserve and reuse
the contracts for `read`, `readbytes`, `write`, `lines`, `csv`, `jsonparse`,
`openread`, `openwrite`, `readline`, `readchunk`, `writechunk`, `flush`,
`close`, `transfer`, and their existing task-aware forms. Do not create
synonyms for operations already covered.

## Required first-release additions

Add the following operations with final names adapted to Osiris's existing
lowercase/snake_case style:

- whole-file operations: `append`, `read_limit`, `copy_file`, and
  `truncate_file`;
- metadata queries: `exists`, `file_size`, `is_file`, and `is_dir`;
- directory operations: `mkdir`, `mkdir_all`, and `read_dir`;
- path mutation: `rename` and `remove`;
- streams: `openappend`, `seek`, `tell`, and `truncate`;
- standard stream access only if it can be expressed as ordinary borrowed
  `stream` values with explicit non-ownership and close behavior.

`read_dir` must return entries in deterministic lexical order. Define a stable
public entry representation before exposing metadata beyond the entry name.
Do not return a host `os.FileInfo` or C `dirent` shape.

## Semantic and safety contract

Write or update `docs/design/FILESYSTEM-IO.md` with:

- accepted path and data value kinds;
- overwrite, append, creation, permission, and truncation rules;
- half-open seek offsets and the accepted origins (`start`, `current`, `end`);
- ownership of every returned stream and whether standard streams are borrowed;
- close/flush idempotence and operations on a closed stream;
- cancellation and task behavior for blocking operations;
- bounded-memory behavior for `read_limit`, `copy_file`, and directory reads;
- symlink behavior, atomicity guarantees, and cross-filesystem rename behavior;
- mapping of host failures to stable structured error kinds and safe metadata.

Every destructive operation must resolve only the explicit path supplied by
the program. Do not add recursive deletion in this task. Tests must operate
only inside isolated temporary directories.

## Execution paths

- Implement one runtime contract shared by interpreter and bytecode VM.
- Preserve byte/text distinctions and exact byte counts at the `Value`
  boundary.
- Add async wrappers only through the existing task/context machinery; do not
  create detached goroutines or a second scheduler.
- Native C must either implement the same ownership/error contract or reject
  the operation during compilation with a clear source-aware diagnostic.
- Keep platform-specific code behind narrow, build-tagged adapters when host
  behavior differs.

## Documentation

- Expand the canonical I/O page with an individual listing for every new
  function, including accepted values, return type/value, ownership, errors,
  native support, and examples.
- Update search, backend support, and the Technical Reference complete surface
  indexes. All self-contained examples must run through the documentation test;
  filesystem examples use explicit sandbox fixtures.

## Required tests

- interpreter/VM parity for each new helper;
- temporary-directory tests for append, truncation, copying, renaming,
  directory ordering, and metadata;
- seek/tell tests at start/current/end, negative offsets, end-of-file, and
  closed streams;
- short writes, bounded reads, cancellation, close-unblocks-operation, and
  flush behavior under `go test -race`;
- permissions, missing paths, already-existing paths, non-directory parents,
  symlinks, cross-device rename behavior where testable, and redacted
  structured-error metadata;
- Windows, macOS, and Linux path/platform coverage where CI is available;
- generated-C/executable parity for supported native forms and compile
  diagnostics for unsupported forms;
- checked documentation fixtures and search coverage.

## Acceptance criteria

- The required file and stream operations have explicit ownership,
  determinism, cancellation, and structured-error contracts.
- Directory results are stable and whole-file helpers have documented memory
  limits.
- Interpreter/VM parity and race-enabled lifecycle tests pass.
- Native behavior is either proven equivalent or rejected at compile time.
- Canonical and technical documentation are complete and validated.

## Non-goals

- No direct C `FILE*`, variadic `printf`/`scanf`, or mutable global stream
  state.
- No recursive removal, memory mapping, filesystem watching, advisory locking,
  terminal control, archive/compression format, or raw file-descriptor escape
  hatch.
- No claim that a buffered hosted implementation is zero-copy.

## Completion record — 2026-07-30

### Delivered contract

- Added the 17 required global operations: `append`, `read_limit`,
  `copy_file`, `truncate_file`, `exists`, `file_size`, `is_file`, `is_dir`,
  `mkdir`, `mkdir_all`, `read_dir`, `rename`, `remove`, `openappend`, `seek`,
  `tell`, and stream `truncate`.
- `docs/design/FILESYSTEM-IO.md` defines path/data values, modes and umask,
  overwrite/append/truncation, owned streams, close/flush behavior, logical
  seek positions, task cancellation, memory bounds, symlinks, rename
  portability, non-atomic copies, and stable error metadata.
- `read_limit` returns at most 1,048,576 exact `uint8` elements.
  `copy_file` and append writes use a reusable 32 KiB chunk boundary.
  `read_dir` defaults to 4,096 entries, has a 65,536-entry hard maximum, and
  fails rather than returning a partial result.
- Directory entries are public base-name strings sorted in lexical byte order.
  No host metadata object is exposed.
- `openread`, `openwrite`, and `openappend` return owned streams. Standard
  streams were deliberately not added because the current value model cannot
  express their borrowed, non-closeable ownership without exceptional rules.
- New calls use the existing context-builtin and task/session machinery. No
  detached scheduler or filesystem goroutine path was introduced.
- Native C resolves the names into IR but rejects every new filesystem builtin
  at its identifier source span. Compilation never evaluates a filesystem call
  and does not synthesize output.

### Test and platform evidence

- `internal/osiris/filesystem_io_test.go` proves interpreter/VM parity for all
  additions; append/copy/truncate/rename/metadata/directory behavior; logical
  `seek`/`tell` at start/current/end and beyond EOF; negative and closed-stream
  failures; short and zero writes; cancellation; close-unblocks-read;
  concurrent flush lifecycle; permissions; missing/already-existing and
  non-directory paths; symlink behavior; cross-device error classification;
  redacted debug metadata; inference; and source-aware native diagnostics.
- Filesystem tests use `t.TempDir()` only. Cross-filesystem rename was not
  available between mounted test roots on this host, so the portable `EXDEV`
  mapping and no-fallback behavior are locked with a direct structured-error
  test.
- `TestCanonicalDocumentationExamples` now executes every canonical I/O entry
  against its own explicit temporary fixture. The reference checker reports
  279 exact runtime built-ins and finds every detailed listing and alias.
- Build-only target checks passed for `linux/amd64` and `windows/amd64`; the
  native host run was `darwin/arm64`.

### Validation evidence

Toolchain: Go 1.26.1 `darwin/arm64`, Node 24.12.0, Apple Clang 15.0.0.

```text
go test ./...                                                        PASS
go test -race ./...                                                  PASS
go vet ./...                                                         PASS
go test ./internal/osiris -run 'TestFilesystem|TestSeekTell|
  TestCopyFileLoop|TestStreamClose|TestCanonicalDocumentationExamples'
  -count=1                                                           PASS
go test -race ./internal/osiris -run 'TestFilesystem|TestSeekTell|
  TestCopyFileLoop|TestStreamClose' -count=1                         PASS
GOOS=linux GOARCH=amd64 go test -c -o /tmp/osiris-linux-amd64.test
  ./internal/osiris                                                  PASS
GOOS=windows GOARCH=amd64 go test -c
  -o /tmp/osiris-windows-amd64.test.exe ./internal/osiris            PASS
go run ./cmd/osiris help                                             PASS
npm run check-reference                                             PASS
npm run check-examples                                              PASS
npm run build                                                       PASS
git diff --check                                                    PASS
```

The Vite build retains its pre-existing advisory that the main minified chunk
is slightly above 500 kB; it is a warning, not a failed build or an I/O
correctness issue.

### Acceptance

- [x] Explicit ownership, deterministic ordering, cooperative cancellation,
      and structured filesystem errors are documented and tested.
- [x] Directory and whole-file memory limits are public and enforced.
- [x] Interpreter/VM parity and race-enabled lifecycle tests pass.
- [x] Native C rejects unsupported forms at compile time with source
      locations.
- [x] Canonical I/O listings, search, support matrix, Technical Reference
      index, checked fixtures, and production docs build pass.
