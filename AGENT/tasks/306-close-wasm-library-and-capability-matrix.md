# Task 306: Close WASM library and capability matrix rows

**Priority:** P0
**Dependencies:** 302,305
**Status:** Blocked

## Objective

Close WASM rows for standard libraries, virtual filesystem/environment,
database, telemetry, regex, i18n, testing, unsafe boundaries, networking
profiles, and tooling.

## Acceptance criteria

- Every profile import and capability has deterministic fake-provider tests,
  explicit denial tests, cancellation/cleanup tests, and individual docs.
- No ambient host access or custom Go semantic fallback remains in a claimed
  stable row.
- Artifact ABI, import list, limits, and result encoding are versioned and
  validated by CI.
