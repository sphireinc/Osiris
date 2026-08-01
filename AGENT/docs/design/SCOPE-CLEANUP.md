# Scope-bound cleanup

Osiris uses `defer { ... }` as its universal lexical cleanup construct. The
body is registered when execution reaches the statement and is executed once
when the containing scope exits. Registration is ordered; execution is reverse
registration order (LIFO).

```osiris
let file = openread("input.txt")
defer { close(file) }
let first = readline(file)
```

The cleanup body captures the current lexical scope, so values are resolved as
they were at registration time. A cleanup body may register nested cleanup; the
nested action is released before its enclosing cleanup returns. Cleanups run on
normal fallthrough, `return`, `break`, `continue`, propagated `try` failure,
task cancellation, and any other exit from the scope. A cleanup is never
duplicated by a backend or optimizer.

Cancellation is deferred while cleanup actions run. This gives owned files,
streams, sockets, and future FFI handles a chance to release exactly once; the
original cancellation remains the primary failure. If both the body and one or
more cleanups fail, the deterministic error retains the body error first and
appends cleanup errors in LIFO execution order. Cleanup failures never panic or
erase the original source span.

`defer` is lexical, not a garbage-collector finalizer. Existing explicit
`close` calls remain valid and idempotent. Native C currently rejects programs
containing `defer` with a source-spanned diagnostic until its generated-C CFG
has explicit cleanup edges; it never evaluates the program to manufacture a
native result.

The interpreter and VM implement the contract today. The focused evidence is
`internal/osiris/scope_cleanup_test.go`: LIFO order, cancellation, dual
failures, and the native boundary are checked for the relevant artifacts.
