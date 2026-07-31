#!/usr/bin/env sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/osiris-benchmark-workloads.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
GOCACHE=${GOCACHE:-/private/tmp/osiris-bench-gocache} GOMODCACHE=${GOMODCACHE:-/private/tmp/osiris-bench-gomodcache} \
  go build -o "$work/osiris" "$root/cmd/osiris"

for workload in text-transform module-math tensor-kernel task-pipeline aegis-mission-control; do
  "$work/osiris" run "$root/benchmarks/$workload.osr" > "$work/run-$workload"
  "$work/osiris" vm "$root/benchmarks/$workload.osr" > "$work/vm-$workload"
  cmp "$work/run-$workload" "$work/vm-$workload"
  if [ "$workload" = "aegis-mission-control" ]; then
    "$work/osiris" build -o "$work/$workload" "$root/benchmarks/$workload.osr"
    "$work/$workload" > "$work/native-$workload"
    cmp "$work/run-$workload" "$work/native-$workload"
  fi
done
printf '%s\n' 'Benchmark workload parity passed, including the native showcase path.'
