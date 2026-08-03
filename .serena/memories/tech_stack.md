# Osiris toolchain

- Runtime/compiler: Go module `github.com/sphireinc/osiris`, currently declared for Go 1.26.
- CLI entrypoint: `go run ./cmd/osiris ...`; the package under `internal/osiris` is the semantic/compiler core.
- Public docs: `newdocs/` is the canonical Vite/React reference and Pages source; `docs/` is retained legacy/compatibility material.
- Native builds depend on the host C compiler through the Go native backend; WASM and GPU are separate artifact/capability emitters.
- Serena’s Go semantic indexing requires `gopls` to be installed and visible to the Serena server environment.
- Read `README.md` and `AGENT/AGENTS.md` for user-facing setup and policy details.