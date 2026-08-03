# Useful Osiris commands

- Source of truth for the complete command list: `AGENT/AGENTS.md` §Commands; keep this memory from becoming a second policy document.
- Fast CLI smoke: `go run ./cmd/osiris help`; `go run ./cmd/osiris run examples/named-values.osr`; `go run ./cmd/osiris vm examples/named-values.osr`; `go run ./cmd/osiris ir examples/named-values.osr`; `go run ./cmd/osiris c examples/named-values.osr`.
- Go validation: `go test ./...`; focused package test with `go test ./internal/osiris -run 'TestName$' -count=1`; race and vet via `go test -race ./...` and `go vet ./...`.
- Documentation validation from `docs/`: `npm ci`, `npm run check-reference`, `npm run check-examples`, `npm run build`.
- Before/after inspection: `git status --short`; finish with `gofmt` and `git diff --check`.
- Serena memory integrity: `serena memories check` from the repository root.