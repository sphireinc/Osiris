# Task 123 CLI, event, and artifact reference

The command syntax is:

```text
osiris test [flags] [packages...]
```

`packages...` is a list of source files or directories. An omitted list means
the current directory. `-backend=<interpreter|vm|native>` selects the public
execution path. `-json` emits one JSON event per expanded declaration.
`-run=<pattern>` filters by exact name, glob, or substring. `-filter=<pattern>`
is an alias for `-run`.

`-list` performs discovery and emits inventory only. In JSON mode its object
has `schema_version`, `files`, and `tests`; each test has `name`, `file`,
`location`, and declaration booleans. `-parallel=<n>` bounds workers.
`-timeout=<duration>` applies a context deadline per declaration.
`-deterministic=<bool>` controls deterministic provider/order policy and is
true by default. `-race` enables bounded interpreter/VM concurrency checks and
is invalid with native.

`-integration` selects host providers. `-capabilities=a,b` grants exact
capability names after `-integration`; the value is comma-separated and empty
items are ignored.

`-coverage=<path>` writes the coverage artifact. `-artifacts=<dir>` selects
the fuzz artifact root and defaults to `.osiris/test-artifacts`.
`-bench` enables benchmark declarations. `-bench-json=<path>` writes the
schema-versioned benchmark report. `-fuzz` enables fuzz declarations.
`-fuzz-runs=<n>`, `-fuzz-seed=<int>`, and `-fuzz-corpus=<path>` bound and seed
fuzz execution. `-update` enables golden rewriting. `-cache-dir=<dir>` selects
the inventory manifest directory, and `-no-cache` disables its write.

## JSON test event

The event object has `schema_version=1`, `event="test"`, `name`, `file`,
`backend`, `status`, `passed`, `duration_ns`, and `location`. Optional fields
are `skipped`, `expected_failure`, `unexpected_pass`, `outputs`, `assertions`,
`error`, `coverage`, `benchmark`, and `artifacts`. Unknown additive fields may
be ignored by consumers after checking the schema version.

`status` is one of `ok`, `skip`, `expected_failure`, `unexpected_pass`, `fail`,
`compile_error`, or `cancel`. `passed` is true for `ok`, `skip`, and
`expected_failure`; it is false for the other statuses. `outputs` contains
canonical `Value.String()` strings. `assertions` contains the exact source
expression, pass boolean, message, and source location. `error` is omitted for
successful events and otherwise contains the public error string.

## Benchmark JSON

The benchmark document has `schema_version=1`, `workload`, `backend`,
`iterations_per_run`, `warmup_runs`, `measured_runs`, `environment`, `costs_ms`,
and `caveats`. `environment` has `os`, `arch`, and `go`. `costs_ms` is keyed by
backend and each value has `samples` and `average`. Samples are raw measured
milliseconds rounded to three decimal places; average is the rounded arithmetic
mean. `caveats` documents startup and parse costs so this output remains
compatible with the existing benchmark JSON policy.

## Fuzz artifacts

The failure JSON has `schema_version`, `name`, `seed`, `run`, `input`, `shrunk`,
`error`, and `replay`. The replay path points to an `.osr` source with the
shrunk input substituted into `{{input}}`. Artifact filenames are SHA-256
derived from declaration, seed, and shrunk input. Shrinking is deterministic,
single-byte deletion, and capped at 64 attempts.

## Exit behavior

The existing CLI codes remain in force: `0` means all declarations completed,
including skips and expected failures; `64` is usage; `65` is a source/native
compile failure; `66` is an unreadable input or artifact path; `70` is a runtime,
assertion, timeout, unexpected-pass, or ordinary test failure; `71` is an
internal failure. Human output is `STATUS name` with an error suffix. JSON test
events go to stdout; command diagnostics remain on stderr under the existing
`--diagnostics=json` envelope.
