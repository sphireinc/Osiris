# Native compiler foundation: real C lowering

Source: OPTCHK.md, TYPECHK.txt, and docs/design/NATIVE-COMPILER.md.

**Architecture decision:** Osiris is a general ahead-of-time compiler. C is the
first portable native backend; the system compiler supplies target code
generation and optimization. The current snapshot backend is transitional and
must not remain the implementation of osiris build.

## Current implementation slice

In progress: make the existing IR a valid executable-codegen input before
writing broad C emission. The current backend bypasses IR and calls
Session.ExecProgram; expression-bodied functions currently lower to an empty
return. The first patch sequence is therefore:

1. [x] preserve expression-bodied function return values in IR;
2. [x] define the small generated-C tagged value/runtime layer;
3. [x] emit main-function constants, copies, arithmetic, branches, loops, and
   emit from IR; and
4. [x] add native/interpreter parity tests for scalar control flow before direct function calls or broader
   runtime features.

5. [x] lower direct non-capturing single-block scalar functions and verify them
   through a generated executable.

6. [x] generalize non-capturing direct functions to IR control-flow graphs,
   including local bindings, branches, loops, and returns.

7. [x] lower direct lexical closures through hidden captured-value parameters.

Static direct members of `std/math` now resolve during C lowering for the
implemented native scalar and tensor helpers. Next: broaden module coverage,
add host-backed calls, and handle dynamic module values explicitly.

## Scope

Lower existing IR to readable C for this executable subset:

- literals and local bindings
- bool, integer, f32, and f64 arithmetic
- branches and canonical loops
- direct functions and returns
- emit to runtime stdout

Generated executables must evaluate inputs and control flow at runtime. The
backend may reject unsupported nodes, but it must never run the interpreter to
precompute their outputs.

## Acceptance

- `CompileC` no longer calls `Session.ExecProgram`.
- Generated C contains runtime control flow and values, not an output string
  table.
- Native/interpreter parity tests cover the supported subset and changing
  runtime input changes executable behavior without recompilation.
- Unsupported nodes produce a precise compile-time diagnostic.
- Generated C remains readable enough to inspect in code review.

## Completion record — 2026-07-28

The executable C foundation is complete. `CompileCWithOptions` builds Osiris
IR through `Runtime.BuildIR` and emits that IR with `emitCIRWithOptions`; it
does not call `Session.ExecProgram`. The generated source has an inspectable
tagged runtime plus direct scalar specialization, functions, closures,
branches, loops, and runtime `emit` operations. Unsupported IR is rejected at
compile time with the operation named in the diagnostic.

Validation on 2026-07-28:

```sh
GOCACHE=/private/tmp/osiris-gocache GOMODCACHE=/private/tmp/osiris-gomodcache \
  go test ./internal/osiris -run \
  'TestCBackend(Lowering|DeterministicSource|RejectsUnsupportedIROperation|ControlFlowParity|DirectFunctionParity|DirectClosureParity|FunctionBranchParity|FunctionLoopParity)$' \
  -count=1
```

Passed (`ok github.com/sphireinc/osiris/internal/osiris`, 9.360s). The tests
assert the absence of the historical snapshot output table, deterministic
readable C, executable interpreter/native parity for the supported control-flow
and function subset, and a precise unsupported-IR diagnostic. Follow-on native
coverage belongs to the completed focused tasks 028–033 and the remaining
numeric/task-specific queue, not this foundation record.
