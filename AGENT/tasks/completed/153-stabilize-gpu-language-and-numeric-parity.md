# Task 153: Stabilize GPU language, numeric, and library parity

**Priority:** P1
**Dependencies:** 152,140,141,100,103,120,137
**Status:** Complete

## Objective

Provide stable GPU execution for the supported language and numerical surface,
with exact documented semantics and differential evidence against interpreter,
VM, and native C where applicable.

## Exact scope and ownership

- Own GPU lowering/runtime files introduced by task 152 and GPU conformance
  fixtures; do not modify native C or WASM files.
- Cover scalar/vector/tensor numeric families, constructors, ranges, shape
  operations, elementwise kernels, reductions, dot/matmul, signal kernels,
  deterministic math, and pure stdlib operations that have a GPU mapping.
- Define stable behavior for errors/results/options, state, modules, patterns,
  tasks, and host-only APIs through explicit GPU capability profiles rather than
  silently omitting operations.
- Add device-independent CPU reference checks, precision/tolerance contracts,
  memory-limit tests, cancellation, and deterministic kernel ordering.

## Matrix transitions

Promote GPU core syntax, typed checking, state, types/numeric, math, array,
range, tensor, matrix, shape, linear-algebra, reductions, signal, and mapped
stdlib rows to `stable`; all remaining language forms need a stable profile or
must be removed from the matrix rather than left unsupported.

## Parallelism

This is the final GPU implementation task and may run concurrently with WASM
and native host tasks.

## Completion evidence

- `gpu_reference_kernels.go` and `gpu_parity.go` provide a device-independent
  profile and reference kernels for elementwise arithmetic, constructors,
  ranges, shape transforms, reductions, deterministic math, dot/matmul, and
  signal operations.
- `internal/osiris/gpu_backend_test.go` compares mapped operations with both
  interpreter and VM results and covers shape errors, resource limits,
  ownership, cancellation, and repeatability.
- `docs/design/GPU.md` documents numeric tolerance, mapped operations,
  capability rejection, and resource ownership.
- The focused GPU suite passes after the unary-kernel output-shape fix.
