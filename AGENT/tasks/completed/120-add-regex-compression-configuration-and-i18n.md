# Task 120: Add regex, compression, configuration, and internationalization

**Priority:** P2
**Dependencies:** 036, 040, 046, 103, 116, 117
**Status:** Complete

## Objective

Fill common text/data platform gaps with bounded regular expressions,
compression/archive formats, configuration parsing/schema validation, and
deterministic locale-aware formatting.

## Required slices

- Regex compile/match/find/replace/split with explicit syntax/version, Unicode
  behavior, capture representation, and ReDoS/resource policy.
- Streaming gzip/zlib and a narrowly scoped archive format (for example ZIP or
  TAR) with checksum, path traversal, size/ratio, and entry-count limits.
- Configuration parsing for JSON plus selected human-authored formats, with
  ordered data representation and schema validation returning structured
  path-aware errors.
- Locale objects and deterministic number/date/message formatting with an
  embedded/versioned data policy; never depend silently on host locale data.

## Implementation requirements

- Prefer portable, versioned components with documented licenses and cache
  inputs. FFI-backed providers must use task 117.
- APIs must support streams, cancellation, scope-bound cleanup, and explicit
  limits.
- Archive extraction must reject absolute paths, `..`, symlink escapes,
  duplicate-policy ambiguity, bombs, and malformed metadata.
- Preserve Unicode/version and map ordering deterministically.

## Tests and documentation

- Table-driven Unicode/regex, malicious pattern, compression vectors, archive
  traversal/bomb, schema error-path, locale/version, streaming cancellation,
  fuzz, and cross-backend tests.
- Checked documentation examples for every supported operation and limit.

## Acceptance criteria

- [x] Each slice has a precise versioned, bounded, deterministic contract.
- [x] Malicious regex/archive/config inputs are covered by fuzz/security tests.
- [x] Locale output does not depend on ambient host configuration.
- [x] Resource, backend, documentation, and full validation pass.

## Non-goals

- No promise to implement every PCRE feature, every archive/config format, or
  the host platform's complete locale database.

## Completion record — 2026-08-02

- Host contracts are implemented in `internal/osiris/task120_*.go`, including
  bounded RE2 regex, deterministic gzip/zlib, safe USTAR archives, ordered
  JSON/properties parsing, path-aware schema validation, and embedded locale
  formatting.
- `std/regex`, `std/compress`, `std/config`, and `std/i18n` are registered in
  `internal/osiris/module.go`; interpreter and VM coverage is in
  `internal/osiris/task120_123_stdlib_test.go`.
- Granular API documentation is in
  `AGENT/docs/design/TASK-120-REGEX-COMPRESSION-CONFIG-I18N.md` and
  `docs/src/std-task-reference.js`. The documentation checker verifies every
  module export and technical index.
- Focused task tests, full `go test ./...`, `go vet ./...`, `go build ./...`,
  focused race tests, documentation checks, and checked examples passed with
  Go 1.26.1.
