# Tensor and matrix shape inference

Osiris inference records a tensor shape only when it is statically derivable.
It is an analysis aid, not a substitute for runtime shape validation.

## Known shapes

- Rectangular nested lists retain every dimension: `tensor([[1, 2], [3, 4]])`
  infers `tensor[2x2]`.
- `matrix(rows, columns, data)`, `zeros`, `ones`, `full`, `reshape`, and
  `transpose` infer their documented result shapes when their dimensions are
  literal.
- Tensor binary arithmetic uses the same right-aligned broadcasting rule as
  the runtime. `tensor([10, 20]) + tensor([[1, 2], [3, 4]])` infers
  `tensor[2x2]`, irrespective of operand order.

## Conservative fallback

If dimensions are dynamic or known shapes are incompatible, inference retains
the `tensor` family without inventing a result shape. Runtime evaluation and
native lowering remain responsible for exact shape checks and diagnostics.
