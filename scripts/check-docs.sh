#!/usr/bin/env sh
set -eu

# `docs/` is frozen compatibility material. Keep its tracked local links
# intact, but validate the public React source separately below.
for page in docs/*.html; do
  [ -f "$page" ] || continue
  grep -q 'style.css' "$page" || { printf '%s\n' "missing stylesheet reference: $page" >&2; exit 1; }
  sed -n 's/.*href="\([^"#?]*\.html\)".*/\1/p' "$page" | while IFS= read -r link; do
    [ -z "$link" ] && continue
    [ -f "docs/$link" ] || { printf '%s\n' "broken docs link in $page: $link" >&2; exit 1; }
  done
done

[ -f newdocs/package.json ] || { printf '%s\n' 'missing newdocs package manifest' >&2; exit 1; }
[ -f newdocs/src/support-matrix.json ] || { printf '%s\n' 'missing canonical React support matrix' >&2; exit 1; }
printf '%s\n' 'Legacy compatibility documentation checks passed.'
