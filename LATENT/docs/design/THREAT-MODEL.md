# Threat model

Osiris is not a sandbox. Running source, compiling native code, loading local
modules, and opening files operate with the invoking user's authority.

| Boundary | Threat | Current mitigation/limitation |
| --- | --- | --- |
| Generated C and compiler | Source injecting compiler arguments or shell syntax | Compiler is invoked with `exec.CommandContext` and a fixed argument vector; generated source is a file argument, never a shell fragment. Native compilation is not a sandbox. |
| Temp artifacts | Symlink/race or overly broad permissions | Go temporary directories/files and atomic cache replacement are used; release/test scripts use private temporary directories. |
| Modules/cache | Traversal, ambiguous identity, tampered cache | Canonical local identities, project-root traversal rejection, lock/cache SHA-256 validation, and offline verification. |
| LSP | Oversized/malformed framed input or hostile edits | Framed JSON is parsed defensively; malformed source/edit ranges preserve state. Payload size limits are an explicit remaining resource-control requirement. |
| Value codecs | Malformed/recursive/oversized input | Header and cursor validation reject malformed records. Decoding untrusted arbitrarily large payloads is not yet a sandboxed operation. |
| Errors/traces | Host paths or secrets in output | Structured errors distinguish public and debug metadata; redacted rendering removes debug metadata and stack traces. Source paths remain user-visible diagnostic context. |

Security reports should be sent privately to the project maintainers with a
minimal reproducer, affected version/commit (`osiris version`), platform, and
impact. Do not publish a working exploit before maintainers acknowledge it.
Reports receive an acknowledgement target of seven days; fixes are coordinated
with a changelog Security entry and migration guidance when user action is
needed.

Run `./scripts/test-security-boundaries.sh` for the checked malicious/resource
corpus. It covers oversized LSP and codec payloads, invalid incremental edits,
structured-error redaction, module trust-boundary behavior, and native backend
validation. Native compiler arguments remain fixed by the Go backend; callers
must still treat a configured `CC` and numerical-provider path as trusted host
configuration rather than untrusted source input.
