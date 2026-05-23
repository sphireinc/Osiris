#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$ROOT"

CC=${CC:-cc}
GOCACHE=${GOCACHE:-/private/tmp/osiris-gocache}
GOMODCACHE=${GOMODCACHE:-/private/tmp/osiris-gomodcache}

WORKDIR=${WORKDIR:-$(mktemp -d "${TMPDIR:-/tmp}/osiris-bench.XXXXXX")}

cleanup() {
  status=$?
  trap - EXIT INT TERM HUP
  if [ "${BENCH_KEEP:-0}" = "1" ]; then
    printf 'bench scratch retained at %s\n' "$WORKDIR" >&2
    exit "$status"
  fi
  rm -rf "$WORKDIR"
  exit "$status"
}
trap cleanup EXIT INT TERM HUP

OSIRIS_BIN=${OSIRIS_BIN:-"$WORKDIR/osiris"}

PHYSICS_OSR=${PHYSICS_OSR:-examples/physics.osr}
PHYSICS_OPT=${PHYSICS_OPT:-"$WORKDIR/physics.opt.osr"}
PHYSICS_C=${PHYSICS_C:-"$WORKDIR/physics.osiris.c"}
PHYSICS_OBJ=${PHYSICS_OBJ:-"$WORKDIR/physics.osiris.o"}
PHYSICS_NATIVE=${PHYSICS_NATIVE:-"$WORKDIR/physics.osiris-native"}
PHYSICS_C_BIN=${PHYSICS_C_BIN:-"$WORKDIR/physics.osiris-c"}
PHYSICS_WAT=${PHYSICS_WAT:-"$WORKDIR/physics.osiris.wat"}
PHYSICS_WASM=${PHYSICS_WASM:-"$WORKDIR/physics.osiris.wasm"}
PHYSICS_GPU=${PHYSICS_GPU:-"$WORKDIR/physics.osiris.cl"}
PHYSICS_GPU_JSON=${PHYSICS_GPU_JSON:-"$WORKDIR/physics.osiris.json"}
PHYSICS_C_HUMAN=${PHYSICS_C_HUMAN:-"$WORKDIR/physics-c"}

TENSOR_OSR=${TENSOR_OSR:-examples/tensor.osr}
TENSOR_OPT=${TENSOR_OPT:-"$WORKDIR/tensor.opt.osr"}
TENSOR_C=${TENSOR_C:-"$WORKDIR/tensor.osiris.c"}
TENSOR_OBJ=${TENSOR_OBJ:-"$WORKDIR/tensor.osiris.o"}
TENSOR_NATIVE=${TENSOR_NATIVE:-"$WORKDIR/tensor.osiris-native"}
TENSOR_C_BIN=${TENSOR_C_BIN:-"$WORKDIR/tensor.osiris-c"}
TENSOR_WAT=${TENSOR_WAT:-"$WORKDIR/tensor.osiris.wat"}
TENSOR_WASM=${TENSOR_WASM:-"$WORKDIR/tensor.osiris.wasm"}
TENSOR_GPU=${TENSOR_GPU:-"$WORKDIR/tensor.osiris.cl"}
TENSOR_GPU_JSON=${TENSOR_GPU_JSON:-"$WORKDIR/tensor.osiris.json"}

bench() {
  label=$1
  shift
  printf '\n== %s ==\n' "$label"
  /usr/bin/time -p "$@"
}

echo "[1/4] building osiris"
GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" go build -o "$OSIRIS_BIN" ./cmd/osiris

echo "[2/4] optimizing workloads"
GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" opt "$PHYSICS_OSR" > "$PHYSICS_OPT"
GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" opt "$TENSOR_OSR" > "$TENSOR_OPT"

echo "[3/4] building native and artifact outputs"
bench "physics build (standard native)" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" build -o "$PHYSICS_NATIVE" "$PHYSICS_OPT"
bench "physics raw c emit" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" c -o "$PHYSICS_C" "$PHYSICS_OPT"
bench "physics object file" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" obj -o "$PHYSICS_OBJ" "$PHYSICS_OPT"
bench "physics wasm wat" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" wasm -o "$PHYSICS_WAT" "$PHYSICS_OPT"
bench "physics wasm binary" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" wasm -binary -o "$PHYSICS_WASM" "$PHYSICS_OPT"
bench "physics gpu source" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" gpu -o "$PHYSICS_GPU" "$PHYSICS_OPT"
bench "physics gpu plan" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" gpu -json -o "$PHYSICS_GPU_JSON" "$PHYSICS_OPT"
bench "physics cc -O3 generated c" "$CC" -O3 -march=native -flto -DNDEBUG "$PHYSICS_C" -lm -o "$PHYSICS_C_BIN"
bench "physics cc -O3 handwritten c" "$CC" -O3 -march=native -flto -DNDEBUG examples/physics.c -lm -o "$PHYSICS_C_HUMAN"

bench "tensor build (standard native)" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" build -o "$TENSOR_NATIVE" "$TENSOR_OPT"
bench "tensor raw c emit" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" c -o "$TENSOR_C" "$TENSOR_OPT"
bench "tensor object file" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" obj -o "$TENSOR_OBJ" "$TENSOR_OPT"
bench "tensor wasm wat" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" wasm -o "$TENSOR_WAT" "$TENSOR_OPT"
bench "tensor wasm binary" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" wasm -binary -o "$TENSOR_WASM" "$TENSOR_OPT"
bench "tensor gpu source" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" gpu -o "$TENSOR_GPU" "$TENSOR_OPT"
bench "tensor gpu plan" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" gpu -json -o "$TENSOR_GPU_JSON" "$TENSOR_OPT"
bench "tensor cc -O3 generated c" "$CC" -O3 -march=native -flto -DNDEBUG "$TENSOR_C" -lm -o "$TENSOR_C_BIN"

echo "[4/4] running execution benchmarks"
bench "physics interpreter" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" run "$PHYSICS_OPT"
bench "physics vm" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" vm "$PHYSICS_OPT"
bench "physics build executable" "$PHYSICS_NATIVE"
bench "physics generated c executable" "$PHYSICS_C_BIN"
bench "physics handwritten c executable" "$PHYSICS_C_HUMAN"

bench "tensor interpreter" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" run "$TENSOR_OPT"
bench "tensor vm" env GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" "$OSIRIS_BIN" vm "$TENSOR_OPT"
bench "tensor build executable" "$TENSOR_NATIVE"
bench "tensor generated c executable" "$TENSOR_C_BIN"
