# Named units design

Named units are a planned layer over the existing dimension-aware quantity
values. A name is not merely display text: it selects a normalized scale and
dimension while preserving deterministic arithmetic and conversion rules.

## Declaration and registry

The proposed declaration is `unit Name = scale * dimensions`, for example
`unit meter = 1 * length` and `unit kilometer = 1000 * meter`. Declarations
are module-scoped, immutable, and versioned with the module that exports them.
A registry resolves aliases to one canonical definition and rejects cycles,
zero/non-finite scales, incompatible redefinitions, and ambiguous imports.

Quantities continue to store a normalized exact magnitude plus a dimension
vector. The chosen named-unit presentation is metadata, not a second numeric
value: `3 kilometer` and `3000 meter` compare equal after normalization while
formatting can retain an explicitly requested display unit.

## Arithmetic and conversion

Addition and subtraction require equal dimensions and normalize scales before
arithmetic. Multiplication, division, and powers compose dimensions; a named
derived unit is displayed only when the resulting dimensions and scale match
an unambiguous registered name. `convert(value, Unit)` requires compatible
dimensions and returns a quantity displayed in the target unit. Affine units
(such as Celsius) require an explicit, separately designed conversion model
and are excluded from the first release.

## Compiler and ABI gate

Inference tracks dimension vectors and may retain a named display unit for
literals/conversions, but it must not infer a preferred name from an ambiguous
derived dimension. The codec includes the normalized magnitude, dimensions,
and optional registry-qualified display name; decoding validates the referenced
definition. Native lowering may support a quantity only when it preserves
scale/dimensions and reports an unsupported diagnostic otherwise. No named-unit
syntax is currently implemented or advertised.
