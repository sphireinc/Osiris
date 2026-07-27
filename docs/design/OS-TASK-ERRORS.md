# OS and task error mapping

Osiris normalizes host errors at the language boundary. Programs branch on
stable error kinds, never host errno text or Go error strings.

| Host condition | Osiris kind | Notes |
| --- | --- | --- |
| context cancellation | `cancelled` | Cooperative task/session cancellation. |
| context or socket deadline | `timeout` | A deadline is not an empty result. |
| missing path | `not_found` | Includes ordinary filesystem absence. |
| permission denial | `permission_denied` | Does not expose OS-specific text as a contract. |
| already exists | `already_exists` | Creation conflict. |
| closed local handle | `io_error` | Resource was closed locally. |
| malformed network address | `invalid_argument` | Caller-supplied address is invalid. |
| refused/unavailable dial | `unavailable` | Retryability remains caller policy. |
| would-block (`EAGAIN`/`EWOULDBLOCK`) | `unavailable` | The numeric native code is debug-only metadata. |
| other network operation | `network_error` | Inspect structured metadata, not message text. |
| EOF | `out_of_range` | End-of-input, not a task cancellation. |

Task groups cancel siblings after a failure. Deterministic scheduling preserves
source-key order; concurrent scheduling returns the first observed failure and
does not fabricate an aggregate error record until a stable public aggregate
format is designed. Public error rendering and machine serialization omit debug
metadata; `redact` also removes stack/debug details, drops conventional
sensitive metadata keys, and replaces conventional `token=…`, `password=…`,
and bearer-style secret text before cross-boundary use. Sensitive values should
still be placed in debug metadata whenever possible; text-pattern redaction is
a defensive boundary, not a substitute for structured handling.

Would-block conditions use the existing portable `unavailable` kind; the
portable poll foundation currently provides synthetic events only. Future OS
polling adapters must preserve the same mapping and retain a safe native code
in debug-only metadata, never as a cross-platform message contract.
