#!/usr/bin/env sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/osiris-release-test.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM

OSIRIS_VERSION=0.1.0-test OSIRIS_COMMIT=release-smoke OSIRIS_BUILD_DATE=1970-01-01T00:00:00Z \
  OSIRIS_RELEASE_DIR="$work/release" "$root/scripts/release-dry-run.sh"
(cd "$work/release" && shasum -a 256 -c SHA256SUMS)

os=$(go env GOOS)
arch=$(go env GOARCH)
archive="$work/release/osiris-0.1.0-test-$os-$arch"
mkdir -p "$work/extract"
case "$os" in
  windows) unzip -q "$archive.zip" -d "$work/extract"; binary="$work/extract/osiris.exe" ;;
  *) tar -xzf "$archive.tar.gz" -C "$work/extract"; binary="$work/extract/osiris" ;;
esac
"$binary" version | grep -F 'osiris 0.1.0-test' >/dev/null
"$binary" version | grep -F 'commit: release-smoke' >/dev/null
"$binary" run "$root/examples/named-values.osr" >/dev/null
printf '%s\n' 'Release archive smoke test passed.'
