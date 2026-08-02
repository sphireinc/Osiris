# Task 123: language-native test workflow

This document defines the first source-only testing slice. The parser grammar is
unchanged in this slice, so declarations are comments beginning with
`// osiris:` inside a discovered `*_test.osr` or `.test.osr` file. Directive
lines are removed before execution and therefore retain the original line
numbers in parser and assertion diagnostics.

## File discovery and declaration syntax

`*_test.osr` and `*.test.osr` are discovered recursively. A direct file path is
accepted with any suffix. Directories named `.git`, `.osiris`, or beginning with
`.` are skipped. Paths and declarations are sorted lexicographically by their
absolute cleaned path and source order.

`// osiris:test name=<name>` starts one declaration. `name` is required for an
explicit declaration and is the stable result key. A file without a test
directive is one implicit declaration named after the file. `name` accepts a
quoted value when it contains spaces.

`// osiris:expect <value>` requires exactly one canonical output value.
`// osiris:expect outputs=["<value>","<value>"]` requires the complete output
list; the JSON array must contain strings.

`// osiris:assert output[<index>] == <value>` compares one zero-based canonical
`Value.String()` result. `// osiris:assert output_count == <n>` compares the
number of emitted values. `// osiris:assert output contains <value>` searches
for one exact canonical value. Values may be quoted or unquoted. An assertion
returns a structured pass/fail record with `expression`, `passed`, `message`,
and `location` fields.

`// osiris:skip reason=<text>` records a successful skip and does not execute
the declaration. `// osiris:xfail reason=<text>` marks a declaration whose
failure is expected; failure becomes `expected_failure`, while a pass becomes
`unexpected_pass` and fails the command.

`// osiris:case name=<case> key=<value> ...` adds one table-driven expansion.
Every `{{key}}` token in setup, body, and cleanup is replaced with its value.
The result name is `<test-name>[<case>]`. Case declarations are expanded in
source order and run independently with a fresh test runtime.

`// osiris:fixture setup`, `// osiris:fixture body`, and
`// osiris:fixture cleanup` select source sections. Setup executes before the
body and cleanup is attempted with `defer` even when the body fails. Fixture
code should use public language APIs such as `std/os`; emitted fixture output
is ignored. Native fixture state is process-local to each fragment and is
therefore intended for cleanup checks that do not require shared host state.

`// osiris:golden path=<file>` compares a stable JSON document containing the
schema version, declaration name, backend, and canonical outputs. `-update`
rewrites this exact document; without `-update`, missing or different files
fail with a source-located diagnostic.

`// osiris:benchmark warmup=<n> samples=<n>` declares a benchmark. Warmups are
executed but not recorded. Measured samples retain rounded milliseconds and are
serialized with the repository benchmark fields described below.

`// osiris:fuzz seed=<int> runs=<n> corpus=<path> max_size=<n>` declares a
bounded fuzz target. The body must contain `{{input}}`, which is replaced by a
quoted deterministic string. Corpus files are read in sorted name order, then
inputs are generated from a seeded alphabet. A failing input is shrunk by
bounded single-byte deletion and writes both a JSON failure record and an
`.osr` replay source.

## Runtime contract

Each declaration has a fresh `NewTestRuntime` unless `-integration` is set. The
test runtime freezes time, uses deterministic in-memory `std/os` providers, and
does not grant host capabilities implicitly. `-capabilities=a,b` grants the
listed capabilities only in integration mode.

`interpreter` calls `Runtime.Run`; `vm` calls `Runtime.RunVM`; `native` calls
`CompileC`, `CompileCExecutable`, and executes the generated program as a child
process. No evaluator-private entry point is used. Native compilation failures
are reported as compile failures, and unsupported native language surface is
not silently rerouted to the interpreter.

`-timeout=<duration>` creates one context deadline per declaration. Parser,
checker, runtime, native compilation, child-process execution, benchmark
iterations, and fuzz iterations observe that context. Cancellation is a
`cancel`/runtime failure, never a pass.

`-parallel=<n>` starts at most `n` workers. Results are placed back into
discovery order, so parallelism cannot reorder text or JSON events. A value
less than one is normalized to one. `-race` is accepted for interpreter/VM
bounded concurrent execution checks and is rejected for native because the
native executable has no integrated race instrumentation in this slice.

`-deterministic=true` is the CLI default. It fixes discovery order, worker
result order, fake providers, fuzz seeds, corpus order, artifact names, and
JSON field order. Benchmark sample values remain measurements and are not
promised to be numerically identical between hosts.

The cache policy is inventory-only. `-cache-dir=<dir>` writes
`inventory-v1.json`; it never skips execution or reuses a stale test result.
`-no-cache` disables that write. Golden, coverage, benchmark, and fuzz files
are explicit artifacts and are never implicitly treated as cache entries.

## Coverage model

`-coverage=<file>` writes `LanguageCoverageReport` JSON. `schema_version` is
`1`; `backend` names the selected backend; `available` is true only when the
interpreter's public `Runtime.Trace` path supplied execution frames. `statements`
and `functions` each contain `total`, `covered`, and `missed`. `branches` uses
stable source-map statements for loop/match branch declarations. `source_map`
is the ordered `SourceMapEntry` list with `index`, `parent`, `kind`, `statement`,
and `range`. VM and native reports retain the map and static totals but set
`available=false` with a reason naming the missing backend probes.

## Results and artifacts

Every `LanguageTestResult` has `name`, `file`, `backend`, `status`, `passed`,
`location`, and optional `outputs`, `assertions`, `coverage`, `benchmark`, and
`artifacts`. `skipped`, `expected_failure`, and `unexpected_pass` are explicit
booleans. `error` is the stable public error string; the non-serialized Go
`Error` field retains the original error for library callers. `duration_ns` is
zero in deterministic CLI mode and is available to library callers otherwise.

Every `LanguageTestLocation` has `source`, `start.line`, `start.column`,
`end.line`, and `end.column`; all coordinates are one-based. Artifacts have
`kind` and `path`. Artifact kinds in this slice are `golden`, `coverage`,
`benchmark`, `fuzz-failure`, and `fuzz-replay`.

## Example

```osiris
// osiris:test name="sum table"
// osiris:case name=two value=2
// osiris:case name=five value=5
// osiris:assert output[0] == "3u64"
emit {{value}} + 1
```

Run it with:

```sh
osiris test -run 'sum table' -parallel 2 -json path/to/example_test.osr
```
