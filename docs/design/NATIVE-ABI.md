# Native runtime ABI

## Status

This is the initial C backend ABI. It applies to generated code and the native
runtime only; it is not a public foreign-function interface.

## Scalar values

Generated C uses an OValue tagged value at dynamic boundaries:

- O_NULL
- O_BOOL
- O_UINT (uint64_t)
- O_INT (int64_t)
- O_FLOAT (double plus optional 32- or 64-bit source-width metadata)
- O_STRING (immutable UTF-8 pointer)

Hot paths may specialize an OValue operation to a native scalar only when
inference proves its kind. The C backend currently emits scalar-only `main`
CFGs as `uint8_t`, `uint16_t`, `uint32_t`, `uint64_t`, `int64_t`, `float`,
`double`, and `int` locals, including loop-carried values. It preserves
f32/f64 and unsigned-integer width metadata on that path. Same-width narrow
unsigned `+`, `-`, `*`, bitwise operations, and shifts are specialized; their
arithmetic widens before calculating and narrows explicitly to retain Osiris
wrapping semantics without C signed-promotion undefined behavior. The proven
`u8`/`u16`/`u32` subset also supports mixed narrow widths, yielding the wider
operand width for arithmetic, remainder, and bitwise operations, or the left width for a
shift. The same promotion rule is proven for supported `i8`/`i16`/`i32`
operations. Same-width `u64` and `i64` remainder are also specialized with an explicit
zero-divisor check; `i64` additionally uses checked direct `+` and `*` with
stable overflow diagnostics and direct fixed-width `&`, `|`, and `^`.
Same-width `i64` shifts validate the count and use unsigned-bit operations
with explicit sign extension for right shifts before reconstructing the fixed
width signed result. Same-width
signed `i8`, `i16`, and `i32` `+`, `-`, `*`, unary negation, remainder, `&`,
`|`, and `^` are likewise specialized through an `int64_t` intermediate and
explicit signed-width normalization. Their shifts reject a negative or
out-of-width count and use unsigned-bit operations with explicit sign
extension for right shifts; remainder checks zero explicitly. Mixed-family,
division, and promotion-sensitive paths stay tagged until their error contract
is proven. A
non-capturing straight-line scalar helper (including local scalar bindings) is
inlined into such a caller. A scalar `if` helper with straight-line branches is
also inlined as typed C control flow. Short-circuit `&&` and `||` lower through
that typed CFG path, preserving branch-only operand evaluation. Other
functions and uncertain values use
the tagged ABI. Generated tagged code also
calls an operator-specific entry point (`oadd`, `omul`, and so on), rather than
passing an operator string through a generic dispatch at every instruction.

## Calling convention

Generated functions return OValue and accept OValue parameters. A directly
called lexical closure passes its captured OValues as hidden trailing
parameters; this avoids allocating a closure object for the direct-call case.
Escaped or indirect closures use a tagged closure object containing a native
call target and an owned copy of its captures. Host calls remain unsupported
until their explicit ABI form is added.

The tagged scalar ABI preserves the declared width of `u8`, `u16`, `u32`,
`u64`, `byte`, `i8`, `i16`, `i32`, and `i64` literals and casts. Narrow
integers not covered by the proven unboxed subsets stay on this path until they
have the same wrapping and overflow contract; they must never be widened
silently to `u64` in generated output.

Within the tagged ABI, same-family fixed-width integer `+`, `-`, `*`, exact
division, remainder, bitwise operations, shifts, unary negation, and `abs`
retain the interpreter's result width. Mixed signed/unsigned arithmetic and
non-exact integer division require bigint/decimal lowering and are not treated
as ordinary C integer operations.

## Static standard-library resolution

The C backend resolves direct members of a statically imported `std/math`
module at compile time. Supported members lower to native runtime helpers; the
import and member IR instructions do not become dynamic module objects in the
executable. An unsupported static member is a compile-time diagnostic. Dynamic
imports, module values that escape, and non-`std/math` modules remain explicit
lowering work rather than silently becoming null or interpreter calls.

`std/math.pi` and `std/math.tau` lower as exact IEEE-754 hexadecimal C
literals. Native float printing selects the shortest decimal that round-trips
to its stored value, so these constants and other generated floats retain the
same visible rendering as Osiris values.

The native scalar `std/math` subset includes roots, exponential/logarithmic,
trigonometric, inverse-trigonometric, hyperbolic, inverse-hyperbolic,
error/gamma, rounding, variadic `min`/`max`, `clamp`, and `hypot` operations.
Operations whose interpreter
contract rejects a NaN or infinity domain result (`asin`, `acos`, `acosh`, and
`atanh`) perform that check before returning a native value.

`std/math.vec` and `std/math.vector` lower to the contiguous one-dimensional
f64 tensor representation for empty, list-backed, variadic, and existing
tensor inputs. Integer-element tensor style preservation is outside this
initial f64 ABI and must not be inferred from this support.

`std/math.zeros`, `std/math.ones`, and `std/math.full` lower for one or more
non-negative dimensions, or a shape list, with numeric fill values for `full`.
The native tensor ABI stores rank and a copied shape beside its contiguous f64
buffer, so construction, printing, `shape`, equality, and direct elementwise
same-shape/scalar kernels retain multidimensional shape rather than silently
flattening it. Tensor-to-tensor `+`, `-`, `*`, and `/` use NumPy-style shape
broadcasting through precomputed contiguous strides. Indexing remains separate
lowering work.

`at(tensor, index)` retains flat indexing for a scalar index. For rank-aware
lookup, `at(tensor, [i, j, ...])` accepts one non-negative coordinate per
dimension and returns the selected f64 element; coordinate rank and bounds are
checked by both the interpreter and native runtime. A non-empty coordinate
prefix returns an owned rank-reduced contiguous tensor slice; a full coordinate
returns one element.

For tensors, `setat(tensor, index, value)` supports the same flat or full
coordinate indexing forms. It allocates a new data buffer and copies the shape,
so replacing an element is alias-safe and never mutates the original tensor.
For a non-empty coordinate prefix, the replacement must be a tensor whose
shape exactly matches the unselected trailing dimensions; its contiguous data
replaces that owned slice in the newly allocated result.
`take(tensor, count)` copies the first `count` flattened elements into a new
one-dimensional tensor, clamping a count above the source length.
`slice(tensor, start, end[, axis])` copies a half-open range on the selected
axis (the leading axis by default) and preserves every other dimension. Both
endpoints and an optional zero-based axis must be non-negative; endpoints
beyond the selected dimension clamp to it, while an end before start is
rejected.

`tensor(...)` recursively infers and validates rectangular nested numeric
lists. Its explicit shape/data form validates the supplied element count.
`matrix(...)` supports the same forms plus `matrix(rows, columns, data)`.
Ragged input and non-numeric leaves are rejected by the native runtime.
`reshape(tensor, shape)` validates the new shape's element count and produces a
new owned contiguous tensor with the requested rank and dimensions.
`identity(size)` and `identity(rows, columns)` construct an owned rank-two f64
tensor with ones on the shared diagonal; `eye` is its native alias. Both forms
validate non-negative dimensions and use a direct diagonal-fill kernel.
`transpose(matrix)` and `matmul(left, right)` lower for rank-two f64 tensors.
They validate rank and compatible inner dimensions, then use direct contiguous
copy and triple-loop kernels respectively.
`outer(left, right)` lowers for f64 tensors of any rank by flattening each
operand and constructing the corresponding owned rank-two product tensor.
`trace(matrix)` lowers for rank-two f64 tensors and sums the shared diagonal of
a rectangular or square matrix. `det(matrix)` lowers for square rank-two f64
tensors through the interpreter-matched partial-pivot elimination algorithm;
the empty determinant is one and a singular matrix returns zero.
`solve(matrix, rhs)` lowers for a non-empty square rank-two f64 matrix and a
rank-one vector or rank-two RHS tensor with matching row count. It uses
partial-pivot Gauss-Jordan elimination, returns a vector for one RHS column,
and reports singular matrices explicitly. `inv(matrix)` uses that same native
path against an identity RHS.
`cholesky(matrix)` lowers for square rank-two f64 tensors. It matches the
current interpreter by reading the lower triangle directly and requiring every
computed diagonal pivot to be positive; it returns an owned lower-triangular
factor and does not independently enforce symmetry.
`qr(matrix)` lowers for rank-two f64 tensors through the interpreter-matched
classical Gram-Schmidt algorithm. It returns an owned list `[Q, R]`, where `Q`
has the input's shape and `R` is square on the input column count; dependent
columns remain zero rather than causing an error.

`std/math.arange`, `std/math.linspace`, and `std/math.logspace` also lower to
the one-dimensional f64 ABI. Their optional arguments and default counts match
the interpreter; `arange` retains its one-million-element safety limit.

## Ownership

| Value family | Returned/retained representation | Ownership and cleanup rule |
| --- | --- | --- |
| Scalars and `OValue` itself | copied by value | No heap ownership; payload pointers follow their family rule. |
| Strings | static literal or arena allocation/view | Literals are borrowed static storage; concatenation is registered in the native arena. Slices/views retain the source lifetime through that arena. |
| Lists | arena-owned copied `OValue[]` | `olist` copies members into arena allocation. List copies share immutable storage. |
| Closures | arena-owned capture array | Direct closures pass borrowed copied `OValue` arguments; escaped closures own an arena copy of captures and never retain Go memory. |
| f64 tensors | arena-owned data plus shape | Constructors and transformations return owned independent data/shape; copy-on-write helpers never mutate an input tensor. Temporary shape/data buffers are freed before return or registered by the constructor. |
| Complex tensors | arena-owned complex data plus shape | Same as f64 tensors; mixed calculations allocate a new owned complex tensor. |
| Errors | static message / scalar `OValue` failure | `ofail` does not borrow caller-owned temporary memory. Native code has no heap error object yet. |
| Numerical-provider outputs | provider-owned temporary buffers copied into arena tensors | Provider adapters copy results before returning `OValue`; the generated runtime never frees provider-owned memory. |

The generated executable releases its arena with `ofreeallocs` at process exit.
Helpers that allocate temporary work buffers (`malloc`/`calloc`) must either
free them on every success and error path or transfer them exactly once to an
arena-registering constructor. Borrowed pointers must never refer to Go memory.

- Strings use an immutable UTF-8 pointer-and-length representation. Literals
  point to static storage. Concatenation allocates a null-terminated buffer in
  a tracked native arena that is released at process exit; copies of an OValue
  safely share that storage. `len` counts Unicode scalar values without an
  additional allocation.
- Lists use a pointer-and-length OValue array in the same process-lifetime
  arena. Literal construction copies OValues into that array; `len`, `at`,
  structural equality, printing, truthiness, and `for` lowering are supported.
- String `at` uses Unicode code-point indexing, matching the interpreter's
  rune-oriented string semantics rather than indexing raw UTF-8 bytes. The
  returned string is a non-owning view into the source string, whose lifetime
  is already covered by the native arena/static-string contract.
- The initial tensor form is a contiguous f64 buffer plus owned rank/shape
  metadata in the process-lifetime arena. `tensor([numeric...])` validates
  elements once, and equal-shape tensor pairs use direct elementwise `+`, `-`,
  `*`, or `/` loops after a single full-shape check. Numeric scalar broadcasting
  in either operand position uses the same contiguous direct-loop strategy.
  General tensor-to-tensor broadcasting, reshaping, coordinate indexing,
  copy-on-write element and prefix-slice replacement, arbitrary-axis range
  slicing, and rank-two transpose/matmul are now native operations. Strided
  slicing, broader mutation, and non-f64 element types remain unsupported.
  `shape` returns the full u64 dimension list and
  `flatten` returns a numeric list. `sum` and
  `product` reduce the contiguous buffer directly, including their empty
  identities. `dot` validates equal lengths once and uses a direct
  multiply-accumulate loop. `norm` uses a direct sum-of-squares and square-root
  reduction.
- An OValue is passed and returned by value.
- Bytes, errors, closures, and tasks require explicit allocation and release
  rules before native support is added.
- Generated code may not retain pointers to Go-managed Value storage.

## Errors and unsupported operations

## Numeric lowering contract

| Family/operator | Native rule | Unsupported/rejected boundary |
| --- | --- | --- |
| `u8`–`u64` same-family `+ - *` | Preserve width through tagged helpers; unboxed proven `u64` operations use overflow intrinsics. | Overflow requiring bigint promotion is never silently wrapped on an unboxed path. |
| `i8`–`i64` same-family arithmetic | Preserve signed width in tagged helpers; proven unboxed paths report signed overflow. | Minimum-value negation and overflowing arithmetic report a native diagnostic. |
| Integer `/ %` | Exact same-family division/remainder only. | Divide/remainder by zero, non-exact division, and mixed-family integer arithmetic require a non-native numeric path. |
| Integer shifts/bitwise | Tagged width-preserving operators only. | Invalid shift ranges and mixed signed/unsigned operations are rejected rather than delegated to C undefined behavior. |
| `f32` / `f64` | Preserve explicit width metadata and native shortest-roundtrip rendering; proven scalar `/` and `%` lower directly with zero-divisor checks. f32 remainder follows f64 `fmod` then narrows; the tagged fallback uses the same rule. | Domain operations that produce forbidden NaN/infinity return a native failure. |
| Complex | Use explicit real/imaginary arithmetic and checked zero divisor. | Ordering comparisons are rejected. |
| Decimal, rational, bigint, interval, fixed, wide/packed families | No general native scalar lowering. | Compiler emits a stable unsupported-lowering diagnostic; never maps these to C `double` or integer arithmetic. |
| Integer exponentiation | No direct libm lowering. | `pow`/`expt` wait for exact integer-power and promotion parity. |

Negative zero, NaN, and infinities retain their observable float form through
the native printer when the interpreter permits them. Native compilation must
not normalize them through integer formatting or an unchecked libm result.

Unsupported IR operations are compile-time diagnostics; code generation never
silently substitutes a null value for an operation it cannot lower. Supported arithmetic checks
numeric operands before converting them, so strings and other dynamic values
cannot silently become zero. Divide-by-zero reports a native diagnostic and
produces `null` at the recoverable runtime boundary. The unboxed u64 and i64
operations that can overflow use compiler overflow intrinsics and terminate
the native process with a diagnostic; division and unsupported fixed-width
operations stay on the tagged path until they have the same contract.

## Compatibility rule

Every new native kind or helper requires interpreter/native parity tests and a
documented ownership/error rule before emitted code can use it.
