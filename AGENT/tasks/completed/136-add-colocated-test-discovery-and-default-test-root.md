# Task 136: Add Go-style colocated test discovery and default test root

**Priority:** P1
**Dependencies:** 035, 040, 123
**Status:** Complete

## Objective

Make the normal Osiris testing workflow behave like Go's colocated test
workflow: a developer should be able to keep a module and its tests beside one
another, run `osiris test` from that directory, and have the test runner find
and execute the package tests without requiring a separate `tests/` directory
argument. Preserve explicit file and directory arguments as supported,
documented alternatives.

This task extends task 123's language-native test declarations and execution
contracts. It changes discovery and package assembly semantics; it does not
replace assertions, fixtures, benchmarks, coverage, fuzzing, golden files,
backend selection, deterministic providers, JSON events, or their existing
CLI flags.

## Required file conventions

The canonical colocated convention is:

```text
todo.osr
todo.test.osr
```

The following forms are supported:

- `*.test.osr` is the preferred Osiris spelling and corresponds to Go's
  `_test.go` convention.
- `*_test.osr` remains a compatibility spelling for existing task-123 test
  files.
- Test suffix matching is case-sensitive and uses the lower-case `.osr`
  extension so discovery is reproducible across case-sensitive and
  case-insensitive filesystems.
- A test file may contain one or more existing `// osiris:` declarations. A
  file without an explicit declaration retains task 123's implicit test name.
- A colocated test file is associated with the normal source package in its
  directory by its filename stem. The association must be visible in the
  inventory and stable JSON events.

The implementation must define diagnostics for orphaned test files, duplicate
module/test stems, unsupported source extensions, malformed declarations, and
ambiguous package membership. It must not silently execute a source file twice
because it was found through both a colocated association and an explicit
argument.

## Required command modes

### Default mode

```text
osiris test
```

With no path arguments, the command uses the current working directory as the
test root. It discovers the package's colocated test files, assembles the
corresponding source package, and runs every discovered declaration in stable
order. The behavior must be equivalent to explicitly naming the current
directory:

```text
osiris test .
```

The default root must be stated in `--help`, the CLI reference, the testing
design documents, and executable documentation examples.

### Explicit directory mode

```text
osiris test ./path/to/package
osiris test ./path/to/package/...
```

Directory arguments remain supported for scripts and CI. The task must define
whether a plain directory means one package or recursive package discovery and
must provide an explicit recursive spelling if the two modes differ. Existing
task-123 recursive discovery behavior must not regress without a migration
note. Hidden directories, `.git`, `.osiris`, generated artifacts, dependency
caches, and test-artifact directories must be excluded deterministically.

### Explicit file mode

```text
osiris test ./path/to/todo.test.osr
osiris test ./path/to/legacy_test.osr
```

Direct file arguments remain a precise escape hatch. The runner must execute
the selected test file with the correct neighboring package context, or emit a
clear diagnostic when the requested file cannot form a valid package. Multiple
file arguments are deduplicated and sorted under the same deterministic policy
as directory discovery.

## Package assembly and isolation

- Define which ordinary `.osr` files belong to a package when a colocated test
  is run. Source order, module declarations, imports, exports, and duplicate
  symbols must have one deterministic rule.
- Define whether a test file is compiled into the package, compiled as an
  external test package, or executed as a source-only test program. The chosen
  rule must allow tests to exercise the public package surface without
  introducing hidden host-language hooks.
- Keep each test declaration's fresh runtime, deterministic fake `std/os`
  providers, capability policy, timeout, cleanup, and backend selection from
  task 123.
- Ensure package-level source is parsed and checked once where safe, while
  preserving source locations and per-test failure attribution.
- Ensure interpreter, VM, and native execution receive equivalent package
  inputs and never silently fall back between backends.

## Discovery, filtering, and result contracts

- Use one canonical absolute, cleaned, lexicographically sorted discovery
  order for default, directory, file, and mixed arguments.
- Preserve `-run`, `-filter`, `-list`, `-json`, `-backend`, `-integration`,
  `-capabilities`, `-timeout`, `-parallel`, `-deterministic`, `-race`,
  coverage, benchmark, fuzz, golden, cache, and artifact behavior.
- Include package path, source file, test file, association mode, and source
  location in inventory and JSON event data where additive schema evolution is
  required.
- Keep no-tests-found, invalid-path, orphan, compile, runtime, cancellation,
  and assertion failures mapped to stable documented exit behavior.
- Make `osiris test` and `osiris test .` produce equivalent inventory and
  result ordering for the same working directory.
- Add a migration and compatibility policy for existing `tests/` directories;
  directory-based suites must continue to work unchanged.

## Documentation requirements

Document the complete strategy, not just the filename suffix:

- the preferred colocated layout with module/test examples;
- the legacy `*_test.osr` spelling and compatibility policy;
- default current-directory behavior for `osiris test`;
- explicit file, directory, and recursive-directory arguments;
- package assembly and source visibility rules;
- discovery exclusions, ordering, deduplication, and diagnostics;
- test isolation, fake providers, capabilities, backend selection, and
  cancellation;
- every affected CLI flag, inventory field, JSON event field, and exit status;
- migration from a dedicated `tests/` directory to colocated tests and when a
  dedicated directory remains preferable.

Update the canonical React reference, CLI reference, task-123 design/CLI
documents, executable examples, support matrix, and task completion record.
Do not use a category entry as a substitute for individual CLI or discovery
field documentation.

## Tests and acceptance criteria

- [ ] `osiris test` and `osiris test .` discover the same colocated package
      tests and produce identical deterministic ordering.
- [ ] `*.test.osr` and `*_test.osr` both work, with explicit precedence and
      duplicate rules.
- [ ] A colocated test can exercise its neighboring source package through the
      documented public visibility model.
- [ ] Explicit file arguments, directory arguments, mixed arguments, and the
      existing dedicated `tests/` directory workflow continue to work.
- [ ] Recursive discovery, hidden/generated-directory exclusions, filtering,
      inventory mode, JSON events, and no-tests-found diagnostics are tested.
- [ ] Interpreter, VM, and native backend paths receive the same assembled
      package and preserve task-123 isolation and failure semantics.
- [ ] Malformed, orphaned, duplicate, and ambiguous layouts have stable
      source-located diagnostics.
- [ ] Documentation examples execute successfully and reference/navigation
      checks pass.
- [ ] Focused tests, full Go tests, vet/build, race-sensitive discovery tests,
      and the complete documentation validation suite pass.

## Non-goals

- Do not add a third-party test framework or require Go tooling to run Osiris
  tests.
- Do not remove explicit paths or make a dedicated `tests/` directory invalid.
- Do not redesign task 123's assertion, fixture, benchmark, fuzz, golden,
  coverage, backend, or JSON event semantics except where discovery metadata
  must be extended.

## Completion evidence

- `DiscoverLanguageTests` now treats the current directory as the default root,
  discovers preferred `*.test.osr` sidecars and compatibility `*_test.osr`
  files, excludes generated/hidden trees, and resolves direct production-file
  arguments to their colocated sidecar.
- Colocated test cases are assembled with their neighboring production source;
  explicit test files, recursive directories, and dedicated `tests/` suites
  remain supported with deterministic deduplication and ordering.
- Added focused discovery, package-assembly, compatibility-suffix, and CLI
  coverage, plus granular reference entries for every new mode and contract.
- Verification: focused task-136 and language-test tests passed.
