# Task 109: Compile the mission-control showcase through native C

**Priority:** P2  
**Dependencies:** 002, 004, 013, 018, 025, 028, 029, 030, 032, 033, 035, 040, 045, 046, 100, 101, 103, 104  
**Status:** Complete  

## Objective

Compile `benchmarks/aegis-mission-control.osr` into a standalone native
executable through Osiris's IR-to-C backend without changing the showcase to
remove language features.

The native executable must run the mission analysis itself and produce output
that is byte-for-byte identical to the interpreter and public VM paths. It must
not invoke the Osiris interpreter, VM, Go runtime, source file, or another
Osiris process at run time, and native compilation must never evaluate the
program to manufacture its report.

This is an integration target for a deliberately broad program, not permission
to claim that every member of every imported module is natively supported.
Only parity-proven surfaces may be added to the public native support matrix.

## Baseline and confirmed blockers

The showcase currently succeeds through both public execution paths:

```sh
go run ./cmd/osiris run benchmarks/aegis-mission-control.osr
go run ./cmd/osiris vm benchmarks/aegis-mission-control.osr
```

Their standard output is byte-identical. The characterized output SHA-256 on
2026-07-30 is:

```text
16d0a61eb6aa77a9a5a0ed1735678bf5eb152962e43d38170c88cbc8ec4771c2
```

Native compilation stops at the first source-level module gate:

```text
benchmarks/aegis-mission-control.osr:21:8: native C backend:
static module "std/text" is not supported
```

That message does not describe the complete work. A direct IR build currently
stops at:

```text
try expressions are not yet lowered into IR
```

Temporarily replacing only that expression during the audit exposes:

```text
let? result binding is not yet lowered into IR
```

Temporarily replacing both constructs allows IR construction and exposes
`async.spawn`, `async.await`, and `async.parallel`. The current generic C
emitter has no `async` case. More importantly, `irBuilder.evalExpr` evaluates
all special-form arguments before emitting the async instruction, so the
present IR does not preserve the deferred evaluation semantics of `spawn` or
`parallel`. Do not implement C emission for that eager form.

The current C tagged ABI contains null, bool, fixed-width integers, floats,
f64 complex, strings, lists, real/complex tensors, closures, and enums. The
showcase additionally reaches values and behavior not represented there:

- exact decimals, arbitrary-precision integers, and exact dimensioned
  quantities;
- durations, datetimes, locations, and bytes;
- structured errors and result propagation;
- deferred task values and deterministic task scheduling;
- portable binary-codec encode/decode for the archive value.

The native module resolver currently recognizes only static `std/math`
members. The early `std/text`/`std/time` gates and the later general
non-`std/math` gate must be replaced only as each required member obtains
parity-safe lowering.

## Required source contract

Treat the checked-in showcase as the acceptance fixture. It must retain its:

- six imports, macros, typed bindings, and explicit `Lazy<T>` values;
- nominal and backed enums, guarded `match`, and `switch`;
- recursive function, lexical captures, `for` and `while` loops;
- three `spawn` tasks, three `await` operations, and ordered `parallel`;
- time, text, crypto, signal, statistics, matrix, and complex calculations;
- `try`/`catch`, `let?`, exact decimal, bigint, quantity, and codec usage; and
- bounded ten-value mission report.

Do not satisfy this task by deleting an import, replacing a value with a
precomputed literal, serializing a canned report, reducing task bodies to
ordinary calls, removing the caught error, changing the report, or maintaining
a second native-only copy of the program.

If an independent language bug requires a source correction, first add a
focused regression task/test, explain the semantic correction in this file,
and regenerate all three execution-path baselines. Cosmetic rewrites do not
qualify.

## Exact native surface required by the showcase

### Static modules

Implement static identity propagation and direct native lowering for these
members:

| Module | Members used by the showcase |
| --- | --- |
| `std/crypto` | `random`, `hash` |
| `std/list` | `range` |
| `std/math` | existing members plus `mean`, `std`, `quantile`, and `phase` |
| `std/signal` | `hann`, `convolve` |
| `std/text` | `slug`, `fields`, `upper`, `join` |
| `std/time` | `date`, `add_date`, `add`, `parse_duration`, `format`, `duration_string`, `sub`, `utc_location`, and `rfc3339` |

Keep modules as compile-time identities. Do not introduce dynamic module
objects merely to support these calls. Conflicting CFG or closure identities
must remain a compile-time diagnostic.

Members outside this table remain unsupported unless they are separately
designed, tested, documented, and needed for a coherent shared helper. The
module-level documentation and support matrix must say that native coverage is
partial rather than implying the full interpreter surface.

### Global operations and constants

The program also requires native lowering for:

- `cat`, including canonical conversion of every argument value used here;
- `rotl` and `bxor` with exact fixed-width validation and result widths;
- `dec`, `bigint`, `complex`, `abs`, `uint`, `u64`, and `f64`;
- `ok` plus successful `let?` extraction and error propagation;
- `encode` and `decode`;
- `len`, `at`, `matrix`, `transpose`, `matmul`, `trace`, `det`, and `norm`;
- `spawn`, `await`, and `parallel`; and
- the named constants `kilo`, `meter`, and `mega`.

Several operations in this list already lower for the value families used by
the program. Preserve their current fast paths and extend only the missing
families. Do not route proven tensor, complex, fixed-width, or scalar work
through a new generic slow path.

## Implementation requirements

### 1. Correct IR before C emission

Add source-spanned IR lowering for `try`/`catch` and `let?`.

- A fallible operation must carry success or structured failure explicitly;
  ordinary `null` must not be confused with failure.
- The selected `catch` handler receives the original structured error.
- `let?` extracts a successful result and propagates an unsuccessful result
  with its kind, message, cause, source, stack, metadata, and redaction state.
- CFG verification, SSA block arguments, dominance, liveness, purity,
  optimization, visualization, and deterministic IR printing must understand
  the new exceptional edges/instructions.
- Optimizers must not duplicate, delete, speculate, or reorder fallible work
  across a handler or propagation boundary.

Redesign async IR so task expressions are captured as thunks/closures and are
not evaluated at `spawn` or `parallel` construction time.

- `spawn` returns a task before executing its body.
- `await` executes or waits for that task and propagates its structured
  terminal error.
- `parallel` evaluates independent captured expressions and returns results in
  source order.
- Captures retain their lexical values and ownership.
- IR inspection must show calls inside generated task functions, not before
  `async.spawn` in the parent CFG.

Do not preserve the current eager async IR just because this showcase's tasks
are deterministic.

### 2. Extend the tagged C ABI coherently

Document and implement owned native representations for the additional value
families reached by the fixture:

- an arbitrary-precision signed integer;
- a normalized exact rational shared by decimal and quantity magnitudes;
- a quantity dimension vector over the seven SI base dimensions;
- signed-nanosecond duration, datetime, and location;
- immutable bytes;
- structured error and result values; and
- task state, thunk, result, and error storage.

The executable must be portable and self-contained. Do not silently depend on
GMP, ICU, the Go runtime, the host timezone database, or another unrecorded
shared library. A vendored/generated portable component is acceptable only
with its source, version, license, deterministic generation process, cache-key
inputs, and platform tests documented.

Specify for every new family:

- owned, borrowed, retained, and copied fields;
- clone/equality/truthiness/rendering behavior;
- overflow, allocation, and malformed-input errors;
- cleanup on success, catch, propagated failure, and process exit;
- interaction with lists, closures, tasks, enums, hashing, and codecs; and
- whether unboxed specialization is allowed.

Exact values must remain exact. Do not map `dec("1845000000.25")`, the bigint
literal, or `1560.8 * kilo * meter` to binary64. Quantity dimensions and
normalized rational magnitudes must match the interpreter.

Replace the current `ofail` convention, which writes a message and returns
`null`, with an explicit success/failure mechanism capable of supporting
`try`/`catch`. Unhandled failures still produce the stable native diagnostic
and nonzero exit status. Caught failures must not write to standard error.

### 3. Add a portable deterministic native task runtime

For this first native task slice, a cooperative single-process scheduler is
acceptable and preferable to mandatory platform threads.

- Queue task thunks without running them at construction.
- Execute ready work in stable order.
- Store exactly one terminal value or structured error.
- Make repeated `await` deterministic and safe.
- Preserve child captures until terminal completion and free them exactly
  once.
- Propagate cooperative cancellation through the documented task context,
  even though this fixture does not cancel a task.
- Implement `parallel` through captured branches and preserve result order.

Do not use synchronous eager lowering disguised as tasks. Native source and
tests must prove the task body is deferred. Host-thread parallel execution can
remain a later optimization after the portable scheduler is correct.

### 4. Implement the required module helpers

Port the existing Osiris contracts rather than inventing similar C behavior.

- Text helpers operate on valid UTF-8 and preserve Unicode code-point
  semantics. Case conversion and whitespace behavior need deterministic
  Unicode-version handling. Cover non-ASCII input even though the fixture is
  ASCII.
- `std/time` must implement the fixture's UTC civil-date construction,
  checked nanosecond duration parsing/arithmetic, Gregorian `add_date`,
  RFC 3339 formatting, and value rendering. It must not consult the clock or
  host timezone database for this program.
- `std/crypto.random` must reproduce the current seeded deterministic
  algorithm bit-for-bit. `std/crypto.hash` must use the current SHA-256
  framing and canonical value strings exactly, including enum, list, complex,
  time, and fixed-width values.
- `std/list.range(32)` must produce the same list element kinds, values, limit
  checks, and errors as the interpreter.
- `std/signal.hann` and `convolve` must match element order, output length,
  floating operations, validation, and rendering. Characterize any Go-versus-C
  libm difference before accepting a helper.
- `std/math.mean`, `std`, `quantile`, and complex `phase` must match reduction
  order, interpolation, population/sample convention, special values, signed
  zero, and final float bits. Keep existing native math gates when parity is
  not proven.
- `cat` must use the language's unquoted canonical concatenation form rather
  than the emitted value renderer's quoted-string form.

Any generated Unicode or crypto tables/components belong in the modular native
runtime structure established by task 029, not in another monolithic embedded
C string.

### 5. Implement compatible native codecs

Native `encode` and `decode` must use the existing versioned Osiris binary
format; do not create a C-only archive.

The showcase archive specifically contains strings, a UTC datetime, nested
lists, fixed/default numerics, f64 complex values, and a digest string. Add
portable support for those tags and any required location/duration subvalues.

- Interpreter-encoded bytes must decode natively and native-encoded bytes must
  decode through the interpreter and VM.
- Preserve canonical integer widths, float bits, datetime location identity,
  complex components, list order, and exact size/depth limits.
- Reject truncated, overlong, non-canonical, unknown-tag, allocation-overflow,
  and trailing-byte inputs with structured errors.
- Byte buffers and decoded aggregates require explicit ownership and cleanup.
- Fuzz the native-facing decoder through a bounded C harness or generated
  executable corpus in addition to ordinary Go codec fuzzing.

### 6. Preserve and extend native module analysis

Generalize the existing static-module analysis without weakening it.

- Imports, copies, SSA block arguments, lexical captures, direct calls, and
  loops retain a module identity only when every incoming source agrees.
- Known supported members lower to direct helpers.
- Known but unsupported members receive a stable source-spanned diagnostic.
- Unknown/dynamic member access remains rejected.
- No module import or call is evaluated during compilation.

Remove the early whole-module `std/text` and `std/time` gates only after the
required members have real lowerings. Source spans must remain attached to
unsupported imports/members and helper calls.

## Required tests

Add focused tests before implementation where practical.

### IR and semantics

1. IR golden/assertion tests for:
   - successful and failing `try`/`catch`;
   - successful and failing `let?`;
   - deferred `spawn`, `await`, and ordered `parallel`;
   - captures inside task thunks; and
   - structured failures crossing task and result boundaries.
2. Prove the parent CFG does not execute a task expression before scheduling
   it. Use an observable counter/error fixture, not only an IR string search.
3. Prove exceptional/task edges remain valid after optimization and retain
   source spans and deterministic IR order.

### ABI and helper parity

4. Interpreter/VM/native executable parity for each new value family,
   including equality, rendering, nesting, error paths, and cleanup.
5. Table-driven parity for every required module member and global operation.
   Include arity/type/domain failures and Unicode/special-value boundary cases.
6. Exact seeded-random and SHA-256 framing vectors.
7. UTC leap-year/month-end/time-overflow cases for the required `std/time`
   slice.
8. Codec cross-direction fixtures plus malformed-input and bounded fuzz cases.
9. Task success, task failure, repeated await, stable scheduling,
   cancellation, capture lifetime, and ordered `parallel` tests.
10. C ABI aliasing and ownership tests for nested lists containing every new
    heap-backed family.

### Full showcase

11. Add `TestCBackendMissionControlShowcaseParity` or an equivalently focused
    integration test that:
    - builds the unchanged canonical source;
    - compares interpreter, public VM, and native stdout byte-for-byte;
    - compares exit status and empty standard error on success;
    - checks the characterized SHA-256;
    - executes the binary repeatedly to prove determinism; and
    - runs it from an empty temporary directory after the source and compiler
      process have exited.
12. Add generated-C assertions proving:
    - task bodies exist as deferred native functions;
    - orbit/signal/matrix loops and calculations execute at run time;
    - time, text, crypto, codec, exact-number, and error helpers are called;
    - the final `a1eafc...` mission digest is not embedded as a precomputed
      literal; and
    - no interpreter, VM, `Session.ExecProgram`, subprocess, or output-snapshot
      fallback is present.
13. Compile and execute representative focused fixtures and the showcase under
    ASan/UBSan where available. Exercise success, caught failure, propagated
    failure, task failure, malformed decode, and allocation cleanup.
14. Add the native path to
    `scripts/check-benchmark-workloads.sh` for this workload only after all
    native parity tests pass.

Do not make generated-C assertions depend on unstable temporary names when a
semantic helper, control-flow shape, or forbidden literal is a more durable
check.

## Documentation and support reporting

Update:

- `docs/design/NATIVE-ABI.md` with all new representations and ownership;
- `docs/design/NATIVE-COMPILER.md` with exceptional and deferred-task IR;
- `docs/design/TIME.md`, `docs/design/NAMED-UNITS.md`, and
  `docs/design/TASK-LIFECYCLE.md` with the proven native subset;
- `LEFTOFF.md` with the exact completed native surface and remaining gates;
- `benchmarks/WORKLOADS.md` with the native build/run command and environment;
- `newdocs/src/support-matrix.json` and the relevant canonical reference pages
  with accurate partial-native notes; and
- CLI/native build documentation with the standalone command.

Every public support claim needs a test reference. Do not mark an entire module
as fully native merely because the showcase's members compile.

## Standalone and validation evidence

The completion record must include:

```sh
go run ./cmd/osiris build \
  -o /tmp/aegis-mission-control \
  benchmarks/aegis-mission-control.osr
/tmp/aegis-mission-control
```

Record OS, architecture, Go version, C compiler path/version, compiler flags,
binary size, build time, and raw execution samples. This is correctness and
integration evidence, not a performance claim.

Also run:

```sh
go test ./internal/osiris -run 'TestCBackendMissionControlShowcaseParity$' -count=1
go test ./internal/osiris -run 'TestCBackendMissionControlShowcaseParity$' -race -count=1
./scripts/check-benchmark-workloads.sh
go test ./...
go test -race ./...
go vet ./...
git diff --check
./scripts/check-task-index.sh
```

Run the canonical documentation reference/example checks and the production
Vite build under Node 24. Run portable C compilation/execution in at least the
maintained macOS and Linux CI environments. Record any sanitizer or platform
skip explicitly; a skip is not proof of the skipped property.

## Acceptance criteria

- [x] The unchanged `benchmarks/aegis-mission-control.osr` builds through
      `osiris build` into a standalone native executable.
- [x] Interpreter, public VM, and native stdout are byte-for-byte identical,
      with matching successful exit behavior and repeated-run determinism.
- [x] `try`/`catch` and `let?` have source-spanned, SSA-aware IR lowering with
      structured error/result semantics.
- [x] Async IR defers task bodies, and the portable native scheduler preserves
      `spawn`, `await`, `parallel`, captures, failures, cancellation, and
      deterministic ordering.
- [x] Exact decimal, bigint, quantity, time, bytes, error/result, and task
      values have documented C ownership and parity-safe implementations.
- [x] Every module member and global operation listed in this task has focused
      success/error parity tests; unlisted members remain honest diagnostics.
- [x] Native codecs are binary-compatible in both directions and enforce the
      existing resource/security limits.
- [x] Generated C proves real run-time computation and contains no evaluator,
      VM, subprocess, precomputed digest, or output-snapshot fallback.
- [x] The binary runs without the source tree, Go runtime, or Osiris executable
      present and uses no undeclared external runtime dependency.
- [x] Native ownership/aliasing tests and available ASan/UBSan runs report no
      leak, use-after-free, double-free, overflow, or undefined behavior.
- [x] The support matrix, detailed reference pages, benchmark documentation,
      native design documents, and `LEFTOFF.md` accurately describe the new
      partial surface.
- [x] Focused normal/race tests, full normal/race/vet suites, workload parity,
      documentation checks, production docs build, task-index check, and
      `git diff --check` pass.
- [x] This task contains final design decisions, raw commands/results, output
      digest, environment details, and deliberately deferred members before it
      moves to `tasks/completed/`.

## Completion record — 2026-07-31

### Design decisions

- `try`, `resultbind`, and async operations remain explicit effectful IR
  instructions. Bodies for `try`, `spawn`, and each `parallel` branch are
  closure-converted native functions, so their work is not evaluated in the
  parent CFG.
- The portable native runtime is a generic language component named
  `cbackend_portable_runtime.go`; it has no benchmark-specific runtime or
  support category. The mission-control program is only an end-to-end
  integration fixture.
- Static modules remain compile-time identities. Conflicting CFG identities
  still fail with a source-spanned dynamic-module diagnostic, while ordinary
  structured-value members such as `error.kind` use tagged value access.
- Results retain either a success payload or structured error record.
  Structured native errors preserve storage for kind/message, cause,
  source/location, stack, public/debug metadata, retryability, severity, and
  redaction state across result, task, and catch boundaries.
- Native tasks use deterministic cooperative execution. `spawn` retains a
  thunk, `await` memoizes one terminal result/error, `cancel` matches the
  pending/completed public contract, and `parallel` evaluates captured
  branches in source order.
- The codec uses the existing `OSR1` format and canonical tags. It has no
  native-only archive format and rejects malformed headers, truncation,
  non-canonical integers, unknown tags, excessive depth/size, and trailing
  bytes.
- General language claims use capability names and focused parity tests.
  Benchmark names appear only in the benchmark/task integration record.

### Focused and generated-code evidence

```text
GOCACHE=/tmp/osiris-go-cache go test ./internal/osiris \
  -run 'TestCBackendMissionControlShowcaseParity$' -count=1
ok github.com/sphireinc/osiris/internal/osiris 3.471s

GOCACHE=/tmp/osiris-go-cache go test ./internal/osiris \
  -run 'TestCBackendMissionControlShowcaseParity$' -race -count=1
ok github.com/sphireinc/osiris/internal/osiris 8.183s
```

`TestCBackendRequiredNativeSurfaceParity` independently covers the required
module members and global operations without naming or depending on the
showcase. Additional focused tests cover Unicode text, UTC/nanosecond time,
crypto vectors, structured result/task failures and cancellation, repeated
await, codec cross-direction bytes, required error paths, and a bounded
malformed-tag corpus. The integration test asserts deferred task functions and
time/text/crypto/signal/matrix/codec/exact-value/error helpers in generated C,
rejects evaluator/VM/subprocess/file-loading markers, and rejects the final
mission digest as an embedded literal.

### Standalone environment and raw samples

```text
OS: Darwin 25.5.0, arm64
Go: go1.26.1 darwin/arm64
C compiler: /usr/bin/cc
C version: Apple clang 15.0.0 (clang-1500.0.40.1)
flags: -std=c11 -O3 -march=native -DNDEBUG -flto -lm

/usr/bin/time -p env GOCACHE=/tmp/osiris-go-cache \
  go run ./cmd/osiris build -o /tmp/aegis-mission-control \
  benchmarks/aegis-mission-control.osr
real 3.10
user 2.34
sys 0.32

binary size: 252856 bytes

run 1: real 0.19, user 0.00, sys 0.00
run 2: real 0.00, user 0.00, sys 0.00
```

Both runs were launched with `/tmp` as the working directory after
compilation. Both stdout files were byte-identical and had SHA-256:

```text
16d0a61eb6aa77a9a5a0ed1735678bf5eb152962e43d38170c88cbc8ec4771c2
```

The executable wrote no standard error and exited successfully. It contains
no Go runtime, Osiris executable, interpreter/VM call, source-file read, or
subprocess fallback.

### Sanitizer and portability evidence

- macOS Apple Clang `-fsanitize=undefined`: success; stdout SHA-256 matched
  `16d0a61e...4771c2`.
- Apple ASan aborts inside its own `sanitizer_malloc_mac.inc` initialization
  before program code. This is recorded as a host sanitizer-runtime failure,
  not Osiris evidence.
- Clean ephemeral `alpine:3` on Linux/aarch64, Alpine Clang 22.1.3 with
  `build-base`, `compiler-rt`, and `-fsanitize=address,undefined`: the current
  generated program completed without a sanitizer report and produced the
  exact expected SHA-256.
- The same Linux sanitizer environment also exercised caught task failure,
  malformed decode, and an unhandled propagated `Result` failure. The first
  two were caught without stderr; the unhandled case exited `1` with
  `osiris native error: unhandled`.

No unrelated application image was used for this evidence.

### Full validation

```text
GOCACHE=/tmp/osiris-go-cache go test ./...                 PASS
GOCACHE=/tmp/osiris-go-cache go test -race ./...           PASS
GOCACHE=/tmp/osiris-go-cache go vet ./...                  PASS
./scripts/check-benchmark-workloads.sh                     PASS
./scripts/check-task-index.sh                              PASS
git diff --check                                           PASS
Node 24.12.0 pnpm run check-reference                      PASS
Node 24.12.0 pnpm run check-examples                       PASS
Node 24.12.0 pnpm run build                                PASS
```

The production documentation build retains its existing warning that the
single JavaScript chunk exceeds 500 kB; the build succeeds.

### Deliberately deferred surface

Native support remains capability-based rather than module-wide. Unlisted
members of `std/crypto`, `std/list`, `std/math`, `std/signal`, `std/text`, and
`std/time` still fail during compilation. Host-thread parallel execution,
streams, async I/O, `race`, `select`, deadlines, host timezone data, dynamic
module objects, and additional codec tags remain separate work. These
deferrals do not affect the required language surface or standalone fixture.

## Non-goals

- Do not remove or precompute showcase behavior to fit the existing backend.
- Do not restore compile-time interpretation or a snapshot-printing backend.
- Do not shell out to Osiris or link the Go interpreter into the executable.
- Do not claim complete native support for `std/crypto`, `std/list`,
  `std/math`, `std/signal`, `std/text`, or `std/time`.
- Do not add implicit binary64 approximations for exact values.
- Do not use eager synchronous evaluation as the implementation of `spawn` or
  `parallel`.
- Do not require host threads for this first portable scheduler slice.
- Do not redesign unrelated filesystem, networking, provider-backed linear
  algebra, WASM, GPU, or dynamic-module behavior.
- Do not make performance claims without the repository's multi-sample
  benchmark process.
