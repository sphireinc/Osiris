#!/usr/bin/env sh
set -eu

# The current public site is authored static HTML. Keep its local reference
# links valid while Task 039 decides whether newdocs becomes the public source.
for page in docs/*.html; do
  [ -f "$page" ] || continue
  grep -q 'style.css' "$page" || { printf '%s\n' "missing stylesheet reference: $page" >&2; exit 1; }
  sed -n 's/.*href="\([^"#?]*\.html\)".*/\1/p' "$page" | while IFS= read -r link; do
    [ -z "$link" ] && continue
    [ -f "docs/$link" ] || { printf '%s\n' "broken docs link in $page: $link" >&2; exit 1; }
  done
done

[ -f docs/support-matrix.json ] || { printf '%s\n' 'missing canonical support matrix' >&2; exit 1; }
[ -f newdocs/package.json ] || { printf '%s\n' 'missing newdocs package manifest' >&2; exit 1; }
printf '%s\n' 'Static documentation checks passed.'
