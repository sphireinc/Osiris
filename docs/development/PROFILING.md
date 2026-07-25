# Profiling and extraction policy

Use `PROFILE_DIR=/tmp/osiris-profiles ./scripts/profile-workload.sh` to capture
CPU and allocation profiles for the native-loop representative workload. Local
profiles remain ignored; commit a textual `go tool pprof -top` summary and the
workload/environment metadata when it motivates an extraction.

An extraction must state its observed hotspot/coupling reason, retain package
acyclicity, and pass the conformance suite before and after. File length alone
is not a reason to split code. Compare compile time and allocations only on a
matching OS, architecture, Go version, workload revision, and benchmark setup.

## Baseline: 2026-07-24

On darwin/arm64 (Apple M1 Pro, Go 1.26.1), the scalar-loop profile recorded
approximately 4.84 s/op, 1.76 GB/op, and 12.5M allocations/op. The CPU top
was dominated by runtime condition-variable, GC, and allocation work rather
than an isolated source file. This is evidence to investigate evaluator/task
allocation lifetimes first; it is not evidence for a cosmetic file split.
