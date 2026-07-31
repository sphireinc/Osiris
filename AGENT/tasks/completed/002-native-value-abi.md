# Native runtime ABI and scalar specialization

Source: OPTCHK.md, TYPECHK.txt, and docs/design/NATIVE-COMPILER.md.

Define a small C runtime ABI before expanding code generation. The initial
contract is documented in docs/design/NATIVE-ABI.md. It must cover
ownership, strings/bytes, errors, functions, output, and explicit dynamic
fallback boundaries. Define native representations and calling conventions for
bool, fixed-width integers, f32, and f64.

Keep generic boxed Value only at dynamic boundaries. C-level performance means
hot scalar and kernel loops do not dispatch through interpreter values.

Acceptance: documented ABI, generated C runtime helpers, scalar passing/return
and overflow tests, and parity tests for conversion into/out of fallback values.

Progress: tagged scalar values, width-preserving f32/f64 casts, UTF-8 string
length and concatenation with process-lifetime arena ownership, immutable list
literals/indexing/iteration, direct user-function ABI, checked unboxed scalar
arithmetic, straight-line scalar helper inlining (including locals), and
branching scalar helper inlining, and operator-specific C entry points are
implemented and covered by executable parity tests. This task remains open for
tensors, host calls, and a unified native
recoverable-error/process-error contract.

## Completion record — 2026-07-28

This historical umbrella is complete through focused successor work. The
versioned native ABI in `docs/design/NATIVE-ABI.md` now specifies tagged
dynamic values, unboxed scalar eligibility, calling conventions, ownership,
temporary-buffer transfer, failure behavior, and unsupported boundaries.
Tasks 028 and 029 completed the ownership audit, sanitizer path, deterministic
runtime-component layout, and generated-C review coverage; task 032 completed
the numeric width/overflow contract; task 047 deliberately keeps host calls
out of the executable ABI pending a future capability-gated FFI feature.

Native recoverable runtime failures use `ofail` / `onativefail` to report a
stable `osiris native error:` diagnostic and produce the documented null value
at the tagged boundary. Proven unboxed scalar paths report their stable
diagnostic and terminate rather than invoking undefined host operations. This
is an intentional process boundary, not a missing host-call fallback.

Validation on 2026-07-28:

```sh
GOCACHE=/private/tmp/osiris-gocache GOMODCACHE=/private/tmp/osiris-gomodcache \
  go test ./internal/osiris -run \
  'TestCBackend(DirectFunctionParity|DirectClosureParity|FixedWidthIntegerParity|FixedWidthIntegerArithmeticParity|ScalarCastAndStringLenParity|ListLiteralParity|ListIndexAndForParity|ContiguousTensorKernelParity|ComplexScalarParity)$' \
  -count=1
```

Passed (`ok github.com/sphireinc/osiris/internal/osiris`, 4.929s). These
executable parity tests cover scalar argument/return ABI, closure captures,
fixed-width conversion and overflow paths, tagged string/list fallback values,
and owned f64/complex tensor representations. Future FFI implementation must
be a new scoped task; it is not an untracked requirement of this ABI record.
