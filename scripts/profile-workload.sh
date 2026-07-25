#!/usr/bin/env sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
out=${PROFILE_DIR:-"$root/artifacts/profiles"}
mkdir -p "$out"
GOCACHE=${GOCACHE:-/private/tmp/osiris-profile-gocache} GOMODCACHE=${GOMODCACHE:-/private/tmp/osiris-profile-gomodcache} \
  go test ./internal/osiris -run '^$' -bench '^BenchmarkRuntimeNativeLoop$' -benchtime=3s -cpuprofile "$out/native-loop.cpu.pprof" -memprofile "$out/native-loop.mem.pprof"
printf 'profiles written to %s\n' "$out"
