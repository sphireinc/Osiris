# OS errors, task errors, and safe observability

Source: ERROR-CHK.txt.

Map NotFound, PermissionDenied, AlreadyExists, Timeout, Cancelled, and
would-block host errors to stable language kinds while safely preserving native
codes. Specify sibling cancellation and multiple task failure aggregation.
Add safe human/machine formatting and redaction.

Acceptance: table-driven cross-platform mapping tests; deterministic task-group
tests; secrets are redacted from messages, causes, metadata, and stacks.
