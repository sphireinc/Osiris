# Async — PHASE 6: OPTIMIZATION AND NUMERIC PERFORMANCE

Source: roadmaps/ASYNC-PLAN-CHK.txt, section PHASE 6: OPTIMIZATION AND NUMERIC PERFORMANCE.

## Native compiler integration

P0 prerequisite: complete tasks 001-c-backend-real-lowering and 002-native-value-abi first. This work must lower to generated C, not exist only in the Go runtime.

## Remaining checklist

- [ ] Vectorize elementwise tensor kernels.
- [ ] Improve cache locality for dense tensor and matrix loops.
- [ ] Parallelize large tensor and matrix work across threads.
- [ ] Keep memory layout explicit for dense numeric data.
- [ ] Hoist shape and bounds checks out of hot loops when safe.
- [ ] Fuse adjacent tensor operations when aliasing and purity allow it.
- [ ] Specialize hot functions by shape and scalar kind.
- [ ] Avoid boxed values in numeric hot loops.
- [ ] Reuse scratch buffers when it is safe.
- [ ] Keep generic dispatch out of inner kernels.
- [ ] Avoid needless transposes in optimized numeric code.
- [ ] Keep async overhead out of hot numeric kernels.

