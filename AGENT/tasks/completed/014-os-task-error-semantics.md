# OS errors, task errors, and safe observability

Source: ERROR-CHK.txt.

Map NotFound, PermissionDenied, AlreadyExists, Timeout, Cancelled, and
would-block host errors to stable language kinds while safely preserving native
codes. Specify sibling cancellation and multiple task failure aggregation.
Add safe human/machine formatting and redaction.

Acceptance: table-driven cross-platform mapping tests; deterministic task-group
tests; secrets are redacted from messages, causes, metadata, and stacks.

## Completion record

Completed 2026-07-25.

- `classifyGoError` now has table-driven coverage for cancellation, deadlines,
  filesystem conditions, closed handles, EOF, address/dial/network errors, and
  would-block (`EAGAIN`/`EWOULDBLOCK`). Would-block uses the portable
  `unavailable` kind; an errno is retained only as `native_code` debug metadata.
- Deterministic scheduling sorts task keys, reports the selected failure, and
  does not start later sibling work after that failure.
- `redact` recursively removes stacks and debug metadata, drops sensitive
  metadata keys, and sanitizes conventional secret-bearing text in errors,
  public metadata, and causes.
- Public contract: `newdocs/src/reference-app.jsx` and
  `newdocs/src/api-data.js`; durable design: `docs/design/OS-TASK-ERRORS.md`.

Validation:

```text
GOCACHE=/private/tmp/osiris-gocache GOMODCACHE=/private/tmp/osiris-gomodcache go test ./internal/osiris
./scripts/check-task-index.sh
cd newdocs && npm run check-reference && npm run check-examples && npm run build
git diff --check
```
