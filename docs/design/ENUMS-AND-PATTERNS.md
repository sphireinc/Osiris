# Enums and pattern matching

Osiris is adopting PHP-inspired closed enums and an expression-valued,
strictly matched `match` / `switch` form with C#-inspired structural patterns.
The normative implementation plan is
[`tasks/018-enums-and-pattern-matching.md`](../../tasks/018-enums-and-pattern-matching.md).

The present implementation supports `match` and `switch` expressions with
literal, binding, wildcard, fixed-length list patterns, and qualified enum
patterns. A top-level declaration such as
`enum Result { Ok(value), Err(message) }` creates a closed nominal namespace:
zero-payload cases are values (`Status.Ready`) and payload cases are checked
constructors (`Result.Ok(value)`). Both execute in the interpreter and VM,
may be selected by nested patterns, and compare by nominal type, case, and
payload. Duplicate and empty declarations are rejected at parse time.

This is an implementation checkpoint, not the completed enum feature. It
supports final `default` arms, `when` guards, and `|` grouped alternatives:
a guard runs only after its pattern binds successfully, a false guard proceeds
to the next arm, bindings do not escape the arm, and every alternative must
bind the same names. It also supports relational, type-binding, conjunction,
and negation patterns. It does not yet provide broader structural patterns,
full compile-time exhaustiveness diagnostics, or native tagged-value lowering.
For statically resolvable enum subjects, omitted cases and redundant arms are
source-located diagnostics; dynamic subjects retain the runtime unmatched-arm
error when no arm applies.

The central semantic choices are:

- enums are closed nominal types; enum equality is never primitive coercion;
- match evaluates its subject once, has no fall-through, and evaluates only
  the selected arm;
- literal alternatives are strict and ordered; `default` is the sole final
  catch-all spelling;
- pattern bindings live only in their selected arm;
- native C lowering uses tagged enum values and direct dispatch where proven,
  with full tagged-runtime fallback rather than interpreter execution.

## Merge gate

No additional enum syntax may merge independently of its value representation,
formatter, interpreter, VM, LSP, exhaustiveness diagnostics, and native
behavior. The completed enum release must define source spans for
duplicate/unreachable arms,
non-exhaustive closed-enum matches, illegal binding reuse, and invalid guards.
Guards run only after the structural pattern succeeds; a failed guard proceeds
to the next arm without leaking bindings.

Native lowering must either support every accepted enum/match form through its
tagged ABI (including owned payload cleanup) or reject that program during
native compilation with a stable unsupported-lowering diagnostic. It must
never evaluate the interpreter to construct a native result. Formatter
round-trip and interpreter/VM/native corpus parity are release gates.

The existing match/list-pattern surface, payload enum checkpoint, guards,
final default arms, and grouped alternatives remain supported as documented.
Relational patterns (`<`, `<=`, `>`, `>=`), type-and-binding patterns such as
`number n`, conjunction (`and`), and negation (`not`) are supported. Negated
patterns cannot bind names. Broader C#-style structural patterns remain
planned—not public language features—until this gate is met.
