# Benchmarking and regression tracking

Run a parity-checked execution benchmark with:

```sh
RUNS=15 BENCH_ITERS=5000 BENCH_JSON=artifacts/native-loop.json ./bench.sh
```

The JSON schema records raw samples, host/Go metadata, workload identity,
warmups, parity status, separate native build time, and execution-only costs.
Do not compare results from different OS/architecture, Go version, C compiler,
provider configuration, workload revision, or iteration count.

## Required workload matrix

Scheduled tracking runs representative scalar loop, text transform, local
module, tensor kernel, and task/scheduler workloads. A workload is accepted
only after interpreter/VM/native parity where native lowering is supported;
otherwise it records the supported paths and reason without inventing a native
number. Native compile time is a separate field, never part of native runtime.

## Alerts

Scheduled CI stores results as artifacts and may open an investigation when
the median of at least 15 samples regresses by 15% or more against a matching
baseline on the same runner image. It is deliberately non-blocking: one noisy
run creates an alert for confirmation, not a failed pull request. A regression
is actionable only after a repeat run and a matching environment comparison.
