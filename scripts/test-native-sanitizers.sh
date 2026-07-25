#!/usr/bin/env sh
set -eu

if ! command -v clang >/dev/null 2>&1; then
  printf '%s\n' 'Skipping native sanitizer suite: clang is unavailable.'
  exit 0
fi

if [ "$(uname -s)" = "Darwin" ]; then
  printf '%s\n' 'Skipping native sanitizer suite: this Darwin ASan runtime is not supported by the generated executable test harness.'
  exit 0
fi

CC=clang OSIRIS_C_SANITIZERS=1 go test ./internal/osiris -run '^(TestCBackendDirectClosureParity|TestCBackendComplexScalarParity|TestCBackendEigProviderComplexResidual|TestCBackendTensorShapeAndFlattenParity)$' -count=1
