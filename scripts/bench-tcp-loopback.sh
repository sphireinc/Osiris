#!/usr/bin/env sh
# Collect repeatable raw Go benchmark samples for the Osiris TCP runtime.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

runs=${TCP_RUNS:-15}
bench_time=${TCP_BENCH_TIME:-1s}
out=${TCP_BENCH_OUT:-}
show_raw=${TCP_BENCH_SHOW_RAW:-0}
cache=${GOCACHE:-/private/tmp/osiris-tcp-bench-gocache}
modcache=${GOMODCACHE:-/private/tmp/osiris-tcp-bench-gomodcache}

case $runs in ''|*[!0-9]*) echo "TCP_RUNS must be a positive integer" >&2; exit 2;; esac
if [ "$runs" -eq 0 ]; then echo "TCP_RUNS must be a positive integer" >&2; exit 2; fi

printf '\n  OSIRIS TCP LOOPBACK BASELINE\n'
printf '  ============================\n\n'
printf '  OS / architecture : %s / %s\n' "$(uname -s)" "$(uname -m)"
printf '  Go                : %s\n' "$(go version)"
printf '  samples           : %s per benchmark\n' "$runs"
printf '  benchmark time    : %s per sample\n\n' "$bench_time"

run_bench() {
  GOCACHE="$cache" GOMODCACHE="$modcache" go test ./internal/osiris \
    -run '^$' -bench '^BenchmarkTCPLoopback' -benchmem -count="$runs" -benchtime="$bench_time" -v
}

# print_summary keeps the human-facing report compact while the complete Go
# benchmark output remains available in the raw artifact. Benchmark result
# lines are intentionally parsed by their unit tokens because Go omits the
# throughput column for the cancelled-accept workload.
print_summary() {
  summary_dir=$(mktemp -d "${TMPDIR:-/tmp}/osiris-tcp-summary.XXXXXX")
  trap 'rm -rf "$summary_dir"' EXIT HUP INT TERM
  awk -v dir="$summary_dir" '
    $1 ~ /^BenchmarkTCPLoopback/ && $4 == "ns\/op" {
      name=$1
      sub(/-[0-9]+$/, "", name)
      rate="-"
      bytes="-"
      allocs="-"
      for (i=5; i<=NF; i++) {
        if ($i == "MB/s") rate=$(i-1)
        if ($i == "B\/op") bytes=$(i-1)
        if ($i == "allocs\/op") allocs=$(i-1)
      }
      print $3, rate, bytes, allocs > (dir "/" name)
    }
  ' "$1"

  printf '  MEASURED SUMMARY (median across %s samples)\n' "$runs"
  printf '  %-42s %12s %23s %12s %12s %10s\n' 'workload' 'median' 'range' 'MB/s' 'B/op' 'allocs/op'
  printf '  %-42s %12s %23s %12s %12s %10s\n' '----------------------------------------' '------------' '-----------------------' '------------' '------------' '----------'
  for sample in "$summary_dir"/*; do
    [ -f "$sample" ] || continue
    name=$(basename "$sample")
    values=$(awk '{print $1}' "$sample" | sort -n)
    count=$(printf '%s\n' "$values" | awk 'END { print NR }')
    median=$(printf '%s\n' "$values" | awk -v count="$count" '
      NR == int((count + 1) / 2) { lower = $1 }
      NR == int(count / 2 + 1) { upper = $1 }
      END { if (count % 2) printf "%.0f", upper; else printf "%.1f", (lower + upper) / 2 }
    ')
    range=$(printf '%s\n' "$values" | awk 'NR == 1 { low = $1 } { high = $1 } END { printf "%.0f–%.0f", low, high }')
    rate=$(awk 'NR == 1 { print $2 }' "$sample")
    bytes=$(awk 'NR == 1 { print $3 }' "$sample")
    allocs=$(awk 'NR == 1 { print $4 }' "$sample")
    printf '  %-42s %9s ns/op %18s ns/op %12s %12s %10s\n' "$name" "$median" "$range" "$rate" "$bytes" "$allocs"
  done
  printf '\n'
  rm -rf "$summary_dir"
  trap - EXIT HUP INT TERM
}

if [ -n "$out" ]; then
  mkdir -p "$(dirname "$out")"
  run_bench > "$out"
  print_summary "$out"
  if [ "$show_raw" = 1 ]; then
    sed 's/^/  /' "$out"
  fi
  printf '\n  raw samples       : %s\n\n' "$out"
else
  scratch=$(mktemp "${TMPDIR:-/tmp}/osiris-tcp-samples.XXXXXX")
  trap 'rm -f "$scratch"' EXIT HUP INT TERM
  run_bench > "$scratch"
  print_summary "$scratch"
  sed 's/^/  /' "$scratch"
  rm -f "$scratch"
  trap - EXIT HUP INT TERM
fi
