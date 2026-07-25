#!/usr/bin/env sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
GOCACHE=${GOCACHE:-/private/tmp/osiris-security-gocache} GOMODCACHE=${GOMODCACHE:-/private/tmp/osiris-security-gomodcache} \
  go test ./internal/osiris -run 'Test(DecodeValueBytesRejectsOversizedPayload|ReadLSPMessageRejectsOversizedPayload|LanguageServerInvalidIncrementalChangePreservesDocument|StructuredErrorModel|Module|CBackend)' -count=1
printf '%s\n' 'Security boundary corpus passed.'
