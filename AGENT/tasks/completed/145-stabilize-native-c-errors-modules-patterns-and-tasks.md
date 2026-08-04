# Task 145: Stabilize native C errors, modules, patterns, and tasks

**Priority:** P1
**Dependencies:** 144
**Status:** Complete

## Objective

Extend the native C backend from core execution into complete structured error,
Result/Option, module/macro, pattern, and cooperative task parity.

## Exact scope and ownership

- Continue the native ownership of `internal/osiris/cbackend_native.go` and
  native runtime/error helpers; do not modify other backend implementations.
- Lower structured errors, causes, Result/Option propagation, `try`/`catch`,
  static imports/exports/version selectors, hygienic macro output, enum and
  pattern forms, guards, exhaustiveness witnesses, `spawn`/`await`/`cancel`,
  ordered `parallel`, and supported cleanup edges.
- Eliminate interpreter fallback; unsupported IR must fail during native
  compilation with a stable source span until implemented.
- Add differential, sanitizer, cancellation, failure, and ownership tests.

## Matrix transitions

Promote native C errors/results/options, modules/packages/macros, patterns, and
the supported task/control-flow surface to `stable`.

## Parallelism

Sequential after task 144 and before tasks 146–148. The shared native files are
not safe for concurrent agent ownership.

## Completion evidence

- Generated C now has a distinct owned `O_OPTION` ABI alongside `O_RESULT`,
  including `some`, `none`, `issome`, `isnone`, `unwrap`, `expect`, `or_else`,
  `to_option`, `to_result`, `collect`, and postfix propagation through the
  `resultbind` IR operation.
- Native error values retain the declared structured fields and expose the
  parity-tested kind/message/operation/resource/source/cause/stack/metadata,
  retryability, and severity members without interpreter fallback.
- Static module identities resolve through the explicit native module table;
  dynamic modules and unsupported members fail with source-spanned native
  diagnostics. Enum, list, type, relational, logical, grouped, guarded, and
  repeated-binding patterns lower through generated CFG tests.
- Task creation, repeated await, cancellation, ordered parallel evaluation,
  structured failure, try/catch, and cleanup edges are covered by the native
  surface and Task 145 differential fixtures.
- `internal/osiris/task145_native_parity_test.go` exercises the generated
  Option ABI. The native C/structured-result/deferred-task/pattern suite passed
  with normal, undefined-sanitizer, and race-focused runs.
- `docs/design/NATIVE-C.md` documents the ABI, ownership, task contract,
  module policy, pattern lowering, and diagnostic boundaries.
