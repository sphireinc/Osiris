# Named units and global constants

Osiris exposes an immutable built-in catalog of unit quantities, mathematical
scales, physical constants, and error-kind names. The ordered
`namedConstantDefinitions` table in `internal/osiris/named.go` is the
implementation source for canonical names and aliases. Registration rejects
duplicate names during package initialization, and documentation checks compare
the complete table with both public constant references.

## Quantity representation

A quantity stores an exact normalized `big.Rat` magnitude and a dimension
vector over the seven SI base dimensions. Aliases are lookup names only:
`Foot`, `foot`, and `ft` construct indistinguishable values. The runtime does
not retain a display-unit label, so equal normalized quantities format using
their dimensions rather than whichever alias created them.

Exact decimal and rational definitions enter the catalog through
`quantityFactoryText`; they never pass through `float64`. Addition and
subtraction require identical dimensions. Multiplication and division compose
dimension exponents, and comparisons reject incompatible dimensions. The
binary codec records the exact rational magnitude and sorted dimensions.

Plane angle remains dimensionless. `Radian`, `Degree`, `Arcminute`,
`Arcsecond`, and `Revolution` are default-number scales rounded to binary64,
not quantities with a fictitious angle dimension. Affine temperature scales
are not multiplicative constants and are excluded.

## Naming and standards

Each concept has one canonical PascalCase name, normally a readable lower- or
camel-case alias, and only unambiguous conventional symbols. Names are
case-sensitive. The catalog deliberately omits ambiguous spellings such as
`in`, `ton`, and unqualified `fluidOunce`; `Gallon` means the U.S. liquid
gallon, while imperial measures are explicitly prefixed.

Definitions are pinned to:

- BIPM, *The International System of Units (SI Brochure)*, 9th edition,
  version 4.01 (June 2026), for SI and accepted non-SI units;
- NIST Special Publication 811, 2008 edition, Web version 3.2, Appendix B,
  for international and U.S. customary conversions;
- the UK Weights and Measures Act 1985 for the exact 4.54609 L imperial
  gallon;
- IAU 2012 Resolution B2 and IAU 2015 Resolution B2 for astronomical-unit and
  parsec definitions; and
- NIST's 2022 CODATA recommended values, database version 9.0 (May 2024), for
  physical constants.

The detailed public reference distinguishes exact definitions from measured
concise CODATA values and from irrational definitions stored at a stated
decimal precision.

## Compiler boundary

Inference derives a quantity's dimension vector directly from its registered
value. Interpreter and VM lookup use the same runtime table, and codecs
preserve exact normalized values. Native C directly lowers supported
default-number globals. It rejects named quantities at compile time with a
specific diagnostic until the C ABI can preserve arbitrary rational
magnitudes and dimension vectors; compilation never evaluates the program to
manufacture output.

User-defined `unit` declaration syntax, automatic display-unit selection, and
affine conversion remain future work. Any such design must build on the same
normalization and collision rules rather than changing the meaning of the
built-in catalog.
