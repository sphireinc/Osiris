# Implementation priority

## P0 — establish the native compiler

1. `001-c-backend-real-lowering.md`: executable C subset; remove snapshot
   evaluation from `CompileC`.
2. `002-native-value-abi.md`: runtime ABI and unboxed scalar contract.
3. `006-numeric-literal-contract.md`: stable source/inference type contract.
4. `003-native-loop-optimization.md`: canonical loops and safe basic lowering.
5. `004-native-numeric-and-tensor-lowering.md`: scalar hot paths and first
   contiguous tensor kernel.
6. `005-native-tooling-validation.md`: native parity and benchmark gates.

## P1 — extend native semantic coverage

Tasks 007, 009, 010, 013, and 014 must target the C runtime and generated C,
not only the Go interpreter. Task 012 provides polling foundations only when
the native task runtime needs them.

## P2 — broaden the surface after measurement

Tasks 008, 011, 015, and 016 remain useful, but they must not delay P0. FFI,
polling backends, LLVM/JIT, sparse types, and wide/exotic numeric forms require
the P0 ABI and lowering contracts first.
