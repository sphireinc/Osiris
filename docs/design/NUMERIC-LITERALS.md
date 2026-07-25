# Numeric literal contract

This document defines the source-literal boundary shared by the lexer, parser,
runtime values, codec, inference report, formatter, and native lowering.

## Defaults

- An unsuffixed integer literal is an unsigned `uint64`: `42` formats and
  infers as `42u64` / `uint64`.
- An unsuffixed fractional or exponent literal is the default `number`
  (f64-backed): `1.25`, `6e3`.
- `_` is a digit separator only. It is removed before numeric conversion:
  `1_000u16`, `0xff_ffu16`.
- Integer bases are decimal, binary (`0b`), octal (`0o`), and hexadecimal
  (`0x`). Bases apply to integral literal families.

## Suffixes

| Source suffix | Value family | Inference spelling |
| --- | --- | --- |
| `u`, `u8`, `u16`, `u32`, `u64` (and wider declared widths) | unsigned integer | `uint64`, `uint8`, ... |
| `i8`, `i16`, `i32`, `i64` (and wider declared widths) | signed integer | `int8`, `int16`, ... |
| `b` | byte / `uint8` | `uint8` |
| `f16`, `f32`, `f64`, ... | IEEE float with source width metadata | `f16`, `f32`, `f64`, ... |
| `bf16`, `bf32`, ... | bfloat with source width metadata | `bf16`, `bf32`, ... |
| `d` | exact decimal rational | `decimal` |
| `i` | imaginary complex component | `complex`, `complex<f32>`, or `complex<f64>` |

Packed unsigned (`uf...`) forms are retained as an implementation-specific
fixed packing family and report `packeduint`; they are not an implicit
promotion target for ordinary unsigned literals.

## Complex component precision

Complex values carry the precision of explicitly typed float components. The
imaginary suffix applies after an f32/f64 suffix, so `1.25f32i` is
`complex<f32>` and `1.25f64i` is `complex<f64>`. The constructor follows the
same rule: `complex(f32(1), f32(2))` creates `complex<f32>`, while any f64 or
default-number participant yields `complex<f64>`. Untyped `2i` retains the
historical default `complex` family.

Complex arithmetic uses the same widening rule. It rounds every f32 component
after construction and after each arithmetic operation; `real` and `imag`
return the matching `f32` or `f64` component family. Width is retained by the
interpreter, VM, inference report, canonical text rendering, and value codec.

## Conversion and errors

Literal syntax never silently changes family. An explicit suffix selects the
runtime kind and remains visible through inference and codec round trips.
Unary `-` is an operator applied after literal parsing, so `-7i16` is a signed
integer expression while `7i16` is the underlying literal.

Malformed digits, unsupported suffixes, floating-point integer annotations,
and malformed decimal text are parser errors with the numeric token's source
location. Overflow/promotion semantics belong to the selected arithmetic
family; they are not decided by reinterpreting a literal at a later stage.

## Native boundary

Native lowering preserves explicit supported widths and rejects unsupported
numeric paths rather than silently widening or executing the interpreter at
compile time. The current C complex ABI is f64/default-complex only:
`complex<f32>` literals receive a compile-time diagnostic until a dedicated
f32 complex ABI exists. See `NATIVE-ABI.md` for the currently lowered subset.
