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
reports the induction variable, step, and bound. The native C pipeline now
also performs a proof-gated LICM slice over canonical while-loop IR: numeric
constants and arithmetic/comparison expressions depending only on previously
proven invariants move to the unique preheader. Calls, allocation, division,
indexing, mutation, loop parameters, and effectful values are excluded. The
generated-C placement and interpreter/VM/native executable parity are covered
by `TestCBackendProofGatedLoopInvariantCodeMotionParity`. Checked bounds
elimination now has its first proof-gated slice: a canonical `for` over a
directly constructed list replaces only its header-proven `at(iter, index)`
with unchecked list access. Dynamic containers, tensors, strings, user-written
`at`, effects, and any noncanonical shape retain checked lowering.
`TestCBackendProofGatedForListBoundsEliminationParity` proves generated-C and
executable parity; `TestCBackendBoundsEliminationLeavesDirectAtChecked` locks
the out-of-loop error boundary.

## Completion record — 2026-07-28

The first bounded unrolling transform is complete. It admits only a canonical,
single-block `for` loop over a directly constructed list with 1--4 elements,
at most eight body instructions, and at most 24 cloned instructions. It
rewrites the header/body block-parameter edges for each clone and follows the
original backedge payload, so carried mutable lexical values and the implicit
iteration item advance exactly as they do in the loop. It does not perform
textual loop-variable replacement. Larger lists, nested CFG bodies, dynamic
iterables, and growth beyond the fixed budget retain their original loop.

`TestCBackendSmallStaticForListUnrollingParity` asserts the optimized IR,
the generated-C entry jump that bypasses the loop header, and interpreter/VM/C
executable parity for a carried total. `TestCBackendSmallStaticForListUnrollingIsProfitabilityGated`
locks the five-element rejection boundary. The earlier bounds test continues
to assert the checked direct-indexing error boundary.

Focused validation passed on darwin/arm64 (Apple M1 Pro), Go 1.26.1, Apple
clang 15.0.0:

```sh
go test ./internal/osiris -run \
  'TestCBackend(SmallStaticForListUnrolling|ProofGatedForListBoundsEliminationParity|BoundsEliminationLeavesDirectAtChecked)' \
  -count=1
go test ./internal/osiris -run '^$' \
  -bench 'BenchmarkCBackend(UnboxedScalarLoop|SmallStaticForListUnrolled)$' \
  -count=5 -benchtime=200ms
```

The focused tests passed in 1.475s. The five raw process-execution samples
for `BenchmarkCBackendSmallStaticForListUnrolled` were 210761167,
70254611, 100256521, 43998167, and 33485139 ns/op; the established
one-million-iteration scalar-loop control samples were 84768181, 208622458,
104634188, 107143646, and 43931325 ns/op. The small-list path adds no
per-iteration native loop dispatch and remains within the same
process-launch-dominated range; no performance claim beyond this bounded
regression evidence is made.
