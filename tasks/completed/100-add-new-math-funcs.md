# Task 100: Expand the mathematical and bit-operation surface

**Priority:** P2  
**Dependencies:** 025, 032, 039, 040  
**Status:** Complete  

## Objective

Expand Osiris's existing global numeric helpers and `std/math` module with a
coherent set of missing scalar, complex, classification, decomposition, and
bit operations. Treat the C numeric library as an inventory to audit, not an
API to copy. Osiris names, value kinds, structured errors, determinism, and
interpreter/VM/native parity remain the contract.

Reference inventories:

- <https://cppreference.com/c/numeric/math>
- <https://cppreference.com/c/numeric/complex>
- <https://cppreference.com/c/numeric/tgmath>
- <https://cppreference.com/c/numeric/bit_manip>
- <https://cppreference.com/c/numeric/fenv>

## Existing surface to preserve

Before adding anything, inventory `internal/osiris/runtime.go`,
`internal/osiris/module.go`, `internal/osiris/numeric.go`, and the canonical
`newdocs/` reference. Do not duplicate or rename existing operations such as
`abs`, `min`, `max`, `clamp`, the roots/logarithms, trigonometric and
hyperbolic families, `gamma`, `lgamma`, complex projections, fixed-width
constructors, or the existing bitwise/shift/rotate helpers.

## Required first-release additions

Implement and document these Osiris-level operations unless the inventory
proves an exact equivalent already exists:

- real arithmetic: `atan2`, `exp2`, `trunc`, `fmod`, `remainder`, `fma`,
  `positive_difference`, and `copysign`;
- floating representation: `nextafter`, `frexp`, `ldexp`, and `modf`;
- classification: `isfinite`, `isinf`, `isnan`, `isnormal`, and `signbit`;
- angle helpers: `degrees`, `radians`, `sinpi`, `cospi`, and `tanpi`;
- integer helpers: `divmod`, `gcd`, and `lcm`;
- fixed-width unsigned bit helpers: `count_ones`, `count_zeros`,
  `leading_zeros`, `trailing_zeros`, `bit_width`, `bit_floor`, `bit_ceil`,
  `has_single_bit`, and `byte_swap`;
- complex extensions: support mathematically valid complex inputs for the
  existing `exp`, `log`, `sqrt`, `pow`, `sin`, `cos`, `tan`, `sinh`, `cosh`,
  and `tanh` families, and add `phase` if no existing helper expresses it.

Names may be adjusted only when an existing Osiris convention clearly calls
for a better spelling. Record every naming decision and keep aliases to a
minimum. Do not publish C width suffixes such as `atan2f` or `atan2l`; dispatch
must follow the Osiris `Value` kind and width.

## Semantic contract

- Define accepted kinds, result kind/width, rounding, signed-zero, NaN,
  infinity, domain, pole, overflow, and underflow behavior for every operation
  before implementation.
- Preserve exact integer results where Osiris already promises exactness.
  Never route integers, decimal, bigint, or fixed-width values through
  `float64` merely because the host function accepts a double.
- Multi-result operations use ordinary Osiris values with an explicitly
  documented shape. For example, `frexp`, `modf`, and `divmod` must not expose
  host structs.
- Bit operations require explicit fixed-width unsigned inputs, retain width
  and packed style, and define the zero case for counts and bit ceilings.
- Domain and representation failures use stable structured error kinds and
  messages. Do not leak platform `errno`, floating exception flags, or host
  library wording.
- Results must be deterministic on supported platforms. Characterize Go/C
  libm last-bit differences before enabling direct native lowering.

## Execution paths

- Register the public names in the runtime and `std/math` export table as
  appropriate. Global and qualified spellings must have one semantic
  implementation rather than drifting copies.
- Cover interpreter and bytecode VM behavior.
- Extend inference for result kinds, widths, and multi-result shapes.
- Native C may lower an operation only after executable parity is proven for
  every advertised input family. Otherwise compilation must reject it with a
  clear unsupported diagnostic; interpreter execution during compilation is
  forbidden.
- Update codecs only when a new result shape or value kind actually requires
  it.

## Documentation

- Add an individual canonical `newdocs/` listing for every public function or
  family, including accepted values, return type, return value, errors, and a
  checked code example.
- Update the `std/math` page, the relevant data/bit page, search data, backend
  support matrix, and the Technical Reference complete-surface indexes.
- Keep `newdocs/scripts/check-reference.mjs` exact against runtime and module
  registrations so no newly added name can exist only in an index.

## Required tests

- focused table-driven runtime tests for ordinary and boundary inputs;
- interpreter/VM parity for every new public spelling;
- fixed-width and packed-style preservation for bit operations;
- complex identities and branch-cut characterization;
- domain, overflow, NaN, infinity, signed-zero, and zero-input cases;
- inference assertions;
- generated-C assertions plus executable native parity for each supported
  lowering, and compile diagnostics for deliberately unsupported forms;
- executable documentation examples and global-search coverage.

Run focused tests first, then:

```sh
go test ./...
go test -race ./...
go vet ./...
cd newdocs
npm run check-reference
npm run check-examples
npm run build
```

## Acceptance criteria

- The required additions have explicit reviewed semantics and no accidental
  host-C naming or error behavior.
- Interpreter and VM agree for all supported inputs.
- Every native claim has generated-C and executable parity evidence; all other
  forms fail at native compile time.
- Every public name has a detailed, searchable, example-backed canonical
  reference entry and appears in the exact technical surface audit.
- Full validation passes, or any unrelated pre-existing failure is recorded
  precisely with focused passing evidence for this task.

## Non-goals

- Do not expose C's type-generic macro machinery; Osiris already dispatches by
  runtime value kind.
- Do not expose the process-global floating-point environment, mutable rounding
  modes, sticky exception flags, or C `errno`. Those conflict with deterministic
  concurrent execution and require a separate design.
- Do not blindly implement every C23 decimal, quantum, payload, narrowing, or
  implementation-capability macro.
- Do not add platform-dependent long-double behavior or silently claim native
  support because the host compiler provides a similarly named function.

## Completion record — 2026-07-30

### Implemented surface

- Added 35 shared global and `std/math` callables:
  `atan2`, `exp2`, `trunc`, `fmod`, `remainder`, `fma`,
  `positive_difference`, `copysign`, `nextafter`, `frexp`, `ldexp`, `modf`,
  `isfinite`, `isinf`, `isnan`, `isnormal`, `signbit`, `degrees`, `radians`,
  `sinpi`, `cospi`, `tanpi`, `divmod`, `gcd`, `lcm`, `count_ones`,
  `count_zeros`, `leading_zeros`, `trailing_zeros`, `bit_width`, `bit_floor`,
  `bit_ceil`, `has_single_bit`, `byte_swap`, and `phase`.
- Extended `exp`, `log`, `sqrt`, `pow`/`expt`, `sin`, `cos`, `tan`, `sinh`,
  `cosh`, and `tanh` to their valid complex forms. Both public spellings call
  the same Go implementation; no module/global copies can drift.
- Extended inference for retained float widths, complex results, `bool` and
  `u64` classifications/counts, width-preserving bit results, and the
  two-element list shapes returned by `frexp`, `modf`, and `divmod`.

### Semantic decisions

- New real floating operations accept only the floating `number` kind and
  retain/promote its explicit width and bfloat style. Exact integers,
  decimals, big integers, quantities, and other exact families are never
  silently routed through `float64`.
- `fmod` uses a quotient truncated toward zero; `remainder` uses a
  nearest-even quotient. A zero divisor is structured `invalid_argument`.
  `fma` has one fused rounding followed by the Osiris width boundary.
- `exp2` and `ldexp` return signed/positive zero on finite underflow and use
  structured `out_of_range` when a finite result overflows its retained
  width. Other IEEE exceptional results are values, not host-library errors.
- `frexp` returns `[fraction, exponent_i64]`; `modf` returns
  `[integral_part, fractional_part]`; both floating elements retain style.
  Signed zero is preserved.
- `sinpi` and `cospi` reduce finite inputs modulo two and return exact
  integer/half-integer values. `tanpi` also returns exact quarter-turn values
  and reports its half-integer poles as structured `invalid_argument`.
- `divmod` truncates toward zero and gives the remainder the dividend's sign.
  Matching fixed-width signed/unsigned inputs retain their family;
  mixed/arbitrary-precision forms return `bigint`. `gcd` is non-negative.
  `lcm` widens to `bigint` instead of wrapping when an unsigned result exceeds
  the promoted input width.
- Bit helpers accept only explicit fixed-width unsigned values, use the
  declared width for all zero/count cases, and preserve width plus packed
  style for value results. `bit_ceil(0)` is one and an unrepresentable next
  power is structured `out_of_range`; `byte_swap` reverses the full declared
  byte width.
- Complex operations use Go's principal complex functions, retaining
  component width. `phase` is in `[-Pi, Pi]`; signed imaginary zero selects
  the side of the negative-real branch cut. A complex logarithm base may not
  be zero or one.

### Native decision

No newly added operation or complex transcendental form is advertised as
native C. The compiler reports a compile-time unsupported diagnostic rather
than using an evaluator or an uncharacterized C libm call. Complex-value
tracking covers constants, constructors, copies, arithmetic, CFG block
arguments, captures, direct function parameters, and returned values. The
existing parity-proven real/f32 lowerings remain covered by the full native
test suite.

### Documentation evidence

- `newdocs/src/supplemental-reference.json` contains an individual searchable
  listing, accepted values, result type/shape, errors, semantic detail, and a
  checked example for every new operation.
- Existing complex-family entries in `legacy-reference.json` now describe
  complex inputs, retained widths, principal branches, and native limits.
- The data reference links the fixed-width bit family, the support matrix
  records the interpreter/VM and native contracts, and the Technical
  Reference contains an exact 181-export `std/math` index.
- `check-reference.mjs` now compares that index against the actual module
  table in addition to its exact 262-name global built-in audit.

### Validation evidence

Toolchain: macOS arm64, Go 1.26.1, Node 24.12.0, pnpm 10.30.1, Apple clang
15.0.0.

```text
go test ./internal/osiris -run '^TestExpandedMath' -count=1  PASS
go test ./...                                                PASS
go test -race ./...                                          PASS
go vet ./...                                                 PASS
pnpm run check-reference                                     PASS
pnpm run check-examples                                      PASS
pnpm run build                                               PASS
git diff --check                                             PASS
```

Focused tests cover every new global and qualified spelling, interpreter/VM
parity, structured errors, binary32 adjacency and overflow, underflow, NaN,
infinity, signed zero, exact negative division, non-wrapping LCM, packed and
wide unsigned styles, complex identities/branch cuts, inference, direct and
interprocedural native diagnostics, and documentation execution. The full
suite retains the existing generated-C assertions and executable parity
evidence for every still-supported native math lowering.
