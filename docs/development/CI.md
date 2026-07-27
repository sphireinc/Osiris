# Continuous integration

The required GitHub checks are deliberately layered:

| Check | Purpose | Platform |
| --- | --- | --- |
| `fast` | formatting, vet, unit tests, repository/task/support contracts | Linux |
| `race` | bounded race-enabled runtime suite | Linux |
| `native` | compile and execute representative generated C with GCC and Clang | Linux |
| `docs` | migrated-reference contract checks and a clean React build | Linux |
| `platform` | ordinary Go test coverage on supported host families | Linux, macOS, Windows |

Native C execution is intentionally Linux-only today: the generated runtime
uses the system C toolchain and the project does not yet publish a Windows C
toolchain contract. Windows therefore runs the Go test suite, whose native
tests skip when a usable C compiler is absent. WebAssembly and GPU commands are
artifact-generation experiments, not required execution targets.

The native sanitizer suite is Linux/Clang-only. The local Darwin AddressSanitizer
runtime currently aborts during its own initialization before a generated
executable begins, so it is an explicit skip rather than a false correctness
signal.

`newdocs/` is the canonical public React reference source, including its
checked-in API and support-matrix data. CI checks its content, search contract,
compatibility redirects, executable examples, and deterministic build. The
dedicated Pages workflow publishes it from `main`. `docs/` is retained only as
frozen legacy compatibility and durable design material; its former URLs are
emitted as redirects in the published site.
