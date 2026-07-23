# Polling — PHASE 15: CROSS-PLATFORM SEMANTICS

Source: roadmaps/POLLING-CHK.txt, section PHASE 15: CROSS-PLATFORM SEMANTICS.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Define the minimum supported backend per operating system.
- [ ] Add backend-specific tests that skip when unavailable.
- [ ] Normalize event names across operating systems.
- [ ] Normalize timeout behavior across operating systems.
- [ ] Normalize interrupted wait behavior across operating systems.
- [ ] Normalize closed-handle behavior across operating systems where possible.
- [ ] Document unavoidable platform differences.
- [ ] Document which features are Linux-only, BSD/macOS-only, Windows-only, or fallback-only.
- [ ] Ensure unsupported features fail clearly instead of silently degrading.
- [ ] Provide a way to inspect selected backend at runtime.
- [ ] Provide a way to inspect supported capabilities at runtime.

