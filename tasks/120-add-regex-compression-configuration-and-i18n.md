# Task 120: Add regex, compression, configuration, and internationalization

**Priority:** P2
**Dependencies:** 036, 040, 046, 103, 116, 117
**Status:** Blocked

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

- [ ] Each slice has a precise versioned, bounded, deterministic contract.
- [ ] Malicious regex/archive/config inputs are covered by fuzz/security tests.
- [ ] Locale output does not depend on ambient host configuration.
- [ ] Resource, backend, documentation, and full validation pass.

## Non-goals

- No promise to implement every PCRE feature, every archive/config format, or
  the host platform's complete locale database.
