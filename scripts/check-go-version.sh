#!/bin/sh
# Verifies the Go release line supported by this checkout before a build or CI
# job begins. Keep this in sync with the go directive in go.mod.
set -eu

required_release='1.26'

if ! command -v go >/dev/null 2>&1; then
  printf '%s\n' "Osiris requires Go ${required_release}.x, but 'go' was not found." >&2
  exit 1
fi

actual=$(go env GOVERSION 2>/dev/null || true)
case "$actual" in
  go${required_release}.*)
    printf '%s\n' "Go toolchain accepted: ${actual} (requires ${required_release}.x)"
    ;;
  *)
    printf '%s\n' "Osiris requires Go ${required_release}.x (see go.mod), found ${actual:-unknown}." >&2
    printf '%s\n' 'Install the supported release line, then rerun this command.' >&2
    exit 1
    ;;
esac
