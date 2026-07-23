# Native loop optimization

Source: OPTCHK.md.

Add canonical induction-variable analysis, loop-invariant code motion, and
provably-safe bounds-check elimination. Then add a profitability-gated first
transform: small fixed-trip-count unrolling. Defer fusion and tiling.

Acceptance: before/after IR or C assertions, semantic regression tests for
mutable state and bounds failures, and a benchmark showing no regression.

Progress: scalar-only native CFG lowering now keeps loop-carried integer
values unboxed and hoists pure scalar constants (including loop bounds) to
function entry. The executable regression test asserts both the generated C
shape and loop result. `BenchmarkCBackendUnboxedScalarLoop` compiles once and
then measures native execution of a one-million-iteration unboxed loop.
Canonical while-loop analysis now proves positive constant induction steps and
reports the induction variable, step, and bound. General LICM, checked bounds
elimination, unrolling, and benchmark gates remain open.
