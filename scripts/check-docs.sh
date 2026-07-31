#!/usr/bin/env sh
set -eu

[ -f docs/package.json ] || { printf '%s\n' 'missing docs package manifest' >&2; exit 1; }
[ -f docs/src/support-matrix.json ] || { printf '%s\n' 'missing canonical React support matrix' >&2; exit 1; }
printf '%s\n' 'Documentation source checks passed.'
