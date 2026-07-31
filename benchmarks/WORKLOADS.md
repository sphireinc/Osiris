# Benchmark workloads

| Workload | Surface | Native timing |
| --- | --- | --- |
| `native-loop.osr` | scalar loop | required parity/timing baseline |
| `text-transform.osr` | text module | interpreter/VM parity; native only when module lowering supports it |
| `module-math.osr` | static standard module | parity/timing where supported |
| `tensor-kernel.osr` | rank-two tensor kernel | parity/timing where supported |
| `task-pipeline.osr` | task/await scheduling | interpreter/VM only until native task ABI exists |
| `aegis-mission-control.osr` | broad deterministic language showcase | interpreter/VM/native-C byte parity integration target |

The scheduled workflow begins with the native scalar baseline because it is
the stable cross-backend workload. Additional workloads are collected only
after each runner declares its supported paths rather than treating an
unsupported native lowering diagnostic as a benchmark result.

`aegis-mission-control.osr` is the human-facing showcase rather than a
single-kernel performance baseline. It combines typed and lazy bindings,
macros, recursion, closures, loops, enums and patterns, tasks, structured
recovery, codecs, time, text, crypto, signal processing, statistics, units,
complex values, and dense linear algebra into one bounded mission-analysis
program. Run it directly:

```sh
go run ./cmd/osiris run benchmarks/aegis-mission-control.osr
go run ./cmd/osiris vm benchmarks/aegis-mission-control.osr
go run ./cmd/osiris build -o /tmp/aegis-mission-control benchmarks/aegis-mission-control.osr
/tmp/aegis-mission-control
```

Its small ordered report is the checksum. All three paths currently produce
SHA-256
`16d0a61eb6aa77a9a5a0ed1735678bf5eb152962e43d38170c88cbc8ec4771c2`.
`scripts/check-benchmark-workloads.sh` compares the three outputs. This is an
integration/correctness workload, not a performance claim; do not feed it to
`bench.sh`.
