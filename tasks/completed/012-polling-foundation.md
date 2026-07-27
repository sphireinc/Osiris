# Polling foundation and platform adapters

Source: POLLING-CHK.txt.

The checklist claims a polling API but no implementation exists. First define
a small internal context/registration/wait interface and one portable backend.
Only then add build-tagged kqueue, epoll, and Windows adapters; keep poll fallback.

Acceptance: context lifecycle, registration/removal, timeout, readiness events,
and unsupported-backend errors are tested; selected backend/capabilities are
inspectable and platform tests skip correctly.

## Completion evidence — 2026-07-25

- Added the internal, concurrent-safe portable `pollContext` with opaque-token
  registration, interest modification/removal, advisory event records,
  bounded context-aware waits, timeout, and idempotent close/wakeup behavior.
- `PollCapabilities` reports the selected `portable` backend and its exact
  limitation: synthetic signals are supported; OS-handle readiness is not.
  Explicit requests for unimplemented backends such as `epoll` fail clearly.
- `TestPortablePollContextLifecycleAndReadiness` and
  `TestPortablePollContextCancellationAndValidation` cover lifecycle,
  readiness filtering, timeout, cancellation, and unsupported backend paths.
- `docs/design/POLLING.md` records the internal boundary, lifecycle rules,
  readiness semantics, and the deferred build-tagged adapter work.
