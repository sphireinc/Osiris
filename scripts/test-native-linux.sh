#!/usr/bin/env sh
set -eu

# Run Linux generated-C evidence in a real Linux kernel/userspace. The
# repository is mounted for the Go build and compiler scratch work; this
# script does not edit tracked source files.

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
image=${OSIRIS_LINUX_IMAGE:-golang:1.26-bookworm}
platform=${OSIRIS_LINUX_PLATFORM:-linux/arm64}
module_cache=${OSIRIS_GO_MODCACHE:-}

if [ -z "$module_cache" ] && command -v go >/dev/null 2>&1; then
	module_cache=$(go env GOMODCACHE 2>/dev/null || true)
fi

if ! command -v docker >/dev/null 2>&1; then
	printf '%s\n' 'Linux generated-C evidence blocked: docker is unavailable.' >&2
	exit 2
fi

docker_info=$(docker info --format '{{.OSType}}/{{.Architecture}}' 2>&1) || {
		printf '%s\n' 'Linux generated-C evidence blocked: the Docker daemon is unavailable.' >&2
		printf '%s\n' "$docker_info" >&2
		printf '%s\n' 'Start Colima or another Linux container runtime, then rerun this script.' >&2
		exit 2
	}
case "$docker_info" in
	linux/*) ;;
	*)
		printf 'Linux generated-C evidence blocked: Docker reports %s, not a Linux daemon.\n' "$docker_info" >&2
		exit 2
		;;
esac

set -- docker run --rm --platform "$platform" \
	-v "$repo_dir:/workspace" \
	-w /workspace \
	-e GOCACHE=/tmp/osiris-go-cache \
	-e GOMODCACHE=/go/pkg/mod \
	-e GOPROXY=off \
	-e GOFLAGS=-mod=readonly
if [ -n "$module_cache" ] && [ -d "$module_cache" ]; then
	set -- "$@" -v "$module_cache:/go/pkg/mod:ro"
else
	printf '%s\n' 'No host Go module cache found; the Linux image must have the module dependencies already cached.' >&2
fi

set -- "$@" "$image" sh -eu -c '
		compiler_wrapper=$(mktemp /tmp/osiris-cc-wrapper.XXXXXX)
		trap "rm -f \"$compiler_wrapper\"" EXIT
		printf "%s\\n" "#!/bin/sh" "exec cc -D_GNU_SOURCE \"\$@\"" >"$compiler_wrapper"
		chmod 755 "$compiler_wrapper"
		export CC="$compiler_wrapper"
		printf "%s\n" "--- Linux toolchain ---"
		go version
		cc --version | head -n 1
		printf "%s\n" "C evidence compiler: $CC (adds -D_GNU_SOURCE for POSIX declarations)"
		printf "%s\n" "--- deterministic generated-C parity ---"
		go test ./internal/osiris -run "^TestNativeHostDeferredLinuxGeneratedCDeterministicParity$" -count=1 -timeout=600s
		printf "%s\n" "--- AddressSanitizer generated-C parity ---"
		OSIRIS_C_SANITIZERS=address go test ./internal/osiris -run "^TestNativeHostDeferredLinuxSanitizerContract$" -count=1 -timeout=600s
		printf "%s\n" "Linux generated-C evidence passed."
	'
"$@"
