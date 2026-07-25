#!/usr/bin/env sh
set -eu

# Build unsigned, local release artifacts only. Publishing and signing stay in
# a separately trusted release workflow.
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
out=${OSIRIS_RELEASE_DIR:-"$root/dist/release"}
version=${OSIRIS_VERSION:-0.1.0-dev}
commit=${OSIRIS_COMMIT:-$(git -C "$root" rev-parse --verify --short=12 HEAD 2>/dev/null || printf unknown)}
date=${OSIRIS_BUILD_DATE:-${SOURCE_DATE_EPOCH:+$(date -u -r "$SOURCE_DATE_EPOCH" +%Y-%m-%dT%H:%M:%SZ)}}
date=${date:-unknown}

mkdir -p "$out"
rm -f "$out"/osiris-* "$out"/SHA256SUMS

targets='darwin/arm64 darwin/amd64 linux/amd64 windows/amd64'
for target in $targets; do
  goos=${target%/*}
  goarch=${target#*/}
  suffix=''
  [ "$goos" = windows ] && suffix=.exe
  base="osiris-${version}-${goos}-${goarch}"
  stage=$(mktemp -d "${TMPDIR:-/tmp}/osiris-release.XXXXXX")
  trap 'rm -rf "$stage"' EXIT HUP INT TERM
  GOCACHE=${GOCACHE:-/private/tmp/osiris-release-gocache} GOMODCACHE=${GOMODCACHE:-/private/tmp/osiris-release-gomodcache} \
    CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath -ldflags "-s -w -X main.version=$version -X main.commit=$commit -X main.buildDate=$date" -o "$stage/osiris$suffix" ./cmd/osiris
  cp README.txt "$stage/README.txt"
  cp docs/development/RELEASES.md "$stage/RELEASES.md"
  if [ "$goos" = windows ]; then
    (cd "$stage" && zip -X -q "$out/$base.zip" "osiris$suffix" README.txt RELEASES.md)
  else
    (cd "$stage" && tar --format=ustar --owner=0 --group=0 --numeric-owner ${SOURCE_DATE_EPOCH:+--mtime="@$SOURCE_DATE_EPOCH"} -czf "$out/$base.tar.gz" "osiris$suffix" README.txt RELEASES.md)
  fi
  rm -rf "$stage"
  trap - EXIT HUP INT TERM
done

(cd "$out" && shasum -a 256 osiris-* > SHA256SUMS)
printf 'release dry run complete: %s\n' "$out"
