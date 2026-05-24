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
  title: "Osiris Manual",
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
          title: "Manual style",
          text: "This site uses a PHP-manual-style layout: dense reference cards, semantic chapters, and a readable index."
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
        "Values are inspected through the runtime rather than hidden behind magic. The runtime preserves exactness, width metadata, tensor shape, and module identity wherever it can.",
        "Imports are alias-based and version-aware. File modules, std modules, and local module graphs are resolved explicitly, and the loader records manifests for repeatable builds."
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
        },
        {
          title: "Deterministic pipeline",
          text: "The language favors predictable outputs, reproducible builds, and inspection tools like IR, SSA, CFG, trace, and heap views."
        },
        {
          title: "GitHub Pages docs",
          text: "This site is static and can live directly under docs/ with no build step."
        }
      ]
    }
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
          details: ["This is the flexible text joiner used heavily in manual-style examples."],
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
          name: "f32 / f64",
          syntax: "f32(value), f64(value)",
          returns: "float",
          summary: "Construct an explicit float with fixed width semantics.",
          details: [
            "number remains the default literal type, but explicit float widths are useful when the precision choice matters.",
            "These are the hardware-friendly float constructors."
          ],
          example: "emit f32(3.5)"
        },
        {
          name: "decimal / rat / rational",
          syntax: "decimal(value), rat(value), rational(value)",
          returns: "exact decimal",
          summary: "Construct exact decimal and rational-backed values.",
          details: [
            "These forms keep arithmetic exact where possible instead of collapsing into approximate float math.",
            "They are useful for finance, symbolic work, and exact fractions."
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
          name: "real / imag / magnitude",
          syntax: "real(z), imag(z), magnitude(z)",
          returns: "numeric scalar",
          summary: "Read the real part, imaginary part, or magnitude of a complex value.",
          details: ["These helpers are convenience accessors for complex-valued work."],
          example: "emit magnitude(complex(3, 4))"
        },
        {
          name: "abs / min / max / clamp",
          syntax: "abs(x), min(a, b), max(a, b), clamp(x, lo, hi)",
          returns: "numeric scalar",
          summary: "Common numeric comparison and range helpers.",
          details: ["These are the everyday primitives for scalar numeric logic."],
          example: "emit clamp(-3, 0, 10)"
        },
        {
          name: "floor / ceil / round",
          syntax: "floor(x), ceil(x), round(x)",
          returns: "numeric scalar",
          summary: "Round a numeric value toward common integral boundaries.",
          details: ["Use these for formatting, binning, and discretization."],
          example: "emit round(3.5)"
        },
        {
          name: "sign / step / heaviside",
          syntax: "sign(x), step(x), heaviside(x)",
          returns: "numeric scalar",
          summary: "Discrete sign and threshold helpers.",
          details: ["These are useful in numeric control flow, piecewise math, and activation functions."],
          example: "emit sign(-12)"
        },
        {
          name: "relu / sigmoid / softplus",
          syntax: "relu(x), sigmoid(x), softplus(x)",
          returns: "numeric scalar",
          summary: "Common activation-style transforms.",
          details: ["These are useful in scientific and machine-learning-style expressions."],
          example: "emit relu(-2)"
        },
        {
          name: "sin / cos / tan",
          syntax: "sin(x), cos(x), tan(x)",
          returns: "numeric scalar",
          summary: "Trigonometric functions for ordinary angle math.",
          details: ["The runtime exposes the usual transcendental math surface directly."],
          example: "emit sin(0.5)"
        },
        {
          name: "asin / acos / atan",
          syntax: "asin(x), acos(x), atan(x)",
          returns: "numeric scalar",
          summary: "Inverse trigonometric functions.",
          details: ["Useful for geometry, control, and signal work."],
          example: "emit atan(1)"
        },
        {
          name: "sinh / cosh / tanh",
          syntax: "sinh(x), cosh(x), tanh(x)",
          returns: "numeric scalar",
          summary: "Hyperbolic trigonometric functions.",
          details: ["Useful for geometry, transforms, and numerical analysis."],
          example: "emit tanh(0.5)"
        },
        {
          name: "asinh / acosh / atanh",
          syntax: "asinh(x), acosh(x), atanh(x)",
          returns: "numeric scalar",
          summary: "Inverse hyperbolic functions.",
          details: ["These complete the common hyperbolic math family."],
          example: "emit asinh(1)"
        },
        {
          name: "exp / expm1",
          syntax: "exp(x), expm1(x)",
          returns: "numeric scalar",
          summary: "Exponential functions.",
          details: ["expm1 keeps precision for values near zero."],
          example: "emit exp(1)"
        },
        {
          name: "pow / expt",
          syntax: "pow(x, y), expt(x, y)",
          returns: "numeric scalar",
          summary: "Raise a value to a power.",
          details: ["Exact integer inputs use the exact integer path where possible."],
          example: "emit pow(2, 10)"
        },
        {
          name: "log / log10 / log2 / log1p",
          syntax: "log(x), log10(x), log2(x), log1p(x)",
          returns: "numeric scalar",
          summary: "Logarithmic functions with common bases and precision-friendly variants.",
          details: ["log1p is useful when x is small and precision matters."],
          example: "emit log2(8)"
        },
        {
          name: "sqrt / cbrt / hypot / norm",
          syntax: "sqrt(x), cbrt(x), hypot(a, b), norm(value)",
          returns: "numeric scalar",
          summary: "Root and magnitude helpers.",
          details: ["norm is useful for vector and tensor-style magnitude checks."],
          example: "emit hypot(3, 4)"
        },
        {
          name: "erf / erfc / gamma / lgamma",
          syntax: "erf(x), erfc(x), gamma(x), lgamma(x)",
          returns: "numeric scalar",
          summary: "Special functions for scientific computation.",
          details: ["These are the common scientific and statistical special functions."],
          example: "emit gamma(5)"
        }
      ]
    },
    {
      id: "arrays",
      title: "Array constructors and range generators",
      summary: "Convenience functions for building dense numeric structures and simple sequences.",
      items: [
        {
          name: "zeros / ones / full",
          syntax: "zeros(shape...), ones(shape...), full(shape..., value)",
          returns: "tensor",
          summary: "Construct arrays filled with zero, one, or a chosen value.",
          details: ["These are the base constructors for dense numeric work."],
          example: "emit zeros(2, 3)"
        },
        {
          name: "eye / identity",
          syntax: "eye(n), identity(n)",
          returns: "matrix",
          summary: "Construct a square identity matrix.",
          details: ["Useful for linear algebra and matrix initialization."],
          example: "emit eye(3)"
        },
        {
          name: "arange",
          syntax: "arange(start, stop, step)",
          returns: "list or tensor",
          summary: "Generate a numeric range with an explicit step.",
          details: ["This is the usual numeric sequence constructor."],
          example: "emit arange(0, 10, 2)"
        },
        {
          name: "linspace",
          syntax: "linspace(start, stop, count)",
          returns: "list or tensor",
          summary: "Generate evenly spaced samples over an interval.",
          details: ["Useful when you want fixed sample counts rather than fixed steps."],
          example: "emit linspace(0, 1, 5)"
        },
        {
          name: "logspace",
          syntax: "logspace(start, stop, count, base)",
          returns: "list or tensor",
          summary: "Generate samples spaced on a logarithmic scale.",
          details: ["Useful for magnitude sweeps and signal work."],
          example: "emit logspace(0, 2, 3, 10)"
        },
        {
          name: "vector / vec",
          syntax: "vector(values...), vec(values...)",
          returns: "tensor",
          summary: "Construct a one-dimensional tensor from values.",
          details: ["This is the convenience constructor used for small numeric vectors."],
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
          name: "tensor / matrix",
          syntax: "tensor(values...), matrix(rows, cols, values...)",
          returns: "tensor / matrix",
          summary: "Construct a dense tensor or a matrix with explicit shape metadata.",
          details: [
            "Tensor and matrix are the center of the scientific type story.",
            "Shapes are treated as first-class metadata for analysis and lowering."
          ],
          example: "emit matrix(2, 2, [1, 2, 3, 4])"
        },
        {
          name: "shape / flatten",
          syntax: "shape(value), flatten(value)",
          returns: "tensor or list",
          summary: "Inspect or flatten shape-aware values.",
          details: ["shape reports rank and dimensions; flatten linearizes the data."],
          example: "emit shape(matrix(2, 3, [1, 2, 3, 4, 5, 6]))"
        },
        {
          name: "squeeze / expand_dims",
          syntax: "squeeze(value), expand_dims(value, axis)",
          returns: "tensor",
          summary: "Remove or add singleton dimensions.",
          details: ["These are the classic dimension-shaping helpers."],
          example: "emit squeeze(tensor([[[1]]]))"
        },
        {
          name: "concat / stack",
          syntax: "concat(values...), stack(values...)",
          returns: "tensor",
          summary: "Combine tensors along an axis or by stacking dimensions.",
          details: ["These are used for batch building and shape assembly."],
          example: "emit concat(matrix(1, 2, [1, 2]), matrix(1, 2, [3, 4]))"
        },
        {
          name: "broadcast_to / where / clip",
          syntax: "broadcast_to(value, shape), where(cond, a, b), clip(value, lo, hi)",
          returns: "tensor",
          summary: "Broadcast values and perform conditional or bounded elementwise selection.",
          details: ["These are the everyday array-shaping helpers for vectorized computation."],
          example: "emit where([true, false], [1, 2], [3, 4])"
        },
        {
          name: "gather / scatter / pad / tile",
          syntax: "gather(value, index), scatter(base, index, values), pad(value, ...), tile(value, ...)",
          returns: "tensor",
          summary: "Index, expand, and reshape tensor data in common data-processing patterns.",
          details: ["These support selection, expansion, repetition, and data placement workflows."],
          example: "emit tile(vec(1, 2), 2)"
        },
        {
          name: "transpose / dot / matmul",
          syntax: "transpose(value), dot(a, b), matmul(a, b)",
          returns: "tensor or scalar",
          summary: "Linear algebra primitives for transposition, dot products, and matrix multiplication.",
          details: ["These are the core dense linear algebra operations."],
          example: "emit matmul(matrix(2, 2, [1, 2, 3, 4]), matrix(2, 2, [5, 6, 7, 8]))"
        },
        {
          name: "reshape",
          syntax: "reshape(value, shape)",
          returns: "tensor",
          summary: "Reinterpret a tensor with a new shape when the data layout allows it.",
          details: ["Reshape keeps the underlying data but changes the shape metadata."],
          example: "emit reshape(vec(1, 2, 3, 4), [2, 2])"
        },
        {
          name: "outer / trace",
          syntax: "outer(a, b), trace(matrix)",
          returns: "tensor or scalar",
          summary: "Construct an outer product or read the diagonal trace of a matrix.",
          details: ["Useful for vector calculus and matrix diagnostics."],
          example: "emit trace(matrix(2, 2, [1, 2, 3, 4]))"
        },
        {
          name: "det / solve / inv",
          syntax: "det(matrix), solve(matrix, rhs), inv(matrix)",
          returns: "scalar or tensor",
          summary: "Determinant, linear system solve, and matrix inverse helpers.",
          details: ["These are the classic dense linear algebra workhorses."],
          example: "emit solve(matrix(2, 2, [1, 2, 3, 4]), [5, 11])"
        },
        {
          name: "qr / cholesky",
          syntax: "qr(matrix), cholesky(matrix)",
          returns: "tensor",
          summary: "Matrix factorization helpers.",
          details: ["These are useful for numerical linear algebra and decomposition-based workflows."],
          example: "emit qr(matrix(2, 2, [1, 2, 3, 4]))"
        },
        {
          name: "eig / eigh / svd",
          syntax: "eig(matrix), eigh(matrix), svd(matrix)",
          returns: "tensor",
          summary: "Eigenvalue and singular-value decomposition helpers.",
          details: ["These expose the higher-end linear algebra surface directly in the language."],
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
          name: "mean / var / std",
          syntax: "mean(value), var(value), std(value)",
          returns: "numeric scalar",
          summary: "Compute average, variance, and standard deviation.",
          details: ["These are the standard descriptive statistics primitives."],
          example: "emit mean([1, 2, 3, 4])"
        },
        {
          name: "median / quantile / percentile",
          syntax: "median(value), quantile(value, q), percentile(value, p)",
          returns: "numeric scalar",
          summary: "Compute common order statistics.",
          details: ["Useful for robust summaries and threshold analysis."],
          example: "emit percentile([1, 2, 3, 4], 75)"
        },
        {
          name: "argmin / argmax",
          syntax: "argmin(value), argmax(value)",
          returns: "index",
          summary: "Return the position of the smallest or largest element.",
          details: ["These are common when you need a value's location instead of the value itself."],
          example: "emit argmax([3, 7, 4, 9])"
        },
        {
          name: "any / all",
          syntax: "any(value), all(value)",
          returns: "bool",
          summary: "Collapse boolean collections into a single truth value.",
          details: ["These are useful for guards and validation checks."],
          example: "emit all([true, true, false])"
        },
        {
          name: "cumsum / cumprod",
          syntax: "cumsum(value), cumprod(value)",
          returns: "list or tensor",
          summary: "Return cumulative sums or cumulative products.",
          details: ["These are useful for prefix-style numeric accumulation."],
          example: "emit cumsum([1, 2, 3, 4])"
        }
      ]
    },
    {
      id: "data",
      title: "I/O and data helpers",
      summary: "File access, byte work, parsing, and serialization helpers for deterministic data pipelines.",
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
          name: "hex / bin",
          syntax: "hex(value), bin(value)",
          returns: "text",
          summary: "Format values as hexadecimal or binary text.",
          details: ["These are useful for inspection, logging, and low-level data checks."],
          example: "emit hex(bytes(\"AB\"))"
        },
        {
          name: "pack / unpack",
          syntax: "pack(value...), unpack(value)",
          returns: "bytes or values",
          summary: "Pack structured values into bytes or recover values from packed data.",
          details: ["Useful for binary layouts and compact encoding work."],
          example: "emit unpack(pack(1, 2, 3))"
        },
        {
          name: "at / setat",
          syntax: "at(value, index), setat(value, index, new)",
          returns: "element or updated value",
          summary: "Read or replace an element at a given index.",
          details: ["These helpers are used for byte lists, lists, and other indexable collections."],
          example: "emit at([1, 2, 3], 1)"
        },
        {
          name: "encode / decode",
          syntax: "encode(value), decode(bytes)",
          returns: "bytes or value",
          summary: "Serialize an Osiris value and restore it later.",
          details: ["This is the round-trip codec path for value snapshots and binary interchange."],
          example: "emit decode(encode([1u8, 2u16, \"go\"]))"
        },
        {
          name: "read / readbytes",
          syntax: "read(path), readbytes(path)",
          returns: "text or byte list",
          summary: "Read a file as text or raw bytes.",
          details: ["read is the text path, readbytes is the raw binary path."],
          example: "emit read(\"input.txt\")"
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
          name: "lines / csv / jsonparse",
          syntax: "lines(data), csv(data), jsonparse(data)",
          returns: "list or structured value",
          summary: "Parse text or bytes into line, CSV, or JSON structures.",
          details: [
            "lines splits on newline boundaries.",
            "csv returns rows and columns as Osiris values.",
            "jsonparse converts JSON into a native Osiris value tree."
          ],
          example: "emit csv(readbytes(\"input.csv\"))"
        }
      ]
    },
    {
      id: "crypto",
      title: "std/crypto and std/list",
      summary: "Small utility modules for hashing, noise, ranges, and lightweight collection work.",
      items: [
        {
          name: "hash / mix / noise",
          syntax: "hash(value...), mix(value...), noise(value...)",
          returns: "text or numeric value",
          summary: "Hash and noise helpers for fingerprints, mixing, and deterministic pseudo-random style work.",
          details: ["These are the current crypto-flavored helpers exposed through std/crypto."],
          example: "emit hash(\"osiris\")"
        },
        {
          name: "len / range / take",
          syntax: "len(value), range(start, stop, step), take(value, count)",
          returns: "numeric value or list",
          summary: "Basic collection helpers for length, range generation, and slicing.",
          details: ["These are the smallest list utilities and are used throughout examples and tests."],
          example: "emit range(0, 5, 1)"
        },
        {
          name: "sum / product",
          syntax: "sum(value), product(value)",
          returns: "numeric value",
          summary: "Reduce a collection by addition or multiplication.",
          details: ["These are the standard aggregate reducers for numeric collections."],
          example: "emit sum([1, 2, 3, 4])"
        }
      ]
    }
  ]
};
