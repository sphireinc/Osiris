#!/usr/bin/env sh
set -eu

if [ "${OSIRIS_UPDATE_GOLDENS:-}" != "1" ]; then
  printf '%s\n' 'Refusing to update conformance goldens.' >&2
  printf '%s\n' 'Review expected behavior first, then run: OSIRIS_UPDATE_GOLDENS=1 ./scripts/update-conformance-goldens.sh' >&2
  exit 2
fi

go test ./internal/osiris -run '^TestConformanceCorpus$' -count=1
printf '%s\n' 'Golden files updated. Review the diff before committing.'
