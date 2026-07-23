# Native compiler validation, tooling, and reproducibility

Source: OPTCHK.md and docs/design/NATIVE-COMPILER.md.

Make osiris opt report transformations; add native/interpreter/VM parity,
generated-C snapshots, benchmark JSON, and a small codegen fuzz target. After
real lowering works, add a content-addressed compilation cache and manifested
artifacts.

Acceptance: stable CI-friendly commands; every optimization has a regression
test; cache keys include compiler identity/flags; clean native builds reproduce
for a fixed toolchain; performance claims include a checked benchmark.

Progress: native executable parity tests cover each implemented lowering slice;
generated C is checked for deterministic byte-for-byte output;
`FuzzCBackendSource` exercises parser-to-IR-to-C emission without compilation
time evaluation; and `CompileCExecutableCached` stores a manifest-backed,
content-addressed executable cache keyed by source/compiler/flags/target.
`osiris build -cache-dir` integrates that cache into the user-facing build
command. `BenchmarkCBackendUnboxedScalarLoop` provides a checked native-loop
execution metric. `./bench.sh` now builds a verified scalar workload, checks
interpreter/VM/native output parity, warms each path, samples repeated
execution runs, and reports native compile time separately from an
average/best/worst execution table. Optimization reports, benchmark JSON, and
CI commands remain open.
