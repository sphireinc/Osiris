# Task 104: Expand global constants and measurement units

**Priority:** P2  
**Dependencies:** 007, 025, 032, 039, 040  
**Status:** Complete  

## Objective

Make Osiris's global constant catalog broad enough for everyday scientific,
engineering, commercial, and unit-aware calculations. Add exact customary and
metric measurement units, useful derived scales, mathematical constants, and
a carefully sourced set of physical constants.

This task extends the existing immutable registry in
`internal/osiris/named.go`; it does not introduce new unit-declaration syntax.
The canonical value representation, aliases, inference, codecs, interpreter,
VM, native compiler boundary, technical index, and expanded documentation must
stay synchronized.

Authoritative starting points:

- BIPM SI units and definitions: <https://www.bipm.org/en/measurement-units>
- NIST measurement-unit guidance: <https://www.nist.gov/pml/owm/si-units>
- NIST/CODATA fundamental constants: <https://physics.nist.gov/constants>
- NIST Handbook 44/133 conversion tables for U.S. customary units

Record the exact source, edition, and definition used for every non-SI or
measured constant. Do not copy unsourced conversion values from a general
reference table.

## Registry and naming rules

- Follow the current unit pattern: a canonical PascalCase name, a readable
  lowercase/camelCase alias, and a conventional symbol only when the symbol is
  unambiguous in Osiris source.
- Preserve case-sensitive symbols. Do not claim that `m`, `M`, `s`, or `S`
  are interchangeable.
- Audit every proposed spelling against keywords, built-ins, module exports,
  types, and existing constants. A new constant must never silently shadow a
  callable or change the meaning of an existing program.
- Do not use reserved `in` as the inch symbol. Do not add ambiguous unqualified
  `ton`, `fluidOunce`, or similar names.
- `Gallon`, `gallon`, and `gal` are defined as the U.S. liquid gallon, exactly
  231 cubic inches. Imperial volume names must be explicitly prefixed.
- Aliases share one canonical semantic definition and must compare equal.
- Keep the public catalog data-driven enough that implementation and
  documentation audits can enumerate the exact same names.

## Minimum unit catalog

Implement at least the following concepts and suitable non-conflicting aliases.
Additional broadly standardized units are welcome when they follow the same
source, exactness, naming, and test requirements.

### Length

- `Inch`/`inch`
- `Foot`/`foot`/`ft`
- `Yard`/`yard`/`yd`
- `Mile`/`mile`/`mi`
- `NauticalMile`/`nauticalMile`/`nmi`
- `Angstrom`/`angstrom`
- `AstronomicalUnit`/`astronomicalUnit`/`au`
- `LightYear`/`lightYear`/`ly`
- `Parsec`/`parsec`/`pc`

Use exact SI definitions where standards make them exact. If a value depends
on an irrational or measured source, preserve the published decimal or
mathematical definition without a host `float64` round trip.

### Area and volume

- `Hectare`/`hectare`/`ha`
- `Acre`/`acre`
- `Milliliter`/`milliliter`/`mL`
- `CubicCentimeter`/`cubicCentimeter`/`cc`
- `Gallon`/`gallon`/`gal` (U.S. liquid)
- `USQuart`/`usQuart`/`qt`
- `USPint`/`usPint`/`pt`
- `USCup`/`usCup`/`cup`
- `USFluidOunce`/`usFluidOunce`/`fl_oz`
- `USTablespoon`/`usTablespoon`/`tbsp`
- `USTeaspoon`/`usTeaspoon`/`tsp`
- `ImperialGallon`/`imperialGallon`/`imp_gal`
- `ImperialQuart`/`imperialQuart`
- `ImperialPint`/`imperialPint`
- `ImperialFluidOunce`/`imperialFluidOunce`

U.S. dry measures are excluded unless they receive explicit `USDry...` names
and sourced definitions.

### Mass

- `Gram`/`gram`/`g`
- `MetricTonne`/`metricTonne`/`tonne`
- `Ounce`/`ounce`/`oz` (avoirdupois)
- `Pound`/`pound`/`lb` (avoirdupois)
- `Stone`/`stone`/`st`
- `ShortTon`/`shortTon`
- `LongTon`/`longTon`
- `Dalton`/`dalton`/`Da`

Do not add an unqualified `Ton`.

### Small time units

- `Nanosecond`/`nanosecond`/`ns`
- `Microsecond`/`microsecond`/`us`
- `Millisecond`/`millisecond`/`ms`
- `JulianYear`/`julianYear`

Coordinate these quantity constants with task 101's qualified `std/time`
duration constants. Do not add month or calendar-year quantities: their length
is context-dependent.

### Speed and frequency

- `Knot`/`knot`/`kn`
- `KilometerPerHour`/`kilometerPerHour`/`kph`
- `MilePerHour`/`milePerHour`/`mph`
- `RevolutionPerMinute`/`revolutionPerMinute`/`rpm`

### Pressure

- `Bar`/`bar`
- `Millibar`/`millibar`/`mbar`
- `Atmosphere`/`atmosphere`/`atm`
- `Torr`/`torr`
- `PoundsPerSquareInch`/`poundsPerSquareInch`/`psi`

### Energy and power

- `Electronvolt`/`electronvolt`/`eV`
- `Calorie`/`calorie`/`cal`, explicitly the thermochemical calorie
- `Kilocalorie`/`kilocalorie`/`kcal`
- `BritishThermalUnit`/`britishThermalUnit`/`BTU`, explicitly the
  International Table definition
- `KilowattHour`/`kilowattHour`/`kWh`
- `MechanicalHorsepower`/`mechanicalHorsepower`/`hp`

### Angles and dimensionless scales

The current quantity model treats plane angle as dimensionless. Until a
separate angle dimension is adopted, expose these as documented numeric scale
constants rather than pretending they are exact rational quantities:

- `Radian`/`radian`/`rad`
- `Degree`/`degree`/`deg`
- `Arcminute`/`arcminute`
- `Arcsecond`/`arcsecond`
- `Revolution`/`revolution`/`turn`

Define each in terms of `Pi` where appropriate and characterize the resulting
default-number precision.

## Minimum mathematical constant catalog

Add default-number constants and clear aliases for:

- `Sqrt2`
- `SqrtHalf`
- `Ln2`
- `Ln10`
- `Log2E`
- `Log10E`
- `EulerGamma`
- `Catalan`
- `Apery`

Document representation and precision. Do not add NaN or infinity constants
without first defining their equality, ordering, serialization, and native
semantics.

## Minimum physical constant catalog

Add the following with a pinned NIST/CODATA edition and the published
precision. Dimensioned values must be quantities:

- vacuum permeability and vacuum permittivity;
- electron, proton, and neutron mass;
- atomic mass constant;
- Faraday constant;
- Stefan–Boltzmann constant;
- Rydberg constant;
- Bohr radius;
- Bohr magneton;
- nuclear magneton.

Keep exact defined constants distinct from measured constants. Documentation
must not imply more precision or immutability across standards editions than
the selected source provides.

## Exactness and value semantics

- Store exact defined conversions as `big.Rat` values. Add a rational-text or
  numerator/denominator factory if necessary; never pass an exact definition
  through binary floating point first.
- Preserve normalized magnitudes and dimension vectors through arithmetic,
  comparisons, inference, codecs, VM execution, and error handling.
- Addition/subtraction continue to require compatible dimensions.
  Multiplication/division must derive the same dimensions regardless of which
  alias was used.
- Affine temperature scales are not multiplicative constants. Do not add
  `Celsius` or `Fahrenheit` values that make expressions such as
  `20 * Celsius` silently wrong. Their conversion model belongs in a separate
  task.
- Update the named-unit design document if implementation discoveries change
  a durable registry or display-unit decision.

## Execution paths

- Register all names in the runtime and make them available in interpreter and
  VM programs with identical values.
- Extend inference so every quantity constant reports its dimension vector and
  every numeric constant reports the correct kind.
- Extend public codecs only as required to preserve the existing quantity
  contract.
- Native C must either lower a constant and its quantity operations with exact
  semantic parity or reject the program at compile time with a clear
  diagnostic. Never evaluate the program during compilation.
- Keep registration and generated documentation ordering deterministic.

## Required documentation

- Add every canonical constant concept to Language → Global Constants with:
  its canonical name and aliases, description, source/definition, return type,
  exact or measured value, dimensions where applicable, and a code example.
- Update the Technical Reference `Global named constants` index to match the
  implementation exactly. Do not replace the detailed page with the index.
- Update search data, executable documentation examples, and backend support
  notes.
- Extend `newdocs/scripts/check-reference.mjs` so missing, stale, duplicate, or
  colliding implementation/documentation names fail validation.

## Required tests

- exact alias equality and inferred-dimension tests for every catalog entry;
- a table-driven registry test comparing implemented names with the detailed
  and technical documentation indexes;
- exact conversion identities, including:
  - `12 * inch == foot`;
  - `3 * foot == yard`;
  - `1760 * yard == mile`;
  - `gallon == 231 * inch * inch * inch`;
  - `ImperialGallon == 4.54609d * liter`;
  - `pound == 0.45359237d * kilogram`;
  - `atmosphere == 101325 * pascal`;
  - `torr == atmosphere / 760`;
  - `knot == NauticalMile / hour`;
  - `KilowattHour == 3600000 * joule`;
- incompatible-dimension failures;
- interpreter/VM parity and codec round trips;
- inference output for representative length, volume, mass, pressure, energy,
  speed, angle, mathematical, and physical constants;
- generated-IR/C assertions, executable native parity for supported quantity
  paths, and compile diagnostics for unsupported paths;
- checked examples and global-search coverage for canonical names and aliases.

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

- The minimum catalog is implemented with sourced, reviewed definitions and no
  name collision or ambiguous customary unit.
- Exact conversions remain exact through interpreter, VM, inference, and
  serialization.
- Native behavior is proven equivalent or rejected at compile time.
- Every public name appears in the unchanged-purpose technical index and every
  canonical concept has a detailed expanded-reference entry with a checked
  example.
- The implementation/documentation audit and full validation pass.

## Non-goals

- No user-defined `unit` declaration syntax in this task.
- No affine Celsius/Fahrenheit conversion, currency, exchange rates, calendar
  months/years, locale-specific measures, or unsourced historical units.
- No automatic choice of display unit after arithmetic.
- No silent compatibility aliases for ambiguous names.

## Completion record — 2026-07-29

Implemented the minimum catalog and retained the existing SI, prefix, physical,
and error-kind surface in one ordered `namedConstantDefinitions` table. The
finished registry contains 159 canonical concepts and 375 public spellings.
Initialization rejects duplicate spellings, registration follows catalog
order, aliases share one factory, and the collision test proves that every
spelling parses as an identifier without shadowing a global callable or type
constructor. Ambiguous `in`, `ton`, and unqualified customary-volume aliases
remain absent.

Exact unit conversions enter quantities through `quantityFactoryText` or
integer/fraction factories, all backed by `big.Rat`; no exact customary or
metric definition passes through `float64`. `SpeedOfLight` and `Avogadro` are
now dimensioned quantities. Plane-angle and mathematical constants are
documented binary64 default numbers. Physical values are pinned to NIST's 2022
CODATA database version 9.0 (May 2024), with exact definitions and measured
concise values distinguished in the reference.

Durable sources and representation rules are recorded in
`docs/design/NAMED-UNITS.md`. The detailed Global Constants page now supplies
each canonical concept's aliases, type/dimensions, definition or measured
value, source, representation, and executable example. The unchanged-purpose
technical index contains the exact 375-name implementation set. The reference
checker extracts the ordered Go catalog and rejects missing, stale, duplicate,
or global-callable-colliding names; search coverage is checked for every alias.

Interpreter and VM use the same registered values. Focused evidence covers
every alias and inferred dimension, every quantity codec round trip, all
required exact identities, incompatible dimensions, representative inference,
documentation/index parity, and checked examples. Native C directly lowers
supported default-number globals and has generated-IR/C plus executable parity
coverage. Exact rational quantities are rejected during compilation with a
specific named-quantity/ABI diagnostic; compilation never evaluates them.

Validation evidence:

```text
PATH=/opt/homebrew/bin:/opt/homebrew/sbin:$PATH GOCACHE=/tmp/osiris-go-cache \
  go test ./internal/osiris -run \
  'TestNamed|TestExpanded|TestNativeNumericGlobalParityAndQuantityDiagnostic|TestCanonicalDocumentationExamples|TestCBackendRejectsUnsupportedIROperation' \
  -count=1
ok github.com/sphireinc/osiris/internal/osiris

PATH=/opt/homebrew/bin:/opt/homebrew/sbin:$PATH GOCACHE=/tmp/osiris-go-cache go test ./...
ok github.com/sphireinc/osiris/cmd/osiris
ok github.com/sphireinc/osiris/internal/osiris

PATH=/opt/homebrew/bin:/opt/homebrew/sbin:$PATH GOCACHE=/tmp/osiris-go-cache go test -race ./...
ok github.com/sphireinc/osiris/cmd/osiris
ok github.com/sphireinc/osiris/internal/osiris

PATH=/opt/homebrew/bin:/opt/homebrew/sbin:$PATH GOCACHE=/tmp/osiris-go-cache go vet ./...
(no output)

nvm use 24
npm run check-reference
Reference checks passed: 17 API groups, 6 chapters, 227 built-ins,
19 special forms, 375 named constants, 17 redirects.

npm run check-examples
ok github.com/sphireinc/osiris/internal/osiris

npm run build
vite build: 49 modules transformed; production build completed.
```
