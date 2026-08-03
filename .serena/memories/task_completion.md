# Osiris completion gates

- Use `AGENT/AGENTS.md` §Required workflow and §Testing expectations as the authoritative checklist; do not maintain a duplicate long-form process here.
- Minimum evidence pattern: focused regression tests first; then relevant full-package/full-suite, race, vet, generated-artifact or CLI checks, and docs/example checks when the contract is user-facing.
- Compiler/backend work must inspect generated IR/C/WASM or CLI output and verify unsupported-feature diagnostics and backend parity.
- Finish with formatting, `git diff --check`, and `git status --short`; preserve unrelated worktree changes.
- Update the assigned task record with evidence, decisions, and explicit deferrals. Never mark acceptance criteria from a single interpreter-only result.
- For durable design changes, record the contract in `docs/design/`; public language/API material belongs in `newdocs/`.