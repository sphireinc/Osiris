# Benchmarking and regression tracking

Run a parity-checked execution benchmark with:

```sh
RUNS=15 BENCH_ITERS=5000 BENCH_JSON=artifacts/native-loop.json ./bench.sh
```

## TCP loopback baseline

Before changing TCP defaults, collect the runtime-level loopback baseline:

```sh
TCP_RUNS=15 TCP_BENCH_TIME=1s \
  TCP_BENCH_OUT=artifacts/tcp-loopback.txt ./scripts/bench-tcp-loopback.sh
```

The console report presents the median/range/allocation summary first and
stores all raw Go benchmark samples in `TCP_BENCH_OUT`; set
`TCP_BENCH_SHOW_RAW=1` when the raw rows are also wanted on stdout. It contains
15 raw Go benchmark samples each for 64-byte default
round-trip latency, 64 KiB default round-trip throughput, an explicit 64 KiB
bulk-transfer comparison (`TCP_NODELAY=false`, 64 KiB write batch on both
endpoints), and cancelled-accept allocation cost. It exercises Osiris TCP
streams over one established loopback connection and reports allocations with
`-benchmem`; it is not a raw `net.Conn` benchmark.
Compare only matching OS, architecture, Go version, benchmark duration, and
payload. Treat a 15% median regression across 15 samples as an investigation
threshold, and confirm it with a second matching run before changing defaults.

The JSON schema records raw samples, host/Go metadata, workload identity,
warmups, parity status, separate native build time, and execution-only costs.
Do not compare results from different OS/architecture, Go version, C compiler,
provider configuration, workload revision, or iteration count.

## Required workload matrix

Scheduled tracking runs representative scalar loop, fixed-width scalar loop
(`benchmarks/native-narrow-integers.osr`), floating scalar loop
(`benchmarks/native-float-arithmetic.osr`), text transform, local module, tensor
kernel, and task/scheduler workloads. A workload is accepted only after
interpreter/VM/native parity where native lowering is supported; otherwise it
records the supported paths and reason without inventing a native number.
Native compile time is a separate field, never part of native runtime.

## Alerts

Scheduled CI stores results as artifacts and may open an investigation when
the median of at least 15 samples regresses by 15% or more against a matching
baseline on the same runner image. It is deliberately non-blocking: one noisy
run creates an alert for confirmation, not a failed pull request. A regression
is actionable only after a repeat run and a matching environment comparison.
