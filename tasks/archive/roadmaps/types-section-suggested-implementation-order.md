# Types — 9. Suggested Implementation Order

Source: roadmaps/TYPECHK.txt, section ## 9. Suggested Implementation Order.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Phase 1: signed integers, explicit float widths, `u128`, `f32`, `f64`
- [ ] Phase 3: `complex<f32...f256>`, `quat`, `dual`
- [ ] Phase 4: `simd<N,T>`, `matrix<M,N,T>`, `tensor<Shape,T>`
- [ ] Phase 5: `u256`, `i256`, `u512`, `i512`, `u1024`, `i1024`, `f128`, `f256`, `f512`, `f1024`, `bigint`, `bigfloat`
- [ ] Phase 6: `sparse_matrix`, `sparse_tensor`, `modint<M>`, richer `quantity<T,U>`

