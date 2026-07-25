# Polling — PHASE 4: POLLING AND EVENT NOTIFICATION API

Source: roadmaps/POLLING-CHK.txt, section PHASE 4: POLLING AND EVENT NOTIFICATION API.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Support Linux `epoll` as a high-performance backend.
- [ ] Support BSD/macOS `kqueue` as a high-performance backend.
- [ ] Support Solaris/illumos event ports where available.
- [ ] Support Windows polling through an appropriate native backend.
- [ ] Support POSIX `poll` as the lowest common denominator.
- [ ] Decide whether legacy `select` support is needed as an additional fallback.
- [ ] Document backend performance characteristics and platform limitations.
- [ ] Expose backend availability for diagnostics and testing.
- [ ] Allow explicit backend selection for tests and advanced users.

