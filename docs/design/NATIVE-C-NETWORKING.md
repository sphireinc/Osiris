# Native C networking boundary

This document records the integrated native networking adapter for task 148.
The hosted `std/net` contract remains the semantic reference; generated C opts
into each host resource explicitly and uses `O_RESOURCE` ownership rather than
the FFI library representation.

## Implemented native slice

`internal/osiris/cbackend_network_runtime.go` contains the socket component;
`cbackend_tls_http_runtime.go`, `cbackend_openssl_provider.go`, and
`cbackend_native_http_bridge.go` provide the TLS/HTTP and map layers. The
shared emitter appends these components and dispatches them through
`emitNativeResourceCall`. On POSIX targets it provides:

| C entry point | Osiris operation | Capability | Ownership |
| --- | --- | --- | --- |
| `onet_tcp_listen` | `std/net.tcp_listen` | `net.tcp_listen` | owns a listening descriptor |
| `onet_tcp_accept` | `std/net.tcp_accept` | `net.tcp_accept` | returns an owned connected descriptor |
| `onet_tcp_dial` | `std/net.tcp_dial` | `net.tcp_dial` | owns a connected descriptor |
| `onet_read` | native `readchunk` integration | `net.tcp_read` | bounded byte-list result |
| `onet_write` | native `writechunk` integration | `net.tcp_write` | drains the caller data before returning |
| `onet_udp_bind` | `std/net.udp_bind` | `net.udp_bind` | owns a datagram descriptor |
| `onet_udp_connect` | `std/net.udp_connect` | `net.udp_connect` | owns a datagram descriptor |
| `onet_udp_send` | `std/net.udp_send` | `net.udp_send` | copies byte-list input before syscall |
| `onet_udp_receive` | `std/net.udp_receive` | `net.udp_receive` | returns bytes, peer, and truncation fields |
| `onet_dns_lookup` | `std/net.dns_lookup` | `net.dns_lookup` | returns sorted, deduplicated owned strings |
| `onet_close` | native `close` integration | `net.close` | closes once; later close returns `false` |

The public argument and result shapes for the socket operations are:

| Operation | Signature | Result |
| --- | --- | --- |
| `tcp_listen` | `tcp_listen(address: string)` | `Result<tcp_listener>` containing an owned listener resource. |
| `tcp_accept` | `tcp_accept(listener, timeout: i64 \| null)` | `Result<tcp_stream>` containing an owned connected stream. |
| `tcp_dial` | `tcp_dial(address: string, timeout: i64 \| null)` | `Result<tcp_stream>` containing an owned connected stream. |
| `udp_bind` | `udp_bind(address: string)` | `Result<udp_socket>` containing an owned datagram resource. |
| `udp_connect` | `udp_connect(address: string)` | `Result<udp_socket>` containing an owned connected datagram resource. |
| `udp_send` | `udp_send(socket, bytes: string \| list<u8>, peer: string \| null)` | `Result<u64>` byte count. |
| `udp_receive` | `udp_receive(socket, size: u64 \| null, timeout: i64 \| null)` | `Result<list<u8>, string, bool>` represented by the hosted three-element result tuple: bytes, peer, truncated. |
| `dns_lookup` | `dns_lookup(host: string, policy: string \| null)` | `Result<list<string>>` of sorted, unique numeric addresses. |
| resource `readchunk` | `readchunk(resource, size: u64, timeout: i64 \| null)` | `Result<list<u8>>` for network/TLS resources; the same dispatcher also serves host streams and watches. |
| resource `writechunk` | `writechunk(resource, bytes: string \| list<u8>, timeout: i64 \| null)` | `Result<u64>` bytes written. |
| resource `close` | `close(resource)` | `Result<bool>`; the first close owns the descriptor transition. |

All address and packet inputs are bounded. `poll` supplies explicit
operation deadlines; `0` means no explicit deadline, matching the hosted
surface. Descriptor state is marked closed before the close result is
returned, and the tombstone remains tracked so copied language values cannot
double-close freed memory. A future resource ABI can replace the tombstone
with ref-counted ownership without changing the language operations.

Non-POSIX targets fail with a structured network error at the adapter
boundary. The component never shells out, uses an interpreter fallback, or
consults ambient capability state.

## Integration status and limits

The generated ABI now contains dedicated `O_RESOURCE` and `O_MAP` values. The
native emitter appends the socket, host-resource, TLS/HTTP, OpenSSL-provider,
and map-bridge components in that order. `close`, `readchunk`, and
`writechunk` inspect the resource magic before dispatch, so sockets, TLS
streams, watch streams, and file/process streams cannot reach `dlclose` or a
wrong-handle syscall.

TCP/UDP/DNS are executable POSIX paths. Plain HTTP uses the default transport
bridge and is executable through the same socket resource. TLS dial/listen
uses the explicit OpenSSL provider when a TLS capability is granted and the
build can discover OpenSSL through `pkg-config`; without that provider it
returns a structured capability error:

- `tls_dial`, `tls_dial_config`, and `tls_listen` require the verified OpenSSL
  provider or another installed `OirisNativeTLSProvider`.
- `http_request_stream` exposes an owned body resource and uses the native
  body-read parser. The client follows at most ten 301/302/303/307/308
  redirects, resolves all four supported `Location` forms, applies the
  method/body and cross-authority credential rules, drains and closes each
  intermediate response, and checks cancellation between hops.
- `http_serve` supports bounded static responses and direct generated closure
  handlers through `ocall`. A handler receives a request map and returns a
  validated response map; handler errors and cancellation close the accepted
  connection before propagating a structured error.

Those are ABI prerequisites, not silently omitted dispatch entries.

## Implemented integration points

`emitCIRWithOptionsAndFFI` appends the network, host-resource, TLS/HTTP,
OpenSSL-provider, and HTTP-map components. `nativeStaticModuleMember`,
`nativeModuleCapability`, `nativeHostResourceMembers`, and
`nativeNetworkRuntimeCapabilities` form the explicit admission table.
`emitNativeResourceCall` routes every admitted operation to a named C symbol;
global `readchunk`, `writechunk`, and `close` use the shared resource dispatcher
so a handle's magic selects the correct owner.

The OpenSSL provider is enabled only for TLS capabilities. The compiler asks
`pkg-config --cflags --libs openssl` for include and linker flags and fails
closed if the provider cannot be discovered. This keeps plain native programs
free of an ambient TLS dependency and makes the provider part of the build
environment rather than silently loading a library at runtime.

The focused checks are `TestNativeNetworkRuntimeCompilesAsCComponent`,
`TestTask148NativeTCPResourceExecutes`,
`TestTask148NativeUDPDNSResourcesExecute`,
`TestTask148NativeHTTPExecutesWithPortableTransport`,
`TestNativeTLSHTTPRuntimeFollowsBoundedRedirects`,
`TestNativeTLSHTTPRuntimeExecutesGeneratedClosureHandler`, and
`TestTask148NativeTLSExecutesWithOpenSSLProvider`. Loopback execution requires
the test environment to permit local sockets; the TLS test also requires
OpenSSL's `pkg-config` metadata.
