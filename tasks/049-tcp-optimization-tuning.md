# Task 049: TCP optimization and tuning

**Priority:** P1 after network correctness  
**Dependencies:** 011, 012, 043, 045  
**Status:** Active. The TCP resource contract and portable polling boundary are complete; the reproducible baseline harness is in place and socket tuning must wait for its first allowed-host sample set.

## Objective

Turn the minimal, correct TCP API into a measured high-performance runtime
surface. Safe defaults must come first; OS-specific tuning remains opt-in,
capability-checked, and never changes portable semantics. This task is
deliberately downstream of cancellation, structured errors, and polling.

## Required work

- Establish reproducible loopback throughput, latency, allocation, and
  cancellation benchmarks with raw samples and regression thresholds.
- Define a typed `SocketConfig` contract with portable defaults and explicit
  unsupported-platform behavior. Include no-delay, keepalive, connect/read/
  write deadlines, read/write buffer sizes, and controlled write batching.
- Make `TCP_NODELAY` the documented low-latency default for newly created TCP
  connections, while permitting an explicit bulk-transfer override.
- Add bounded buffering and stream-to-stream transfer APIs only after ownership,
  backpressure, cancellation, and error propagation are specified and tested.
- Introduce build-tagged platform adapters behind the task-012 polling
  interface. Evaluate `sendfile`/`splice`, `SO_REUSEPORT`, IPv6 dual stack,
  quick ACK, congestion control, deferred accept, and native event engines;
  expose only facilities with portable fallback or explicit capability errors.
- Record why the present Go-hosted runtime cannot promise a no-GC/RAII runtime
  model. A future native runtime may use async state machines, ownership, and
  work-stealing, but this task must not make that unsupported claim early.

## Acceptance criteria

- Loopback benchmarks compare baseline and tuned paths with environment,
  workload, samples, and allocation evidence.
- Default and override socket options have deterministic integration tests;
  unavailable options return structured capability errors rather than silently
  succeeding.
- Transfer and batching paths preserve byte order, backpressure, close,
  deadline, and task-cancellation semantics under race-enabled tests.
- The canonical React reference documents defaults, portability, native-C
  support status, and examples. The support matrix and task evidence are
  updated with every supported capability.

## Checkpoint: baseline harness (2026-07-25)

- Added runtime-level loopback benchmarks for 64-byte round-trip latency,
  64 KiB round-trip throughput, and cancelled-accept allocations in
  `internal/osiris/tcp_benchmark_test.go`. They use Osiris TCP stream
  operations rather than raw `net.Conn` calls.
- Added `scripts/bench-tcp-loopback.sh`, which prints host/Go configuration and
  preserves repeated raw `go test -benchmem` samples when `TCP_BENCH_OUT` is
  supplied. `docs/development/BENCHMARKS.md` defines the matching 15-sample,
  15% median investigation threshold.
- This sandbox forbids loopback listeners, so no misleading baseline number was
  recorded here. Run the documented command on an allowed host before changing
  socket defaults; retain the output in an ignored `artifacts/` path or CI
  artifact for comparison.

## Checkpoint: repeated sandbox confirmation (2026-07-26)

`TCP_RUNS=1 TCP_BENCH_TIME=100ms ./scripts/bench-tcp-loopback.sh` confirmed
that all required workload families are skipped here because loopback
listeners are forbidden. The host/architecture and Go version were reported,
but no samples were produced. This is an environment limitation, not a zero
result or an acceptable baseline; task completion still requires the
documented 15-sample allowed-host run.

## Checkpoint: allowed-host harness correction (2026-07-26)

An allowed-host run using `TCP_RUNS=15 TCP_BENCH_TIME=1s` reached the
benchmarks, proving that the host can provide measurements. Its first artifact
was intentionally not accepted as a baseline: both round-trip benchmark
families failed before recording samples because the echo worker used
`Value.String()` (the quoted language-source display representation) instead
of the underlying string payload. `BenchmarkTCPLoopbackCancelledAccept`
completed all 15 samples (83.20--84.56 ns/op, 96 B/op, 2 allocs/op), but that
alone does not satisfy the latency/throughput baseline requirement.

`internal/osiris/tcp_benchmark_test.go` now decodes incoming values with
`AsString()` on both echo and assertion paths. A local focused benchmark run
passes with all workloads skipped only because this sandbox forbids loopback
listeners. Preserve the failed raw artifact for audit, then collect a fresh
full baseline on the allowed host before changing a socket default:

```sh
TCP_RUNS=15 TCP_BENCH_TIME=1s \
  TCP_BENCH_OUT=artifacts/tcp-loopback-fixed.txt \
  ./scripts/bench-tcp-loopback.sh
```

## Checkpoint: first allowed-host baseline (2026-07-26)

The corrected 15-sample baseline completed on an Apple M1 Pro (`Darwin/arm64`,
Go `go1.26.1 darwin/arm64`) with one-second samples and `-benchmem`. Raw
samples are retained outside version control at
`artifacts/tcp-loopback-fixed.txt`.

| Workload | Median | Range | Allocation result |
| --- | ---: | ---: | ---: |
| 64-byte round trip | 62,749 ns/op | 62,501--63,796 ns/op | 480 B/op, 6 allocs/op |
| 64 KiB round trip | 202,154 ns/op (324.18 MB/s) | 200,048--214,241 ns/op | 574,880 B/op, 46 allocs/op |
| cancelled accept | 83.24 ns/op | 82.91--85.35 ns/op | 96 B/op, 2 allocs/op |

This is the **default low-latency configuration**, including `TCP_NODELAY` and
a 4 KiB buffered writer. It is not a claim about raw `net.Conn`, native-C, or
no-GC performance. The next comparison will retain this exact workload and
environment while evaluating an explicit bulk-transfer override on both
endpoints;
no portable default changes on the strength of this single baseline.

## Checkpoint: explicit bulk comparison harness (2026-07-26)

Added `BenchmarkTCPLoopbackBulkRoundTrip64KiB`. It applies the existing,
explicit bulk-transfer policy (`TCP_NODELAY=false`, 64 KiB buffered writer) to
**both** accepted and dialed streams before either is exposed. The ordinary
`tcp_accept` path continues to call `acceptContext` and therefore retains
`DefaultSocketConfig`; the new configuration-taking accept helper is internal
to preserve the public API contract. The benchmark script automatically
includes this fourth workload in its raw output.

Focused SocketConfig and benchmark checks pass in the sandbox (network
workloads are skipped there because loopback listeners are prohibited). Run
the same 15-sample command again on the baseline host before selecting a
default or claiming a performance improvement.

Collect that evidence on the same Apple M1 Pro host with:

```sh
TCP_RUNS=15 TCP_BENCH_TIME=1s \
  TCP_BENCH_OUT=artifacts/tcp-loopback-bulk.txt \
  ./scripts/bench-tcp-loopback.sh
```

The artifact includes both default workloads and
`BenchmarkTCPLoopbackBulkRoundTrip64KiB`, so it is the only valid input for a
default-selection decision.

## Checkpoint: readable benchmark report (2026-07-26)

`scripts/bench-tcp-loopback.sh` now prints a compact per-workload median,
range, throughput, allocation-byte, and allocation-count table before the raw
artifact path. Raw Go samples remain complete in `TCP_BENCH_OUT`, and
`TCP_BENCH_SHOW_RAW=1` restores their stdout display for diagnosis. The parser
uses Go's unit tokens rather than fixed field positions, so cancellation
samples (which have no throughput field) and transfer samples share one stable
report. `sh -n` and the sandbox no-listener smoke run pass; the latter
intentionally has an empty measured table rather than a fabricated result.

## Checkpoint: bulk endpoint configuration coverage (2026-07-27)

`TestSocketConfigBulkBatchAppliesToBothEndpoints` now creates the same
explicit bulk policy used by the benchmark (`TCP_NODELAY=false`, 64 KiB
batching), dials and accepts with that policy, and proves the client and
accepted stream each own a 64 KiB `bufio.Writer`. It deliberately checks the
observable batching contract rather than relying on a non-portable socket
option getter. The focused normal and `-race` commands pass; the loopback
portion remains an explicit skip in this sandbox, not a synthetic success.

## Checkpoint: portable configuration boundary (2026-07-25)

- Added typed `SocketConfig` validation and application for portable TCP
  controls: no-delay, keepalive, keepalive period, connect/read/write timeout,
  kernel read/write buffers, and bounded user-space write batching.
- `DefaultSocketConfig` enables `TCP_NODELAY` and keepalive on each newly
  accepted or dialed TCP connection. The bulk-transfer override is explicit
  (`NoDelay: socketBool(false)`); no deadline is silently added, preserving the
  existing context-cancellation semantics.
- Focused tests cover defaults, validation, and configured loopback dial. The
  latter skips in this sandbox because loopback listeners are unavailable.
- The canonical React `std/net` reference now documents the default,
  portability boundary, native-C unsupported status, and checked examples;
  reference search verifies `tcp_nodelay`.

## Checkpoint: transfer contract (2026-07-25)

`docs/design/TCP-TRANSFER.md` specifies the bounded transfer API, explicit
non-ownership, byte-order/backpressure rules, cancellation/close semantics,
and the required separate lifecycle/read/write synchronization. The existing
stream lock is deliberately not bypassed by a naïve `io.Copy`; implementation
must first make close able to unblock a transfer without a data race.

## Checkpoint: bounded transfer implementation (2026-07-25)

- `transfer(source, destination[, limit])` is a context-aware public builtin
  backed by a fixed 32 KiB buffer. It flushes successful writes, returns an
  exact `uint64` count, forbids self-transfer, and retains caller ownership.
- Stream lifecycle, read, and write coordination are now separate so closing a
  descriptor can unblock a pending operation rather than waiting on a copy.
- `TestTransferStreamsPreservesBoundedBytesAndFlushes` covers the bounded
  helper contract, while `TestTransferBuiltinInterpreterVMParity` exercises
  parsing, builtin registration, flushing, and output parity in both language
  execution engines. The focused `go test -race` transfer/TCP suite passes.
  The React reference has a searched, checked API entry including arguments,
  return value, errors, and native unsupported status.

## Checkpoint: runtime boundary (2026-07-26)

`docs/design/TCP-NETWORKING.md` now records why the present Go-hosted stream
implementation cannot promise no-GC behavior, deterministic RAII cleanup,
pinned user buffers, zero-copy, or native event-engine semantics. It separates
the current explicit-close resource contract from a future native runtime and
lists the ownership, async-state-machine, platform-adapter, scheduler, and
measurement evidence required before making any of those claims.

The canonical React reference now has a searched "Hosted runtime and native
boundary" language-model card. It distinguishes explicit `close` for OS
resources from general deterministic reclamation, names the Go-hosted
interpreter/VM reality, and states that the current C backend rejects network
and task-runtime programs instead of implying a native no-GC promise.

## Non-goals

- Do not add a general event-loop rewrite, no-GC guarantee, io_uring promise,
  or raw arbitrary socket-option escape hatch in this task.
- Do not optimize before task 011 supplies cancellation and structured network
  errors, or before task 012 defines the portable polling abstraction.

## Design input

The following submitted design brief is retained as source guidance. It is not
itself a commitment to every OS-specific mechanism; the required-work and
acceptance sections above define the reviewable implementation scope.

If you are designing a modern programming language or building its core standard library, 
your goal should be to hide complex network mechanics behind safe, high-performance 
abstractions while exposing precise surgical knives for power users.


1. High-Performance Defaults

TCP_NODELAY by default: Enable Nagle’s algorithm override natively on all newly initialized sockets. Most modern use cases (APIs, RPCs, microservices) require low latency over bandwidth optimization. Provide a flag to disable it for large file transfers.

Auto-managed connection timeouts: Enforce a default connection, read, and write deadline on all standard library socket wrappers. Never allow an unconfigured socket to block or leak runtime threads indefinitely.

Intelligent initial buffer sizing: Dynamic auto-tuning socket buffer sizing based on the operating system's detected capacities, rather than hardcoded 4KB or 8KB legacy constraints.


2. Direct-to-Kernel System Calls

Native zero-copy abstractions: Expose a clear stream-to-stream pipeline API (like Go's io.Copy or Java's transferTo) that internally maps to low-level kernel routines like Linux sendfile or splice. This bypasses user-space memory entirely.

Dual-stack sockets by default: When a server binds to an IPv6 address, automatically configure IPV6_V6ONLY = 0 so the runtime seamlessly accepts both IPv4 and IPv6 traffic without forcing the developer to manage duplicate listener logic.

SO_REUSEPORT load balancing: Provide a native configuration option to bind multiple distinct runtime threads or processes to the exact same IP/Port combo, letting the OS kernel handle incoming connection balancing at the hardware level.


3. Memory & Runtime Mechanics

Off-heap buffer pooling: For managed or garbage-collected (GC) languages, implement a native, reusable network buffer pool (e.g., ring buffers or arena allocators). This prevents network I/O from thrashing the GC with transient byte-array allocations.

Asynchronous socket batching: Build user-space buffer aggregation mechanics into your runtime socket wrappers. Provide a programmatic way to block writes temporarily (similar to Node's socket.cork()) so multiple small application payloads are flushed to the OS as a single TCP packet.

Direct event-loop bindings: Ensure your language's async runtime interfaces directly with high-performance native OS multiplexers (epoll on Linux, kqueue on macOS/BSD, IOCP on Windows) via thin wrappers rather than heavy cross-platform translation layers.

4. Exposed Control Knobs

Provide an ergonomic, strongly-typed configuration object for low-level overrides, making it easy to toggle settings like:

SocketConfig {
    KeepAlive: true,
    KeepAliveIdleTime: 60s,
    QuickAck: true,                // Maps to TCP_QUICKACK (disables delayed ACKs)
    CongestionControl: "bbr",      // Allows setting TCP_CONGESTION dynamically
    DeferAccept: true              // Maps to TCP_DEFER_ACCEPT (wakes up thread only when data arrives)
}



For a cross-platform, garbage-collection-free (no-GC) language aiming for maximum TCP performance, the best concurrency model is Async/Await backed by a multi-threaded, work-stealing Event Loop.This model—pioneered in this space by Rust’s tokio framework—perfectly balances raw performance with safety in the absence of a GC.

 Why This Model Wins1. Zero-Cost Memory Safety (RAII Compatibility)Without a GC, you cannot use Go-style green threads (goroutines). Green threads require a runtime managed stack that can grow and shrink dynamically, which requires a GC or heavy runtime overhead.The Async Solution: Async/Await compiles functions down into state machines. The compiler calculates the exact size of the future/promise at compile time.The No-GC Benefit: Memory is allocated upfront on the stack or in explicit heap buffers. It obeys standard Resource Acquisition Is Initialization (RAII) rules. When the network operation finishes, memory is cleanly freed without any runtime tracking or GC pauses.2. Native OS Multiplexing (Cross-Platform)Your language runtime can map this single async model directly to the most powerful notification engine of each operating system:Linux: epoll (or the newer io_uring for true asynchronous I/O)macOS / BSD: kqueueWindows: IOCP (I/O Completion Ports)An event loop abstracts these variations away. The developer writes unified async/await code, while your runtime engine translates it into zero-overhead native OS system calls.3. Maximum Core Utilization via Work-StealingA multi-threaded event loop assigns a pool of worker threads (usually matching the machine's CPU core count). If one thread finishes processing its assigned TCP connections and becomes idle, it "steals" pending network tasks from busier threads. This prevents CPU cores from sitting idle during traffic spikes.🛠️ Architecture Checklist for the Language WriterIf you choose this model, you must build the following components into your runtime library:📂 1. Epoll / Kqueue / IOCP Abstraction LayerDo not write platform-specific network code in the upper layers. Create a unified, internal system trait/interface.Linux/macOS: Use readiness-based models. The OS tells you when a socket is ready to be read from or written to.Windows: Use a completion-based model. The OS takes a memory buffer, performs the network I/O in the kernel, and tells you when the transfer is finished.Architecture Tip: Design your internal runtime around the completion-based model. It maps cleanly to Windows and Linux io_uring, and can be emulated on epoll/kqueue by managing user-space buffers.🏎️ 2. Explicit "Pinning" and OwnershipWithout a GC, passing a memory buffer to an asynchronous TCP read operation is dangerous. If the user drops or frees the buffer before the OS finishes writing to it, memory corruption occurs.The Fix: Your language compiler must enforce strict ownership. The async network function must take ownership of the buffer array and return it back to the developer only when the network event finishes.🧵 3. Thread-Pool Isolation for Blocking TasksNetwork applications eventually need to interact with disk I/O, database drivers, or heavy CPU cryptography (like TLS handshakes). If a developer executes a blocking operation inside your network event loop, it freezes the entire thread, stalling thousands of other TCP connections.The Fix: Bake a secondary, separate thread pool into the runtime explicitly for blocking tasks (e.g., spawn_blocking()). Keep the primary network loop pristine and unblocked.
