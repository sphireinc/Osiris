#!/bin/sh
# Benchmark interpreter, VM, and precompiled native execution for the same
# verified Osiris program. This measures execution only; compilation is shown
# separately and is never included in the native runtime samples.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$ROOT"

RUNS=${RUNS:-7}
WARMUP=${WARMUP:-1}
BENCH_ITERS=${BENCH_ITERS:-1000}
GOCACHE=${GOCACHE:-/private/tmp/osiris-bench-gocache}
GOMODCACHE=${GOMODCACHE:-/private/tmp/osiris-bench-gomodcache}
WORKDIR=${WORKDIR:-$(mktemp -d "${TMPDIR:-/tmp}/osiris-bench.XXXXXX")}
OSIRIS_BIN=${OSIRIS_BIN:-"$WORKDIR/osiris"}
WORKLOAD=${WORKLOAD:-benchmarks/native-loop.osr}
NATIVE_BIN=${NATIVE_BIN:-"$WORKDIR/native-loop"}

case $RUNS in ''|*[!0-9]*) echo "RUNS must be a positive integer" >&2; exit 2;; esac
case $WARMUP in ''|*[!0-9]*) echo "WARMUP must be a non-negative integer" >&2; exit 2;; esac
case $BENCH_ITERS in ''|*[!0-9]*) echo "BENCH_ITERS must be a positive integer" >&2; exit 2;; esac
if [ "$RUNS" -eq 0 ] || [ "$BENCH_ITERS" -eq 0 ]; then
  echo "RUNS and BENCH_ITERS must be positive integers" >&2
  exit 2
fi

cleanup() {
  status=$?
  trap - EXIT INT TERM HUP
  if [ "${BENCH_KEEP:-0}" = "1" ]; then
    printf 'Benchmark scratch retained at %s\n' "$WORKDIR" >&2
  else
    rm -rf "$WORKDIR"
  fi
  exit "$status"
}
trap cleanup EXIT INT TERM HUP

now() { perl -MTime::HiRes=time -e 'printf "%.9f", time'; }
elapsed_ms() { awk -v start="$1" -v end="$2" 'BEGIN { printf "%.3f", (end-start)*1000 }'; }

printf '\n'
printf '  OSIRIS EXECUTION BENCHMARK\n'
printf '  ==========================\n\n'
printf '  workload : %s\n' "$WORKLOAD"
printf '  iterations per run : %s\n' "$BENCH_ITERS"
printf '  measured runs : %s (after %s warm-up run%s)\n\n' "$RUNS" "$WARMUP" "$( [ "$WARMUP" -eq 1 ] || printf s )"

printf '  [1/4] Building benchmark runner...\n'
GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" go build -o "$OSIRIS_BIN" ./cmd/osiris

SOURCE="$WORKDIR/workload.osr"
sed "s/1000000/$BENCH_ITERS/" "$WORKLOAD" > "$SOURCE"

printf '  [2/4] Compiling native executable...\n'
build_start=$(now)
"$OSIRIS_BIN" build -o "$NATIVE_BIN" "$SOURCE"
build_end=$(now)
build_ms=$(elapsed_ms "$build_start" "$build_end")

printf '  [3/4] Verifying matching results...\n'
interpreter_output=$("$OSIRIS_BIN" run "$SOURCE")
vm_output=$("$OSIRIS_BIN" vm "$SOURCE")
native_output=$("$NATIVE_BIN")
if [ "$interpreter_output" != "$vm_output" ] || [ "$interpreter_output" != "$native_output" ]; then
  echo 'Benchmark aborted: interpreter, VM, and native outputs differ.' >&2
  printf '%s\n' "  interpreter: $interpreter_output" >&2
  printf '%s\n' "  vm         : $vm_output" >&2
  printf '%s\n' "  native     : $native_output" >&2
  exit 1
fi
printf '  checksum : %s\n' "$native_output"

measure() {
  name=$1
  shift
  warm=0
  while [ "$warm" -lt "$WARMUP" ]; do
    "$@" >/dev/null
    warm=$((warm + 1))
  done
  sample=0
  total=0
  best=
  worst=0
  while [ "$sample" -lt "$RUNS" ]; do
    start=$(now)
    "$@" >/dev/null
    end=$(now)
    ms=$(elapsed_ms "$start" "$end")
    total=$(awk -v total="$total" -v ms="$ms" 'BEGIN { printf "%.6f", total + ms }')
    if [ -z "$best" ] || awk -v ms="$ms" -v best="$best" 'BEGIN { exit !(ms < best) }'; then best=$ms; fi
    if awk -v ms="$ms" -v worst="$worst" 'BEGIN { exit !(ms > worst) }'; then worst=$ms; fi
    sample=$((sample + 1))
  done
  average=$(awk -v total="$total" -v runs="$RUNS" 'BEGIN { printf "%.3f", total / runs }')
  printf '%-16s| %11sms | %11sms | %11sms\n' "$name" "$average" "$best" "$worst"
  if [ "$name" = native ]; then native_average=$average; fi
  if [ "$name" = interpreter ]; then interpreter_average=$average; fi
  if [ "$name" = vm ]; then vm_average=$average; fi
}

printf '  [4/4] Measuring execution...\n\n'
printf '  %-16s| %14s | %14s | %14s\n' 'path' 'average' 'best' 'worst'
printf '  %s\n' '----------------+----------------+----------------+----------------'
measure interpreter "$OSIRIS_BIN" run "$SOURCE"
measure vm "$OSIRIS_BIN" vm "$SOURCE"
measure native "$NATIVE_BIN"

native_speedup=$(awk -v base="$interpreter_average" -v native="$native_average" 'BEGIN { printf "%.2f", base / native }')
vm_speedup=$(awk -v base="$interpreter_average" -v vm="$vm_average" 'BEGIN { printf "%.2f", base / vm }')
printf '\n'
printf '  Native compile time : %s ms (reported separately)\n' "$build_ms"
printf '  VM speedup          : %sx vs interpreter\n' "$vm_speedup"
printf '  Native speedup      : %sx vs interpreter\n' "$native_speedup"
printf '\n'
printf '  All paths produced the same checksum. Timings include process startup;\n'
printf '  interpreter and VM runs include parsing, while native runs are precompiled.\n'
printf '  Tune with RUNS=15 BENCH_ITERS=5000 ./bench.sh\n\n'
