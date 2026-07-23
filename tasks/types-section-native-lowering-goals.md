# Types — 8. Native Lowering Goals

Source: roadmaps/TYPECHK.txt, section ## 8. Native Lowering Goals.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Unboxed scalar fast paths for all fixed-width primitives
- [ ] Special native lowering for `f32`, `f64`, `u64`, `i64`
- [ ] Multiword lowering for `u128`, `u256`, `i128`, `i256`
- [ ] Tensor and matrix kernel specialization
- [ ] Alias-aware view lowering
- [ ] SIMD-aware packed math lowering

