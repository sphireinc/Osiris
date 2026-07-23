# Types — 7. Type-Checking And Inference Milestones

Source: roadmaps/TYPECHK.txt, section ## 7. Type-Checking And Inference Milestones.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Make literal syntax unambiguous for all numeric families
- [ ] Separate default-literal `number` from explicit float widths
- [ ] Infer numeric promotion rules
- [ ] Infer shape parameters for matrices and tensors
- [ ] Track precision and fallback boundaries

