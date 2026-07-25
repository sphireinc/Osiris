# Benchmark workloads

| Workload | Surface | Native timing |
| --- | --- | --- |
| `native-loop.osr` | scalar loop | required parity/timing baseline |
| `text-transform.osr` | text module | interpreter/VM parity; native only when module lowering supports it |
| `module-math.osr` | static standard module | parity/timing where supported |
| `tensor-kernel.osr` | rank-two tensor kernel | parity/timing where supported |
| `task-pipeline.osr` | task/await scheduling | interpreter/VM only until native task ABI exists |

The scheduled workflow begins with the native scalar baseline because it is
the stable cross-backend workload. Additional workloads are collected only
after each runner declares its supported paths rather than treating an
unsupported native lowering diagnostic as a benchmark result.
