# Dual-number scalar contract

`dual(primal, tangent)` is Osiris's first-order scalar automatic-
differentiation value. It represents `primal + tangent ε`, where `ε² = 0`.
It is a language value rather than a compiler-only annotation, so it has stable
formatting, equality, inference, VM behavior, and binary codec representation.

## Supported scalar operations

For `x = (a, a')` and `y = (b, b')`:

- `x + y = (a + b, a' + b')`
- `x - y = (a - b, a' - b')`
- `x * y = (a b, a' b + a b')`
- `x / y = (a / b, (a' b - a b') / b²)`; a zero primal denominator fails.
- unary `-x = (-a, -a')`.

An ordinary numeric operand becomes `(value, 0)` only at one of these dual
operation boundaries. `primal(value)` exposes the ordinary value and returns a
non-dual argument unchanged; `tangent(number)` returns `0`.

## Deliberate boundary

This first increment is scalar-only. It does not claim tensor lifting,
transcendental derivative rules, higher-order derivatives, units, complex
duals, or native C lowering. The interpreter and VM support the contract;
the C backend rejects `dual` calls with its normal unsupported-builtin
diagnostic rather than erasing the tangent component.

Follow-up work must add a tagged native ABI and executable parity before native
support is advertised, and must specify every math function's derivative rule
before accepting dual operands.
