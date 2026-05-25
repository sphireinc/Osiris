function widthFamily(prefix, widths, label, summary) {
  return {
    kind: "family",
    name: `${prefix}N`,
    syntax: `${prefix}N(value)`,
    returns: label,
    summary,
    details: [
      `This family includes ${widths.join(", ")}.`,
      "Osiris keeps the width explicit so the compiler, encoder, and runtime can preserve it through analysis and native lowering."
    ],
    members: widths,
    example: `${widths[widths.length - 1]}(42)`
  };
}

window.OSIRIS_MANUAL = {
  title: "Osiris Reference",
  subtitle: "Language semantics, built-in functions, modules, and native interop.",
  sections: [
    {
      id: "overview",
      eyebrow: "Preface",
      title: "What Osiris is",
      summary: "Osiris is a deterministic, inspectable language for exact math, text transforms, data pipelines, module loading, and native code generation.",
      paragraphs: [
        "Osiris keeps the core language small while giving the runtime enough structure to understand modules, exports, numeric exactness, tensors, and the build pipeline.",
        "The language is designed to be readable, testable, and useful for math-heavy work without turning every program into a framework.",
        "When the language needs to grow, it grows around the visible pipeline: input, transform, inspect, and output."
      ],
      cards: [
        {
          title: "Guiding rule",
          text: "Prefer explicit data flow, explicit module boundaries, and explicit numeric intent."
        },
        {
          title: "Default numeric model",
          text: "The default literal type remains number, while specialized constructors handle exact, wide, packed, tensor, and fixed-precision forms."
        },
        {
          title: "Control flow",
          text: "if, while, for, match, switch, try/catch, return, break, and continue make the core language expressive without hiding the flow of computation."
        },
        {
          title: "Native path",
          text: "The standard performance path is build-time lowering through C and native compilation, while interpreter and VM flows remain available."
        }
      ]
    },
    {
      id: "semantics",
      eyebrow: "Chapter",
      title: "Language semantics",
      summary: "Programs are statement lists with lexical scope, explicit imports, exact math where possible, and controlled output through emit.",
      paragraphs: [
        "A file is normally a list of statements separated by newlines or semicolons. The usual shape is package, export, import, macro, let, and emit, but that order is a convention rather than a law.",
        "Values are inspected through the runtime rather than hidden behind magic. The runtime preserves exactness, width metadata, tensor shape, module identity, and other structural information wherever it can.",
        "Imports are alias-based and version-aware. File modules, std modules, and local module graphs are resolved explicitly, and the loader records manifests for repeatable builds.",
        "The language is synchronous. Statements execute in source order, emit appends values to the output stream in the order they are produced, and file I/O calls such as read, readbytes, write, csv, jsonparse, and lines block the current execution until the host finishes the operation."
      ],
      cards: [
        {
          title: "Scoping",
          text: "let introduces lexical bindings. Shadowing is allowed, and loop-local state is carried forward by the execution model when needed."
        },
        {
          title: "Control flow",
          text: "if, while, for, return, break, continue, match, switch, and try/catch are the visible control-flow tools."
        },
        {
          title: "Exactness",
          text: "Exact operands stay exact where possible, including rational division and wide integer exponentiation."
        },
        {
          title: "Modules",
          text: "Module imports are explicit, version selectors are supported, and exported names are the public contract."
        },
        {
          title: "Output stream",
          text: "emit writes values into the program output stream in execution order, which makes the observable result of a script easy to reason about and easy to test."
        },
        {
          title: "Error propagation",
          text: "Use try/catch for explicit handling and the postfix ? operator for early propagation from fallible expressions when you want the current function to return the failure directly."
        },
        {
          title: "Blocking I/O",
          text: "I/O is currently synchronous and host-backed. read, write, readbytes, csv, jsonparse, and lines all run as blocking operations on the current thread of execution."
        },
        {
          title: "Memory management",
          text: "Osiris values are managed by the host runtime. Source programs do not allocate or free memory manually, and the runtime leans on Go-managed storage for temporary objects, module state, and evaluation artifacts."
        },
        {
          title: "Garbage collection",
          text: "There is no user-facing garbage collector to tune. Lifetime is governed by the host runtime's collector and the shape of the current evaluation graph, so programmers focus on data flow instead of manual reclamation."
        }
      ]
    },
    {
      id: "errors",
      eyebrow: "Chapter",
      title: "Error model",
      summary: "Errors are structured values with stable kinds, public metadata, optional causes, and redaction rules that keep sensitive data out of normal output.",
      paragraphs: [
        "Osiris treats recoverable failures as structured error records instead of plain strings. The runtime preserves the error kind, message, source location, stack frames, and public metadata whenever those details are available.",
        "The public surface is intentionally small and machine-readable. The standard helpers let code create, wrap, inspect, redact, and serialize errors without having to parse error text.",
        "Errors are immutable records. When a program adds context, it gets a new record back instead of mutating the original failure. That keeps error propagation predictable across function boundaries, tasks, and native wrappers."
      ],
      cards: [
        {
          title: "Base model",
          text: "An Osiris error is a structured runtime record with a stable kind/code, a user-facing message, and optional cause, source, stack, metadata, and severity fields. The record is immutable, comparable by structure, and safe to serialize."
        },
        {
          title: "Taxonomy",
          text: "The standard kinds stay intentionally small: InvalidArgument, NotFound, AlreadyExists, PermissionDenied, Unauthenticated, Unavailable, Timeout, Cancelled, Conflict, ResourceExhausted, OutOfMemory, OutOfRange, Unsupported, NotImplemented, ParseError, TypeError, EncodingError, IOError, NetworkError, DatabaseError, InternalError, and UnknownError."
        },
        {
          title: "Metadata",
          text: "Public metadata covers operation, resource, retryability, severity, user_message, correlation_id, component, remediation, source, location, timestamp, and any extra public key/value pairs. Debug metadata is kept separate so sensitive details do not leak into ordinary output."
        },
        {
          title: "Debug data",
          text: "Debug-only metadata is stored separately so the normal display path and serialized form do not leak sensitive details unless the caller explicitly asks for them. The `redact` helper strips stack details and debug-only state while preserving the public cause chain."
        },
        {
          title: "Helpers",
          text: "Use error, wrap, cause, unwrap, withmeta, withdebug, errmeta, and redact to create, inspect, and sanitize error records. `matches` and `is` are the standard kind checks, while `try/catch` handles recoverable failures explicitly."
        },
        {
          title: "Result and option",
          text: "Result values use ok and err constructors. Option values use some and none. unwrap extracts payloads, expect attaches a user-facing failure, or_else supplies a fallback value, and the postfix ? operator propagates a recoverable failure to the current function boundary."
        },
        {
          title: "Fatal errors",
          text: "panic, trap, and assert raise fatal structured errors. They are treated as unrecoverable, bypass ordinary catch handling, and are intended for impossible states rather than expected branching."
        },
        {
          title: "Boundaries",
          text: "Parser errors, runtime errors, task errors, and native host errors are normalized into structured Osiris errors before they leave the runtime. The same record model is preserved across task boundaries and native wrappers so logging and redaction remain consistent."
        }
      ]
    },
    {
      id: "native",
      eyebrow: "Chapter",
      title: "Native interop and build flow",
      summary: "Osiris builds through a native pipeline and can expose host-backed modules explicitly instead of pretending every foreign package is a dynamic script import.",
      paragraphs: [
        "The native build path lowers Osiris to C and then compiles that C with the system compiler, which keeps performance and portability practical.",
        "C libraries are the natural first native interop target because their ABI model is stable enough for explicit wrappers and build-time linking.",
        "Go-backed std modules already fit the same wrapper pattern: the host exposes a module, and Osiris imports that module by name."
      ],
      cards: [
        {
          title: "C libraries",
          text: "Use explicit wrappers and build-time linking for native libraries such as libm, libc-style helpers, codecs, databases, or math backends."
        },
        {
          title: "Go host modules",
          text: "Wrap Go code into Osiris-shaped modules instead of trying to dynamically import arbitrary Go packages."
        }
      ]
    }
  ],
  types: [
    { name: "number", summary: "Default numeric literal type." },
    { name: "bool", summary: "Boolean truth values." },
    { name: "byte", summary: "8-bit byte value." },
    { name: "decimal", summary: "Exact decimal-backed numeric type." },
    { name: "rational", summary: "Exact rational numeric form." },
    { name: "rat", summary: "Alias-shaped exact rational numeric form." },
    { name: "complex", summary: "Complex numeric type with real and imaginary parts." },
    { name: "bigint", summary: "Arbitrary-precision integer." },
    { name: "bigfloat", summary: "Arbitrary-precision floating-point number." },
    { name: "quantity", summary: "Unit-aware numeric value." },
    { name: "interval", summary: "Exact interval with lower and upper bounds." },
    { name: "fixed", summary: "Signed fixed-point numeric value." },
    { name: "modint", summary: "Modular integer type." },
    { name: "result", summary: "Structured success-or-failure value.", members: ["ok", "err"] },
    { name: "option", summary: "Structured optional value.", members: ["some", "none"] },
    { name: "f32", summary: "32-bit explicit float." },
    { name: "f64", summary: "64-bit explicit float." },
    { name: "bf", summary: "Bfloat-style low-precision float family.", members: ["bf8", "bf16", "bf32", "bf64", "bf128", "bf256", "bf512", "bf1024"] },
    { name: "i", summary: "Signed fixed-width integer family.", members: ["i8", "i16", "i32", "i64", "i128", "i256", "i512", "i1024"] },
    { name: "u", summary: "Unsigned fixed-width integer family.", members: ["u8", "u16", "u32", "u64", "u128", "u256", "u512", "u1024"] },
    { name: "uf", summary: "Packed unsigned numeric family.", members: ["uf8", "uf16", "uf32", "ufnum", "uf64", "uf128", "uf256", "uf512", "uf1024"] },
    { name: "tensor", summary: "Dense, shape-aware tensor value." },
    { name: "matrix", summary: "Dense matrix value with shape metadata." },
    { name: "vector", summary: "One-dimensional tensor alias." },
    { name: "simd", summary: "Packed vector type for vectorized lowering." },
    { name: "quat", summary: "Quaternion type for 3D rotation math." },
    { name: "dual", summary: "Dual number type for automatic differentiation." },
    { name: "sparse_matrix", summary: "Sparse matrix representation." },
    { name: "sparse_tensor", summary: "Sparse tensor representation." },
    { name: "poly", summary: "Polynomial value type." }
  ],
  groups: [
    {
      id: "core",
      title: "Core language constructs",
      summary: "The syntax that shapes programs, modules, statements, and control flow.",
      items: [
        {
          kind: "construct",
          name: "package",
          syntax: "package name [version]",
          returns: "module metadata",
          summary: "Declare the identity of a file module.",
          details: [
            "Package declarations belong at the top of files that are meant to be imported.",
            "The optional version becomes part of the module contract and can be selected by import selectors."
          ],
          example: "package sigil 1.0.0"
        },
        {
          kind: "construct",
          name: "export",
          syntax: "export name, other",
          returns: "export list",
          summary: "Expose only the listed bindings from a file module.",
          details: [
            "Exports are explicit so imported modules have a stable public surface.",
            "Everything else remains internal to the module unless exported."
          ],
          example: "export visible, answer"
        },
        {
          kind: "construct",
          name: "import",
          syntax: "import \"./local.osr\" as local",
          returns: "module value",
          summary: "Load a file module or a std module and bind it to an alias.",
          details: [
            "Imports are always aliased.",
            "Version selectors are supported for both local and std modules.",
            "Std modules are backed by the host runtime, while file modules are parsed and executed as Osiris source."
          ],
          example: "import \"std/text@1\" as text"
        },
        {
          kind: "construct",
          name: "let",
          syntax: "let name = expression",
          returns: "binding",
          summary: "Introduce a lexical binding.",
          details: [
            "Bindings are scoped to the surrounding block or file.",
            "Shadowing is allowed and is used deliberately throughout the language."
          ],
          example: "let seed = \"osiris::demo\""
        },
        {
          kind: "construct",
          name: "func",
          syntax: "func name(params) { ... }",
          returns: "function value",
          summary: "Define a user function with lexical capture and recursion support.",
          details: [
            "Functions can be declared in block or expression form depending on the surrounding syntax.",
            "Captured variables are resolved through lexical scope, not dynamic lookup."
          ],
          example: "func add1(x) => x + 1"
        },
        {
          kind: "construct",
          name: "macro",
          syntax: "macro name(args) => expression",
          returns: "expanded syntax",
          summary: "Define a compile-time macro.",
          details: [
            "Macros expand before normal execution and are guarded by an expansion budget.",
            "This keeps accidental infinite expansion from hanging the compiler and tooling."
          ],
          example: "macro shout(x) => cat(x, \"!\")"
        },
        {
          kind: "construct",
          name: "emit",
          syntax: "emit expression",
          returns: "emitted output",
          summary: "Append a value to the program output stream.",
          details: [
            "Emit is the visible output primitive used by scripts and tests.",
            "It is designed to keep data flow obvious rather than hidden behind implicit printing."
          ],
          example: "emit text.upper(\"osiris\")"
        },
        {
          kind: "construct",
          name: "if",
          syntax: "if condition { ... } else { ... }",
          returns: "branch result",
          summary: "Select a branch based on a truthy or falsey condition.",
          details: [
            "Conditional expressions are part of the core language, not a library trick.",
            "The false branch is optional."
          ],
          example: "if x > 0 { emit x } else { emit 0 }"
        },
        {
          kind: "construct",
          name: "while",
          syntax: "while condition { ... }",
          returns: "loop result",
          summary: "Repeat a block while a condition remains true.",
          details: [
            "The loop body can carry state forward explicitly through bindings.",
            "Use while when the stop condition is naturally predicate-based."
          ],
          example: "while i < n { let i = i + 1 }"
        },
        {
          kind: "construct",
          name: "for",
          syntax: "for name in iterable { ... }",
          returns: "loop result",
          summary: "Iterate over a sequence or range with a named loop variable.",
          details: [
            "For loops are best for ordered iteration over ranges, lists, and other sequence-like values."
          ],
          example: "for x in range(0, 10) { emit x }"
        },
        {
          kind: "construct",
          name: "return",
          syntax: "return expression",
          returns: "function result",
          summary: "Exit a function early and yield a result.",
          details: [
            "Return is only meaningful inside function bodies.",
            "It keeps recursive and branching code readable."
          ],
          example: "return total"
        },
        {
          kind: "construct",
          name: "break",
          syntax: "break",
          returns: "loop exit",
          summary: "Exit the nearest loop.",
          details: [
            "Break is used for early loop termination when the stopping condition is data-dependent."
          ],
          example: "break"
        },
        {
          kind: "construct",
          name: "continue",
          syntax: "continue",
          returns: "loop control",
          summary: "Skip to the next loop iteration.",
          details: [
            "Continue is useful when the loop body contains filtering or guard logic."
          ],
          example: "continue"
        },
        {
          kind: "construct",
          name: "try/catch",
          syntax: "try { ... } catch err { ... }",
          returns: "handled value or fallback",
          summary: "Catch runtime failures explicitly.",
          details: [
            "The catch binding receives the error value.",
            "This keeps failures visible and local to the source that handles them."
          ],
          example: "try { uint(\"7\") } catch err { emit err }"
        },
        {
          kind: "construct",
          name: "match / switch",
          syntax: "match value { ... } / switch value { ... }",
          returns: "branch result",
          summary: "Choose a branch by pattern or equivalent switch-style matching.",
          details: [
            "switch is an alias-shaped branch table over match semantics.",
            "It exists to make multi-branch code easier to read."
          ],
          example: "switch n { 0 => \"zero\"; _ => \"other\" }"
        }
      ]
    },
    {
      id: "text",
      title: "std/text",
      summary: "String transforms, normalization, and lightweight text shaping.",
      items: [
        {
          name: "trim",
          syntax: "trim(text)",
          returns: "text",
          summary: "Remove surrounding whitespace from a string.",
          details: ["Useful before comparisons, tokenization, or slug generation."],
          example: "emit text.trim(\"  osiris  \")"
        },
        {
          name: "upper",
          syntax: "upper(text)",
          returns: "text",
          summary: "Convert a string to uppercase.",
          details: ["Good for normalization and presentation output."],
          example: "emit text.upper(\"sigil\")"
        },
        {
          name: "lower",
          syntax: "lower(text)",
          returns: "text",
          summary: "Convert a string to lowercase.",
          details: ["Good for canonicalization and case folding."],
          example: "emit text.lower(\"SIGIL\")"
        },
        {
          name: "replace",
          syntax: "replace(text, old, new)",
          returns: "text",
          summary: "Replace all occurrences of one substring with another.",
          details: ["Use it for simple normalization and templated rewriting."],
          example: "emit text.replace(\"a-b-c\", \"-\", \":\")"
        },
        {
          name: "repeat",
          syntax: "repeat(text, count)",
          returns: "text",
          summary: "Repeat a string a fixed number of times.",
          details: ["Useful for separators, banners, and test fixtures."],
          example: "emit text.repeat(\"=\", 24)"
        },
        {
          name: "split",
          syntax: "split(text, sep)",
          returns: "list",
          summary: "Split text into parts around a delimiter.",
          details: ["Splitting preserves the text/data boundary and returns Osiris values."],
          example: "emit text.split(\"a,b,c\", \",\")"
        },
        {
          name: "join",
          syntax: "join(parts, sep)",
          returns: "text",
          summary: "Join a list of strings with a separator.",
          details: ["This is the inverse of split for common text pipelines."],
          example: "emit text.join([\"a\", \"b\"], \",\")"
        },
        {
          name: "chunks",
          syntax: "chunks(text, size)",
          returns: "list",
          summary: "Break text into fixed-width pieces.",
          details: ["Useful for wrapping, formatting, and byte-oriented inspection."],
          example: "emit text.chunks(\"abcdef\", 2)"
        },
        {
          name: "slug",
          syntax: "slug(text)",
          returns: "text",
          summary: "Turn text into a predictable slug form.",
          details: ["Useful for filenames, stable identifiers, and presentation-safe keys."],
          example: "emit text.slug(\"Hello, World\")"
        },
        {
          name: "cat",
          syntax: "cat(parts...)",
          returns: "text",
          summary: "Concatenate values as text.",
          details: ["This is the flexible text joiner used heavily in reference examples."],
          example: "emit text.cat(\"osiris\", \"::\", \"demo\")"
        }
      ]
    },
    {
      id: "math",
      title: "std/math",
      summary: "Exact arithmetic, numeric constructors, transcendental functions, tensors, and linear algebra.",
      items: [
        widthFamily("i", ["i8", "i16", "i32", "i64", "i128", "i256", "i512", "i1024"], "signed integer", "Construct an explicit signed fixed-width integer."),
        widthFamily("u", ["u8", "u16", "u32", "u64", "u128", "u256", "u512", "u1024"], "unsigned integer", "Construct an explicit unsigned fixed-width integer."),
        widthFamily("uf", ["uf8", "uf16", "uf32", "ufnum", "uf64", "uf128", "uf256", "uf512", "uf1024"], "packed unsigned", "Construct a packed unsigned numeric form for dense layouts."),
        widthFamily("bf", ["bf8", "bf16", "bf32", "bf64", "bf128", "bf256", "bf512", "bf1024"], "bfloat-style float", "Construct a low-precision or bfloat-style floating-point value."),
        {
          name: "f32",
          syntax: "f32(value)",
          returns: "float",
          summary: "Construct a 32-bit explicit float.",
          details: [
            "number remains the default literal type, but explicit float widths are useful when precision choice matters.",
            "Use this when you want hardware-friendly single-precision semantics."
          ],
          example: "emit f32(3.5)"
        },
        {
          name: "f64",
          syntax: "f64(value)",
          returns: "float",
          summary: "Construct a 64-bit explicit float.",
          details: [
            "number remains the default literal type, but explicit float widths are useful when precision choice matters.",
            "Use this when you want the familiar double-precision path."
          ],
          example: "emit f64(3.5)"
        },
        {
          name: "decimal",
          syntax: "decimal(value)",
          returns: "exact decimal",
          summary: "Construct an exact decimal value.",
          details: ["Use this for decimal math that should stay exact instead of collapsing into approximate float math."],
          example: "emit decimal(1) / 3"
        },
        {
          name: "rat",
          syntax: "rat(value)",
          returns: "exact rational",
          summary: "Construct an exact rational value.",
          details: ["Use this as the alias-shaped rational constructor when fraction form matters."],
          example: "emit rat(7, 25) * 25"
        },
        {
          name: "rational",
          syntax: "rational(value)",
          returns: "exact rational",
          summary: "Construct an exact rational value.",
          details: [
            "This is the explicit rational constructor for fraction-preserving math.",
            "It keeps arithmetic exact where possible instead of collapsing into approximate float math."
          ],
          example: "emit rational(7, 25) * 25"
        },
        {
          name: "bigint",
          syntax: "bigint(value)",
          returns: "arbitrary-precision integer",
          summary: "Construct an exact integer value with unbounded width.",
          details: ["Useful for combinatorics, cryptography, and large exact powers."],
          example: "emit bigint(\"999999999999\") ^ 279"
        },
        {
          name: "bigfloat",
          syntax: "bigfloat(value)",
          returns: "arbitrary-precision float",
          summary: "Construct a high-precision floating-point value.",
          details: ["Useful when float64 is not enough and exact or controlled precision matters."],
          example: "emit bigfloat(\"1.0000000000000000001\")"
        },
        {
          name: "interval",
          syntax: "interval(lower, upper)",
          returns: "interval",
          summary: "Construct an interval with exact lower and upper bounds.",
          details: ["Useful for bounded numeric reasoning and conservative error tracking."],
          example: "emit interval(1, 2)"
        },
        {
          name: "fixed",
          syntax: "fixed(width, frac, value)",
          returns: "fixed-point value",
          summary: "Construct a signed fixed-point number.",
          details: ["Good for DSP, finance, and deterministic numeric pipelines."],
          example: "emit fixed(32, 16, 1.5)"
        },
        {
          name: "modint",
          syntax: "modint(modulus, value)",
          returns: "modular integer",
          summary: "Construct an integer in modular arithmetic.",
          details: ["Useful for number theory, hashing, and finite-field style work."],
          example: "emit modint(17, 5) + modint(17, 7)"
        },
        {
          name: "complex",
          syntax: "complex(real, imag)",
          returns: "complex number",
          summary: "Construct a complex value from real and imaginary parts.",
          details: ["Complex arithmetic is available natively and is preserved through the runtime and analysis pipeline."],
          example: "emit complex(3, 4)"
        },
        {
          name: "real",
          syntax: "real(z)",
          returns: "numeric scalar",
          summary: "Read the real part of a complex value.",
          details: ["Use this when you need the real projection of a complex result."],
          example: "emit real(complex(3, 4))"
        },
        {
          name: "imag",
          syntax: "imag(z)",
          returns: "numeric scalar",
          summary: "Read the imaginary part of a complex value.",
          details: ["Use this when you need the imaginary projection of a complex result."],
          example: "emit imag(complex(3, 4))"
        },
        {
          name: "magnitude",
          syntax: "magnitude(z)",
          returns: "numeric scalar",
          summary: "Read the magnitude of a complex value.",
          details: ["This is the radial length of the complex plane representation."],
          example: "emit magnitude(complex(3, 4))"
        },
        {
          name: "abs",
          syntax: "abs(x)",
          returns: "numeric scalar",
          summary: "Return the absolute value of a number.",
          details: ["Use this for signed magnitude and distance-like calculations."],
          example: "emit abs(-3)"
        },
        {
          name: "min",
          syntax: "min(a, b)",
          returns: "numeric scalar",
          summary: "Choose the smaller of two values.",
          details: ["Useful anywhere you need a lower bound selection."],
          example: "emit min(3, 7)"
        },
        {
          name: "max",
          syntax: "max(a, b)",
          returns: "numeric scalar",
          summary: "Choose the larger of two values.",
          details: ["Useful anywhere you need an upper bound selection."],
          example: "emit max(3, 7)"
        },
        {
          name: "clamp",
          syntax: "clamp(x, lo, hi)",
          returns: "numeric scalar",
          summary: "Constrain a value to a closed range.",
          details: ["This is the common bounded numeric primitive."],
          example: "emit clamp(-3, 0, 10)"
        },
        {
          name: "floor",
          syntax: "floor(x)",
          returns: "numeric scalar",
          summary: "Round down toward negative infinity.",
          details: ["Use this for binning and lower-bound discretization."],
          example: "emit floor(3.9)"
        },
        {
          name: "ceil",
          syntax: "ceil(x)",
          returns: "numeric scalar",
          summary: "Round up toward positive infinity.",
          details: ["Use this for upper-bound discretization and allocation math."],
          example: "emit ceil(3.1)"
        },
        {
          name: "round",
          syntax: "round(x)",
          returns: "numeric scalar",
          summary: "Round to the nearest integer-like value.",
          details: ["Useful for formatting, discretization, and centered rounding."],
          example: "emit round(3.5)"
        },
        {
          name: "sign",
          syntax: "sign(x)",
          returns: "numeric scalar",
          summary: "Return the sign of a value.",
          details: ["This yields a directional scalar useful in piecewise math."],
          example: "emit sign(-12)"
        },
        {
          name: "step",
          syntax: "step(x)",
          returns: "numeric scalar",
          summary: "Return a step-function style threshold value.",
          details: ["Useful in control flow and activation-style transforms."],
          example: "emit step(-12)"
        },
        {
          name: "heaviside",
          syntax: "heaviside(x)",
          returns: "numeric scalar",
          summary: "Return the Heaviside threshold value.",
          details: ["This is a conventional piecewise helper in scientific code."],
          example: "emit heaviside(-12)"
        },
        {
          name: "relu",
          syntax: "relu(x)",
          returns: "numeric scalar",
          summary: "Apply a rectified-linear transform.",
          details: ["This is useful in machine-learning-style numeric code."],
          example: "emit relu(-2)"
        },
        {
          name: "sigmoid",
          syntax: "sigmoid(x)",
          returns: "numeric scalar",
          summary: "Apply a sigmoid transform.",
          details: ["Useful for smooth activations and bounded probabilistic transforms."],
          example: "emit sigmoid(1)"
        },
        {
          name: "softplus",
          syntax: "softplus(x)",
          returns: "numeric scalar",
          summary: "Apply a softplus transform.",
          details: ["This is a smooth, positive activation-style transform."],
          example: "emit softplus(1)"
        },
        {
          name: "sin",
          syntax: "sin(x)",
          returns: "numeric scalar",
          summary: "Compute the sine of an angle.",
          details: ["The runtime exposes the usual transcendental math surface directly."],
          example: "emit sin(0.5)"
        },
        {
          name: "cos",
          syntax: "cos(x)",
          returns: "numeric scalar",
          summary: "Compute the cosine of an angle.",
          details: ["Useful for ordinary angle math and projection work."],
          example: "emit cos(0.5)"
        },
        {
          name: "tan",
          syntax: "tan(x)",
          returns: "numeric scalar",
          summary: "Compute the tangent of an angle.",
          details: ["Useful in geometry, control, and signal processing."],
          example: "emit tan(0.5)"
        },
        {
          name: "asin",
          syntax: "asin(x)",
          returns: "numeric scalar",
          summary: "Compute the inverse sine.",
          details: ["Useful for geometry and angle reconstruction."],
          example: "emit asin(0.5)"
        },
        {
          name: "acos",
          syntax: "acos(x)",
          returns: "numeric scalar",
          summary: "Compute the inverse cosine.",
          details: ["Useful for geometry and angle reconstruction."],
          example: "emit acos(0.5)"
        },
        {
          name: "atan",
          syntax: "atan(x)",
          returns: "numeric scalar",
          summary: "Compute the inverse tangent.",
          details: ["Useful for geometry, coordinate transforms, and signal work."],
          example: "emit atan(1)"
        },
        {
          name: "sinh",
          syntax: "sinh(x)",
          returns: "numeric scalar",
          summary: "Compute the hyperbolic sine.",
          details: ["Useful for geometry, transforms, and numerical analysis."],
          example: "emit sinh(0.5)"
        },
        {
          name: "cosh",
          syntax: "cosh(x)",
          returns: "numeric scalar",
          summary: "Compute the hyperbolic cosine.",
          details: ["Useful for geometry, transforms, and numerical analysis."],
          example: "emit cosh(0.5)"
        },
        {
          name: "tanh",
          syntax: "tanh(x)",
          returns: "numeric scalar",
          summary: "Compute the hyperbolic tangent.",
          details: ["Useful for geometry, transforms, and numerical analysis."],
          example: "emit tanh(0.5)"
        },
        {
          name: "asinh",
          syntax: "asinh(x)",
          returns: "numeric scalar",
          summary: "Compute the inverse hyperbolic sine.",
          details: ["This completes the hyperbolic math family."],
          example: "emit asinh(1)"
        },
        {
          name: "acosh",
          syntax: "acosh(x)",
          returns: "numeric scalar",
          summary: "Compute the inverse hyperbolic cosine.",
          details: ["This completes the hyperbolic math family."],
          example: "emit acosh(2)"
        },
        {
          name: "atanh",
          syntax: "atanh(x)",
          returns: "numeric scalar",
          summary: "Compute the inverse hyperbolic tangent.",
          details: ["This completes the hyperbolic math family."],
          example: "emit atanh(0.5)"
        },
        {
          name: "exp",
          syntax: "exp(x)",
          returns: "numeric scalar",
          summary: "Compute the natural exponential.",
          details: ["Use this for growth, decay, and continuous transforms."],
          example: "emit exp(1)"
        },
        {
          name: "expm1",
          syntax: "expm1(x)",
          returns: "numeric scalar",
          summary: "Compute exp(x) - 1 with better precision near zero.",
          details: ["This keeps small-value precision intact."],
          example: "emit expm1(1)"
        },
        {
          name: "pow",
          syntax: "pow(x, y)",
          returns: "numeric scalar",
          summary: "Raise a value to a power.",
          details: ["Exact integer inputs use the exact integer path where possible."],
          example: "emit pow(2, 10)"
        },
        {
          name: "expt",
          syntax: "expt(x, y)",
          returns: "numeric scalar",
          summary: "Alias-shaped power helper for exact integer style code.",
          details: ["This exists for users who prefer the expt spelling in exact-math contexts."],
          example: "emit expt(2, 10)"
        },
        {
          name: "log",
          syntax: "log(x)",
          returns: "numeric scalar",
          summary: "Compute the natural logarithm.",
          details: ["Use this for logarithmic scaling and inverse exponential work."],
          example: "emit log(8)"
        },
        {
          name: "log10",
          syntax: "log10(x)",
          returns: "numeric scalar",
          summary: "Compute the base-10 logarithm.",
          details: ["Useful when decimal decades are the meaningful scale."],
          example: "emit log10(1000)"
        },
        {
          name: "log2",
          syntax: "log2(x)",
          returns: "numeric scalar",
          summary: "Compute the base-2 logarithm.",
          details: ["Useful for binary scale, bit-width reasoning, and powers-of-two analysis."],
          example: "emit log2(8)"
        },
        {
          name: "log1p",
          syntax: "log1p(x)",
          returns: "numeric scalar",
          summary: "Compute log(1 + x) with better precision near zero.",
          details: ["This is the precision-friendly logarithm variant."],
          example: "emit log1p(0.01)"
        },
        {
          name: "sqrt",
          syntax: "sqrt(x)",
          returns: "numeric scalar",
          summary: "Compute the square root.",
          details: ["This is the standard root function and is used throughout scientific code."],
          example: "emit sqrt(9)"
        },
        {
          name: "cbrt",
          syntax: "cbrt(x)",
          returns: "numeric scalar",
          summary: "Compute the cube root.",
          details: ["Useful for geometric and scaling transforms."],
          example: "emit cbrt(27)"
        },
        {
          name: "hypot",
          syntax: "hypot(a, b)",
          returns: "numeric scalar",
          summary: "Compute the Euclidean length of a 2D vector.",
          details: ["This avoids manual overflow-prone squaring patterns."],
          example: "emit hypot(3, 4)"
        },
        {
          name: "norm",
          syntax: "norm(value)",
          returns: "numeric scalar",
          summary: "Compute a magnitude-style norm.",
          details: ["Useful for vector and tensor-style magnitude checks."],
          example: "emit norm(vec(3, 4))"
        },
        {
          name: "erf",
          syntax: "erf(x)",
          returns: "numeric scalar",
          summary: "Compute the error function.",
          details: [
            "The error function erf(x) is the normalized integral of a Gaussian curve from 0 to x.",
            "In standard form, erf(x) = 2 / sqrt(pi) * integral from 0 to x of e^(-t^2) dt.",
            "It appears in probability theory, diffusion models, heat flow, statistics, and Gaussian tail calculations."
          ],
          example: "emit erf(1)"
        },
        {
          name: "erfc",
          syntax: "erfc(x)",
          returns: "numeric scalar",
          summary: "Compute the complementary error function.",
          details: [
            "The complementary error function erfc(x) is defined as 1 - erf(x).",
            "It is written as erfc(x) = 2 / sqrt(pi) * integral from x to infinity of e^(-t^2) dt.",
            "It is useful when a problem is expressed in terms of the tail area of a Gaussian distribution instead of the central mass."
          ],
          example: "emit erfc(1)"
        },
        {
          name: "gamma",
          syntax: "gamma(x)",
          returns: "numeric scalar",
          summary: "Compute the gamma function.",
          details: [
            "The gamma function extends the factorial to non-integer values.",
            "For positive integers n, gamma(n) = (n - 1)! and for half-integers it connects to square roots of pi.",
            "It is used throughout combinatorics, analytic continuation, statistical distributions, and special-function identities."
          ],
          example: "emit gamma(5)"
        },
        {
          name: "lgamma",
          syntax: "lgamma(x)",
          returns: "numeric scalar",
          summary: "Compute the logarithm of the gamma function.",
          details: [
            "lgamma(x) returns the natural logarithm of the gamma function, which is often easier to compute safely than gamma(x) itself.",
            "It is especially useful when gamma(x) would overflow or when a formula needs stable log-domain arithmetic.",
            "Statistics, probability, and asymptotic analysis often use lgamma to keep large products numerically manageable."
          ],
          example: "emit lgamma(5)"
        }
      ]
    },
    {
      id: "arrays",
      title: "Array constructors and range generators",
      summary: "Convenience functions for building dense numeric structures and simple sequences.",
      items: [
        {
          name: "zeros",
          syntax: "zeros(shape...)",
          returns: "tensor",
          summary: "Construct an array filled with zeros.",
          details: [
            "zeros creates a dense tensor whose entire storage is initialized to 0.",
            "It is the standard starting point for matrix assembly, grid initialization, and buffer preparation before a computation writes real values into place."
          ],
          example: "emit zeros(2, 3)"
        },
        {
          name: "ones",
          syntax: "ones(shape...)",
          returns: "tensor",
          summary: "Construct an array filled with ones.",
          details: [
            "ones creates a dense tensor whose storage is initialized to 1.",
            "It is commonly used when a multiplicative identity-like shape is needed for normalization, accumulation, or initialization of gain factors."
          ],
          example: "emit ones(2, 3)"
        },
        {
          name: "full",
          syntax: "full(shape..., value)",
          returns: "tensor",
          summary: "Construct an array filled with a chosen value.",
          details: [
            "full creates a dense tensor whose entries are all set to the provided value.",
            "This is useful when the computation needs a sentinel value, a constant field, or a prefilled buffer that will later be refined in place."
          ],
          example: "emit full(2, 3, 7)"
        },
        {
          name: "eye",
          syntax: "eye(n)",
          returns: "matrix",
          summary: "Construct a square identity matrix.",
          details: [
            "eye constructs a square matrix with ones on the main diagonal and zeros everywhere else.",
            "This is the canonical identity matrix and is used in matrix algebra, solver setup, and as a neutral element for matrix multiplication."
          ],
          example: "emit eye(3)"
        },
        {
          name: "identity",
          syntax: "identity(n)",
          returns: "matrix",
          summary: "Construct a square identity matrix.",
          details: [
            "identity is the alias-shaped form of eye and produces the same square identity matrix.",
            "It exists so the language can read naturally in code that prefers the word identity over the shorter eye spelling."
          ],
          example: "emit identity(3)"
        },
        {
          name: "arange",
          syntax: "arange(start, stop, step)",
          returns: "list or tensor",
          summary: "Generate a numeric range with an explicit step.",
          details: [
            "arange generates a sequence that advances from start toward stop using a fixed step size.",
            "It is the regular numeric ramp constructor and is useful for sampled grids, iteration tables, and synthetic axis data."
          ],
          example: "emit arange(0, 10, 2)"
        },
        {
          name: "linspace",
          syntax: "linspace(start, stop, count)",
          returns: "list or tensor",
          summary: "Generate evenly spaced samples over an interval.",
          details: [
            "linspace returns a fixed number of values that divide the interval between start and stop into equal segments.",
            "It is the usual choice when you care about sample count, endpoint coverage, and predictable spacing rather than a literal step size."
          ],
          example: "emit linspace(0, 1, 5)"
        },
        {
          name: "logspace",
          syntax: "logspace(start, stop, count, base)",
          returns: "list or tensor",
          summary: "Generate samples spaced on a logarithmic scale.",
          details: [
            "logspace returns samples whose exponents are evenly spaced, then raises the chosen base to those exponents.",
            "It is useful for frequency sweeps, order-of-magnitude searches, and any workflow where logarithmic spacing is more meaningful than linear spacing."
          ],
          example: "emit logspace(0, 2, 3, 10)"
        },
        {
          name: "vector",
          syntax: "vector(values...)",
          returns: "tensor",
          summary: "Construct a one-dimensional tensor from values.",
          details: [
            "vector turns a variadic list of values into a one-dimensional dense tensor.",
            "It is the explicit, readable form of a small numeric vector and is useful when the data is naturally flat."
          ],
          example: "emit vector(1, 2, 3)"
        },
        {
          name: "vec",
          syntax: "vec(values...)",
          returns: "tensor",
          summary: "Construct a one-dimensional tensor from values.",
          details: [
            "vec is the compact spelling of vector and produces the same one-dimensional dense tensor.",
            "It is useful when the code wants a short, visually light constructor for small numeric sequences."
          ],
          example: "emit vec(1, 2, 3)"
        }
      ]
    },
    {
      id: "tensor",
      title: "Tensor, matrix, shape, and linear algebra",
      summary: "Shape-aware dense numeric work with broadcast semantics, views, and linear algebra helpers.",
      items: [
        {
          name: "tensor",
          syntax: "tensor(values...)",
          returns: "tensor",
          summary: "Construct a dense tensor with explicit shape metadata.",
          details: [
            "tensor is the core dense numeric container in Osiris and carries shape metadata alongside its flat storage.",
            "The runtime, optimizer, and native lowering passes all treat shape as a first-class property so tensor code can be analyzed and specialized instead of handled as generic lists."
          ],
          example: "emit tensor([1, 2, 3, 4])"
        },
        {
          name: "matrix",
          syntax: "matrix(rows, cols, values...)",
          returns: "matrix",
          summary: "Construct a dense matrix with explicit shape metadata.",
          details: [
            "matrix is the two-dimensional specialization of tensor and is intended for row-column linear algebra work.",
            "It preserves the same shape metadata and lowering benefits as tensor, but reads more naturally when the data is explicitly a matrix."
          ],
          example: "emit matrix(2, 2, [1, 2, 3, 4])"
        },
        {
          name: "shape",
          syntax: "shape(value)",
          returns: "tensor",
          summary: "Inspect the shape of a shape-aware value.",
          details: [
            "shape returns the rank and dimension sizes for a tensor-like value.",
            "It is the inspection primitive that makes it easy to reason about broadcasting, reshape safety, and matrix dimensions."
          ],
          example: "emit shape(matrix(2, 3, [1, 2, 3, 4, 5, 6]))"
        },
        {
          name: "flatten",
          syntax: "flatten(value)",
          returns: "list",
          summary: "Flatten a shape-aware value into a linear sequence.",
          details: [
            "flatten discards the shape structure and returns the underlying values in linear order.",
            "It is useful when a tensor needs to be serialized, inspected, or passed into a routine that expects a flat sequence."
          ],
          example: "emit flatten(matrix(2, 3, [1, 2, 3, 4, 5, 6]))"
        },
        {
          name: "squeeze",
          syntax: "squeeze(value)",
          returns: "tensor",
          summary: "Remove singleton dimensions from a tensor.",
          details: [
            "squeeze removes dimensions whose extent is 1.",
            "It is useful after broadcasting or batch construction when the code wants to recover a simpler shape without changing the underlying values."
          ],
          example: "emit squeeze(tensor([[[1]]]))"
        },
        {
          name: "expand_dims",
          syntax: "expand_dims(value, axis)",
          returns: "tensor",
          summary: "Add a singleton dimension to a tensor.",
          details: [
            "expand_dims inserts a new axis of size 1 at the requested position.",
            "It is the opposite of squeeze and is commonly used to make a value broadcast-compatible or to make a batch axis explicit."
          ],
          example: "emit expand_dims(vec(1, 2, 3), 0)"
        },
        {
          name: "concat",
          syntax: "concat(values...)",
          returns: "tensor",
          summary: "Combine tensors along an axis.",
          details: [
            "concat joins compatible tensors along a shared axis so their data becomes one larger tensor.",
            "It is useful for assembling batches, extending datasets, and combining chunks of tensor work without flattening them first."
          ],
          example: "emit concat(matrix(1, 2, [1, 2]), matrix(1, 2, [3, 4]))"
        },
        {
          name: "stack",
          syntax: "stack(values...)",
          returns: "tensor",
          summary: "Stack tensors by adding a new dimension.",
          details: [
            "stack joins tensors by creating a new axis rather than reusing an existing one.",
            "It is the direct way to build a batch dimension or any other higher-rank structure from a collection of equal-shaped inputs."
          ],
          example: "emit stack(matrix(1, 2, [1, 2]), matrix(1, 2, [3, 4]))"
        },
        {
          name: "broadcast_to",
          syntax: "broadcast_to(value, shape)",
          returns: "tensor",
          summary: "Broadcast a value to a requested shape.",
          details: [
            "broadcast_to expands a value across a larger shape according to broadcasting rules.",
            "It is useful when a scalar or smaller tensor must participate in elementwise work alongside a larger shape."
          ],
          example: "emit broadcast_to(1, [2, 3])"
        },
        {
          name: "where",
          syntax: "where(cond, a, b)",
          returns: "tensor",
          summary: "Select values conditionally across shapes.",
          details: [
            "where chooses between two values elementwise based on a boolean condition.",
            "It is the vectorized branch operator for tensor math and is the array-native equivalent of an if expression."
          ],
          example: "emit where([true, false], [1, 2], [3, 4])"
        },
        {
          name: "clip",
          syntax: "clip(value, lo, hi)",
          returns: "tensor",
          summary: "Clamp values to a lower and upper bound.",
          details: [
            "clip constrains each value into the closed interval between lo and hi.",
            "It is useful for bounded activations, normalization, range safety, and any numeric pipeline that should not escape a fixed interval."
          ],
          example: "emit clip(vec(-1, 0, 2), 0, 1)"
        },
        {
          name: "gather",
          syntax: "gather(value, index)",
          returns: "tensor",
          summary: "Gather elements by index.",
          details: [
            "gather builds a new tensor by selecting entries from the source at the provided indexes.",
            "It is the read-side companion to scatter and is common in lookup, sampling, and embedding-style workflows."
          ],
          example: "emit gather(vec(10, 20, 30), [0, 2])"
        },
        {
          name: "scatter",
          syntax: "scatter(base, index, values)",
          returns: "tensor",
          summary: "Scatter elements into a base tensor by index.",
          details: [
            "scatter writes values into a base tensor at the provided positions.",
            "It is the write-side companion to gather and is common in indexed updates, sparse placement, and reconstruction workflows."
          ],
          example: "emit scatter(vec(0, 0, 0), [1], [9])"
        },
        {
          name: "pad",
          syntax: "pad(value, ...)",
          returns: "tensor",
          summary: "Pad tensor data to a larger shape.",
          details: [
            "pad extends a tensor with additional boundary elements according to the selected padding policy.",
            "It is used when windows, borders, or fixed-size kernels need data to exist past the original edge."
          ],
          example: "emit pad(vec(1, 2), 1)"
        },
        {
          name: "tile",
          syntax: "tile(value, ...)",
          returns: "tensor",
          summary: "Repeat tensor data across dimensions.",
          details: [
            "tile repeats tensor data across one or more axes to form a larger repeated structure.",
            "It is useful for patterned expansion, grid construction, and repeated layout generation."
          ],
          example: "emit tile(vec(1, 2), 2)"
        },
        {
          name: "transpose",
          syntax: "transpose(value)",
          returns: "tensor",
          summary: "Transpose a tensor or matrix.",
          details: [
            "transpose swaps axes in a tensor and is the canonical row-column transform for matrices.",
            "It is a core dense layout operation and often changes how data is traversed in later linear algebra steps."
          ],
          example: "emit transpose(matrix(2, 2, [1, 2, 3, 4]))"
        },
        {
          name: "dot",
          syntax: "dot(a, b)",
          returns: "tensor or scalar",
          summary: "Compute a dot product.",
          details: [
            "dot computes the sum of pairwise products across matching elements.",
            "It is the scalar-product primitive behind projections, similarity measures, and the inner loops of matrix multiplication."
          ],
          example: "emit dot(vec(1, 2, 3), vec(4, 5, 6))"
        },
        {
          name: "matmul",
          syntax: "matmul(a, b)",
          returns: "tensor or scalar",
          summary: "Compute matrix multiplication.",
          details: [
            "matmul performs dense matrix multiplication with shape-aware semantics.",
            "It is the central dense linear algebra primitive for transforms, solver setup, and high-throughput numeric code."
          ],
          example: "emit matmul(matrix(2, 2, [1, 2, 3, 4]), matrix(2, 2, [5, 6, 7, 8]))"
        },
        {
          name: "reshape",
          syntax: "reshape(value, shape)",
          returns: "tensor",
          summary: "Reinterpret a tensor with a new shape when the data layout allows it.",
          details: [
            "reshape keeps the underlying values but changes the shape metadata used to interpret them.",
            "It is the preferred way to present the same contiguous storage under a different view when the element count still matches."
          ],
          example: "emit reshape(vec(1, 2, 3, 4), [2, 2])"
        },
        {
          name: "outer",
          syntax: "outer(a, b)",
          returns: "tensor",
          summary: "Construct an outer product.",
          details: [
            "outer forms a matrix or higher-rank tensor from every pairwise combination of the inputs.",
            "It is useful for basis construction, tensor products, and diagnostics that need the full pairwise interaction table."
          ],
          example: "emit outer(vec(1, 2), vec(3, 4))"
        },
        {
          name: "trace",
          syntax: "trace(matrix)",
          returns: "scalar",
          summary: "Read the diagonal trace of a matrix.",
          details: [
            "trace sums the entries on the main diagonal of a square matrix.",
            "It is a standard diagnostic in linear algebra and is often used in invariants, projections, and matrix summaries."
          ],
          example: "emit trace(matrix(2, 2, [1, 2, 3, 4]))"
        },
        {
          name: "det",
          syntax: "det(matrix)",
          returns: "scalar",
          summary: "Compute a matrix determinant.",
          details: [
            "det returns the determinant, which encodes scale factor and invertibility information for a square matrix.",
            "It is a classic dense linear algebra workhorse and is used in solving systems, geometry, and change-of-basis reasoning."
          ],
          example: "emit det(matrix(2, 2, [1, 2, 3, 4]))"
        },
        {
          name: "solve",
          syntax: "solve(matrix, rhs)",
          returns: "tensor",
          summary: "Solve a linear system.",
          details: [
            "solve computes the vector or tensor x such that matrix * x = rhs when the system is solvable.",
            "It is the direct linear-system primitive used in fitting, calibration, physics, and numerical analysis."
          ],
          example: "emit solve(matrix(2, 2, [1, 2, 3, 4]), [5, 11])"
        },
        {
          name: "inv",
          syntax: "inv(matrix)",
          returns: "tensor",
          summary: "Invert a matrix.",
          details: [
            "inv computes the multiplicative inverse of a square matrix when one exists.",
            "It is often used in algebraic derivations, solver formulations, and analytic matrix transforms, although solve is usually preferred numerically."
          ],
          example: "emit inv(matrix(2, 2, [1, 2, 3, 4]))"
        },
        {
          name: "qr",
          syntax: "qr(matrix)",
          returns: "tensor",
          summary: "Compute a QR factorization.",
          details: [
            "qr factors a matrix into orthogonal and triangular components.",
            "It is useful in least-squares solving, orthonormal basis construction, and numerically stable decomposition-based workflows."
          ],
          example: "emit qr(matrix(2, 2, [1, 2, 3, 4]))"
        },
        {
          name: "cholesky",
          syntax: "cholesky(matrix)",
          returns: "tensor",
          summary: "Compute a Cholesky factorization.",
          details: [
            "cholesky factors a symmetric positive-definite matrix into a lower-triangular factor and its transpose.",
            "It is a preferred decomposition for covariance systems, optimization, and fast structured solves."
          ],
          example: "emit cholesky(matrix(2, 2, [4, 2, 2, 3]))"
        },
        {
          name: "eig",
          syntax: "eig(matrix)",
          returns: "tensor",
          summary: "Compute an eigenvalue decomposition.",
          details: [
            "eig computes the eigenstructure of a matrix, returning values that describe how the matrix scales its eigenvectors.",
            "It is central to stability analysis, modal decomposition, dynamical systems, and spectral reasoning."
          ],
          example: "emit eig(matrix(2, 2, [1, 2, 3, 4]))"
        },
        {
          name: "eigh",
          syntax: "eigh(matrix)",
          returns: "tensor",
          summary: "Compute an eigenvalue decomposition for symmetric matrices.",
          details: [
            "eigh is the symmetric or Hermitian-specialized eigen decomposition path.",
            "It is typically preferred when the input is known to be symmetric because that structure allows more stable and efficient computation."
          ],
          example: "emit eigh(matrix(2, 2, [1, 2, 2, 3]))"
        },
        {
          name: "svd",
          syntax: "svd(matrix)",
          returns: "tensor",
          summary: "Compute a singular value decomposition.",
          details: [
            "svd decomposes a matrix into orthogonal factors and singular values that measure the strength of its principal directions.",
            "It is one of the most useful numerical linear algebra tools for compression, rank analysis, pseudoinverses, and stable approximation."
          ],
          example: "emit svd(matrix(2, 2, [1, 2, 3, 4]))"
        }
      ]
    },
    {
      id: "stats",
      title: "Reductions and statistics",
      summary: "Summaries, aggregations, and elementwise collection-wide numeric helpers.",
      items: [
        {
          name: "mean",
          syntax: "mean(value)",
          returns: "numeric scalar",
          summary: "Compute the average.",
          details: [
            "mean adds the elements of a collection and divides by their count.",
            "It is the ordinary center-of-mass style summary used in almost every descriptive numeric workflow."
          ],
          example: "emit mean([1, 2, 3, 4])"
        },
        {
          name: "var",
          syntax: "var(value)",
          returns: "numeric scalar",
          summary: "Compute the variance.",
          details: [
            "var measures how far a collection spreads away from its mean.",
            "It is a primary dispersion statistic and is used throughout uncertainty estimates, modeling, and normalization."
          ],
          example: "emit var([1, 2, 3, 4])"
        },
        {
          name: "std",
          syntax: "std(value)",
          returns: "numeric scalar",
          summary: "Compute the standard deviation.",
          details: [
            "std is the square root of variance and expresses spread in the original units of the data.",
            "It is often easier to interpret than variance and is one of the most common statistics in analysis and reporting."
          ],
          example: "emit std([1, 2, 3, 4])"
        },
        {
          name: "median",
          syntax: "median(value)",
          returns: "numeric scalar",
          summary: "Compute the median.",
          details: [
            "median returns the middle value after the data is ordered, or the midpoint between the two middle values for an even-sized collection.",
            "It is more robust than the mean when the data has outliers or long tails."
          ],
          example: "emit median([1, 2, 3, 4])"
        },
        {
          name: "quantile",
          syntax: "quantile(value, q)",
          returns: "numeric scalar",
          summary: "Compute a quantile.",
          details: [
            "quantile returns the value below which a chosen fraction of the data falls.",
            "It is the general form of percentile-based analysis and is useful for distribution thresholds and robust summaries."
          ],
          example: "emit quantile([1, 2, 3, 4], 0.75)"
        },
        {
          name: "percentile",
          syntax: "percentile(value, p)",
          returns: "numeric scalar",
          summary: "Compute a percentile.",
          details: [
            "percentile is the human-friendly quantile form expressed as a percentage from 0 to 100.",
            "It is useful for summarizing ranks, thresholds, and distribution tails in reports and analysis pipelines."
          ],
          example: "emit percentile([1, 2, 3, 4], 75)"
        },
        {
          name: "argmin",
          syntax: "argmin(value)",
          returns: "index",
          summary: "Return the position of the smallest element.",
          details: [
            "argmin scans the collection and returns the index of the smallest observed value.",
            "It is useful whenever you need the location of an extreme rather than the extreme's numeric value."
          ],
          example: "emit argmin([3, 7, 4, 9])"
        },
        {
          name: "argmax",
          syntax: "argmax(value)",
          returns: "index",
          summary: "Return the position of the largest element.",
          details: [
            "argmax scans the collection and returns the index of the largest observed value.",
            "It is useful whenever you need the location of an extreme rather than the extreme's numeric value."
          ],
          example: "emit argmax([3, 7, 4, 9])"
        },
        {
          name: "any",
          syntax: "any(value)",
          returns: "bool",
          summary: "Collapse a boolean collection to a single truth value.",
          details: [
            "any returns true when at least one value in the collection is true.",
            "It is the standard existential reduction and is useful for validation, filtering, and guard conditions."
          ],
          example: "emit any([true, false, false])"
        },
        {
          name: "all",
          syntax: "all(value)",
          returns: "bool",
          summary: "Collapse a boolean collection to a single truth value.",
          details: [
            "all returns true only when every value in the collection is true.",
            "It is the standard universal reduction and is useful for validation, invariants, and guard conditions."
          ],
          example: "emit all([true, true, false])"
        },
        {
          name: "cumsum",
          syntax: "cumsum(value)",
          returns: "list or tensor",
          summary: "Return cumulative sums.",
          details: [
            "cumsum returns the running prefix sum of a collection.",
            "It is useful for cumulative totals, histogram-like accumulation, and any workflow that needs the trajectory of partial sums."
          ],
          example: "emit cumsum([1, 2, 3, 4])"
        },
        {
          name: "cumprod",
          syntax: "cumprod(value)",
          returns: "list or tensor",
          summary: "Return cumulative products.",
          details: [
            "cumprod returns the running prefix product of a collection.",
            "It is useful for multiplicative growth, compounded factors, and any workflow that needs the trajectory of partial products."
          ],
          example: "emit cumprod([1, 2, 3, 4])"
        }
      ]
    },
    {
      id: "io",
      title: "I/O helpers",
      summary: "File access and structured input helpers for deterministic data pipelines.",
      items: [
        {
          name: "read",
          syntax: "read(path)",
          returns: "text",
          summary: "Read a file as text.",
          details: ["Use this when the file is text and you want a string result."],
          example: "emit read(\"input.txt\")"
        },
        {
          name: "readbytes",
          syntax: "readbytes(path)",
          returns: "byte list",
          summary: "Read a file as raw bytes.",
          details: ["Use this when the file is binary or when you need exact byte access."],
          example: "emit readbytes(\"input.bin\")"
        },
        {
          name: "write",
          syntax: "write(path, data)",
          returns: "bool",
          summary: "Write text or byte-compatible data to a file.",
          details: ["The current implementation returns success as a true boolean value."],
          example: "emit write(\"out.txt\", \"hello\")"
        },
        {
          name: "lines",
          syntax: "lines(data)",
          returns: "list",
          summary: "Split text or bytes into line records.",
          details: ["This is useful for line-oriented file and stream processing."],
          example: "emit lines(read(\"input.txt\"))"
        },
        {
          name: "csv",
          syntax: "csv(data)",
          returns: "list",
          summary: "Parse CSV data into nested Osiris values.",
          details: ["Each row is returned as a list of cell values."],
          example: "emit csv(readbytes(\"input.csv\"))"
        },
        {
          name: "jsonparse",
          syntax: "jsonparse(data)",
          returns: "value",
          summary: "Parse JSON into native Osiris values.",
          details: ["JSON numbers are preserved with exactness where possible."],
          example: "emit jsonparse(readbytes(\"input.json\"))"
        }
      ]
    },
    {
      id: "data",
      title: "Data helpers",
      summary: "Byte work, packing, encoding, and indexable collection helpers.",
      items: [
        {
          name: "bytes",
          syntax: "bytes(value...)",
          returns: "byte list",
          summary: "Convert values into raw bytes.",
          details: ["Use this when you need explicit byte-oriented work rather than text."],
          example: "emit bytes(\"osiris\")"
        },
        {
          name: "hex",
          syntax: "hex(value)",
          returns: "text",
          summary: "Format a value as hexadecimal text.",
          details: ["Useful for inspection, logging, and low-level data checks."],
          example: "emit hex(bytes(\"AB\"))"
        },
        {
          name: "bin",
          syntax: "bin(value)",
          returns: "text",
          summary: "Format a value as binary text.",
          details: ["Useful for inspection, logging, and bit-level debugging."],
          example: "emit bin(5)"
        },
        {
          name: "pack",
          syntax: "pack(value...)",
          returns: "bytes",
          summary: "Pack structured values into bytes.",
          details: ["Useful for binary layouts and compact encoding work."],
          example: "emit pack(1, 2, 3)"
        },
        {
          name: "unpack",
          syntax: "unpack(bytes)",
          returns: "values",
          summary: "Recover values from packed data.",
          details: ["This is the inverse of pack for supported layouts."],
          example: "emit unpack(pack(1, 2, 3))"
        },
        {
          name: "at",
          syntax: "at(value, index)",
          returns: "element",
          summary: "Read an element at a given index.",
          details: ["Works with byte lists, lists, tensors, and other indexable collections."],
          example: "emit at([1, 2, 3], 1)"
        },
        {
          name: "setat",
          syntax: "setat(value, index, new)",
          returns: "updated value",
          summary: "Replace an element at a given index.",
          details: ["Use this for functional-style updates without mutating the original value."],
          example: "emit setat([1, 2, 3], 1, 9)"
        },
        {
          name: "encode",
          syntax: "encode(value)",
          returns: "bytes",
          summary: "Serialize an Osiris value.",
          details: ["This is the round-trip codec path for value snapshots and binary interchange."],
          example: "emit encode([1u8, 2u16, \"go\"])"
        },
        {
          name: "decode",
          syntax: "decode(bytes)",
          returns: "value",
          summary: "Restore an Osiris value from encoded bytes.",
          details: ["This is the inverse of encode for supported value trees."],
          example: "emit decode(encode([1u8, 2u16, \"go\"]))"
        }
      ]
    },
    {
      id: "crypto",
      title: "std/crypto",
      summary: "Hashing, mixing, and deterministic noise helpers.",
      items: [
        {
          name: "hash",
          syntax: "hash(value...)",
          returns: "text",
          summary: "Hash values into a stable text digest.",
          details: ["These are the current crypto-flavored helpers exposed through std/crypto."],
          example: "emit hash(\"osiris\")"
        },
        {
          name: "mix",
          syntax: "mix(value...)",
          returns: "numeric value",
          summary: "Mix values into a deterministic numeric output.",
          details: ["Useful for deterministic scramblers and pseudo-random style work."],
          example: "emit mix(\"osiris\")"
        },
        {
          name: "noise",
          syntax: "noise(value...)",
          returns: "numeric value",
          summary: "Generate deterministic noise from inputs.",
          details: ["Useful for procedural generation and stable randomized transforms."],
          example: "emit noise(\"osiris\")"
        }
      ]
    },
    {
      id: "list",
      title: "std/list",
      summary: "Length, range generation, slicing, and simple reducers.",
      items: [
        {
          name: "len",
          syntax: "len(value)",
          returns: "numeric value",
          summary: "Return the length of a collection.",
          details: ["These are the smallest list utilities and are used throughout examples and tests."],
          example: "emit len([1, 2, 3])"
        },
        {
          name: "range",
          syntax: "range(start, stop, step)",
          returns: "list",
          summary: "Generate a numeric range.",
          details: ["Useful for loops, sampling, and numeric iteration."],
          example: "emit range(0, 5, 1)"
        },
        {
          name: "take",
          syntax: "take(value, count)",
          returns: "list",
          summary: "Take the first N values from a collection.",
          details: ["Useful for slices, previews, and bounded sampling."],
          example: "emit take([1, 2, 3, 4], 2)"
        },
        {
          name: "sum",
          syntax: "sum(value)",
          returns: "numeric value",
          summary: "Reduce a collection by addition.",
          details: ["This is the standard additive reducer for numeric collections."],
          example: "emit sum([1, 2, 3, 4])"
        },
        {
          name: "product",
          syntax: "product(value)",
          returns: "numeric value",
          summary: "Reduce a collection by multiplication.",
          details: ["This is the standard multiplicative reducer for numeric collections."],
          example: "emit product([1, 2, 3, 4])"
        }
      ]
    }
  ]
};
