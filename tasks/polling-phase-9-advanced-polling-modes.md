# Polling — PHASE 9: ADVANCED POLLING MODES

Source: roadmaps/POLLING-CHK.txt, section PHASE 9: ADVANCED POLLING MODES.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Add one-shot watcher mode where supported.
- [ ] Add edge-triggered mode where supported.
- [ ] Add backend capability detection for edge-triggered mode.
- [ ] Add backend capability detection for one-shot mode.
- [ ] Define fallback behavior when advanced modes are requested but unsupported.
- [ ] Document level-triggered polling as the default behavior.
- [ ] Document edge-triggered polling requirements, especially read/write-until-would-block behavior.
- [ ] Add tests proving edge-triggered watchers do not lose events under normal usage.
- [ ] Add tests proving one-shot watchers deactivate or remove themselves after firing.
- [ ] Decide whether advanced modes are stable public API or expert-only API.

