# Polling — PHASE 13: RESOURCE TYPES AND FUTURE EXTENSIONS

Source: roadmaps/POLLING-CHK.txt, section PHASE 13: RESOURCE TYPES AND FUTURE EXTENSIONS.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Add socket polling support.
- [ ] Add stream polling support.
- [ ] Add pipe polling support.
- [ ] Add timer handles.
- [ ] Add signal handles.
- [ ] Add process handles.
- [ ] Add child-process exit notification.
- [ ] Add HTTP client integration.
- [ ] Add DNS resolver integration if the runtime supports async DNS.
- [ ] Add file-system notification integration if in scope.
- [ ] Add TLS transport integration.
- [ ] Add database driver integration hooks.
- [ ] Add foreign-function/native-extension handle integration.
- [ ] Keep the public API extensible without breaking existing handle types.

