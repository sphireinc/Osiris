# Task 046: Threat-model native compilation, modules, LSP, and codecs

**Priority:** P1  
**Dependencies:** 024,028,037,038  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Identify and mitigate untrusted-input risks before wider distribution.

## Required work

- [x] Document assets, trust boundaries, attacker goals, and mitigations.
- [x] Audit generated C escaping, compiler invocation, temp files, module paths, archive/codec bombs, LSP payload size, and error redaction.
- [x] Add resource limits and malicious fixtures.
- [x] Create security reporting guidance.

## Acceptance criteria

- [x] Critical trust boundaries have tests or explicit limitations.
- [x] Compiler arguments cannot be injected from source text.
- [x] Sensitive host paths/data are redacted in user-facing errors.

## Validation

- [x] malicious corpus
- [x] resource-limit tests
- [x] temp-file permission tests

## Non-goals

- Do not claim sandboxing unless an actual sandbox boundary exists.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record

`THREAT-MODEL.md` defines assets, trust boundaries, attack goals, mitigations,
explicit non-sandbox limits, and private reporting guidance. Compiler execution
uses `exec.CommandContext` with a fixed argument vector: source text is never
shell syntax or a compiler-flag fragment. `CC` and numerical-provider paths
remain explicitly trusted host configuration.

LSP framing rejects payloads over 16 MiB before allocation; value codecs reject
payloads over 64 MiB before decoding. Malicious fixtures cover both limits,
invalid LSP edits, module/cache behavior, redaction, and native validation.
Native compiler scratch directories are created through a tested helper with
owner-only Unix permissions. `test-security-boundaries.sh` provides the
repeatable corpus command.

Focused limits/permission tests, the full security corpus, full Go tests,
task-index checks, and diff checks passed.
