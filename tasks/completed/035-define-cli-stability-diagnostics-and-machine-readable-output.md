# Task 035: Define CLI stability, diagnostics, and machine-readable output

**Priority:** P1  
**Dependencies:** 022,024  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Turn the CLI into a stable interface for humans, editors, and automation.

## Required work

- [x] Inventory commands, flags, exit codes, stdout/stderr contracts, and aliases.
- [x] Add `--json` or equivalent structured diagnostics where useful.
- [x] Standardize compile/runtime/internal failure exit codes.
- [x] Test help, invalid invocation, paths, and terminal/non-terminal output.

## Acceptance criteria

- [x] CLI contract is documented.
- [x] Machine-readable output is versioned.
- [x] Tests assert exit code and stream separation.

## Validation

- [x] CLI black-box tests

## Non-goals

- Do not break existing commands without migration aliases and release notes.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record

Added the global, opt-in `--diagnostics=json` mode. It is deliberately placed
before the command name so it never conflicts with existing command-owned
successful-result JSON flags such as `opt -json`, `gpu -json`, `deps -json`,
`heap -json`, and `trace -json`. A v1 diagnostic is written only to stderr:

```json
{"schema_version":1,"command":"run","category":"compile","exit_code":65,"message":"…"}
```

Successful payloads remain on stdout. The CLI emits no terminal colour or
control sequences, so output is identical when redirected. Stable process exit
categories are: success 0, usage 64, compile 65, unreadable input 66, runtime
70, and internal 71. `compile` remains a stable alias of `build`.

The complete command/flag/alias/stream inventory is in
`docs/design/CLI-CONTRACT.md`; the canonical public React reference has a
searchable `osiris CLI diagnostics` entry under Tooling.

Evidence:

- `TestExecuteDiagnosticContract` covers stdout/stderr separation and JSON
  envelopes for success, invalid flags, missing paths, parse failure, runtime
  failure, and an unknown command.
- `TestExecuteHumanUsageWritesOnlyStderr` covers invalid command help and the
  non-JSON terminal-friendly usage contract.
- `TestExecutableAppliesDiagnosticExitCode` builds the executable and verifies
  that the operating system observes exit code 66 and its JSON diagnostic.
- `go test ./cmd/osiris -count=1`, `go test ./...`,
  `npm run check-reference`, `npm run build`,
  `scripts/check-task-index.sh`, and `git diff --check` passed.
