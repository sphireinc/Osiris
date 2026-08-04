# Native C TLS and HTTP provider ABI

This document describes the integrated TLS/HTTP component for task 148:
`internal/osiris/cbackend_tls_http_runtime.go`. The shared emitter appends the
resource, map, TLS-provider, and HTTP bridge components. A missing provider or
bridge is an explicit capability error; there is no interpreter fallback or
shell command.

## Public native entry points

The shared emitter preserves the hosted `std/net` argument order:

| Native symbol | `std/net` operation | Arguments | Result |
| --- | --- | --- | --- |
| `osiris_native_tls_dial` | `tls_dial` and `tls_dial_config` | `address, server_name_or_null, roots_or_null, timeout_or_null` | owned TLS stream resource |
| `osiris_native_tls_listen` | `tls_listen` | `address, certificate_path, key_path` | owned TLS listener resource |
| `osiris_native_http_get` | `http_get` | `url, timeout_or_null` | response map |
| `osiris_native_http_request` | `http_request` | `method, url, headers_or_null, body_or_null, timeout_or_null` | response map |
| `osiris_native_http_request_stream_value` | `http_request_stream` | `method, url, headers_or_null, body_or_null, timeout_or_null` | response map with owned `body_stream` |
| `osiris_native_http_body_read` | body-stream `readchunk` | `body_stream, size, timeout_or_null` | byte list |
| `osiris_native_http_serve` | `http_serve` | `listener, body_or_handler, status_or_null` | blocks until cancellation, shutdown, or provider failure |
| `osiris_tls_http_close` | resource `close` | owned TLS/HTTP resource | `true` on the first close |

`tls_dial` passes null roots, which asks the selected provider to use its
platform trust store. `tls_dial_config` passes caller-supplied PEM roots. The
provider enforces hostname verification, certificate verification, TLS 1.2 or
newer, and HTTP/1.1 ALPN. There is no insecure-verification option.

## Provider contract

`OirisNativeTLSProvider` and `OirisNativeHTTPTransportProvider` are C structs
with `abi_version == 1`, capability bits, and callbacks for dial/listen/accept.
Each callback returns an `OirisTLSHTTPStream` containing owned `read`, `write`,
`close`, and `destroy` operations. Provider callbacks receive operation
timeouts and a cancellation probe; they must not retain an Osiris pointer after
the resource is closed.

The repository supplies an OpenSSL provider in
`internal/osiris/cbackend_openssl_provider.go`. It is enabled only when a TLS
capability is granted and `pkg-config --cflags --libs openssl` succeeds. It
loads platform roots or caller PEM roots, verifies the requested server name,
requires TLS 1.2+, and advertises HTTP/1.1 ALPN. Other platforms may install
an equivalent provider through the same ABI. The build fails closed when no
provider can be discovered; it does not dynamically load an ambient plugin.

`osiris_native_tls_http_install` installs provider and map bridges. The
generated `O_MAP`/`omapget` representation supplies the default HTTP map
constructor and lookup path; `OirisHTTPHandler` remains the only supported
route for a generated-language closure handler.

## HTTP behavior and limits

The component accepts `http://` and `https://` URLs, emits HTTP/1.1 with
`Host`, `Connection: close`, and bounded `Content-Length`, and validates status
and header syntax. It supports fixed-length, chunked, and connection-close
bodies, up to 256 headers, an 8 MiB request/response body limit, and an 8 KiB
header-line limit. Invalid control bytes, malformed chunks, excessive headers,
truncated bodies, and provider over-reads are structured failures.

`http_request_stream` retains the provider stream in an owned body resource.
The response body is consumed only through `osiris_native_http_body_read`; the
shared `readchunk` dispatcher recognizes that resource kind. The parser keeps a
4 KiB staging buffer, so provider read boundaries are not exposed as language
semantics. Every close path releases the provider stream exactly once.

Plain HTTP client and static server transport use the default POSIX socket
bridge. A generated `O_CLOSURE` is invoked synchronously through `ocall`; the
native server does not retain the closure or its captures after the request.
The handler's response map is validated before any response bytes are written.

The client follows at most ten redirects for status 301, 302, 303, 307, and
308. `Location` accepts absolute, scheme-relative, root-relative, and
path-relative targets. For 301/302/303, non-GET requests become GET and their
body/content-length are removed. For 307/308, method and body are preserved.
Cross-authority redirects remove `Authorization`, `Proxy-Authorization`, and
`Cookie`. Every intermediate body is drained and its stream is closed. A
malformed location, limit breach, transport error, or cancellation closes the
current stream and propagates a structured error.

## Integration and evidence

`emitNativeResourceCall` routes the admitted operations directly to these
symbols. Global `readchunk`, `writechunk`, and `close` use the shared resource
dispatcher, which checks resource magic before selecting the TLS, socket, host,
or FFI owner. This prevents a network or TLS pointer from reaching `dlclose`.

The focused tests compile and execute the provider ABI fixture and the generated
native integration path. `TestNativeTLSHTTPRuntimeFollowsBoundedRedirects`
verifies method conversion, relative URL resolution, credential stripping,
intermediate stream closure, and the ten-hop bound.
`TestNativeTLSHTTPRuntimeExecutesGeneratedClosureHandler` verifies direct
generated closure invocation, response validation, handler errors, and
connection cleanup. `TestTask148NativeTLSExecutesWithOpenSSLProvider`
verifies a real TLS loopback certificate, configured PEM roots, hostname
verification, payload read, and close. Plain HTTP is covered by
`TestTask148NativeHTTPExecutesWithPortableTransport`.
